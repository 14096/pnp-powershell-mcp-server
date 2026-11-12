package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const confirmationTTL = 5 * time.Minute

type action struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

type confirmationRequest struct {
	Required bool     `json:"confirmationRequired"`
	Actions  []action `json:"actions"`
}

func parseConfirmationRequest(stdout string) (confirmationRequest, error) {
	var c confirmationRequest
	err := json.Unmarshal([]byte(stdout), &c)
	return c, err
}

func describe(actions []action) string {
	var sb strings.Builder
	for _, a := range actions {
		fmt.Fprintf(&sb, "- %s: %s\n", a.Command, a.Reason)
	}
	return strings.TrimRight(sb.String(), "\n")
}

type decision int

const (
	noAnswer decision = iota
	approved
	declined
)

const elicitKey = "confirm"

func canElicit(req *mcp.CallToolRequest) bool {
	if req == nil || req.Session == nil {
		return false
	}
	p := req.Session.InitializeParams()
	return p != nil && p.Capabilities != nil && p.Capabilities.Elicitation != nil
}

func confirmationPrompt(summary, expr string) *mcp.ElicitParams {
	return &mcp.ElicitParams{
		Mode:    "form",
		Message: "This will run risky operations:\n" + summary + "\n\nExpression:\n" + expr,
		RequestedSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				elicitKey: map[string]any{
					"type":        "boolean",
					"title":       "Run these operations?",
					"description": "Check to allow, leave unchecked to cancel",
					"default":     false,
				},
			},
		},
	}
}

func (s *Server) userDecision(req *mcp.CallToolRequest, expr string) decision {
	if req == nil || req.Params == nil {
		return noAnswer
	}
	resp, ok := req.Params.InputResponses[elicitKey].(*mcp.ElicitResult)
	if !ok || !hmac.Equal([]byte(req.Params.RequestState), []byte(s.sign(expr))) {
		return noAnswer
	}
	if confirmed, _ := resp.Content[elicitKey].(bool); resp.Action == "accept" && confirmed {
		return approved
	}
	return declined
}

func (s *Server) sign(expr string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(expr))
	return hex.EncodeToString(m.Sum(nil))
}

type tokens struct {
	mu      sync.Mutex
	pending map[string]pendingToken
	now     func() time.Time
}

type pendingToken struct {
	exprHash [32]byte
	expires  time.Time
}

func newTokens() *tokens {
	return &tokens{pending: map[string]pendingToken{}, now: time.Now}
}

func (t *tokens) issue(expr string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, v := range t.pending {
		if t.now().After(v.expires) {
			delete(t.pending, k)
		}
	}
	t.pending[tok] = pendingToken{sha256.Sum256([]byte(expr)), t.now().Add(confirmationTTL)}
	return tok
}

func (t *tokens) redeem(tok, expr string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.pending[tok]
	if !ok {
		return false
	}
	delete(t.pending, tok)
	return t.now().Before(p.expires) && p.exprHash == sha256.Sum256([]byte(expr))
}

const elicitTimeout = 45 * time.Second

const multiRoundTripVersion = "2026-07-28"

func protocolVersion(req *mcp.CallToolRequest) string {
	if req == nil || req.Session == nil {
		return ""
	}
	if p := req.Session.InitializeParams(); p != nil {
		return p.ProtocolVersion
	}
	return ""
}

func (s *Server) useMultiRoundTrip(req *mcp.CallToolRequest) bool {
	if s.multiRoundTrip != nil {
		return s.multiRoundTrip(req)
	}
	return protocolVersion(req) >= multiRoundTripVersion
}

func sessionKey(req *mcp.CallToolRequest) any {
	if req == nil || req.Session == nil {
		return struct{}{}
	}
	return req.Session
}

func (s *Server) elicitationDisabled(req *mcp.CallToolRequest) bool {
	_, off := s.noElicit.Load(sessionKey(req))
	return off
}

func (s *Server) canElicit(req *mcp.CallToolRequest) bool {
	if s.supportsElicit != nil {
		return s.supportsElicit(req)
	}
	return s.elicitation && canElicit(req)
}

func (s *Server) askUser(ctx context.Context, req *mcp.CallToolRequest, p *mcp.ElicitParams) decision {
	timeout := s.promptTimeout
	if timeout == 0 {
		timeout = elicitTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	elicit := s.elicit
	if elicit == nil {
		elicit = func(ctx context.Context, p *mcp.ElicitParams) (*mcp.ElicitResult, error) {
			return req.Session.Elicit(ctx, p)
		}
	}
	res, err := elicit(ctx, p)
	if err != nil {
		s.noElicit.Store(sessionKey(req), true)
		return noAnswer
	}
	if confirmed, _ := res.Content[elicitKey].(bool); res.Action == "accept" && confirmed {
		return approved
	}
	return declined
}

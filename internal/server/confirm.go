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

// action is a risky operation found in an expression by the run script.
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

// decision is the user's answer to a confirmation prompt.
type decision int

const (
	noAnswer decision = iota
	approved
	declined
)

// elicitKey identifies the confirmation prompt in MCP input requests/responses.
const elicitKey = "confirm"

// canElicit reports whether the client can show prompts to the user (MCP elicitation).
func canElicit(req *mcp.CallToolRequest) bool {
	if req == nil || req.Session == nil {
		return false
	}
	p := req.Session.InitializeParams()
	return p != nil && p.Capabilities != nil && p.Capabilities.Elicitation != nil
}

// confirmationPrompt builds the prompt the client shows to the user itself, so
// the question cannot be answered by the model.
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

// userDecision reads the user's answer from a retried call. The answer only
// counts if the echoed request state was signed for this exact expression.
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

// sign binds a request state to an expression with a per-process secret.
func (s *Server) sign(expr string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(expr))
	return hex.EncodeToString(m.Sum(nil))
}

// tokens issues single-use, expiring confirmation tokens bound to one exact
// expression. They are the fallback for clients without elicitation: the
// assistant must ask the user in the conversation before presenting the token.
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
	for k, v := range t.pending { // drop expired entries
		if t.now().After(v.expires) {
			delete(t.pending, k)
		}
	}
	t.pending[tok] = pendingToken{sha256.Sum256([]byte(expr)), t.now().Add(confirmationTTL)}
	return tok
}

// redeem reports whether tok was issued for exactly expr and is still valid.
// A token can be redeemed only once.
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

// elicitTimeout bounds how long a direct prompt may stay unanswered. Some
// clients advertise elicitation but never show the prompt; waiting forever
// would leave the tool call hanging.
const elicitTimeout = 45 * time.Second

// multiRoundTripVersion is the first protocol version where servers return
// input requests instead of sending requests to the client mid-call.
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

// sessionKey identifies the client session; requests without one (tests) share a key.
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

// askUser prompts the user directly (older protocol versions). An unanswered or
// failed prompt returns noAnswer and disables prompting for the session, so
// later calls go straight to the conversational fallback instead of waiting again.
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

package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/14096/pnp-powershell-mcp-server/internal/catalog"
	"github.com/14096/pnp-powershell-mcp-server/internal/docs"
	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
	"github.com/14096/pnp-powershell-mcp-server/internal/server"
)

var version = "dev"

//go:embed best-practices.md
var guide string

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return nil
	}
	elicitation := flag.Bool("elicitation", false, "confirm risky operations through the client's native prompt (only for clients that show MCP prompts reliably); default is confirmation in the conversation")
	connURL := flag.String("url", os.Getenv("PNP_MCP_URL"), "site URL to connect to automatically before every run, reusing a login persisted with Connect-PnPOnline -PersistLogin (env PNP_MCP_URL)")
	connClient := flag.String("client-id", os.Getenv("PNP_MCP_CLIENTID"), "Entra ID app client ID for the automatic connection (env PNP_MCP_CLIENTID)")
	connTenant := flag.String("tenant", os.Getenv("PNP_MCP_TENANT"), "tenant for the automatic connection (env PNP_MCP_TENANT)")
	debug := flag.Bool("debug", false, "verbose logging to stderr (includes executed expressions)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "pnp-powershell-mcp-server %s\n\nMCP server (stdio) that runs PnP.PowerShell cmdlets against SharePoint and Microsoft 365.\nIt is normally started by an MCP client, not by hand.\n\nUsage:\n  pnp-powershell-mcp-server [flags]\n  pnp-powershell-mcp-server version\n\nFlags:\n", version)
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "\nDocs: https://github.com/14096/pnp-powershell-mcp-server")
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return nil
	}

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := pwsh.Exec{}
	go func() {
		if d := pwsh.Diagnose(ctx, runner); !d.OK() {
			for _, p := range d.Problems {
				logger.Warn("prerequisite problem", "problem", p)
			}
		} else {
			logger.Info("environment ok", "powershell", d.PSVersion, "pnpPowerShell", d.ModuleVersion)
		}
	}()

	store := catalog.NewStore(runner, catalog.DefaultCacheFile())
	store.Start(ctx)

	opts := []server.Option{server.WithLogger(logger), server.WithGuide(guide)}
	if *connURL != "" {
		if u, err := url.Parse(*connURL); err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("--url must be an absolute https URL, got %q", *connURL)
		}
		opts = append(opts, server.WithConnection(server.Connection{URL: *connURL, ClientID: *connClient, Tenant: *connTenant}))
	}
	if *elicitation {
		opts = append(opts, server.WithElicitation())
	}
	srv := server.New(runner, store, docs.NewClient(docs.DefaultCacheDir()), opts...)
	logger.Info("starting", "version", version, "elicitation", *elicitation)
	return srv.MCP(version).Run(ctx, &mcp.StdioTransport{})
}

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
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

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	elicitation := flag.Bool("elicitation", false, "confirm risky operations through the client's native prompt (only for clients that show MCP prompts reliably); default is confirmation in the conversation")
	debug := flag.Bool("debug", false, "verbose logging to stderr (includes executed expressions)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "pnp-powershell-mcp-server %s\n\nMCP server (stdio) that runs PnP.PowerShell cmdlets against SharePoint and Microsoft 365.\nIt is normally started by an MCP client, not by hand.\n\nUsage:\n  pnp-powershell-mcp-server [flags]\n\nFlags:\n", version)
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

	opts := []server.Option{server.WithLogger(logger)}
	if *elicitation {
		opts = append(opts, server.WithElicitation())
	}
	srv := server.New(runner, store, docs.NewClient(docs.DefaultCacheDir()), opts...)
	logger.Info("starting", "version", version, "elicitation", *elicitation)
	return srv.MCP(version).Run(ctx, &mcp.StdioTransport{})
}

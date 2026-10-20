package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/14096/pnp-powershell-mcp-server/internal/catalog"
	"github.com/14096/pnp-powershell-mcp-server/internal/docs"
	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
	"github.com/14096/pnp-powershell-mcp-server/internal/server"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	log.SetOutput(os.Stderr)
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	runner := pwsh.Exec{}
	store := catalog.NewStore(runner, catalog.DefaultCacheFile())
	if err := store.Load(ctx); err != nil {
		log.Printf("warning: catalog not loaded: %v", err)
	}
	srv := server.New(runner, store, docs.NewClient())
	return srv.MCP(version).Run(ctx, &mcp.StdioTransport{})
}

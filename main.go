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
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/14096/pnp-powershell-mcp-server/internal/catalog"
	"github.com/14096/pnp-powershell-mcp-server/internal/docs"
	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
	"github.com/14096/pnp-powershell-mcp-server/internal/server"
)

var version = "dev"

func resolveVersion() string {
	if version != "dev" && version != "" {
		return withV(version)
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if len(rev) >= 7 {
		return "dev-" + rev[:7] + dirty
	}
	return version
}

func withV(v string) string {
	if v != "" && v[0] >= '0' && v[0] <= '9' {
		return "v" + v
	}
	return v
}

//go:embed best-practices.md
var guide string

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	version = resolveVersion()
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return nil
	}
	elicitation := flag.Bool("elicitation", false, "confirm risky operations through the client's native prompt (only for clients that show MCP prompts reliably); default is confirmation in the conversation")
	connURL := flag.String("url", os.Getenv("PNP_MCP_URL"), "site URL to connect to automatically before every run, reusing a login persisted with Connect-PnPOnline -PersistLogin (env PNP_MCP_URL)")
	connClient := flag.String("client-id", os.Getenv("PNP_MCP_CLIENTID"), "Entra ID app client ID for the automatic connection (env PNP_MCP_CLIENTID)")
	connTenant := flag.String("tenant", os.Getenv("PNP_MCP_TENANT"), "tenant for the automatic connection (env PNP_MCP_TENANT)")
	persistedSpec := flag.String("persisted-connections", os.Getenv("PNP_MCP_PERSISTED_CONNECTIONS"), `JSON array like [{"tenant":"contoso","clientId":"<app-id>","name":"optional-profile-name"}] (or @file) of tenants whose persisted login is reused when an expression runs Connect-PnPOnline -Url for them without -ClientId (env PNP_MCP_PERSISTED_CONNECTIONS)`)
	strict := flag.Bool("strict-connections", os.Getenv("PNP_MCP_STRICT_CONNECTIONS") == "1", "refuse Connect-PnPOnline -Url to a tenant that is not in --persisted-connections instead of falling back to PnP's own token cache (env PNP_MCP_STRICT_CONNECTIONS=1)")
	passEnv := flag.String("pass-env", os.Getenv("PNP_MCP_PASS_ENV"), "comma-separated names of extra environment variables to pass to PowerShell (by default only a safe subset such as PATH and HOME is inherited), e.g. AZURE_CLIENT_ID,AZURE_CLIENT_SECRET (env PNP_MCP_PASS_ENV)")
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

	runner := pwsh.Exec{ExtraEnv: strings.Split(*passEnv, ",")}
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
	persisted, err := server.ParsePersistedConnections(*persistedSpec)
	if err != nil {
		return fmt.Errorf("--persisted-connections: %w", err)
	}
	if len(persisted) > 0 {
		opts = append(opts, server.WithPersistedConnections(persisted))
	}
	if *strict {
		opts = append(opts, server.WithStrictConnections())
	}
	if *elicitation {
		opts = append(opts, server.WithElicitation())
	}
	srv := server.New(runner, store, docs.NewClient(docs.DefaultCacheDir()), opts...)
	logger.Info("starting", "version", version, "elicitation", *elicitation)
	return srv.MCP(version).Run(ctx, &mcp.StdioTransport{})
}

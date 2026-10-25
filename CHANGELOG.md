# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.0.9] - 2026-10-25

- Persisted connections: `--persisted-connections` (or `PNP_MCP_PERSISTED_CONNECTIONS`) takes a JSON array of `{"tenant","clientId"}` (or `@file`). When an expression runs `Connect-PnPOnline -Url` for a listed tenant without `-ClientId`, the server adds the configured client ID and `-Interactive` to reuse the persisted login. Unlisted tenants behave as before.
- Persisted connections accept an optional `name`, and `pnpRunCmdlet` takes a `profile` argument to select one explicitly (unknown profiles fail with `[UNKNOWN_PROFILE]`).
- `--strict-connections` (or `PNP_MCP_STRICT_CONNECTIONS=1`) refuses `Connect-PnPOnline -Url` to a tenant that is not in the persisted connections (`[TENANT_NOT_CONFIGURED]`). It only has an effect together with `--persisted-connections`.

## [0.0.8] - 2026-10-22

- Optional automatic connection: `--url`, `--client-id` and `--tenant` (or `PNP_MCP_URL`, `PNP_MCP_CLIENTID`, `PNP_MCP_TENANT`) make `pnpRunCmdlet` connect to the site before every run, reusing a login persisted with `Connect-PnPOnline -PersistLogin`. Without `--url` behaviour is unchanged.
- A failed automatic connection returns a message with the command to sign in again.
- `Get-PnPAccessToken` and `Get-PnPConnection` now require confirmation; with automatic connection enabled, so does `Connect-PnPOnline` to another site.
- Added the `version` subcommand (`--version` still works).

## [0.0.7] - 2026-10-21

- fix: warning message

## [0.0.6] - 2026-10-21

- `pnpRunCmdlet` returns PowerShell warnings from successful runs (`[WARNINGS]` block and `warnings` field) instead of dropping them.
- Long-running `pnpRunCmdlet` calls send MCP progress notifications every 5 s when the client supplies a progress token.
- Added a `Dockerfile` (with PowerShell and PnP.PowerShell), `make docker` / `make docker-run`, and a README section on testing with Docker.

## [0.0.5] - 2026-10-21

- Added the `pnp://best-practices` resource and the prompts `pnpAuditSitePermissions`, `pnpInventoryLists`, `pnpFindCmdlet` and `pnpTeamsOverview`.
- `pnpRunCmdlet` accepts `offset` to page through truncated output (item index for JSON arrays, character offset otherwise); the `[TRUNCATED]` notice names the next offset.
- `pnpSearchCmdlets` ranks exact verb/noun matches first, also matches parameter names, accepts `verb`/`noun` filters and returns related cmdlets.
- Docs: fixed stale references in best-practices.md and documented the variable-assignment pitfall, paging and search.

## [0.0.4] - 2026-10-21

- Tools now return structured content with a published output schema (`ok` flag, parsed JSON in `data`, `truncated`, confirmation fields, etc.); the text output is unchanged.
- Added human-readable titles and refined annotations for all tools.
- Narrowed the network error detection to avoid false positives.
- `pnpRunCmdlet` notes when `timeoutSec` or `maxChars` was capped at its maximum.
- `pnpGetCmdletDocs` returns a clear error when a cmdlet does not exist.

## [0.0.3] - 2026-10-20

- Management of destructive actions.
- Performance optimizations.

## [0.0.2] - 2026-10-20

- Release pipeline fixes.

## [0.0.1] - 2026-10-20

- Initial release: PnP PowerShell MCP server.

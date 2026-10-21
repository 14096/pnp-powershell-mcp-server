# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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

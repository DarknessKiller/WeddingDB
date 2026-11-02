---
name: use-modern-go
description: Modern Go idioms for writing, modifying, fixing or refactoring Go code in this repo. Load before editing any Go file under backend/ so new code uses language and stdlib features up to the version in backend/go.mod instead of patterns from training data.
---

# Modern Go Guidelines CLI

Always write modern, idiomatic Go code. Use the Modern Go Guidelines CLI as the source of truth for modern Go idioms that may be newer than your knowledge cutoff.

Vendored from https://github.com/JetBrains/go-modern-guidelines (commit `155dc7ca10da5e1f6c841503086957b1b37f5815`, Apache-2.0), skill `use-modern-go`, CLI pinned in `scripts/VERSION`. Re-copy those files to update.

Command:

- Linux or macOS: `sh "<skill-dir>/scripts/run-tool.sh"`
- Windows PowerShell: `& '<skill-dir>\scripts\run-tool.ps1'`

First run and approvals:

On first use, the wrapper installs the Modern Go Guidelines CLI in a local cache directory. Needs network once and the Go toolchain on `PATH`.

Subcommands:

- `list`
- `explain`

Before editing Go code:

1. Call the wrapper's `list` subcommand for the relevant Go file.

   Prefer passing the file you are about to edit:

   ```sh
   sh "<skill-dir>/scripts/run-tool.sh" list --file-path backend/internal/handlers/guest.go
   ```

   On Windows, use the PowerShell wrapper with the same arguments.

   The CLI resolves the applicable Go version from go.mod, go.work, the local Go toolchain, or an explicit override. This repo declares `go 1.27.0` in `backend/go.mod`.

2. If the target Go version is already known, you may pass it directly:

   ```sh
   sh "<skill-dir>/scripts/run-tool.sh" list --go-version 1.27
   ```

3. Read the complete list output before deciding which guidelines apply.

   The list output is ordered newest first. Read the full output because older supported guidelines may still apply.

   Do not pipe the output through head, tail, grep, sed, or any other truncating/filtering command. Important guidelines may otherwise be missed.

4. Treat returned guidelines as authoritative for modern Go style choices in code you are editing.

   If a guideline applies, follow it even when nearby code or repository convention uses an older pattern. Skip it only when it would not compile, would change behavior, or clearly does not match the edited code. Before skipping a returned guideline that seems relevant, call the wrapper's `explain` subcommand for that guideline ID.

Call `explain` only when a specific guideline may apply and you need the detailed explanation or examples. Request only the guideline IDs you intend to evaluate or apply:

```sh
sh "<skill-dir>/scripts/run-tool.sh" explain sync_waitgroup_go
```

Multiple guideline IDs may be requested as positional arguments:

```sh
sh "<skill-dir>/scripts/run-tool.sh" explain atomic_types errors_as_type
```

Do not call `explain` without guideline IDs. Use `list` first to discover the short guideline list for the target Go version, then call `explain` for the specific returned IDs that need more context.

This skill never overrides `CODING_STANDARDS.md`. When both apply, the no-bitwise rule wins.

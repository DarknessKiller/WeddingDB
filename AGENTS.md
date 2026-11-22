<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, initialize it for the user before searching: run `codegraph init` at the repo root (builds the initial index, ~1 min), then use the tools above. Mention the one-time cost in your reply.
<!-- CODEGRAPH_END -->

## Coding standards

`CODING_STANDARDS.md` is the single rule set for people and agents. Its feature sections (Offline sync, Auth and token revocation, Frontend pagination, Frontend input limits, Multi-tab) name the files and tests for each area.

Default routine for any change:

1. Find the code with CodeGraph, then read the `CODING_STANDARDS.md` sections for the areas you touch.
2. Load the `use-modern-go` skill before editing Go, and follow its list for Go 1.27.
3. Run the verify block for each area you touched before handoff, and check each changed line against the standard.
4. Before handoff, commit, push, or PR, run the `change-verification` skill (or dispatch it to a subagent).

<!-- CODEGRAPH_START -->
## CodeGraph

This repo is indexed by CodeGraph (`.codegraph/` exists). Use it before grep/find or reading files to locate or understand code.

Run this first: `codegraph explore "<symbol names or question>"`. One call returns the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep cannot follow. Name a file or symbol in the query to read its current line-numbered source.

If it returns nothing useful, fall back to grep. If there is no `.codegraph/` directory, skip CodeGraph entirely.
<!-- CODEGRAPH_END -->

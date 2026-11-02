---
name: change-verification
description: Verify code and tests before handoff, commit, push, or PR. Use after implementing a change to prove behavior at its production boundary, confirm tests catch the old behavior, run the affected checks, and attribute every failure before reporting done.
---
# Change verification

You own the correctness of the change and its tests. Apply the Go style and verification sections of [`CODING_STANDARDS.md`](../../../CODING_STANDARDS.md).

## 1. Record the baseline

```sh
git rev-parse HEAD
git status --short
```

List every contract and caller the change affects: grep the changed symbols, or use `codegraph explore`. For a new environment variable, wire it in `backend/internal/config` and the `environment:` blocks in `docker-compose.yml`, then run the config path.

Done when the baseline commit, pre-existing working-tree changes, and affected contracts and callers are written down.

## 2. Prove the behavior at its production boundary

Exercise the real path users hit, not a typed mock or a separately registered test route. HTTP changes go through `handlers.RegisterRoutes` with real decoding. Cover success, rejection, and the relevant absence of upstream side effects.

Done when each changed behavior has a passing check that fails if the production wiring is wrong.

## 3. Prove the tests detect the old behavior

Run the new test against the pre-change code, or temporarily revert only your own change and restore it. Name the assertion that catches the regression. Fix a test that stays green instead of weakening it or skipping it.

Done when every new or changed test is shown red without the change.

## 4. Run focused tests and affected gates

Run the verify block for each area touched, plus the focused tests around the change. Re-run after every fix. Record the commands and results; an earlier pass does not clear a later failure.

Done when the final code passes every affected gate.

## 5. Attribute every failure

Prove an unrelated cause by reproducing the same failure on the baseline commit in a separate worktree, or with direct independent evidence. Record commands, baseline commit, and the decisive evidence. Unchanged files and suspected flakes are not proof. Fix failures your change owns; an unknown failure blocks handoff.

A bypass needs proven-unrelated evidence, a stated remaining risk, and explicit user authorization. It covers that invocation only and leaves repository and global hook configuration untouched. Authorization does not clear an owned or unknown failure.

## Completion

Report done only when the final affected checks pass, or every remaining failure is proven unrelated and its bypass explicitly authorized. Disclose unresolved failures and skipped checks. Otherwise report blocked, not verified.

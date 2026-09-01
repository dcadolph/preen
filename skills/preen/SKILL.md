---
name: preen
description: >-
  Turn messy work into a clean, ordered set of atomic commits. Runs the preen
  binary, which splits a dirty working tree, absorbs a run of unpushed commits
  and redoes them, and, only when explicitly asked, rewrites commits that are
  already pushed. It handles the reset itself, shows a plan, and changes
  nothing until approved. Commit message style is configurable with flags or a
  .preen.toml. Run options: --scope preens only part of the tree, --gate runs
  a check after each commit, --dry-run plans and stops, --fixup folds dirty
  changes into the unpushed commits that introduced them, --yes skips the
  approval prompt, --pushed grants the explicit ask a pushed rewrite requires.
  Triggers: "preen", "split my diff", "clean up my commit history", "fix
  my last commits", "reword these commits", "resplit my commits", "fold my
  changes into the right commits".
---

# preen

This skill drives the preen binary. Do not reimplement its behavior with raw
git commands: the binary owns the grouping, the staging, the guardrails, and
the conservation check that proves no content was lost. Your job is to pick
the right flags, relay the plan, and report the result.

## Preflight

- Confirm the binary is available: `preen --version`. If it is missing, offer
  `go install github.com/dcadolph/preen@latest` and stop until the user
  agrees.
- Confirm a git repository and a dirty tree or redoable commits. If there is
  nothing to do, say so and stop.

## Workflow

The Bash tool cannot answer preen's interactive prompt, so run it in two
steps:

1. `preen --dry-run` (plus any flags the request calls for) to get the plan.
   Show the plan to the user unchanged.
2. After the user approves, run the same invocation with `--yes` in place of
   `--dry-run` to apply it.

When the user wants the plan changed, prefer flags that reshape the run
(`--scope`, `--fixup`, `--grouper`, message style flags) and rerun the dry
run. For fine-grained edits like merging or reordering planned commits, tell
the user to run `preen` themselves: its prompt accepts merge, split, move,
reword, drop, and reorder interactively.

## Choosing flags

- Messy working tree: plain `preen`.
- Sloppy unpushed commits: `--absorb` brings them back and redoes them.
- "Fold my changes into the right commits": `--fixup`.
- Only part of the tree: `--scope <path>`, repeatable.
- Run tests after each commit: `--gate "<cmd>"`.
- Report debug prints and leftovers: `--sweep`.
- Rewrite pushed commits: only when the user explicitly asked, in words or
  with `--pushed`. Pass `--pushed`, and `--pushed-base <rev>` when they named
  a range. Every other guardrail stays with the binary.

Message style flags (`--conventional`, `--prefix`, `--max-subject`,
`--no-emdash`, `--no-semicolon`, `--punctuation`, `--body`, and the rest) pass
through as the user asks. A `.preen.toml` at the repository root sets
defaults; flags beat the file.

## Exit codes

Distinct codes tell you what happened without parsing output: a rejected plan
(5), a rolled-back run (6, 7), and a declined prompt (8) are different
results. On a rollback, relay the binary's report of what diverged; the
repository has already been restored.

## Safety

- Never pass `--no-verify` without the user's explicit grant in this session
  or `allow-no-verify = true` in the repository's `.preen.toml`. With that
  consent in place, use it rather than aborting; note the bypass in the
  report.
- Never rewrite pushed history unless the user explicitly asked. The binary
  enforces its own consents (`--pushed`, `--allow-protected`, a separate push
  confirmation); do not work around a refusal.
- Every run leaves a `preen-backup/<timestamp>` ref. Undo with
  `preen restore`, and clean old refs with `preen backups --prune`. Name the
  backup ref in your report so the user knows how to undo.

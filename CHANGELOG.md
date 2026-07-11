# Changelog

## 0.6.0 - 2026-07-11

- Fixup mode: `--fixup` folds dirty changes into the unpushed commits that
  introduced them, autosquash under the hood, conflict-safe with abort and
  restore.
- `--yes` skips the approval prompt for scripted and headless runs.
- Eval harness: `hack/eval.sh` runs the skill headless against fixture repos
  and asserts on git state, style conformance, and the merge and scope guards.
- Limitations section in the README.

## 0.5.0 - 2026-07-11

- Hardened git mechanics: merge commits in an absorb range are never
  flattened, diffs regenerate after every commit, mid-rebase and mid-merge
  states refuse cleanly, binary files stay whole, rename pairs stay together,
  empty stages stop the run, pre-staged files count as a grouping hint.
- Run options: `--scope`, `--gate`, `--dry-run`, with `[run]` and `[protect]`
  config in `.preen.toml` and an `allow-no-verify` standing-consent switch for
  hook-blocked repos.
- Undo guidance now uses `git reset --keep` so work done after a run survives.
- Timestamp spacing constraints: monotonic, never future, never before base.
- New banner and logo.

## 0.4.0 and earlier

Pre-changelog: working-tree splits, unpushed-commit absorption, opt-in pushed
rewrites behind `--force-with-lease`, message style flags, `.preen.toml`,
human-spaced timestamps, sweep pass, demo.

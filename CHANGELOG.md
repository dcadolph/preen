# Changelog

## 0.12.0 - 2026-07-13

- New `--spread <window|auto>` run option spaces commit timestamps across a
  window ending now instead of stamping them all in the same second, for
  example `--spread 2h`. `auto` picks a plausible window from the size of the
  run. `spread = "2h"` under `[run]` in `.preen.toml` sets a default. The
  behavior existed but could only be requested in words, which made it
  unreachable in `--headless` runs.
- New `--pushed [<base>]` run option grants the explicit ask a pushed rewrite
  requires, so headless runs can rewrite published commits. The optional value
  names the commit just before the range to redo; without one the base is the
  merge-base with the default branch on a feature branch. Every other pushed
  rewrite guardrail still applies, and `.preen.toml` cannot set it: consent is
  per invocation.
- Eval cases: c10 asserts spread timestamps strictly increase, never in the
  future, never before the base or the window. c11 asserts `--pushed` redoes
  pushed commits on a feature branch and updates the remote with lease.

## 0.11.1 - 2026-07-12

- Release pipeline: goreleaser builds binaries for macOS, Linux, and Windows
  on every tag and publishes a Homebrew cask to `dcadolph/homebrew-tap`.
  Install with `brew install --cask dcadolph/tap/preen`.
- README badges and install docs for the new channels.

## 0.11.0 - 2026-07-12

- New `--punctuation always|auto|never` message style option controls
  terminal punctuation: `always` ends the subject and every body sentence
  with a period, `never` adds none and drops trailing periods, `auto`
  (default) matches the repository's convention. Replaces `--no-period`,
  which only covered the subject.
- Eval case c3 asserts the setting holds: run with `--punctuation always`,
  every subject must end with a period.

## 0.10.0 - 2026-07-12

- Content-conservation check: preen records the full content tree before it
  touches anything and verifies at the end that committed plus
  still-uncommitted content is unchanged, so a run that lost or invented a
  change is caught instead of trusted. The only allowed differences are paths
  a sweep or `drop` removed or a commit hook reformatted.
- Eval harness asserts conservation on every case: the content tree (HEAD plus
  all uncommitted changes) is identical before and after the run.

## 0.9.0 - 2026-07-11

- Go CLI wrapper: `go install github.com/dcadolph/preen@latest` gives a
  `preen` command that launches Claude Code with the skill from any
  terminal. Interactive by default; `--headless` runs `claude -p` with
  `--yes` and scoped permissions for CI and scripts. Flags after `--` pass
  to the claude CLI verbatim.
- The binary embeds the release's SKILL.md and pins the run to that exact
  text, overriding any installed plugin or user-level skill copy, the same
  pinning the eval harness uses.
- Preflight checks fail fast before a session starts: claude on PATH,
  inside a git repository, no rebase, merge, or cherry-pick in progress.

## 0.8.0 - 2026-07-11

- New eval cases: c7 fixup targeting (fixes land in the commits that
  introduced them, messages preserved), c8 allow-no-verify standing consent,
  c9 hook rejection without consent stops cleanly.
- Standing consent clarified: `allow-no-verify = true` in `.preen.toml` is
  the user's explicit permission, granted ahead of time; a hook rejection is
  retried with `--no-verify` and the bypass is noted in the report.
- Demo: third scenario shows `--fixup` folding review fixes into the commits
  that introduced them; `assets/demo.gif` regenerated.
- Eval harness pins the skill under test: the prompt names the fixture's own
  SKILL.md and declares it overriding, so a user-level skill copy or an
  installed preen plugin can no longer shadow the version being evaluated.

## 0.7.0 - 2026-07-11

- Plan edit grammar: `merge N into M`, `split N`, `move <path> to N`,
  `reword N: <subject>`, `drop <path>`, `reorder N,M,...`, with the plan
  reshown after each move.
- Backup pruning: `--prune-backups` lists and deletes old `preen-backup/`
  refs behind a confirmation, and clean runs offer the same cleanup when old
  backups exist.
- Merge guard refined after eval findings: foreign merges (second parent
  remote-reachable, e.g. pull merges) are never flattened and re-anchor the
  base; the user's own local topic-branch merges may be linearized with
  notice. The check is bound to the plan: every absorb or rewrite plan
  carries a `Merge check:` line quoting the command output, and a plan
  without one is invalid.
- Eval harness fixes: skill install and agent log no longer dirty the
  fixture, untracked-directory assertions use `-uall`, c5 now constructs
  a genuinely foreign merge, logs capture the full agent trace
  (stream-json), and passing cases remove their logs.
- DESIGN.md added.

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

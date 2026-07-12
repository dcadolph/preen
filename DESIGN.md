# preen design

Working design doc: what preen is, the invariants it holds, how it is tested
and released, and what is left to build. Update this when behavior or plans
change.

## Shape

preen is a Claude Code plugin. The product is `skills/preen/SKILL.md`: a
prompt program the agent executes with git. There is no compiled code and no
runtime of its own. Consequences of that shape drive everything else here:
options are conventions the agent follows and verifies, not parsed flags, and
quality is enforced by instruction plus evals, not by a type system.

Packaging is `.claude-plugin/plugin.json` (version lives here) and
`marketplace.json`. Install is via the plugin marketplace or copying the
skill directory.

## What it does

Three rewrite modes plus one distribution mode:

- Working tree: group uncommitted changes into atomic commits.
- Absorb: soft-reset a run of unpushed commits back into the tree and redo
  them clean.
- Pushed rewrite: opt-in only, force-with-lease only, guarded.
- Fixup: fold dirty changes into the unpushed commits that introduced them,
  autosquash under the hood.

Message style and run behavior are configurable per invocation or via
`.preen.toml` (`[commit]`, `[run]`, `[protect]`).

## Invariants

These hold on every run and evals assert them where practical:

- A backup ref (`preen-backup/<ts>`) exists before any history changes.
- A plan is shown before anything moves. `--yes` skips the approval prompt,
  not the plan.
- Merges whose second parent is remote-reachable (`git branch -r --contains
  <sha>^2` non-empty, e.g. pull merges or merges of pushed branches) are
  never flattened; the base re-anchors past them and the check's output
  appears in the plan. Fully unpushed merges may be linearized with notice.
- Mid-rebase, mid-merge, or conflicted repositories refuse cleanly.
- Diffs are regenerated after every commit; patches are never reused across
  commits.
- Plain splits and absorbs never push. Only pushed rewrites push, only with
  `--force-with-lease`.
- No `--no-verify` without standing consent (`allow-no-verify` in
  `.preen.toml`) or an explicit in-session grant.
- preen never invents changes.

## Evals

`hack/eval.sh` is the regression suite: it builds fixture repos, runs the
skill headless (`claude -p "/preen --yes ..."`), and asserts on the resulting
git state. Cases: c1 basic split, c2 absorb, c3 style flags, c4 dry-run,
c5 foreign merge guard, c6 scope, c7 fixup targeting, c8 allow-no-verify
consent, c9 hook rejection without consent. Quick set is c1/c3/c4; `--all`
runs everything. Each case is a real agent run and costs tokens, so it is
run by hand, not in CI. Failed fixtures are kept on disk with their
stream-json agent traces for inspection; passing cases clean up both.

Lessons already banked from the merge guard, which took four rule revisions:
an unconditional "never flatten" got overridden because flattening local
merges is what a history cleaner should do; a prose classification
(foreign versus local, with a "local topic branch" example) got
pattern-matched by branch name instead of checked; a mechanical procedure
(run named commands, decide on their output alone) got skipped outright while
it lived as a procedural paragraph, with the agent substituting a
pushed-ness check on HEAD; what held was binding the evidence to the plan
format: the plan must carry a `Merge check:` line quoting the command
output, so a run that skipped the commands produces a visibly invalid plan.
When an eval keeps failing, first ask whether the spec is wrong, then make
the rule command-driven, and then make its evidence a required field of the
output rather than a step the agent may drift past. Debugging this required
full agent traces, so the harness logs stream-json, not just the final
message.

The harness lesson that came out of the hook-consent case: pin the skill
under test. `/preen` in a fixture resolved against whatever preen the
machine had, and a stale user-level copy under `~/.claude/skills` shadowed
the fixture's file, so some failures were failures of a version that no
longer existed and at least one rule revision reacted to a trace the revised
text never produced. The eval prompt now names the fixture's own SKILL.md
and declares it overriding. Results recorded before that pinning
(c5, c7, c9 passes) count only once reproduced under it.

## Releases

Bump `version` in `.claude-plugin/plugin.json`, add a `CHANGELOG.md` entry,
tag `vX.Y.Z`. Versions to date: 0.5.0 hardening and run options, 0.6.0 fixup
mode plus evals, 0.7.0 plan edit grammar, backup pruning, and the
foreign-versus-local merge rule.

## Backlog

Ordered by value:

1. Publish the announcement post. A draft lives in the dcadolph.dev repo at
   `content/posts/preen-absorbs-your-bad-commits.md` with `draft: true`.

Done since 0.7.0: fixup eval case (c7), hook-interaction cases (c8, c9),
fixup demo scenario with the gif regenerated, and a full `--all` pass (9 of
9) under the pinned harness on 2026-07-11.

## External artifacts

The announcement post draft is in the dcadolph.dev repository, not here. The
plugin is listed on the personal site's projects page. There is no CI, no
external service, and nothing leaves the machine except the agent's normal
API traffic.

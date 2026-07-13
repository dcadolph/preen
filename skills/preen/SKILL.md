---
name: preen
description: >-
  Turn messy work into a clean, ordered set of atomic commits with sensible
  messages. Splits a dirty working tree, absorbs a run of unpushed commits and
  redoes them, and, only when explicitly asked, rewrites commits that are already
  pushed. Handles the reset itself, shows a plan, and changes nothing until
  approved. Commit message style is configurable with flags or a .preen.toml.
  Run options: --scope preens only part of the tree, --gate runs a check after
  each commit, --dry-run plans and stops, --fixup folds dirty changes into the
  unpushed commits that introduced them, --yes skips the approval prompt,
  --spread spaces the commit timestamps across a window, --pushed grants the
  explicit ask a pushed rewrite requires.
  Triggers: "preen", "split my diff", "clean up my commit history", "fix
  my last commits", "reword these commits", "resplit my commits", "fold my
  changes into the right commits".
---

# preen

The fixer for your commit history. Turn a messy working tree, a run of unpushed
commits with a bad history, or, when you ask, already-pushed commits, into a
clean, ordered set of atomic commits with sensible messages: the history the user
would have written if they had committed carefully as they went.

preen does not invent changes. It groups what is already there, shows a plan, and
acts only after approval. It handles the reset for the user, so they never have
to unstage or rewind anything by hand.

## Modes

- Working tree. Uncommitted changes become clean commits.
- Unpushed commits. Local commits that were never pushed are absorbed back into
  the tree automatically and redone as clean commits. No manual reset.
- Pushed commits. Only when explicitly asked, in words or with `--pushed`,
  already-pushed commits are rewritten and force-pushed with
  `--force-with-lease`, behind the guardrails in Safety.
- Fixup. With `--fixup`, dirty changes are folded into the unpushed commits
  that introduced those lines instead of becoming new commits. See Fixup mode.

## Message style

By default preen writes a short imperative subject, matches the repository's
existing convention, keeps the subject under 72 characters, adds a body only when
the why is not obvious, and never adds attribution or tool footers.

Override the style with options on the invocation, for example `/preen --no-emdash
--no-semicolon --max-subject 50 --include-line-numbers`, or set defaults once in a
`.preen.toml` at the repository root. Invocation options win over the config file,
which wins over the defaults.

| Option | Effect |
|--------|--------|
| `--no-emdash` | No em or en dashes anywhere in a message. |
| `--no-semicolon` | No semicolons. |
| `--no-hyphen` | No hyphens. Reword compound terms instead. |
| `--max-subject N` | Cap the subject at N characters. Default 72. |
| `--punctuation always\|auto\|never` | Terminal punctuation on sentences. `always` ends the subject and every body sentence with a period. `never` adds none and drops trailing periods everywhere. `auto` matches the repository's convention. Default auto. |
| `--lower-subject` | Lowercase the first letter of the subject. |
| `--conventional` | Conventional Commits, `type(scope): subject`. |
| `--body always\|auto\|never` | When to include a body. Default auto. |
| `--include-files` | List the touched paths in the body. |
| `--include-line-numbers` | Cite each file's changed line ranges as `path:start-end` in the body, read from `git diff --unified=0` hunk headers. |
| `--prefix TEXT` | Prefix every subject, for example a ticket id. |
| `--sign-off` | Add a `Signed-off-by` trailer. |

A `.preen.toml` uses the option names without the leading dashes:

```
[commit]
no-emdash = true
no-semicolon = true
max-subject = 50
include-line-numbers = true
prefix = "ABC-123"
```

Apply the style to every message, then verify each one conforms before
committing: no banned characters, subject within the cap, punctuation per the
setting, prefix and trailers in place. Rewrite any message that violates the
style rather than committing it.

## Run options

Behavior options, separate from message style:

| Option | Effect |
|--------|--------|
| `--scope <pathspec>` | Preen only the paths matching the pathspec. Everything else stays uncommitted and untouched. |
| `--gate <cmd>` | Run the command after each commit. On failure, stop and regroup. |
| `--dry-run` | Survey, group, and show the plan, then stop. Nothing is staged or committed. |
| `--fixup` | Fold each dirty change into the unpushed commit it belongs to instead of building new commits. See Fixup mode. |
| `--yes` | Skip the approval prompt and run the shown plan. For scripted or headless use. |
| `--spread <window\|auto>` | Space the commit timestamps across a window ending now, for example `--spread 2h`. `auto` picks a plausible window from the size of the run. See Human-spaced timestamps. |
| `--pushed [<base>]` | The explicit ask a pushed rewrite requires, as a flag. The optional value is the commit just before the range to redo. |
| `--prune-backups` | List preen-backup refs, confirm a selection, delete them, and stop. |

`--scope` combines with an absorb run only when every absorbed commit touches
in-scope paths alone. Otherwise stop and explain: absorbing would turn
out-of-scope committed work back into uncommitted changes.

`--pushed` satisfies the explicit-ask requirement for rewriting pushed
commits; every other guardrail in Safety still applies, protected branches
included. Without a value, the base is the merge-base with the default branch
when the current branch is not the default. On the default branch a base is
required: ask for one, or in a headless run stop and report that `--pushed`
needs a value there. Consent to rewrite published history is granted per
invocation, so `.preen.toml` cannot set it.

A `.preen.toml` can set run defaults and extra protected branches:

```
[run]
gate = "go test ./..."
spread = "2h"
allow-no-verify = false

[protect]
branches = ["develop", "release/*"]
```

`allow-no-verify` is standing consent to bypass commit hooks with `--no-verify`
when a hook rejects preen's commits outright. It defaults to false. Standing
consent means exactly that: the user wrote it into their repository to grant
the permission ahead of time, and it satisfies any rule that requires the
user's explicit permission for `--no-verify`. When it is true and a hook
rejects a commit, retry that commit with `--no-verify` and note the bypass in
the report; do not stop to re-ask and do not dismiss the setting as
insufficient. See Safety.

## When to run

Run when the user wants clean history out of a messy state: many uncommitted
changes, a run of sloppy unpushed commits, or an explicit request to fix commits
that are already pushed. Skip when the tree is clean and there is nothing to redo.

## Workflow

### 0. Preflight

- Confirm a git repository: `git rev-parse --git-dir`.
- Stop cleanly when the repository is mid-operation: a rebase in progress (the
  `rebase-merge` or `rebase-apply` directory under `git rev-parse --git-path`
  exists), a merge (`MERGE_HEAD`), a cherry-pick (`CHERRY_PICK_HEAD`), or
  unmerged paths in `git status --porcelain=v1` (states like `UU`). Report the
  state and touch nothing.
- Record the current branch and the undo anchor: `git rev-parse HEAD`.
- Record the content baseline for the conservation check in step 11. On a
  scratch index, stage every change and write its tree:

  ```
  idx="$(mktemp -u)"
  GIT_INDEX_FILE="$idx" git read-tree HEAD
  GIT_INDEX_FILE="$idx" git add -A
  TREE_START="$(GIT_INDEX_FILE="$idx" git write-tree)"
  rm -f "$idx"
  ```

  `TREE_START` is the hash of HEAD plus every staged, unstaged, and untracked
  change. The scratch index leaves the real index untouched. Keep the hash.
- Record what is already staged versus unstaged. Pre-staged files are a signal:
  the user may have marked a commit boundary by hand. Use it as a grouping
  hint.
- Read the message style and run options from the invocation and any
  `.preen.toml`. See Message style and Run options.
- Read the state:
  - Uncommitted work: `git status --porcelain=v1`.
  - Upstream, if any: `git rev-parse --abbrev-ref @{upstream}` (fails when the
    branch has none).
  - Unpushed commits: with an upstream, `git log --oneline @{upstream}..HEAD`;
    without one, compare to the default branch, or show recent commits and ask
    how far back is unpushed.
  - A commit is pushed when a remote branch contains it: `git branch -r
    --contains <sha>` is non-empty.
- If nothing is uncommitted and there are no commits to redo, say so and stop.

### 1. Decide the scope

Establish what preen operates on and the base commit it will reset to:

- Working tree only: base is HEAD.
- Absorb unpushed commits: base is the last pushed commit (`@{upstream}`), or the
  fork point from the default branch when there is no upstream. Confirm by
  showing the commits that will be redone.
- Rewrite pushed commits: only if the user explicitly asked, in words or with
  `--pushed`. Base is the commit just before the range they want fixed: the
  `--pushed` value when one was given, else the merge-base with the default
  branch on a feature branch, else ask. Read Safety before proceeding.

Merge check, required before any absorb or rewrite, no exceptions. Every plan
that involves a reset MUST carry a `Merge check:` line built from the output
of these commands, so a run that skipped them is visibly invalid. With the
candidate base chosen, run:

1. `git log --merges --oneline <base>..HEAD`
2. For each merge listed: `git branch -r --contains <merge-sha>^2`

Checking whether HEAD or the tip commits are pushed is NOT this check; only
`<merge-sha>^2`, the merge's side-branch parent, decides. Then act on the
output alone:

- Command 1 prints nothing: no merges in range. Plan line: `Merge check: no
  merges in <base>..HEAD`.
- Command 2 non-empty for any merge: that merge's side branch is pushed.
  Redoing it would re-commit published work as new commits, so the base MUST
  move forward to the newest such merge; those commits are never redone. Plan
  line quotes the remote branches found.
- Command 2 empty for every merge: the merges are unpushed on both sides and
  may be absorbed and linearized; the plan line says so and flags the
  flattening.

This is mechanical, not a judgment call: the branch's name or apparent
ownership does not matter, only the command output, and `--yes` does not
override it.

Never absorb or rewrite a commit the user did not clearly mean to touch. When in
doubt, show the log and ask for the base.

### 2. Back up, then absorb

Before changing any history, save a recovery ref:

`git branch preen-backup/$(date +%Y%m%d-%H%M%S)`

Then bring the in-scope commits back into the working tree with one soft reset,
which keeps every change and deletes nothing:

`git reset --soft <base>`

Gate the reset first: rerun the step 1 merge check against the chosen base.
If any merge in `<base>..HEAD` has a remote-reachable second parent, the base
is wrong. Recompute it past that merge before resetting.

Working-tree-only runs skip the reset. After this step, all in-scope changes sit
in the index and tree, ready to regroup.

### 3. Survey the changes

- `git status --porcelain=v1` for the file picture.
- `git diff` and `git diff --cached` for the hunks.
- Read notable untracked files so they land in the right group.

### 4. Sweep first (optional)

Scan for debris introduced by the work: stray debug prints, commented-out blocks,
leftover scratch code, accidental files. List anything found and ask whether to
drop it. Never delete without approval.

### 5. Group into atomic commits

Cluster so each commit is one coherent idea that stands on its own.

- One concern per commit.
- Keep formatting-only churn out of feature commits.
- Keep unrelated docs and config in their own commits.
- Prefer commits that build and pass on their own.

### 6. Order the commits

Sequence so each commit is coherent in isolation and dependencies land first. The
goal is a history someone could bisect.

### 7. Show the plan

Present before touching anything:

- A numbered list of commits, each with its message and the files or hunks it
  will contain.
- The base being reset to, the backup ref, and, for a pushed rewrite, the exact
  force-push that will run.
- For any absorb or rewrite, the `Merge check:` line from step 1 with the
  command output it quotes. A plan that resets without this line is invalid:
  go back and run the check, `--yes` included.
- If timestamp spacing is requested, with `--spread` or in words, the planned
  window, for example `Spread over ~2h ending now`.

Ask to approve, edit, or abort. Nothing is committed yet. With `--dry-run`,
stop here. With `--yes`, skip the prompt and proceed with the shown plan;
still show it first so the log records what ran.

Edits use these moves, applied one at a time with the full plan reshown after
each:

- `merge N into M`: combine two planned commits.
- `split N`: break a commit apart; the user says how, by file or by concern.
- `move <path> to N`: reassign a file or hunk to another commit.
- `reword N: <subject>`: replace a commit message.
- `drop <path>`: leave those changes uncommitted.
- `reorder N,M,...`: resequence the plan.

Free-form asks are fine too; map them onto these moves rather than improvising
a new plan shape.

### 8. Commit

For each planned commit, stage precisely, then commit.

- Whole files: `git add -- <paths>`.
- A file split across commits: regenerate the file's current diff, write the
  wanted hunks to a patch, `git apply --cached that.patch`, commit, and leave
  the rest for a later commit. Never `git add -A` blindly.
- Regenerate the remaining diff after every commit. Offsets shift once an
  earlier commit touches a file, and a pre-commit hook may have reformatted it.
  Never reuse a patch computed before a previous commit.
- Binary files never hunk-split. Each goes whole into exactly one commit.
- Keep rename pairs together. Porcelain `R` entries, or a matching delete and
  untracked add, belong in the same commit.
- To split a new untracked file across commits, `git add -N <path>` first so
  its hunks are visible to diff and `git apply --cached`.
- Verify the stage is nonempty before committing: if `git diff --cached
  --quiet` reports no changes, the plan is wrong. Stop and regroup instead of
  creating an empty commit.
- `git commit -m "<subject>"`. Add a body only when the why is not obvious.
- If a hook rewrites files during a commit, re-diff before the next group. If
  a hook rejects the commit outright: with standing consent (`allow-no-verify
  = true` in `.preen.toml`) or an explicit grant in this session, retry the
  same commit with `--no-verify` and note the bypass in the report. Without
  either, stop and show the hook output; never bypass on your own judgment.
- With a gate configured, run it after each commit. On failure, stop, report
  which commit broke it, and regroup or amend with the user. The backup ref
  still covers a full undo.

Compose each message in the configured style. See Message style for the options
and defaults. Before committing, verify each message conforms, no banned
characters, subject within the cap, punctuation per the setting, prefix and
trailers present, and rewrite any that does not.

### 9. Human-spaced timestamps (optional)

With `--spread`, a `spread` value in `.preen.toml`, or a request in words,
spread the timestamps across a window ending now instead of stamping every
commit in the same second. Back-date each commit:

`GIT_AUTHOR_DATE="<ts>" GIT_COMMITTER_DATE="<ts>" git commit -m "..."`

A duration value like `30m`, `2h`, or `1d` sets the window length. `auto`
picks a plausible window from the number and size of the commits: minutes for
a small run, a few hours for a large one. Vary the gaps between commits rather
than dividing the window evenly.

Constraints: timestamps strictly increase, the last is no later than now, and
the first is no earlier than the base commit's date; shrink the window to fit
when it would reach back past the base. With commit signing enabled, note that
signature timestamps will not match back-dated commits.

### 10. Publish

- Working tree or unpushed-absorb runs: do not push. The new commits are local.
  Tell the user they are ready, and offer a normal `git push` when the branch
  already tracks a remote.
- Pushed rewrite: after approval, update the remote with `git push
  --force-with-lease origin <branch>`. Never `git push --force`, which can clobber
  work that arrived after the last fetch.

### 11. Verify and report

- Content conservation, required. Recompute the content tree exactly as the
  step 0 baseline did (`git read-tree HEAD`, `git add -A`, `git write-tree` on
  a scratch index) to get `TREE_END`, and confirm it equals `TREE_START`.
  preen only reshapes history, so committed plus still-uncommitted content is
  conserved and the two trees MUST match. The only allowed differences are
  paths a sweep or a `drop` deliberately removed, and files a commit hook
  reformatted; then `git diff TREE_START TREE_END` must touch exactly those
  paths and nothing else. Any other difference means a change was lost or
  invented: stop and restore from the backup ref with `git reset --keep
  preen-backup/<ts>`, then report what diverged.
- With a gate configured it already ran per commit; otherwise offer a final
  build or test run.
- Finish with `git log --oneline -n <count>`.
- Name the backup ref so the user knows how to undo.

## Fixup mode

With `--fixup`, preen builds no new commits. It distributes the dirty changes
into the existing unpushed commits they belong to, then autosquashes.

- Preflight, backup ref, plan, and approval all apply as in a normal run. The
  soft reset does not: existing commits stay in place.
- Find each change's target: the unpushed commit that last touched those lines,
  via `git log -L<start>,<end>:<path> <base>..HEAD` or `git blame`.
- Only unpushed commits are targets. A change whose target is pushed, or whose
  lines no unpushed commit introduced, goes in the plan as a leftover: offer to
  leave it uncommitted or make it a new commit on top.
- The plan lists each change, its target commit, and the leftovers.
- After approval, stage each group precisely and `git commit --fixup=<sha>`,
  then squash: `GIT_SEQUENCE_EDITOR=true git rebase -i --autosquash <base>`.
- On a rebase conflict, `git rebase --abort`, restore from the backup ref, and
  report which change conflicted. Never leave a rebase half-done.
- Commit messages are preserved, so message style options do not apply.

## Backup pruning

Every run leaves a `preen-backup/<ts>` branch. With `--prune-backups`, list
them with age and tip (`git for-each-ref 'refs/heads/preen-backup/*'`), show
which are fully contained in the current branch, confirm a selection, and
delete with `git branch -D`. Only branches under `preen-backup/` are ever
candidates, and never one created by the current run. A normal run that
finishes cleanly mentions older backups when they exist and offers the same
cleanup.

## Safety

- Everything is reversible. Undo any run with the backup ref: `git reset --keep
  preen-backup/<ts>`, or via the reflog. Prefer `--keep` over `--hard`: `--hard`
  also destroys anything the user did after the run.
- No `--no-verify` without standing consent (`allow-no-verify = true` in
  `.preen.toml`) or an explicit in-session grant. With that consent in place,
  use it rather than aborting the run: the setting exists so hook-blocked
  repositories can still be preened. Never `git push --force`; use
  `--force-with-lease`.
- Plain split and unpushed-absorb never push. Only a pushed rewrite pushes, and
  only after explicit approval.
- Rewriting pushed history is opt-in and dangerous. Before doing it:
  - Confirm the user explicitly asked to rewrite already-pushed commits, in
    words or with `--pushed` on the invocation.
  - Refuse on a shared or protected branch, main and master especially, plus
    any branch matched by `[protect]` in `.preen.toml`, unless the user
    confirms the branch is theirs alone. Warn that anyone who pulled the
    old commits must reset to the rewritten history.
  - Always create the backup ref first.
  - Use `--force-with-lease` so the push aborts if the remote moved.
- Never flatten a merge whose side branch is pushed (`git branch -r
  --contains <merge-sha>^2` non-empty), even when content is preserved.
  Re-anchor the base past it instead. Only fully unpushed merges may be
  linearized. See the merge check in step 1.
- preen never invents changes and never touches a commit the user did not ask it to.

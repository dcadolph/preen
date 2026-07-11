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
  unpushed commits that introduced them, --yes skips the approval prompt.
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
- Pushed commits. Only when explicitly asked, already-pushed commits are
  rewritten and force-pushed with `--force-with-lease`, behind the guardrails in
  Safety.
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
| `--no-period` | Drop a trailing period on the subject. |
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
committing: no banned characters, subject within the cap, prefix and trailers in
place. Rewrite any message that violates the style rather than committing it.

## Run options

Behavior options, separate from message style:

| Option | Effect |
|--------|--------|
| `--scope <pathspec>` | Preen only the paths matching the pathspec. Everything else stays uncommitted and untouched. |
| `--gate <cmd>` | Run the command after each commit. On failure, stop and regroup. |
| `--dry-run` | Survey, group, and show the plan, then stop. Nothing is staged or committed. |
| `--fixup` | Fold each dirty change into the unpushed commit it belongs to instead of building new commits. See Fixup mode. |
| `--yes` | Skip the approval prompt and run the shown plan. For scripted or headless use. |

`--scope` combines with an absorb run only when every absorbed commit touches
in-scope paths alone. Otherwise stop and explain: absorbing would turn
out-of-scope committed work back into uncommitted changes.

A `.preen.toml` can set run defaults and extra protected branches:

```
[run]
gate = "go test ./..."
allow-no-verify = false

[protect]
branches = ["develop", "release/*"]
```

`allow-no-verify` is standing consent to bypass commit hooks with `--no-verify`
when a hook rejects preen's commits outright. It defaults to false. See Safety.

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
- Rewrite pushed commits: only if the user explicitly asked. Base is the commit
  just before the range they want fixed. Read Safety before proceeding.

Check the range for merge commits before settling on a base: `git log --merges
--oneline <base>..HEAD`. A soft reset across a merge flattens it, and everything
the merge brought in becomes part of the diff, ready to be committed as if it
were the user's own work. If the range contains a merge, re-anchor the base to
the merge commit itself so only commits after it are redone, or stop and
explain. Never flatten a merge.

Never absorb or rewrite a commit the user did not clearly mean to touch. When in
doubt, show the log and ask for the base.

### 2. Back up, then absorb

Before changing any history, save a recovery ref:

`git branch preen-backup/$(date +%Y%m%d-%H%M%S)`

Then bring the in-scope commits back into the working tree with one soft reset,
which keeps every change and deletes nothing:

`git reset --soft <base>`

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
- If timestamp spacing is requested, the planned spread.

Ask to approve, edit, or abort. Nothing is committed yet. With `--dry-run`,
stop here. With `--yes`, skip the prompt and proceed with the shown plan;
still show it first so the log records what ran.

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
- If a hook rewrites files during a commit, re-diff before the next group. If a
  hook rejects the commit outright, stop and show the hook output. Use
  `--no-verify` only under standing consent (`allow-no-verify` in
  `.preen.toml`) or an explicit grant in this session.
- With a gate configured, run it after each commit. On failure, stop, report
  which commit broke it, and regroup or amend with the user. The backup ref
  still covers a full undo.

Compose each message in the configured style. See Message style for the options
and defaults. Before committing, verify each message conforms, no banned
characters, subject within the cap, prefix and trailers present, and rewrite any
that does not.

### 9. Human-spaced timestamps (optional)

If the user wants timestamps spread across a plausible window instead of all in
the same second, back-date each commit:

`GIT_AUTHOR_DATE="<ts>" GIT_COMMITTER_DATE="<ts>" git commit -m "..."`

Constraints: timestamps strictly increase, the last is no later than now, and
the first is no earlier than the base commit's date. With commit signing
enabled, note that signature timestamps will not match back-dated commits.

### 10. Publish

- Working tree or unpushed-absorb runs: do not push. The new commits are local.
  Tell the user they are ready, and offer a normal `git push` when the branch
  already tracks a remote.
- Pushed rewrite: after approval, update the remote with `git push
  --force-with-lease origin <branch>`. Never `git push --force`, which can clobber
  work that arrived after the last fetch.

### 11. Verify and report

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

## Safety

- Everything is reversible. Undo any run with the backup ref: `git reset --keep
  preen-backup/<ts>`, or via the reflog. Prefer `--keep` over `--hard`: `--hard`
  also destroys anything the user did after the run.
- No `--no-verify` without standing consent (`allow-no-verify = true` in
  `.preen.toml`) or an explicit in-session grant. Never `git push --force`; use
  `--force-with-lease`.
- Plain split and unpushed-absorb never push. Only a pushed rewrite pushes, and
  only after explicit approval.
- Rewriting pushed history is opt-in and dangerous. Before doing it:
  - Confirm the user explicitly asked to rewrite already-pushed commits.
  - Refuse on a shared or protected branch, main and master especially, plus
    any branch matched by `[protect]` in `.preen.toml`, unless the user
    confirms the branch is theirs alone. Warn that anyone who pulled the
    old commits must reset to the rewritten history.
  - Always create the backup ref first.
  - Use `--force-with-lease` so the push aborts if the remote moved.
- preen never invents changes and never touches a commit the user did not ask it to.

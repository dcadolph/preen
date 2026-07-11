---
name: gus
description: >-
  Turn messy work into a clean, ordered set of atomic commits with sensible
  messages. Splits a dirty working tree, absorbs a run of unpushed commits and
  redoes them, and, only when explicitly asked, rewrites commits that are already
  pushed. Handles the reset itself, shows a plan, and changes nothing until
  approved. Triggers: "gus", "split my diff", "clean up my commit history", "fix
  my last commits", "reword these commits", "resplit my commits".
---

# gus

The fixer for your commit history. Turn a messy working tree, a run of unpushed
commits with a bad history, or, when you ask, already-pushed commits, into a
clean, ordered set of atomic commits with sensible messages: the history the user
would have written if they had committed carefully as they went.

gus does not invent changes. It groups what is already there, shows a plan, and
acts only after approval. It handles the reset for the user, so they never have
to unstage or rewind anything by hand.

## Modes

- Working tree. Uncommitted changes become clean commits.
- Unpushed commits. Local commits that were never pushed are absorbed back into
  the tree automatically and redone as clean commits. No manual reset.
- Pushed commits. Only when explicitly asked, already-pushed commits are
  rewritten and force-pushed with `--force-with-lease`, behind the guardrails in
  Safety.

## When to run

Run when the user wants clean history out of a messy state: many uncommitted
changes, a run of sloppy unpushed commits, or an explicit request to fix commits
that are already pushed. Skip when the tree is clean and there is nothing to redo.

## Workflow

### 0. Preflight

- Confirm a git repository: `git rev-parse --git-dir`.
- Record the current branch and the undo anchor: `git rev-parse HEAD`.
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

Establish what gus operates on and the base commit it will reset to:

- Working tree only: base is HEAD.
- Absorb unpushed commits: base is the last pushed commit (`@{upstream}`), or the
  fork point from the default branch when there is no upstream. Confirm by
  showing the commits that will be redone.
- Rewrite pushed commits: only if the user explicitly asked. Base is the commit
  just before the range they want fixed. Read Safety before proceeding.

Never absorb or rewrite a commit the user did not clearly mean to touch. When in
doubt, show the log and ask for the base.

### 2. Back up, then absorb

Before changing any history, save a recovery ref:

`git branch gus-backup/$(date +%Y%m%d-%H%M%S)`

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

Ask to approve, edit, or abort. Nothing is committed yet.

### 8. Commit

For each planned commit, stage precisely, then commit.

- Whole files: `git add -- <paths>`.
- A file split across commits: write the wanted hunks to a patch and `git apply
  --cached that.patch`, commit, and leave the rest for a later commit. Never `git
  add -A` blindly.
- `git commit -m "<subject>"`. Add a body only when the why is not obvious.

Message style: match the repository's convention. If none is clear, a short
imperative subject under 72 characters, no body unless needed, no attribution or
tool footers.

### 9. Human-spaced timestamps (optional)

If the user wants timestamps spread across a plausible window instead of all in
the same second, back-date each commit:

`GIT_AUTHOR_DATE="<ts>" GIT_COMMITTER_DATE="<ts>" git commit -m "..."`

### 10. Publish

- Working tree or unpushed-absorb runs: do not push. The new commits are local.
  Tell the user they are ready, and offer a normal `git push` when the branch
  already tracks a remote.
- Pushed rewrite: after approval, update the remote with `git push
  --force-with-lease origin <branch>`. Never `git push --force`, which can clobber
  work that arrived after the last fetch.

### 11. Verify and report

- Optionally run a build or test gate between commits if the user names one.
- Finish with `git log --oneline -n <count>`.
- Name the backup ref so the user knows how to undo.

## Safety

- Everything is reversible. Undo any run with the backup ref: `git reset --hard
  gus-backup/<ts>`, or via the reflog.
- Never `--no-verify`. Never `git push --force`; use `--force-with-lease`.
- Plain split and unpushed-absorb never push. Only a pushed rewrite pushes, and
  only after explicit approval.
- Rewriting pushed history is opt-in and dangerous. Before doing it:
  - Confirm the user explicitly asked to rewrite already-pushed commits.
  - Refuse on a shared or protected branch, main and master especially, unless
    the user confirms the branch is theirs alone. Warn that anyone who pulled the
    old commits must reset to the rewritten history.
  - Always create the backup ref first.
  - Use `--force-with-lease` so the push aborts if the remote moved.
- gus never invents changes and never touches a commit the user did not ask it to.

---
name: gus
description: >-
  Split a messy working tree into clean, ordered, atomic commits with sensible
  messages, and optionally human-spaced timestamps. Use when there is a large
  pile of uncommitted changes that should become a logical commit history
  instead of one giant dump. Triggers: "gus", "split my diff", "break these
  changes into commits", "clean up my commit history", "atomize this diff".
---

# gus

The fixer for your commit history. Take one messy working tree and turn it into
a clean, ordered set of commits that reads like real, incremental work. Hide the
messy operation behind a spotless front.

gus does not invent changes and does not push. It groups what is already there,
shows a plan, and commits only after approval.

## When to run

Run when the working tree has many uncommitted changes spanning multiple
concerns and the user wants them broken into logical commits. Skip if the tree
is clean or holds a single trivial change.

## Workflow

### 0. Preflight

- Confirm the current directory is a git repository (`git rev-parse --git-dir`).
- Capture the starting point: `git rev-parse HEAD` (may be empty on a fresh
  repo). This is the undo anchor.
- If nothing is uncommitted (`git status --porcelain` empty), report that there
  is nothing to split and stop.

### 1. Survey the changes

Read the full change surface before grouping:

- `git status --porcelain=v1` for the file-level picture.
- `git diff` for unstaged hunks, `git diff --cached` for staged hunks.
- List untracked files; read the notable ones so they land in the right group.

### 2. Sweep first (optional)

Scan the diff for obvious debris introduced by the work: stray debug prints,
commented-out blocks, leftover scratch code, accidental file additions. List
anything found and ask whether to drop it before committing. Never delete
without approval.

### 3. Group into atomic commits

Cluster the changes so each commit is one coherent idea that stands on its own.
Guidelines:

- One concern per commit. A new loader, the flag that wires it, and its tests
  can be one commit or three depending on size and how the user works.
- Keep formatting-only churn out of feature commits; give it its own commit.
- Keep unrelated docs and config in their own commits.
- Prefer commits that build and pass on their own.

### 4. Order the commits

Sequence so each commit is coherent in isolation and dependencies land before
the code that needs them. The goal is a history someone could bisect.

### 5. Show the plan (the front)

Present the plan before touching anything:

- A numbered list of commits, each with its proposed message and the files or
  hunks it will contain.
- If timestamp spacing is requested, the planned spread.

Ask to approve, edit, or abort. Nothing is staged or committed yet.

### 6. Commit

For each planned commit, stage precisely, then commit.

- Whole files: `git add -- <paths>`.
- Untracked files that belong to the group: add them by path.
- A file split across two commits: write only the wanted hunks to a patch and
  `git apply --cached that.patch`, commit, and leave the rest in the tree for a
  later commit. Never `git add -A` blindly.
- Commit with `git commit -m "<subject>"`. Add a body only when the why is not
  obvious from the subject and diff.

Commit message style: match the repository's existing convention. If none is
clear, default to a short imperative subject under 72 characters, no body unless
needed, no attribution or tool footers.

### 7. Human-spaced timestamps (optional)

If the user wants the history to look like a real work session rather than a
burst of commits in the same second, back-date each commit:

- Build an ordered list of timestamps ending near now, spread across a plausible
  window with irregular gaps.
- Commit each with both dates set:
  `GIT_AUTHOR_DATE="<ts>" GIT_COMMITTER_DATE="<ts>" git commit -m "..."`.

This only affects the new, unpushed commits being created here.

### 8. Verify and report

- Optionally run a build or test gate between commits if the user names one.
- Finish with `git log --oneline -n <count>` so the result is visible.

## Safety

- Never push. Never use `--no-verify`.
- Never rewrite commits that already exist upstream; gus only creates new commits
  from uncommitted work.
- The split is reversible. To undo, reset back to the anchor from step 0 with
  `git reset --soft <anchor>` (or `git reset` to an empty tree on a fresh repo),
  which returns every change to the working tree untouched.

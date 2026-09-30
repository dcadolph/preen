![preen](preen-banner.png)

# preen

[![ci](https://github.com/dcadolph/preen/actions/workflows/ci.yml/badge.svg)](https://github.com/dcadolph/preen/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/dcadolph/preen)](https://github.com/dcadolph/preen/releases/latest)
[![License](https://img.shields.io/github/license/dcadolph/preen)](LICENSE)

Turn a messy working tree into clean, atomic commits. See the plan before
anything moves.

`git` and nothing else. No model, no API key, no network.

![preen demo](assets/demo.gif)

You got in the zone and came out with forty changed files and no commits, or one
giant blob with a message like `wip` that you are not proud of. preen turns that
into a clean, ordered set of small, self-contained commits: the shape of the
history you would have written if you had committed carefully as you went.

Think `git add -p` and `git rebase -i`, done for you. preen reads everything you
changed, groups it into coherent commits, writes a subject for each, orders them
so dependencies land first, and shows you the plan first. Nothing moves until
you approve.

Rewriting history is not a thing to take on faith, so preen does not ask you to.
Before a run it hashes your `HEAD` plus every staged, unstaged, and untracked
change, and after the run it hashes the same thing again. The two must match
exactly. A single differing byte rolls the run back to the recovery branch it
made before it started. Every run leaves a `preen-backup/<timestamp>` branch and
`preen restore` puts you back. [How that works](#it-proves-it-did-not-lose-your-work).

The built-in subjects say what changed, not why: `Add cmd/parser.go and its
test`, `Update dependencies`. Every one of them is built from the file list and
nothing else, so a subject is a claim the commit can be checked against. For
messages that explain intent, hand grouping to any program you like with
`--grouper`, a model included, and reword anything at the approval prompt.

Clean history is worth having on its own. It makes review readable, `git bisect`
useful, and `git blame` honest. preen gets you there without the tedious
hand-staging.

## It proves it did not lose your work

preen only reshapes history, never content, and it enforces that rather than
promising it. The check is a content hash of everything you have, not a diff of
what preen thinks it touched, so it holds whether or not preen understood your
tree correctly.

The tree it hashes covers `HEAD` plus every staged, unstaged, and untracked
change. If the before and after hashes differ by a byte, the run rolls back and
names the paths that diverged rather than leaving you to find them.

Every run leaves a `preen-backup/<timestamp>` branch whether or not anything
went wrong. `preen restore` puts you back where you were, with your work
returned to the working tree in the same state it was in, staged files staged
and untracked files untracked.

## Install

```
brew install --cask dcadolph/tap/preen
```

Or with Go:

```
go install github.com/dcadolph/preen@latest
```

Prebuilt archives for macOS, Linux, and Windows are on the
[releases page](https://github.com/dcadolph/preen/releases/latest).

preen is a single binary. It needs `git` on your `PATH` and nothing else: no
model, no API key, no network.

## Use

Run it against a dirty working tree:

```
preen
```

You get a plan like this:

```
Planned commits (4):

1. Update dependencies
     go.mod

2. Add api/server.go and its test
     api/server.go
     api/server_test.go

3. Add store/db.go
     store/db.go

4. Update docs/guide.md
     docs/guide.md

Held back as generated output (1), left uncommitted:
     api/__pycache__/server.cpython-313.pyc  [__pycache__/: Python bytecode cache]
   Add them to .gitignore, or rerun with --allow-generated to commit them.

Apply this plan? 4 commits [y/n, or ? for edits]:
```

Approve and it stages each group precisely, commits, verifies your content is
unchanged, and tells you how to undo it. Or edit the plan first, one move at a
time, with the full plan reshown after each:

```
merge 2 into 1        fold one commit into another
split 3               break a commit into one per file
move api/x.go to 2    reassign a file
reword 1 Add parser   replace a subject
drop scratch.txt      leave a file uncommitted
reorder 3,1,2         resequence
```

An edit that would stop the plan covering your tree is rejected, so the prompt
cannot walk you into losing a change.

## What it does

- Surveys every uncommitted change: staged, unstaged, and untracked.
- Groups them by package, keeps a test with the code it exercises, and separates
  dependencies, CI, documentation, and configuration, so each commit stands on
  its own. Structure is what the rules can see; a `--grouper` program judges
  intent. Add `--gate` to verify each commit builds and passes.
- Refuses to commit generated output. A `__pycache__` directory your `.gitignore`
  never listed is untracked as far as git is concerned, and a tool that produces
  commits should not help you record a bytecode cache as work. It is named in the
  plan with the reason and left where it was. `--allow-generated` commits it.
- Absorbs a run of unpushed commits back into the tree and redoes them clean
  with `--absorb`, no manual reset.
- Folds dirty changes into the unpushed commits that introduced them with
  `--fixup`, then squashes them away with an autosquash rebase.
- Refuses to redo a commit a remote already has, and moves its base forward past
  any merge whose side branch is published.
- Rewrites published history only when you ask twice, with `--pushed` and, on a
  shared branch, `--allow-protected`, then pushes with `--force-with-lease`
  behind a separate confirmation.
- Runs your build or test gate after each commit with `--gate`, rolling the whole
  run back on failure.
- Preens only part of the tree with `--scope`, leaving the rest dirty.
- Leaves named files out of every commit with `--leave`, still accounted for in
  the plan.
- Plans without acting with `--dry-run`, and skips the approval prompt with
  `--yes` for scripted runs.
- Reports debug prints, scratch markers, commented-out code, and skipped tests
  with `--sweep`, and never removes any of them.
- Undoes any run with `preen restore`, and cleans up old recovery refs with
  `preen backups --prune`.

It never invents changes and never touches a commit you did not ask it to.

## How grouping works

The grouping is deterministic and needs no model. preen separates dependency
manifests, CI configuration, documentation, and configuration from source, then
groups source by package and treats anything you staged by hand as a boundary
you drew deliberately. Dependencies are recorded first and documentation last.

Inside a package it works in units rather than files. A unit is a source file
plus the tests that exercise it, which is the one pairing a file name states
outright: `parser_test.go` belongs with `parser.go`, and `test_runner.py` with
`runner.py`. From there:

- New code is separated from changes to code that was already there, so a new
  file and its test do not ride along with two unrelated fixes.
- Tests for code this run did not otherwise touch become their own commit, named
  for what they test, and land after the code they depend on.
- A file that moved between directories keeps both halves in one commit, so it
  reads as a move rather than a deletion and an unrelated addition.

Subjects are written from that same evidence and nothing else: the files, their
directory, whether each is new or changed, and whether a test came with its
source. That gives `Add cmd/parser.go and its test` and `Update token.go and
tool.go in cmd` instead of `Update cmd`. Where a subject would run past
`--max-subject`, it gives up detail rather than being cut off, falling back
through the file name to the package name.

Two unrelated edits to two files in the same package still share a commit.
Telling them apart means reading the diff for meaning, which a fixed rule cannot
do without guessing. For the same reason the built-in grouper never splits a
file: it cannot know whether two hunks are one idea or two.

When you want that judgment, hand grouping to a program:

```
preen --grouper ./my-grouper
```

The program reads a JSON request on stdin holding every changed file and its
hunks, and writes back the commits it proposes. It can split one file's hunks
across separate commits. The contract is provider agnostic, so any model CLI,
script, or service wrapper can be a grouper, and none of them can touch your
repository: a grouper only answers, and preen verifies every path and hunk index
against the real tree before acting. If it fails, returns nothing, or names
something that is not there, the run falls back to the built-in rules rather
than trusting it.

### When an agent made the changes

A coding agent that just spent an hour in your tree knows why it touched every
hunk, which no rule and no model reading the diff cold can recover. So it can
answer the grouping request itself:

```
preen request > /tmp/request.json
preen --grouping /tmp/answer.json --leave NOTES.md --dry-run
```

`preen request` prints the same request a `--grouper` program reads, plus the
content hash of the tree, and changes nothing. Write the request and the answer
outside the repository: a file written inside it is a change to the tree they
describe. The answer is the same JSON a
grouper writes, carrying that hash back. preen refuses an answer whose hash does
not match the tree, since hunk indexes read from an earlier tree may name
different hunks now. Unlike `--grouper`, a bad answer is an error rather than a
fallback, because whoever wrote it is waiting to fix it. `--leave` keeps the
agent's scratch files out of every commit while the plan still accounts for
them.

The Claude Code skill runs this loop on its own when Claude made the changes in
the same session.

Every guardrail is the same either way. The grouper chooses *what goes where*
and nothing else.

## Generated output

A `__pycache__` directory your `.gitignore` never listed shows up in `git
status` as untracked, which is the same thing a new source file looks like. A
tool whose whole job is producing clean commits should not turn that into `Add
__pycache__`.

So preen recognizes the paths that hold generated output rather than work:
bytecode caches, installed dependencies, coverage reports, and the files editors
and operating systems leave lying around. An untracked path that matches one is
held back from the plan, named with the pattern that caught it and what it
holds, and left in your working tree exactly as it was.

Held back is not dropped. The plan still accounts for the path, you see it
before you approve anything, and the content hash preen conserves across the run
still covers it. Nothing is deleted and nothing is written to your `.gitignore`:
editing that file mid-run would change the very content the conservation check
exists to prove was unchanged.

Only untracked paths are ever held. A path git already tracks was committed
deliberately at some point, and preen does not second-guess that.

Override it where a project vendors one of these directories on purpose:

```
preen --allow-generated
```

```toml
[generated]
allow-all = false                        # commit generated output like anything else
patterns = ["*.snap"]                    # hold these back too
allow = ["third_party/node_modules"]     # commit these despite the patterns
```

A pattern with a slash is a path from the repository root and covers everything
under it. A bare name ending in a slash is a directory, matched wherever it
appears. Anything else is a glob on the file name.

## Message style

preen writes a short imperative subject by default. Dictate the format with
flags:

```
preen --conventional --prefix ABC-123 --max-subject 50 --no-emdash --no-semicolon
```

`--punctuation auto` reads your repository's own recent subjects and follows
whatever they do. `--body`, `--include-files`, and `--include-line-numbers`
control the message body, with line ranges read from the real hunk headers.

Or set defaults once in a `.preen.toml` at the repository root:

```toml
[commit]
no-emdash = true
no-semicolon = true
max-subject = 50
punctuation = "never"
conventional = true
prefix = "ABC-123"
body = "auto"
include-files = false

[run]
gate = "go test ./..."
sweep = true
allow-no-verify = false

[protect]
branches = ["develop", "release/*"]

[generated]
allow-all = false
patterns = ["*.snap"]
allow = ["third_party/node_modules"]
```

Flags beat the config file, which beats the defaults. Every generated message is
checked against the style before it is recorded, so a configured convention is
enforced rather than merely requested.

If your repository has hooks that block automated commits, set
`allow-no-verify = true` under `[run]` to grant standing consent ahead of time.
preen never bypasses a hook on its own judgment. A hook that reformats what the
run commits would normally trip the conservation check; `--allow-hook-rewrites`
accepts content differences confined to the paths the run committed.

## Rewriting published history

preen will redo commits a remote already has, but only when you say so twice.
`--pushed` grants the ask, and on a branch that is shared by name you also need
`--allow-protected`. `main`, `master`, `trunk`, `develop`, `release`, and
`production` are protected out of the box, plus anything listed under
`[protect]` in the config, which comes from the repository and can never be
dropped by a flag.

```
preen --pushed --pushed-base origin/main~4
```

The push is a third, separate confirmation, it shows you the exact command
first, and it always uses `--force-with-lease` so it aborts rather than
clobbering work that arrived after your last fetch. Consent is per invocation:
the config file cannot grant it.

## Commands

```
preen                  Group the working tree into commits.
preen restore [ref]    Undo a run. Defaults to the most recent backup.
preen backups          List recovery refs. --prune deletes the safe ones.
preen request          Print the grouping request, for an agent to answer.
```

Exit codes are distinct, so a script can tell a rolled-back run (6, 7) from a
rejected plan (5) or a declined one (8).

## Undo

```
preen restore
```

This moves the branch back and returns your work to the working tree exactly as
it was: same files, same content, uncommitted. It only ever accepts a
`preen-backup/` ref, so it cannot move your branch somewhere unrelated.

## Development

```
go test ./...
```

The tests run against real git repositories in temp directories rather than a
mock, because matching git's own index and patch behavior is the whole job. The
conservation invariant, the published-merge guard, and the restore round trip
each have their own regression test.

## Claude Code plugin

The repository is also a Claude Code plugin. Add it as a marketplace and asking
Claude to "clean up my commit history" loads a skill that drives the same
binary, with every guardrail intact:

```
/plugin marketplace add dcadolph/preen
/plugin install preen@preen
```

## License

MIT.

---

*preen: what a bird does to put every feather back in place.*

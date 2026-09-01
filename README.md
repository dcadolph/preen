![preen](preen-banner.png)

# preen

[![Latest release](https://img.shields.io/github/v/release/dcadolph/preen)](https://github.com/dcadolph/preen/releases/latest)
[![License](https://img.shields.io/github/license/dcadolph/preen)](LICENSE)

Clean up your commit history, automatically.

![preen demo](assets/demo.gif)

You got in the zone and came out with forty changed files and no commits, or one
giant blob with a message like `wip` that you are not proud of. preen turns that
into a clean, ordered set of atomic commits: the shape of the history you would
have written if you had committed carefully as you went.

Think `git add -p` and `git rebase -i`, done for you. preen reads everything you
changed, groups it into coherent commits, writes a subject for each, orders them
so the history bisects, and shows you the plan first. Nothing moves until you
approve.

The built-in subjects say where, not why: `Add api`, `Update dependencies`. For
messages that explain intent, hand grouping to any program you like with
`--grouper`, a model included, and reword anything at the approval prompt.

Clean history is worth having on its own. It makes review readable, `git bisect`
useful, and `git blame` honest. preen gets you there without the tedious
hand-staging.

## It cannot lose your work

preen only reshapes history, never content, and it enforces that rather than
promising it.

Before a run it hashes a tree holding your `HEAD` plus every staged, unstaged,
and untracked change. After the run it hashes the same thing again. The two must
match exactly. If a single byte differs, the run rolls itself back to the
recovery branch it made before it started and tells you which paths diverged.

Every run leaves a `preen-backup/<timestamp>` branch, and `preen restore` puts
you back where you were, with your work returned to the working tree exactly as
it was.

## Install

```
go install github.com/dcadolph/preen@latest
```

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

2. Add api
     api/server.go
     api/server_test.go

3. Add store
     store/db.go

4. Add guide.md
     docs/guide.md

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
- Groups them into atomic commits, one coherent idea each, ordered so
  dependencies land first and the history could be bisected.
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
groups source by package, keeps a test file with the code it exercises, keeps
rename pairs together, and treats anything you staged by hand as a boundary you
drew deliberately. Dependencies are recorded first and documentation last.

Because a fixed rule cannot know whether two hunks in one file are one idea or
two, the built-in grouper never splits a file.

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

Every guardrail is the same either way. The grouper chooses *what goes where*
and nothing else.

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

## More tools

- [kibble](https://github.com/dcadolph/kibble), test your README's install steps in a clean container
- [slop-chop](https://github.com/dcadolph/slop-chop), strip the AI tells out of your writing
- [vamoose](https://github.com/dcadolph/vamoose), route time off through approval, then tell the team
- [whodar](https://github.com/kordloom/whodar), find who to talk to about X across your work tools

## License

MIT.

---

*preen: what a bird does to put every feather back in place.*

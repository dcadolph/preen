![preen](preen-banner.png)

# preen

[![Release](https://img.shields.io/github/v/release/dcadolph/preen)](https://github.com/dcadolph/preen/releases)
[![License](https://img.shields.io/github/license/dcadolph/preen)](LICENSE)

Clean up your commit history, automatically.

![preen demo](assets/demo.gif)

You got in the zone and came out with forty changed files and no commits, or one
giant blob with a message like `wip` that you are not proud of. preen turns that
into a clean, ordered set of atomic commits with real messages: the history you
would have written if you had committed carefully as you went.

Think `git add -p` and `git rebase -i`, done for you. preen reads everything you
changed, groups it into coherent commits, writes a sensible message for each,
orders them so the history bisects, and shows you the plan first. Nothing moves
until you approve.

Clean history is worth having on its own. It makes review readable, `git bisect`
useful, and `git blame` honest. preen gets you there without the tedious
hand-staging.

## What it does

- Surveys every uncommitted change: staged, unstaged, and untracked.
- Absorbs a run of unpushed commits back into the tree and redoes them clean, no
  manual reset.
- Folds dirty changes into the unpushed commits that introduced them with
  `--fixup`, like git absorb with judgment.
- Groups everything into atomic commits, one coherent idea each.
- Writes a clear message for each, matching your repository's style.
- Orders them so dependencies land first and the history could be bisected.
- Shows the plan and acts only after you approve. Nothing moves before that.
- Optionally runs your build or test gate after each commit, so the history
  actually bisects.
- Optionally sweeps stray debug prints and dead code it spots in the diff.
- Optionally spaces the commit timestamps across a plausible window instead of
  stamping them all in the same second.
- Rewrites already-pushed commits when you explicitly ask, with a backup ref and
  a `--force-with-lease` push, never on a shared branch without confirmation.

It never invents changes and never touches a commit you did not ask it to.

## Already committed the mess?

No unstaging, no rewinding, nothing by hand. Point preen at it and it does the reset
for you:

- Unpushed commits with a bad history: preen absorbs them back into the tree and
  redoes them as clean commits.
- Already pushed: if you explicitly ask, preen rewrites them and force-pushes with
  `--force-with-lease`, after showing you exactly what will change. It refuses to
  rewrite shared branches like `main` unless you confirm the branch is yours
  alone, and it always saves a backup ref you can reset to.

Just run it:

```
/preen
```

## Install

As a plugin:

```
/plugin marketplace add dcadolph/preen
/plugin install preen@preen
```

Or drop the skill in place manually:

```
cp -r skills/preen ~/.claude/skills/preen
```

Or install the CLI, which needs no plugin or skill install at all. It embeds
this release's skill text and launches Claude Code with it:

```
brew install --cask dcadolph/tap/preen
```

```
go install github.com/dcadolph/preen@latest
```

Prebuilt binaries for macOS, Linux, and Windows are attached to each
[release](https://github.com/dcadolph/preen/releases).

## Use

Run it against a dirty working tree:

```
/preen
```

You get a plan like this:

```
Planned 4 commits:
  1. Add config loader          internal/config/loader.go
  2. Wire --output-dir flag     cmd/flag_output.go
  3. Test config loader         internal/config/loader_test.go
  4. Note new flag in README    README.md
Spread over ~2h ending now. Approve? (y / edit / n)
```

Approve and it stages each group precisely, commits with clean messages, and
prints the resulting log. Or edit the plan first: `merge 2 into 1`, `split 4`,
`move README.md to 3`, `reword 2: Add loader tests`, `drop scratch.txt`,
`reorder 2,1,3`.

The CLI takes the same flags from any terminal:

```
preen --fixup --scope internal/
preen --headless --gate 'go test ./...'
```

Plain runs open a Claude Code session where you approve the plan as usual.
`--headless` runs `claude -p` with `--yes` for CI and scripts. Flags after
`--` go to the claude CLI itself. The binary carries its own copy of the
skill, pinned at build time, so it behaves the same on every machine and
overrides any installed preen plugin for that run.

## Message style

preen writes a short imperative subject by default and matches your repo's
convention. Dictate the format with options on the invocation:

```
/preen --no-emdash --no-semicolon --max-subject 50 --include-line-numbers
```

Or set defaults once in a `.preen.toml` at the repo root:

```
[commit]
no-emdash = true
no-semicolon = true
max-subject = 50
prefix = "ABC-123"

[run]
gate = "go test ./..."

[protect]
branches = ["develop"]
```

Options: `--no-emdash`, `--no-semicolon`, `--no-hyphen`, `--max-subject N`,
`--punctuation always|auto|never`, `--lower-subject`, `--conventional`,
`--body always|auto|never`, `--include-files`, `--include-line-numbers`,
`--prefix TEXT`, `--sign-off`.
Invocation options beat the config file, which beats the defaults.

Run options: `--scope <pathspec>` preens only part of the tree and leaves the
rest dirty, `--gate <cmd>` runs your check after each commit and stops on
failure, `--dry-run` shows the plan and stops, `--fixup` folds dirty changes
into the unpushed commits they belong to, `--yes` skips the approval prompt for
scripted runs, `--prune-backups` cleans up old backup refs. If your repo has hooks that block automated commits, set
`allow-no-verify = true` under `[run]` to grant standing consent to bypass
them.

## Limitations

Honesty about what this is: preen is a skill, not a compiled program. The
options are conventions the agent follows and then verifies against its own
output, not a parser; the eval suite checks they hold, but they are enforced by
instruction, not code. Splitting one file's hunks across several commits is the
hardest move and can degrade to whole-file grouping on gnarly diffs. Very large
diffs cost real tokens to survey. Every run creates a backup ref first, so the
worst case is always one reset away.

## Undo

The split is reversible. Reset back to where you started and every change
returns to the working tree:

```
git reset --soft <sha-before-the-split>
```

## Development

`hack/eval.sh` runs the skill headless against fixture repos and asserts on
the resulting git state: clean tree, commit counts, style conformance, merge
and scope guards. It needs the claude CLI and spends real tokens; `hack/eval.sh`
for the quick set, `--all` for everything.

## License

MIT.

---

*preen: what a bird does to put every feather back in place.*

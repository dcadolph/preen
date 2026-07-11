![gus](gus-banner.png)

# gus

Clean up your commit history, automatically.

![gus demo](assets/demo.gif)

You got in the zone and came out with forty changed files and no commits, or one
giant blob with a message like `wip` that you are not proud of. gus turns that
into a clean, ordered set of atomic commits with real messages: the history you
would have written if you had committed carefully as you went.

Think `git add -p` and `git rebase -i`, done for you. gus reads everything you
changed, groups it into coherent commits, writes a sensible message for each,
orders them so the history bisects, and shows you the plan first. Nothing moves
until you approve.

Clean history is worth having on its own. It makes review readable, `git bisect`
useful, and `git blame` honest. gus gets you there without the tedious
hand-staging.

## What it does

- Surveys every uncommitted change: staged, unstaged, and untracked.
- Absorbs a run of unpushed commits back into the tree and redoes them clean, no
  manual reset.
- Groups everything into atomic commits, one coherent idea each.
- Writes a clear message for each, matching your repository's style.
- Orders them so dependencies land first and the history could be bisected.
- Shows the plan and acts only after you approve. Nothing moves before that.
- Optionally sweeps stray debug prints and dead code it spots in the diff.
- Optionally spaces the commit timestamps across a plausible window instead of
  stamping them all in the same second.
- Rewrites already-pushed commits when you explicitly ask, with a backup ref and
  a `--force-with-lease` push, never on a shared branch without confirmation.

It never invents changes and never touches a commit you did not ask it to.

## Already committed the mess?

No unstaging, no rewinding, nothing by hand. Point gus at it and it does the reset
for you:

- Unpushed commits with a bad history: gus absorbs them back into the tree and
  redoes them as clean commits.
- Already pushed: if you explicitly ask, gus rewrites them and force-pushes with
  `--force-with-lease`, after showing you exactly what will change. It refuses to
  rewrite shared branches like `main` unless you confirm the branch is yours
  alone, and it always saves a backup ref you can reset to.

Just run it:

```
/gus
```

## Install

As a plugin:

```
/plugin marketplace add dcadolph/gus
/plugin install gus@gus
```

Or drop the skill in place manually:

```
cp -r skills/gus ~/.claude/skills/gus
```

## Use

Run it against a dirty working tree:

```
/gus
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
prints the resulting log.

## Message style

gus writes a short imperative subject by default and matches your repo's
convention. Dictate the format with options on the invocation:

```
/gus --no-emdash --no-semicolon --max-subject 50 --include-line-numbers
```

Or set defaults once in a `.gus.toml` at the repo root:

```
[commit]
no-emdash = true
no-semicolon = true
max-subject = 50
prefix = "ABC-123"
```

Options: `--no-emdash`, `--no-semicolon`, `--no-hyphen`, `--max-subject N`,
`--no-period`, `--lower-subject`, `--conventional`, `--body always|auto|never`,
`--include-files`, `--include-line-numbers`, `--prefix TEXT`, `--sign-off`.
Invocation options beat the config file, which beats the defaults.

## Undo

The split is reversible. Reset back to where you started and every change
returns to the working tree:

```
git reset --soft <sha-before-the-split>
```

## License

MIT.

---

*gus runs a clean operation. Somebody has to cook the books.*

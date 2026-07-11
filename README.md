![gus](gus-banner.png)

# gus

Clean up your commit history, automatically.

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
- Groups them into atomic commits, one coherent idea each.
- Writes a clear message for each, matching your repository's style.
- Orders them so dependencies land first and the history could be bisected.
- Shows the plan and commits only after you approve. Nothing moves before that.
- Optionally sweeps stray debug prints and dead code it spots in the diff.
- Optionally spaces the commit timestamps across a plausible window instead of
  stamping them all in the same second.

It never invents changes, never rewrites commits that are already pushed, and
never pushes.

## Already committed the mess?

gus works on the working tree. If you already committed a pile of local, unpushed
work with a bad message, hand it back to the tree first, then run gus:

```
git reset --soft <sha-before-the-mess>
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

## Undo

The split is reversible. Reset back to where you started and every change
returns to the working tree:

```
git reset --soft <sha-before-the-split>
```

## License

MIT.

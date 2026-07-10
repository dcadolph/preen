# gus

```
        ~
       (o)>     cluck.
      /( )\
        ^ ^
```

The fixer for your commit history.

You finished a chunk of work and now the tree has forty changed files staged as
one blob. `gus` turns that mess into a clean, ordered set of commits that reads
like real, incremental work. Hide the messy operation behind a spotless front.

## What it does

- Surveys every uncommitted change: staged, unstaged, and untracked.
- Groups them into atomic commits, one coherent idea each.
- Orders them so dependencies land first and the history could be bisected.
- Shows the plan and commits only after you approve. Nothing moves before that.
- Optionally back-dates the commits with irregular gaps so the history looks
  like a work session, not a burst of commits in the same second.
- Optionally sweeps stray debug prints and dead code before committing.

It never invents changes and never pushes.

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

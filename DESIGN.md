# preen design

Working design doc: what preen is, the invariants it holds, how it is tested
and released, and what is left to build. Update this when behavior or plans
change.

## Shape

preen is a Go program. It runs in process with no model, no API key, and no
network; `git` on the `PATH` is the only requirement.

The packages divide along one line: mechanics and judgment.

| Package  | Holds |
| -------- | ----- |
| `repo`   | A typed layer over git, behind a one-method `Runner` interface. |
| `plan`   | The intent of a run as data: renderable, editable, validatable. |
| `group`  | The one judgment call, behind a `Grouper` interface. |
| `style`  | Commit message conventions, applied and verified. |
| `sweep`  | Debris detection. Reports only, never removes. |
| `config` | `.preen.toml`, the per-repository defaults. |
| `run`    | Orchestration and every guardrail. |
| `cmd`    | Argument parsing and rendering, a thin shell over the rest. |

Real git is shelled out to rather than reimplemented. preen rewrites history,
so matching git's own index, patch application, and rebase behavior exactly
matters more than avoiding a process boundary.

## What it does

- Surveys every uncommitted change, including untracked files, and optionally
  absorbs unpushed commits back into the tree to be redone.
- Groups the changes into self-contained commits, ordered so dependencies land
  first. The rules group by structure; semantic splitting needs a grouper.
- Shows a plan and changes nothing until it is approved. The prompt accepts
  merge, split, move, reword, drop, and reorder.
- Stages each group precisely, including a subset of one file's hunks when an
  external grouper splits a file (the built-in grouper never does), commits,
  and optionally runs a gate after each commit.
- Folds changes into the commits that introduced them with `--fixup`.
- Rewrites published history only behind two consents, then pushes behind a
  third, always with `--force-with-lease`.
- Undoes any run with `preen restore`.

## Invariants

These are enforced in code. Each has a regression test, and the tests run
against real repositories rather than a mock.

1. **Content is conserved.** A run hashes a tree of HEAD plus every staged,
   unstaged, and untracked change before and after itself. Any difference rolls
   the run back to the recovery ref. The only accepted exception is a commit
   hook reformatting files the run committed, and only when the caller passed
   `--allow-hook-rewrites`, which is then reported.
2. **A plan accounts for the tree exactly once.** Every change lands in a
   commit or a declared leftover, nothing lands twice, no commit is empty. An
   edit that breaks this is refused and the previous plan stands.
3. **Published work is not redone by accident.** A merge whose second parent is
   reachable from any remote moves the base forward instead of being flattened,
   and redoing a pushed commit requires `--pushed`.
4. **A protected branch is not rewritten.** The built-in names plus anything in
   `[protect]`, overridable only by `--allow-protected`. Protection comes from
   the repository, never from a flag.
5. **A hunk is identified by content, not position.** Committing one hunk
   renumbers the rest, so a planned hunk is found again by its body.
6. **Undo restores the mess.** `preen restore` is a mixed reset: HEAD and the
   index move, the working tree does not. `--hard` and `--keep` both delete
   files the undone commits added, which is a data-loss bug, not a preference.
7. **Nothing is deleted on a guess.** The sweep reports debris and never
   removes it.

## Tests

`go test ./...` is the whole suite. It builds real repositories in temp
directories, with real remotes where publishing matters and real hooks where
hook behavior matters. There is no agent in the loop and no token spend.

The eval harness that drove the old skill through the claude CLI is gone with
the architecture it tested.

## Releases

Tag `vX.Y.Z`; the release workflow cross-builds and attaches archives. Keep
`cmd/version.go` and `.claude-plugin/plugin.json` in step.

## Backlog

- Punctuation `auto` reads the repository's recent subjects, but the body mode
  has no equivalent inference.
- The rebase-conflict path aborts and rolls back; it has no test, because
  producing a reliable conflict in a fixture is fiddly.
- Fixup targeting is per file. Per hunk would need blame attribution and a
  wrong answer rewrites the wrong commit, so it stays coarse deliberately.
- The grouper contract is JSON over stdin and stdout. No reference grouper
  ships with preen.

## External artifacts

The demo fixtures under `hack/` build throwaway repositories for recording the
README demo. They do not test anything.

#!/usr/bin/env bash
# Eval harness for the preen skill. Builds throwaway fixture repos, runs preen
# headless through the claude CLI, and asserts on the resulting git state.
#
# Each case is a full agent run: expect a minute or more and real token spend
# per case. Requires the claude CLI on PATH and an authenticated account.
#
# Usage:
#   hack/eval.sh          # quick set (c1 c3 c4)
#   hack/eval.sh --all    # every case
#   hack/eval.sh c5       # one case by id
set -euo pipefail

SKILL_SRC="$(cd "$(dirname "$0")/.." && pwd)/skills/preen"
MAX_TURNS="${PREEN_EVAL_MAX_TURNS:-50}"
PASS=0
FAIL=0
FAILED_CASES=()

say() { printf '%s\n' "$*"; }

# content_tree prints the tree hash of HEAD plus every staged, unstaged, and
# untracked change in the fixture, computed on a scratch index so the real
# index is untouched. preen only reshapes history, so this fingerprint must be
# identical before and after a run: one that dropped or invented a change
# changes the hash.
content_tree() {
  local dir="$1" idx
  idx="$(mktemp -u "${TMPDIR:-/tmp}/preen-idx.XXXXXX")"
  GIT_INDEX_FILE="$idx" git -C "$dir" read-tree HEAD
  GIT_INDEX_FILE="$idx" git -C "$dir" add -A
  GIT_INDEX_FILE="$idx" git -C "$dir" write-tree
  rm -f "$idx"
}

# new_fixture makes a fresh repo with one seed commit and the skill installed,
# and prints its path.
new_fixture() {
  local dir
  dir="$(mktemp -d "${TMPDIR:-/tmp}/preen-eval.XXXXXX")"
  git -C "$dir" init -q -b main
  git -C "$dir" config user.name "Eval User"
  git -C "$dir" config user.email "eval@example.invalid"
  git -C "$dir" config commit.gpgsign false
  echo "# fixture" > "$dir/README.md"
  printf '.claude/\n' > "$dir/.gitignore"
  git -C "$dir" add README.md .gitignore
  git -C "$dir" commit -qm "Seed fixture"
  mkdir -p "$dir/.claude/skills/preen"
  cp "$SKILL_SRC/SKILL.md" "$dir/.claude/skills/preen/SKILL.md"
  echo "$dir"
}

# add_remote gives the fixture a bare origin with main pushed, so the branch
# has an upstream and pushed-ness checks mean something.
add_remote() {
  local dir="$1" bare
  bare="$(mktemp -d "${TMPDIR:-/tmp}/preen-eval-remote.XXXXXX")"
  git -C "$bare" init -q --bare -b main
  git -C "$dir" remote add origin "$bare"
  git -C "$dir" push -qu origin main
}

# run_preen runs the skill headless in the fixture. The prompt should include
# --yes so the agent does not wait on an approval nobody will give. The prompt
# names the fixture's own SKILL.md explicitly: a user-level skill copy or an
# installed preen plugin would otherwise shadow the version under test.
run_preen() {
  local dir="$1" prompt="$2"
  CONTENT_BEFORE="$(content_tree "$dir")"
  local instr="Read the file .claude/skills/preen/SKILL.md in this repository \
and follow it exactly as the preen skill for this invocation. It overrides any \
other preen skill, plugin, or prior knowledge of preen. Invocation: $prompt"
  (cd "$dir" && claude -p "$instr" \
    --dangerously-skip-permissions \
    --max-turns "$MAX_TURNS" \
    --verbose \
    --output-format stream-json) > "$dir.log" 2>&1 || true
}

assert() {
  local case_id="$1" desc="$2" cond="$3"
  if eval "$cond"; then
    say "  ok: $desc"
  else
    say "  FAIL: $desc"
    CASE_OK=false
  fi
}

# assert_conserved checks that the run neither lost nor invented a change: the
# content fingerprint after the run matches the one run_preen captured before
# it. Holds in every mode, since preen only reshapes history. Cases that
# deliberately drop or sweep paths would compare against a trimmed baseline;
# none of the current cases do.
assert_conserved() {
  local case_id="$1" dir="$2"
  assert "$case_id" "content conserved" "[ \"\$(content_tree '$dir')\" = '$CONTENT_BEFORE' ]"
}

finish_case() {
  local case_id="$1" dir="$2"
  if $CASE_OK; then
    PASS=$((PASS + 1))
    say "$case_id passed"
    rm -rf "$dir" "$dir.log"
  else
    FAIL=$((FAIL + 1))
    FAILED_CASES+=("$case_id")
    say "$case_id FAILED, fixture kept at $dir (log: $dir.log)"
  fi
}

# c1: dirty tree with three unrelated concerns becomes multiple clean commits.
c1() {
  say "c1: basic split of a dirty tree"
  local dir; dir="$(new_fixture)"; CASE_OK=true
  printf 'package a\n\nfunc Add(x, y int) int { return x + y }\n' > "$dir/math.go"
  printf 'Install: go install ./...\n' >> "$dir/README.md"
  printf 'linters:\n  enable: [misspell]\n' > "$dir/.golangci.yml"
  run_preen "$dir" "/preen --yes"
  local count
  count="$(git -C "$dir" rev-list --count HEAD)"
  assert c1 "tree is clean" "[ -z \"\$(git -C '$dir' status --porcelain)\" ]"
  assert c1 "made at least 2 commits (got $((count - 1)))" "[ '$count' -ge 3 ]"
  assert c1 "backup ref exists" "git -C '$dir' branch --list 'preen-backup/*' | grep -q ."
  assert c1 "no wip messages" "! git -C '$dir' log --format=%s | grep -qi '^wip'"
  assert_conserved c1 "$dir"
  finish_case c1 "$dir"
}

# c2: two sloppy unpushed commits plus dirty work are absorbed and redone.
c2() {
  say "c2: absorb unpushed commits"
  local dir; dir="$(new_fixture)"; add_remote "$dir"; CASE_OK=true
  printf 'package b\n\nfunc Sub(x, y int) int { return x - y }\n' > "$dir/sub.go"
  git -C "$dir" add sub.go && git -C "$dir" commit -qm "wip"
  printf '\n// Doc line.\n' >> "$dir/sub.go"
  printf 'Usage: see sub.go\n' >> "$dir/README.md"
  git -C "$dir" add -A && git -C "$dir" commit -qm "more stuff asdf"
  printf 'package b\n\n// Mul multiplies.\nfunc Mul(x, y int) int { return x * y }\n' > "$dir/mul.go"
  run_preen "$dir" "/preen --yes"
  assert c2 "tree is clean" "[ -z \"\$(git -C '$dir' status --porcelain)\" ]"
  assert c2 "wip subjects are gone" "! git -C '$dir' log --format=%s | grep -qiE '^(wip|more stuff)'"
  assert c2 "seed commit untouched" "git -C '$dir' log --format=%s | grep -q 'Seed fixture'"
  assert c2 "nothing was pushed" "[ \"\$(git -C '$dir' rev-list --count origin/main)\" -eq 1 ]"
  assert_conserved c2 "$dir"
  finish_case c2 "$dir"
}

# c3: style flags hold: conventional subjects capped at 50 characters, each
# ending with a period per --punctuation always.
c3() {
  say "c3: style flags (--conventional --max-subject 50 --punctuation always)"
  local dir; dir="$(new_fixture)"; CASE_OK=true
  printf 'package c\n\nfunc Neg(x int) int { return -x }\n' > "$dir/neg.go"
  printf 'Notes on negation.\n' > "$dir/NOTES.md"
  run_preen "$dir" "/preen --yes --conventional --max-subject 50 --punctuation always"
  assert c3 "tree is clean" "[ -z \"\$(git -C '$dir' status --porcelain)\" ]"
  assert c3 "subjects are conventional" \
    "! git -C '$dir' log --format=%s HEAD~\$(($(git -C "$dir" rev-list --count HEAD) - 1))..HEAD 2>/dev/null | grep -vqE '^[a-z]+(\([^)]+\))?!?: '"
  assert c3 "subjects within 50 chars" \
    "! git -C '$dir' log --format=%s | awk 'length(\$0) > 50' | grep -q ."
  assert c3 "subjects end with a period" \
    "! git -C '$dir' log --format=%s HEAD~\$(($(git -C "$dir" rev-list --count HEAD) - 1))..HEAD 2>/dev/null | grep -vq '\.$'"
  assert_conserved c3 "$dir"
  finish_case c3 "$dir"
}

# c4: dry run plans but changes nothing.
c4() {
  say "c4: --dry-run touches nothing"
  local dir; dir="$(new_fixture)"; CASE_OK=true
  printf 'package d\n' > "$dir/d.go"
  local before; before="$(git -C "$dir" rev-parse HEAD)"
  run_preen "$dir" "/preen --dry-run"
  assert c4 "HEAD unmoved" "[ \"\$(git -C '$dir' rev-parse HEAD)\" = '$before' ]"
  assert c4 "file still dirty" "git -C '$dir' status --porcelain | grep -q 'd.go'"
  assert_conserved c4 "$dir"
  finish_case c4 "$dir"
}

# c5: a foreign merge in the absorb range is never flattened. The feature
# branch is pushed before merging, so the merge's second parent is
# remote-reachable and flattening it would re-commit pushed work.
c5() {
  say "c5: foreign merge guard"
  local dir; dir="$(new_fixture)"; add_remote "$dir"; CASE_OK=true
  git -C "$dir" checkout -qb feature
  printf 'feature work\n' > "$dir/feature.txt"
  git -C "$dir" add feature.txt && git -C "$dir" commit -qm "Add feature file"
  git -C "$dir" push -qu origin feature
  git -C "$dir" checkout -q main
  printf 'main work\n' > "$dir/main.txt"
  git -C "$dir" add main.txt && git -C "$dir" commit -qm "Add main file"
  git -C "$dir" merge -q --no-ff feature -m "Merge feature"
  printf 'after merge\n' > "$dir/after.txt"
  git -C "$dir" add after.txt && git -C "$dir" commit -qm "wip after merge"
  local merges_before; merges_before="$(git -C "$dir" rev-list --merges --count HEAD)"
  run_preen "$dir" "/preen --yes"
  assert c5 "foreign merge survived" \
    "[ \"\$(git -C '$dir' rev-list --merges --count HEAD)\" -eq '$merges_before' ]"
  assert c5 "wip commit was redone" \
    "! git -C '$dir' log --format=%s | grep -qi '^wip'"
  assert c5 "tree is clean" "[ -z \"\$(git -C '$dir' status --porcelain -uall)\" ]"
  assert_conserved c5 "$dir"
  finish_case c5 "$dir"
}

# c6: --scope leaves out-of-scope changes dirty.
c6() {
  say "c6: --scope limits the split"
  local dir; dir="$(new_fixture)"; CASE_OK=true
  mkdir -p "$dir/src" "$dir/exp"
  printf 'package src\n' > "$dir/src/a.go"
  printf 'scratch experiment\n' > "$dir/exp/scratch.txt"
  run_preen "$dir" "/preen --yes --scope src/"
  assert c6 "in-scope file committed" "! git -C '$dir' status --porcelain -uall | grep -q 'src/a.go'"
  assert c6 "out-of-scope file still dirty" "git -C '$dir' status --porcelain -uall | grep -q 'exp/scratch.txt'"
  assert_conserved c6 "$dir"
  finish_case c6 "$dir"
}

# c7: --fixup folds dirty edits into the unpushed commits that introduced
# them, preserving messages and creating no new commits.
c7() {
  say "c7: --fixup targets the introducing commits"
  local dir; dir="$(new_fixture)"; add_remote "$dir"; CASE_OK=true
  printf 'package g\n\n// Greet says hello.\nfunc Greet() string { return "hello" }\n' > "$dir/greet.go"
  git -C "$dir" add greet.go && git -C "$dir" commit -qm "Add greet function"
  printf 'package g\n\n// Farewell says goodbye.\nfunc Farewell() string { return "goodby" }\n' > "$dir/farewell.go"
  git -C "$dir" add farewell.go && git -C "$dir" commit -qm "Add farewell function"
  printf 'package g\n\n// Greet says hello.\nfunc Greet() string { return "hello, world" }\n' > "$dir/greet.go"
  printf 'package g\n\n// Farewell says goodbye.\nfunc Farewell() string { return "goodbye" }\n' > "$dir/farewell.go"
  run_preen "$dir" "/preen --yes --fixup"
  assert c7 "tree is clean" "[ -z \"\$(git -C '$dir' status --porcelain -uall)\" ]"
  assert c7 "no new commits" "[ \"\$(git -C '$dir' rev-list --count HEAD)\" -eq 3 ]"
  assert c7 "messages preserved" \
    "[ \"\$(git -C '$dir' log --format=%s origin/main..HEAD | tr '\n' '|')\" = 'Add farewell function|Add greet function|' ]"
  assert c7 "greet fix landed in its commit" \
    "git -C '$dir' show \$(git -C '$dir' log --format=%H --grep='^Add greet function\$'):greet.go | grep -q 'hello, world'"
  assert c7 "farewell fix landed in its commit" \
    "git -C '$dir' show \$(git -C '$dir' log --format=%H --grep='^Add farewell function\$'):farewell.go | grep -q 'goodbye'"
  assert c7 "backup ref exists" "git -C '$dir' branch --list 'preen-backup/*' | grep -q ."
  assert c7 "nothing was pushed" "[ \"\$(git -C '$dir' rev-list --count origin/main)\" -eq 1 ]"
  assert_conserved c7 "$dir"
  finish_case c7 "$dir"
}

# add_blocking_hook installs a pre-commit hook that rejects every commit.
add_blocking_hook() {
  local dir="$1"
  printf '#!/bin/sh\necho "hook: commits are blocked in this repo"\nexit 1\n' \
    > "$dir/.git/hooks/pre-commit"
  chmod +x "$dir/.git/hooks/pre-commit"
}

# c8: allow-no-verify standing consent lets preen commit past a blocking hook.
c8() {
  say "c8: allow-no-verify bypasses a blocking hook"
  local dir; dir="$(new_fixture)"; CASE_OK=true
  printf '[run]\nallow-no-verify = true\n' > "$dir/.preen.toml"
  git -C "$dir" add .preen.toml && git -C "$dir" commit -qm "Add preen config"
  add_blocking_hook "$dir"
  printf 'package h\n\nfunc Half(x int) int { return x / 2 }\n' > "$dir/half.go"
  printf 'Notes on halving.\n' > "$dir/NOTES.md"
  run_preen "$dir" "/preen --yes"
  assert c8 "tree is clean" "[ -z \"\$(git -C '$dir' status --porcelain -uall)\" ]"
  assert c8 "commits were made past the hook" \
    "[ \"\$(git -C '$dir' rev-list --count HEAD)\" -ge 3 ]"
  assert_conserved c8 "$dir"
  finish_case c8 "$dir"
}

# c9: without consent, a hook rejection stops the run: no bypass, no commits.
c9() {
  say "c9: hook rejection without consent stops cleanly"
  local dir; dir="$(new_fixture)"; CASE_OK=true
  add_blocking_hook "$dir"
  printf 'package i\n\nfunc Inc(x int) int { return x + 1 }\n' > "$dir/inc.go"
  local before; before="$(git -C "$dir" rev-parse HEAD)"
  run_preen "$dir" "/preen --yes"
  assert c9 "no commit got past the hook" "[ \"\$(git -C '$dir' rev-parse HEAD)\" = '$before' ]"
  assert c9 "work is still in the tree" "git -C '$dir' status --porcelain -uall | grep -q 'inc.go'"
  assert_conserved c9 "$dir"
  finish_case c9 "$dir"
}

# c10: --spread back-dates commits across the window: timestamps strictly
# increase, none in the future, none before the base commit or the window
# start. The seed commit is re-dated two hours back so the window has room.
c10() {
  say "c10: --spread spaces commit timestamps"
  local dir; dir="$(new_fixture)"; CASE_OK=true
  local past; past=$(( $(date +%s) - 7200 ))
  GIT_COMMITTER_DATE="@$past" GIT_AUTHOR_DATE="@$past" \
    git -C "$dir" commit -q --amend --no-edit
  printf 'package j\n\nfunc Dbl(x int) int { return x * 2 }\n' > "$dir/dbl.go"
  printf 'Notes on doubling.\n' > "$dir/NOTES.md"
  printf 'linters:\n  enable: [misspell]\n' > "$dir/.golangci.yml"
  run_preen "$dir" "/preen --yes --spread 1h"
  local now; now="$(date +%s)"
  local stamps sorted last first_new
  stamps="$(git -C "$dir" log --reverse --format=%ct | tr '\n' ' ')"
  sorted="$(git -C "$dir" log --format=%ct | sort -n -u | tr '\n' ' ')"
  last="$(git -C "$dir" log -1 --format=%ct)"
  first_new="$(git -C "$dir" log --reverse --format=%ct | sed -n 2p)"
  assert c10 "tree is clean" "[ -z \"\$(git -C '$dir' status --porcelain -uall)\" ]"
  assert c10 "made at least 2 commits" "[ \"\$(git -C '$dir' rev-list --count HEAD)\" -ge 3 ]"
  assert c10 "timestamps strictly increase" "[ '$stamps' = '$sorted' ]"
  assert c10 "no timestamp in the future" "[ '$last' -le '$now' ]"
  assert c10 "first new commit not before the base" "[ '$first_new' -ge '$past' ]"
  assert c10 "first new commit inside the window" "[ '$first_new' -ge $(( now - 4200 )) ]"
  assert_conserved c10 "$dir"
  finish_case c10 "$dir"
}

# c11: --pushed grants the explicit ask a pushed rewrite requires: sloppy
# commits already pushed on a feature branch are redone and force-pushed with
# lease. No base value, so the base resolves to the merge-base with main.
c11() {
  say "c11: --pushed rewrites pushed commits"
  local dir; dir="$(new_fixture)"; add_remote "$dir"; CASE_OK=true
  git -C "$dir" checkout -qb feature
  printf 'package k\n\nfunc Sq(x int) int { return x * x }\n' > "$dir/sq.go"
  git -C "$dir" add sq.go && git -C "$dir" commit -qm "wip"
  printf 'Notes on squaring.\n' > "$dir/NOTES.md"
  printf '\n// Doc line.\n' >> "$dir/sq.go"
  git -C "$dir" add -A && git -C "$dir" commit -qm "stuff idk"
  git -C "$dir" push -qu origin feature
  run_preen "$dir" "/preen --yes --pushed"
  local remote_tip
  remote_tip="$(git -C "$dir" ls-remote origin feature | cut -f1)"
  assert c11 "tree is clean" "[ -z \"\$(git -C '$dir' status --porcelain -uall)\" ]"
  assert c11 "wip subjects are gone" \
    "! git -C '$dir' log --format=%s | grep -qiE '^(wip|stuff)'"
  assert c11 "seed commit untouched" "git -C '$dir' log --format=%s | grep -q 'Seed fixture'"
  assert c11 "remote feature matches the rewrite" \
    "[ '$remote_tip' = \"\$(git -C '$dir' rev-parse HEAD)\" ]"
  assert c11 "main was not touched" \
    "[ \"\$(git -C '$dir' rev-list --count origin/main)\" -eq 1 ]"
  assert c11 "backup ref exists" "git -C '$dir' branch --list 'preen-backup/*' | grep -q ."
  assert_conserved c11 "$dir"
  finish_case c11 "$dir"
}

QUICK=(c1 c3 c4)
ALL=(c1 c2 c3 c4 c5 c6 c7 c8 c9 c10 c11)

main() {
  command -v claude >/dev/null || { say "claude CLI not found"; exit 1; }
  local cases=("${QUICK[@]}")
  if [ "${1:-}" = "--all" ]; then
    cases=("${ALL[@]}")
  elif [ -n "${1:-}" ]; then
    cases=("$@")
  fi
  for c in "${cases[@]}"; do "$c"; done
  say ""
  say "passed: $PASS  failed: $FAIL"
  [ "$FAIL" -eq 0 ] || { say "failed cases: ${FAILED_CASES[*]}"; exit 1; }
}

main "$@"

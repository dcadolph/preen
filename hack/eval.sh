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
# --yes so the agent does not wait on an approval nobody will give.
run_preen() {
  local dir="$1" prompt="$2"
  (cd "$dir" && claude -p "$prompt" \
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
  finish_case c2 "$dir"
}

# c3: style flags hold: conventional subjects capped at 50 characters.
c3() {
  say "c3: style flags (--conventional --max-subject 50)"
  local dir; dir="$(new_fixture)"; CASE_OK=true
  printf 'package c\n\nfunc Neg(x int) int { return -x }\n' > "$dir/neg.go"
  printf 'Notes on negation.\n' > "$dir/NOTES.md"
  run_preen "$dir" "/preen --yes --conventional --max-subject 50"
  assert c3 "tree is clean" "[ -z \"\$(git -C '$dir' status --porcelain)\" ]"
  assert c3 "subjects are conventional" \
    "! git -C '$dir' log --format=%s HEAD~\$(($(git -C "$dir" rev-list --count HEAD) - 1))..HEAD 2>/dev/null | grep -vqE '^[a-z]+(\([^)]+\))?!?: '"
  assert c3 "subjects within 50 chars" \
    "! git -C '$dir' log --format=%s | awk 'length(\$0) > 50' | grep -q ."
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
  finish_case c6 "$dir"
}

QUICK=(c1 c3 c4)
ALL=(c1 c2 c3 c4 c5 c6)

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

# Sourced by the gus demo tape for the pushed-history scenario. Builds a repo
# with a few already-pushed messy commits and leaves one more change uncommitted,
# so the tape can commit and push it live before invoking a gus stand-in that
# rewrites the run and force pushes to a local bare remote. Not used at runtime.

cd /tmp
rm -rf gusdemo-pushed gusdemo-remote.git
git init -q --bare gusdemo-remote.git
git init -q -b main gusdemo-pushed
cd gusdemo-pushed
git config user.email demo@example.com
git config user.name demo
git config commit.gpgsign false
git config core.pager cat
git config log.decorate short
git remote add origin /tmp/gusdemo-remote.git

mkdir -p internal/config internal/api internal/store cmd
printf 'module demo\n\ngo 1.22\n' > go.mod
printf '# demo\n\nA small service.\n' > README.md
git add -A
git commit -q -m "Initial project"
git push -q -u origin main

# A few messy commits with lazy messages, already pushed to origin.
cat > internal/config/config.go <<'EOF'
package config

type Config struct{ OutputDir string }
EOF
cat > internal/config/loader.go <<'EOF'
package config

import "os"

func Load() *Config { return &Config{OutputDir: os.Getenv("OUTPUT_DIR")} }
EOF
git add -A && git commit -q -m "working now"

cat > internal/api/handler.go <<'EOF'
package api

import "net/http"

func Health(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }
EOF
git add -A && git commit -q -m "working now"

cat > cmd/flags.go <<'EOF'
package cmd

var OutputDir string
EOF
git add -A && git commit -q -m "more"

git push -q origin main

# One more chunk of work, left uncommitted for the tape to commit and push live.
cat > internal/store/store.go <<'EOF'
package store

import "time"

type Store struct{ Timeout time.Duration }
EOF
printf '# demo\n\nA small service.\n\n## Flags\n\n- --output-dir sets the output directory.\n' > README.md

# Scripted stand-in for the gus skill in pushed-rewrite mode.
gus() {
  git branch gus-backup/demo >/dev/null 2>&1
  git reset --soft HEAD~4 >/dev/null 2>&1
  git reset -q >/dev/null 2>&1
  printf '\n\033[1mgus\033[0m these 4 commits are already pushed. Backup at gus-backup/demo.\n'
  printf 'Planned 5 clean commits, then a force push with lease:\n\n'
  printf '  1. Add config loader     internal/config/config.go, loader.go\n'
  printf '  2. Add health handler    internal/api/handler.go\n'
  printf '  3. Add output dir flag   cmd/flags.go\n'
  printf '  4. Add store             internal/store/store.go\n'
  printf '  5. Update the docs       README.md\n'
  printf '\nrewrite and force push? (y) y\n\n'
  git add internal/config/config.go internal/config/loader.go && git commit -q -m "Add config loader"
  git add internal/api/handler.go && git commit -q -m "Add health handler"
  git add cmd/flags.go && git commit -q -m "Add output dir flag"
  git add internal/store/store.go && git commit -q -m "Add store"
  git add README.md && git commit -q -m "Update the docs"
  git push --force-with-lease -q origin main >/dev/null 2>&1
  printf '\033[32mhistory rewritten and force pushed with lease\033[0m\n'
}

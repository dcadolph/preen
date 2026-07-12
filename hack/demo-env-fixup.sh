# Sourced by the preen demo tape for the fixup scenario. Builds a repo with two
# clean unpushed commits and dirty review fixes touching both, so the tape can
# invoke a preen stand-in that folds each fix into the commit that introduced
# it. Not used at runtime by preen itself.

cd /tmp
rm -rf preendemo-fixup preendemo-fixup-remote.git
git init -q --bare preendemo-fixup-remote.git
git init -q -b main preendemo-fixup
cd preendemo-fixup
git config user.email demo@example.com
git config user.name demo
git config commit.gpgsign false
git config core.pager cat
git config log.decorate short
git remote add origin /tmp/preendemo-fixup-remote.git

mkdir -p internal/config internal/api
printf 'module demo\n\ngo 1.22\n' > go.mod
printf '# demo\n\nA small service.\n' > README.md
git add -A
git commit -q -m "Initial project"
git push -q -u origin main

# Two clean unpushed commits, each with a small flaw a review would catch.
cat > internal/config/loader.go <<'EOF'
package config

import "os"

type Config struct{ OutputDir string }

func Load() *Config { return &Config{OutputDir: os.Getenv("OUTPUT_DIR")} }
EOF
git add -A && git commit -q -m "Add config loader"

cat > internal/api/handler.go <<'EOF'
package api

import "net/http"

func Health(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }
EOF
git add -A && git commit -q -m "Add health endpoint"

# The review fixes, left dirty for the tape.
cat > internal/config/loader.go <<'EOF'
package config

import (
	"errors"
	"os"
)

type Config struct{ OutputDir string }

func Load() (*Config, error) {
	c := &Config{OutputDir: os.Getenv("OUTPUT_DIR")}
	if c.OutputDir == "" {
		return nil, errors.New("output dir required")
	}
	return c, nil
}
EOF
cat > internal/api/handler.go <<'EOF'
package api

import "net/http"

func Health(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
EOF

# Scripted stand-in for the preen skill in fixup mode.
preen() {
  git branch preen-backup/demo >/dev/null 2>&1
  printf '\n\033[1mpreen\033[0m --fixup: each dirty change goes into the commit that introduced it:\n\n'
  printf '  internal/config/loader.go  -> Add config loader\n'
  printf '  internal/api/handler.go    -> Add health endpoint\n'
  printf '\napprove? (y) y\n\n'
  git add internal/config/loader.go
  git commit -q --fixup="$(git log --format=%H --grep='^Add config loader$')"
  git add internal/api/handler.go
  git commit -q --fixup="$(git log --format=%H --grep='^Add health endpoint$')"
  GIT_SEQUENCE_EDITOR=true git rebase -i --autosquash origin/main >/dev/null 2>&1
  printf '\033[32m2 fixes folded into their commits, messages preserved\033[0m\n'
}

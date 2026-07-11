# Sourced by the preen demo tape. Builds a dirty sandbox repo and a preen stand-in
# for the recording, then clears. Not used at runtime by preen itself.

cd /tmp
rm -rf preendemo
mkdir preendemo
cd preendemo
git init -q
git config user.email demo@example.com
git config user.name demo
git config commit.gpgsign false
git config core.pager cat

mkdir -p internal/config internal/api internal/store cmd

# Initial project: everything already exists and is committed, so the later
# changes show up as modified files under a plain git status.
printf 'module demo\n\ngo 1.22\n' > go.mod
printf '# demo\n\nA small service.\n' > README.md
cat > internal/config/config.go <<'EOF'
package config

type Config struct {
	OutputDir string
}
EOF
cat > internal/config/loader.go <<'EOF'
package config

func Load() (*Config, error) {
	return &Config{}, nil
}
EOF
cat > internal/api/handler.go <<'EOF'
package api

import "net/http"

func Root(w http.ResponseWriter, _ *http.Request) {}
EOF
cat > internal/api/router.go <<'EOF'
package api

import "net/http"

func Routes() *http.ServeMux {
	return http.NewServeMux()
}
EOF
cat > internal/store/store.go <<'EOF'
package store

type Store struct{}
EOF
cat > cmd/flags.go <<'EOF'
package cmd

var Verbose bool
EOF
git add -A
git commit -q -m "Initial project"

# A pile of real work, several concerns at once, all edits to existing files.
cat > internal/config/config.go <<'EOF'
package config

import "errors"

type Config struct {
	OutputDir string
}

func (c *Config) Validate() error {
	if c.OutputDir == "" {
		return errors.New("output dir required")
	}
	return nil
}
EOF
cat > internal/config/loader.go <<'EOF'
package config

import "os"

func Load() (*Config, error) {
	c := &Config{OutputDir: os.Getenv("OUTPUT_DIR")}
	return c, c.Validate()
}
EOF
cat > internal/api/handler.go <<'EOF'
package api

import "net/http"

func Root(w http.ResponseWriter, _ *http.Request) {}

func Health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
EOF
cat > internal/api/router.go <<'EOF'
package api

import "net/http"

func Routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("/health", Health)
	return m
}
EOF
cat > internal/store/store.go <<'EOF'
package store

import "time"

type Store struct {
	Timeout time.Duration
}
EOF
cat > cmd/flags.go <<'EOF'
package cmd

var (
	Verbose   bool
	OutputDir string
)
EOF
printf '# demo\n\nA small service.\n\n## Flags\n\n- --output-dir sets the output directory.\n' > README.md

# Scripted stand-in for the preen skill, so the tape can show a real split.
preen() {
  git reset --soft HEAD~1 >/dev/null 2>&1
  git reset -q >/dev/null 2>&1
  printf '\n\033[1mpreen\033[0m absorbed the wip commit. Planned 5 commits:\n\n'
  printf '  1. Add config validation   internal/config/config.go, loader.go\n'
  printf '  2. Add health endpoint     internal/api/handler.go, router.go\n'
  printf '  3. Add output dir flag     cmd/flags.go\n'
  printf '  4. Add store timeout       internal/store/store.go\n'
  printf '  5. Update the README       README.md\n'
  printf '\napprove? (y) y\n\n'
  git add internal/config/config.go internal/config/loader.go && git commit -q -m "Add config validation"
  git add internal/api/handler.go internal/api/router.go && git commit -q -m "Add health endpoint"
  git add cmd/flags.go && git commit -q -m "Add output dir flag"
  git add internal/store/store.go && git commit -q -m "Add store timeout"
  git add README.md && git commit -q -m "Update the README"
  printf '\033[32m7 files became 5 clean commits, ready to push\033[0m\n'
}

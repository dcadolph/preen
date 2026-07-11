# Sourced by the gus demo tape. Builds a dirty sandbox repo and a gus stand-in
# for the recording, then clears the screen. Not used at runtime by gus itself.

cd /tmp
rm -rf gusdemo
mkdir gusdemo
cd gusdemo
git init -q
git config user.email demo@example.com
git config user.name demo
git config commit.gpgsign false
git config core.pager cat

printf 'module demo\n\ngo 1.22\n' > go.mod
printf '# demo\n\nA small service.\n' > README.md
git add -A
git commit -q -m "Initial project"

mkdir -p internal/config internal/api cmd docs

cat > internal/config/config.go <<'EOF'
package config

// Config holds runtime settings.
type Config struct {
	OutputDir string
}
EOF

cat > internal/config/loader.go <<'EOF'
package config

import "os"

// Load reads configuration from the environment.
func Load() (*Config, error) {
	return &Config{OutputDir: os.Getenv("OUTPUT_DIR")}, nil
}
EOF

cat > internal/config/loader_test.go <<'EOF'
package config

import "testing"

func TestLoad(t *testing.T) {
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
EOF

cat > internal/api/handler.go <<'EOF'
package api

import "net/http"

// Health responds with 200 OK.
func Health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
EOF

cat > internal/api/handler_test.go <<'EOF'
package api

import (
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	w := httptest.NewRecorder()
	Health(w, nil)
	if w.Code != 200 {
		t.Fatal("bad status")
	}
}
EOF

cat > cmd/flag_output.go <<'EOF'
package cmd

// OutputDir is where results are written.
var OutputDir string
EOF

cat > docs/config.md <<'EOF'
# Config

Set OUTPUT_DIR to choose where results land.
EOF

printf '# demo\n\nA small service.\n\n## Flags\n\n- --output-dir sets the output directory.\n' > README.md

# Scripted stand-in for the gus skill, so the tape can show a real split.
gus() {
  git reset --soft HEAD~1 >/dev/null 2>&1
  git reset -q >/dev/null 2>&1
  printf '\n\033[1mgus\033[0m absorbed the wip commit. Planned 5 commits:\n\n'
  printf '  1. Add config loader      internal/config/config.go, loader.go\n'
  printf '  2. Add health handler     internal/api/handler.go\n'
  printf '  3. Wire output dir flag   cmd/flag_output.go\n'
  printf '  4. Add tests              internal/config, internal/api\n'
  printf '  5. Update docs            README.md, docs/config.md\n'
  printf '\napprove? (y) y\n\n'
  git add internal/config/config.go internal/config/loader.go && git commit -q -m "Add config loader"
  git add internal/api/handler.go && git commit -q -m "Add health handler"
  git add cmd/flag_output.go && git commit -q -m "Wire output dir flag"
  git add internal/config/loader_test.go internal/api/handler_test.go && git commit -q -m "Add tests"
  git add README.md docs/config.md && git commit -q -m "Update docs"
  printf '\033[32m8 files became 5 clean commits, ready to push\033[0m\n'
}

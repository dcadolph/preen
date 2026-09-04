# Sourced by the preen demo tape. Builds the binary under test and a dirty
# sandbox repository for it to work on, then puts the binary on PATH so the
# recording drives the real program. Not used at runtime by preen itself.

set -e

DEMOBIN="$(mktemp -d)"
go build -o "$DEMOBIN/preen" "$PREENREPO"
export PATH="$DEMOBIN:$PATH"

cd /tmp
rm -rf preendemo
mkdir preendemo
cd preendemo

git init -q -b main
git config user.email demo@example.com
git config user.name demo
git config commit.gpgsign false
git config core.pager cat

mkdir -p api store docs

# The committed starting point, so later edits show up as modifications rather
# than a repository made entirely of untracked files.
printf 'module example.com/service\n\ngo 1.26\n' > go.mod
cat > api/server.go <<'EOF'
package api

// Server serves the HTTP API.
type Server struct {
	addr string
}

// New returns a Server bound to addr.
func New(addr string) *Server {
	return &Server{addr: addr}
}
EOF
cat > docs/guide.md <<'EOF'
# Guide

How to run the service.
EOF
git add -A
git commit -qm "Add the service skeleton"

# The mess: a working session that never stopped to commit. It spans every
# category the grouper separates, so the plan shows real clustering.
cat > go.mod <<'EOF'
module example.com/service

go 1.26

require github.com/lib/pq v1.10.9
EOF

cat > api/server.go <<'EOF'
package api

import "net/http"

// Server serves the HTTP API.
type Server struct {
	addr string
	mux  *http.ServeMux
}

// New returns a Server bound to addr.
func New(addr string) *Server {
	return &Server{addr: addr, mux: http.NewServeMux()}
}

// Handle registers a route.
func (s *Server) Handle(path string, h http.Handler) {
	s.mux.Handle(path, h)
}
EOF

cat > api/auth.go <<'EOF'
package api

import "errors"

// ErrUnauthorized is returned when a token is missing or invalid.
var ErrUnauthorized = errors.New("unauthorized")

// Authenticate verifies a bearer token.
func Authenticate(token string) error {
	if token == "" {
		return ErrUnauthorized
	}
	return nil
}
EOF

cat > api/auth_test.go <<'EOF'
package api

import "testing"

func TestAuthenticate(t *testing.T) {
	if err := Authenticate(""); err == nil {
		t.Error("empty token was accepted")
	}
}
EOF

cat > store/db.go <<'EOF'
package store

import "database/sql"

// DB wraps the connection pool.
type DB struct {
	conn *sql.DB
}

// Open connects to the database.
func Open(dsn string) (*DB, error) {
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
}
EOF

cat > store/migrate.go <<'EOF'
package store

// Migrate applies the schema.
func (d *DB) Migrate() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS users (id INT PRIMARY KEY)`)
	return err
}
EOF

cat > docs/guide.md <<'EOF'
# Guide

How to run the service.

## Authentication

Send a bearer token with every request.
EOF

mkdir -p .github/workflows
cat > .github/workflows/ci.yml <<'EOF'
name: ci
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: go test ./...
EOF

echo "scratch notes, do not commit" > scratch.txt

# Staged by hand, which preen treats as a boundary the author drew deliberately.
git add api/auth.go

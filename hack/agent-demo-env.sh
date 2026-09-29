# Sourced by the agent grouping demo tape. Builds the binary under test and a
# sandbox repository holding two unrelated changes that share a file, which is
# the tree an agent session leaves and the built-in rules cannot split. It then
# writes the answer the agent would give, bound to that tree. Not used at
# runtime by preen itself.

set -e

DEMOBIN="$(mktemp -d)"
go build -o "$DEMOBIN/preen" "$PREENREPO"
export PATH="$DEMOBIN:$PATH"

cd /tmp
rm -rf preenagentdemo preen-request.json preen-answer.json
mkdir preenagentdemo
cd preenagentdemo

git init -q -b main
git config user.email demo@example.com
git config user.name demo
git config commit.gpgsign false
git config core.pager cat

mkdir -p client cmd

printf 'module example.com/fetch\n\ngo 1.26\n' > go.mod
cat > client/client.go <<'EOF'
package client

import "net/http"

// Client fetches pages.
type Client struct {
	http *http.Client
}

// Get fetches url.
func (c *Client) Get(url string) (*http.Response, error) {
	return c.http.Get(url)
}

// New returns a Client with the default transport.
func New() *Client {
	return &Client{http: http.DefaultClient}
}

// Head checks url without fetching the body.
func (c *Client) Head(url string) (*http.Response, error) {
	return c.http.Head(url)
}

// Do sends a prepared request.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.http.Do(req)
}
EOF
cat > cmd/main.go <<'EOF'
package main

import "example.com/fetch/client"

func main() {
	_, _ = client.New().Get("https://example.com")
}
EOF
printf '# fetch\n\nFetches pages.\n' > README.md
git add -A
git commit -qm "Add the fetch client"

# Change one: retry failed requests. It touches the top of client.go.
cat > client/client.go <<'EOF'
package client

import "net/http"

// Client fetches pages.
type Client struct {
	http *http.Client
}

// Get fetches url, retrying a failed attempt.
func (c *Client) Get(url string) (*http.Response, error) {
	return retry(3, func() (*http.Response, error) { return c.http.Get(url) })
}

// New returns a Client with the default transport.
func New() *Client {
	return &Client{http: http.DefaultClient}
}

// Head checks url without fetching the body.
func (c *Client) Head(url string) (*http.Response, error) {
	return c.http.Head(url)
}

// Do sends a prepared request, logging any that run slow.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	defer logSlow(req.URL.String())()
	return c.http.Do(req)
}
EOF
cat > client/timing.go <<'EOF'
package client

import (
	"log"
	"time"
)

// slow is how long a request may take before it is logged.
const slow = 2 * time.Second

// logSlow starts a timer for url and returns the func that stops it.
func logSlow(url string) func() {
	start := time.Now()
	return func() {
		if took := time.Since(start); took > slow {
			log.Printf("slow request: %s took %s", url, took)
		}
	}
}
EOF
cat > client/retry.go <<'EOF'
package client

import "net/http"

// retry calls fn up to attempts times, stopping at the first success.
func retry(attempts int, fn func() (*http.Response, error)) (*http.Response, error) {
	var err error
	for range attempts {
		var resp *http.Response
		if resp, err = fn(); err == nil {
			return resp, nil
		}
	}
	return nil, err
}
EOF
cat > client/retry_test.go <<'EOF'
package client

import (
	"errors"
	"net/http"
	"testing"
)

func TestRetryGivesUp(t *testing.T) {
	errFail := errors.New("fail")
	calls := 0
	_, _ = retry(3, func() (*http.Response, error) { calls++; return nil, errFail })
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}
EOF
printf '# fetch\n\nFetches pages, retrying a failed request up to three times.\n' > README.md

# Change two: log slow requests. It touches the bottom of client.go.
cat > cmd/main.go <<'EOF'
package main

import (
	"log"

	"example.com/fetch/client"
)

func main() {
	log.SetFlags(log.Lmicroseconds)
	_, _ = client.New().Get("https://example.com")
}
EOF

echo "todo: ask about backoff" > notes.md

# The agent's answer, written from what it changed and why. The tree hash comes
# from the request, which binds the answer to this exact tree.
tree="$(preen request | sed -n 's/.*"tree":"\([0-9a-f]*\)".*/\1/p')"
cat > /tmp/preen-answer.json <<EOF
{
  "tree": "$tree",
  "commits": [
    {"subject": "Retry failed requests", "parts": [
      {"path": "client/client.go", "hunks": [0]},
      {"path": "client/retry.go"},
      {"path": "client/retry_test.go"},
      {"path": "README.md"}]},
    {"subject": "Log slow requests", "parts": [
      {"path": "client/client.go", "hunks": [1]},
      {"path": "client/timing.go"},
      {"path": "cmd/main.go"}]}
  ]
}
EOF

# The tape shows a refused run, and this file is sourced into the recording
# shell, so errexit must not outlive the setup.
set +e

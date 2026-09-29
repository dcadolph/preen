package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"

	"github.com/dcadolph/preen/config"
	"github.com/dcadolph/preen/run"
)

// requestUsage is the help text for the request command.
const requestUsage = `usage: preen request [flags]

Print what a grouper would be asked to solve for this tree, as JSON on stdout,
and change nothing. It holds every change preen would commit, each file's
hunks with their text, and the tree's content hash.

This is for the agent that made the changes. It knows why each hunk exists,
which no rule reading the diff can recover, so it answers the request itself
and hands the answer back with --grouping. The answer must carry the request's
tree unchanged, so an answer written for an earlier tree is refused. Write the
request and the answer outside the repository, since a file written inside it
changes the tree they describe.

Flags:
  --scope PATH      Describe only paths under PATH. Repeatable.
  --absorb          Include unpushed commits, as a run with --absorb would.
  --allow-generated Include generated output preen would otherwise hold back.
  --pushed, --pushed-base REV, --allow-protected
                    Describe a pushed rewrite, under the same consents.
  --pretty          Indent the JSON.
  -h, --help        Print this help and exit.

The answer is the same JSON a --grouper program writes, plus the tree:

  {"tree": "<from the request>",
   "commits": [{"subject": "Add the retry budget",
                "parts": [{"path": "client/retry.go"},
                          {"path": "cmd/flags.go", "hunks": [1]}]}]}

A part without hunks takes the whole file. Then:

  preen request > /tmp/request.json
  preen --grouping /tmp/answer.json --dry-run
`

// runRequest prints the grouper request for the tree.
func runRequest(ctx context.Context, env *environment, args []string) (int, error) {
	fs := flag.NewFlagSet("preen request", flag.ContinueOnError)
	fs.SetOutput(env.Err)
	fs.Usage = func() { env.print(requestUsage) }
	var opts run.Options
	var scope stringList
	fs.Var(&scope, "scope", "describe only paths under this prefix (repeatable)")
	fs.BoolVar(&opts.Absorb, "absorb", false, "include unpushed commits")
	fs.BoolVar(&opts.AllowGenerated, "allow-generated", false, "include generated output")
	fs.BoolVar(&opts.Pushed, "pushed", false, "consent to describing commits a remote already has")
	fs.StringVar(&opts.PushedBase, "pushed-base", "", "the commit just before the range to redo")
	fs.BoolVar(&opts.AllowProtected, "allow-protected", false, "permit a protected branch")
	pretty := fs.Bool("pretty", false, "indent the JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return CodeOK, nil
		}
		return CodeErr, err
	}
	opts.Scope = scope

	repository, err := openRepo(ctx, env)
	if err != nil {
		return exitCode(err), err
	}
	cfg, err := config.Load(repository.Root())
	if err != nil {
		return CodeErr, err
	}
	applyConfig(&opts, cfg, args)

	request, err := run.New(repository).Request(ctx, opts)
	if err != nil {
		return exitCode(err), err
	}
	var out []byte
	if *pretty {
		out, err = json.MarshalIndent(request, "", "  ")
	} else {
		out, err = json.Marshal(request)
	}
	if err != nil {
		return CodeErr, err
	}
	env.println(string(out))
	return CodeOK, nil
}

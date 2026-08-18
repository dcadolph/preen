package group

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/dcadolph/preen/plan"
	"github.com/dcadolph/preen/repo"
)

// Request is what a command grouper is asked to solve, written as JSON on
// stdin. It carries the changes and their hunks, never the repository itself,
// so a grouper cannot act on anything: it only answers.
type Request struct {
	// Files are the changed files and the hunks available to divide.
	Files []RequestFile `json:"files"`
}

// RequestFile is one file offered to the grouper.
type RequestFile struct {
	// Path is the file, relative to the repository root.
	Path string `json:"path"`
	// Kind is what happened to it.
	Kind string `json:"kind"`
	// Staged reports that the user staged this path by hand.
	Staged bool `json:"staged"`
	// Splittable reports whether the hunks may be divided across commits.
	Splittable bool `json:"splittable"`
	// Hunks are the file's hunks, in order.
	Hunks []RequestHunk `json:"hunks,omitempty"`
}

// RequestHunk is one hunk offered to the grouper.
type RequestHunk struct {
	// Index is the hunk's position in the file's diff.
	Index int `json:"index"`
	// Header is the @@ line, which names the enclosing function when git knows
	// it.
	Header string `json:"header"`
	// Text is the hunk body, so the grouper can read what actually changed.
	Text string `json:"text"`
}

// Response is what a command grouper returns on stdout.
type Response struct {
	// Commits are the proposed commits, in the order they should be recorded.
	Commits []ResponseCommit `json:"commits"`
}

// ResponseCommit is one proposed commit.
type ResponseCommit struct {
	// Subject is the commit subject.
	Subject string `json:"subject"`
	// Body is the optional message body.
	Body string `json:"body,omitempty"`
	// Parts are the file contributions.
	Parts []ResponsePart `json:"parts"`
}

// ResponsePart is one file's contribution to a proposed commit.
type ResponsePart struct {
	// Path is the file.
	Path string `json:"path"`
	// Hunks are the hunk indexes this commit takes. Omitting them takes the
	// whole file.
	Hunks []int `json:"hunks,omitempty"`
}

// Command groups changes by handing them to an external program.
//
// The program receives the request as JSON on stdin and returns a Response as
// JSON on stdout. That contract is deliberately provider agnostic: any model
// CLI, script, or service wrapper can be a grouper without preen knowing
// anything about it, and none of them can touch the repository.
type Command struct {
	// Name is the program to run.
	Name string
	// Args are the arguments passed to it.
	Args []string
	// Dir is the working directory for the program.
	Dir string
}

// Group runs the command and converts its answer into commits. Every answer is
// checked against the real changes, so a grouper that hallucinates a path, a
// hunk, or a duplicate is rejected rather than trusted.
func (c Command) Group(ctx context.Context, in Input) ([]plan.Commit, error) {
	if c.Name == "" {
		return nil, ErrNoCommand
	}
	payload, err := json.Marshal(buildRequest(in))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRequest, err)
	}
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s: %w: %s", ErrCommand, c.Name, err, strings.TrimSpace(stderr.String()))
	}
	var response Response
	if err := json.Unmarshal(extractJSON(stdout.Bytes()), &response); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResponse, err)
	}
	return convert(response, in)
}

// buildRequest turns the survey into the grouper's input.
func buildRequest(in Input) Request {
	request := Request{Files: make([]RequestFile, 0, len(in.Changes))}
	for _, change := range in.Changes {
		file := RequestFile{
			Path:   change.Path,
			Kind:   string(change.Kind),
			Staged: change.Staged,
		}
		if diff, ok := in.DiffFor(change.Path); ok {
			file.Splittable = diff.Splittable()
			for i, hunk := range diff.Hunks {
				file.Hunks = append(file.Hunks, RequestHunk{
					Index:  i,
					Header: hunk.Header,
					Text:   strings.Join(hunk.Lines, "\n"),
				})
			}
		}
		request.Files = append(request.Files, file)
	}
	return request
}

// extractJSON pulls the JSON object out of a program's stdout, tolerating the
// prose a chat-oriented tool tends to wrap it in.
func extractJSON(out []byte) []byte {
	trimmed := bytes.TrimSpace(out)
	if bytes.HasPrefix(trimmed, []byte("{")) {
		return trimmed
	}
	start := bytes.IndexByte(trimmed, '{')
	end := bytes.LastIndexByte(trimmed, '}')
	if start < 0 || end <= start {
		return trimmed
	}
	return trimmed[start : end+1]
}

// convert turns a grouper's answer into commits, verifying every reference
// against the real changes.
func convert(response Response, in Input) ([]plan.Commit, error) {
	if len(response.Commits) == 0 {
		return nil, fmt.Errorf("%w: no commits proposed", ErrResponse)
	}
	known := make(map[string]repo.Change, len(in.Changes))
	for _, change := range in.Changes {
		known[change.Path] = change
	}
	commits := make([]plan.Commit, 0, len(response.Commits))
	for i, proposed := range response.Commits {
		if strings.TrimSpace(proposed.Subject) == "" {
			return nil, fmt.Errorf("%w: commit %d has no subject", ErrResponse, i+1)
		}
		parts := make([]plan.Part, 0, len(proposed.Parts))
		for _, part := range proposed.Parts {
			change, ok := known[part.Path]
			if !ok {
				return nil, fmt.Errorf("%w: commit %d names %s, which is not a change in this tree",
					ErrResponse, i+1, part.Path)
			}
			converted, err := convertPart(part, change, in)
			if err != nil {
				return nil, err
			}
			parts = append(parts, converted)
		}
		if len(parts) == 0 {
			return nil, fmt.Errorf("%w: commit %d (%q) holds no files", ErrResponse, i+1, proposed.Subject)
		}
		commits = append(commits, plan.Commit{Subject: proposed.Subject, Body: proposed.Body, Parts: parts})
	}
	return commits, nil
}

// convertPart resolves a proposed part's hunks against the real diff, so an
// index the grouper invented cannot reach the repository.
func convertPart(part ResponsePart, change repo.Change, in Input) (plan.Part, error) {
	out := plan.Part{Path: change.Path, From: change.From, Kind: change.Kind}
	if len(part.Hunks) == 0 {
		return out, nil
	}
	diff, ok := in.DiffFor(part.Path)
	if !ok {
		return plan.Part{}, fmt.Errorf("%w: %s has no diff to split", ErrResponse, part.Path)
	}
	if diff.Binary {
		return plan.Part{}, fmt.Errorf("%w: %s is binary and cannot be split", ErrResponse, part.Path)
	}
	for _, index := range part.Hunks {
		if index < 0 || index >= len(diff.Hunks) {
			return plan.Part{}, fmt.Errorf("%w: %s has no hunk %d", ErrResponse, part.Path, index)
		}
		out.Hunks = append(out.Hunks, plan.HunkAt(index, strings.Join(diff.Hunks[index].Lines, "\n")))
	}
	return out, nil
}

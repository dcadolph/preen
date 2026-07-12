package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestSplitArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantOpts options
		Want     error
		In       []string
	}{{ // Test 0: No arguments yields defaults.
		In:       nil,
		WantOpts: options{ClaudeBin: "claude"},
	}, { // Test 1: Skill flags pass through in order.
		In:       []string{"--fixup", "--scope", "internal/"},
		WantOpts: options{ClaudeBin: "claude", SkillArgs: []string{"--fixup", "--scope", "internal/"}},
	}, { // Test 2: Headless is a wrapper flag, not a skill flag.
		In:       []string{"--headless", "--dry-run"},
		WantOpts: options{ClaudeBin: "claude", Headless: true, SkillArgs: []string{"--dry-run"}},
	}, { // Test 3: Equals form of claude-bin.
		In:       []string{"--claude-bin=/opt/claude"},
		WantOpts: options{ClaudeBin: "/opt/claude"},
	}, { // Test 4: Space form of claude-bin.
		In:       []string{"--claude-bin", "claude-next"},
		WantOpts: options{ClaudeBin: "claude-next"},
	}, { // Test 5: Missing claude-bin value errors.
		In:   []string{"--claude-bin"},
		Want: ErrUsage,
	}, { // Test 6: Everything after -- goes to claude, wrapper-looking flags included.
		In:       []string{"--gate", "go test", "--", "--headless", "--model", "opus"},
		WantOpts: options{ClaudeBin: "claude", SkillArgs: []string{"--gate", "go test"}, ClaudeArgs: []string{"--headless", "--model", "opus"}},
	}, { // Test 7: Help short and long forms.
		In:       []string{"-h"},
		WantOpts: options{ClaudeBin: "claude", ShowHelp: true},
	}, { // Test 8: Version flag.
		In:       []string{"--version"},
		WantOpts: options{ClaudeBin: "claude", ShowVersion: true},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := splitArgs(test.In)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error mismatch: got %v, want %v", err, test.Want)
			}
			if test.Want != nil {
				return
			}
			if diff := cmp.Diff(test.WantOpts, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("options mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestComposePrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantPrompt string
		In         []string
		Headless   bool
	}{{ // Test 0: Bare invocation.
		In:         nil,
		WantPrompt: "/preen",
	}, { // Test 1: Plain flags stay unquoted.
		In:         []string{"--fixup", "--scope", "internal/"},
		WantPrompt: "/preen --fixup --scope internal/",
	}, { // Test 2: Values with spaces get quoted.
		In:         []string{"--gate", "go test ./..."},
		WantPrompt: "/preen --gate 'go test ./...'",
	}, { // Test 3: Embedded single quotes are escaped.
		In:         []string{"--gate", "echo 'hi'"},
		WantPrompt: `/preen --gate 'echo '\''hi'\'''`,
	}, { // Test 4: Headless appends --yes.
		In:         []string{"--fixup"},
		Headless:   true,
		WantPrompt: "/preen --fixup --yes",
	}, { // Test 5: Headless does not duplicate --yes.
		In:         []string{"--yes"},
		Headless:   true,
		WantPrompt: "/preen --yes",
	}, { // Test 6: Empty argument stays visible.
		In:         []string{""},
		WantPrompt: "/preen ''",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := composePrompt(test.In, test.Headless)
			if diff := cmp.Diff(test.WantPrompt, got); diff != "" {
				t.Errorf("prompt mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClaudeArgv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantArgv []string
		Opts     options
	}{{ // Test 0: Interactive preloads the prompt only.
		Opts:     options{},
		WantArgv: []string{"/preen"},
	}, { // Test 1: Interactive keeps extra claude args.
		Opts:     options{ClaudeArgs: []string{"--model", "opus"}},
		WantArgv: []string{"/preen", "--model", "opus"},
	}, { // Test 2: Headless adds -p and default permissions.
		Opts:     options{Headless: true},
		WantArgv: []string{"-p", "/preen --yes", "--permission-mode", "acceptEdits", "--allowedTools", "Bash(git:*)"},
	}, { // Test 3: Caller permission mode suppresses defaults.
		Opts:     options{Headless: true, ClaudeArgs: []string{"--permission-mode", "plan"}},
		WantArgv: []string{"-p", "/preen --yes", "--permission-mode", "plan"},
	}, { // Test 4: Skip-permissions suppresses defaults.
		Opts:     options{Headless: true, ClaudeArgs: []string{"--dangerously-skip-permissions"}},
		WantArgv: []string{"-p", "/preen --yes", "--dangerously-skip-permissions"},
	}, { // Test 5: Equals form of allowedTools suppresses defaults.
		Opts:     options{Headless: true, ClaudeArgs: []string{"--allowedTools=Bash"}},
		WantArgv: []string{"-p", "/preen --yes", "--allowedTools=Bash"},
	}}
	// claudeArgv receives the composed invocation directly here; the
	// instruction wrapper is covered by TestRun.
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := claudeArgv(test.Opts, composePrompt(test.Opts.SkillArgs, test.Opts.Headless))
			if diff := cmp.Diff(test.WantArgv, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("argv mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		LookErr   error
		RepoErr   error
		Want      error
		WantOut   string
		In        []string
		WantCode  int
		ChildCode int
	}{{ // Test 0: Help prints usage.
		In:       []string{"--help"},
		WantCode: CodeOK,
		WantOut:  "usage: preen",
	}, { // Test 1: Version prints the version string.
		In:       []string{"--version"},
		WantCode: CodeOK,
		WantOut:  "preen " + Version,
	}, { // Test 2: Missing claude binary.
		In:       nil,
		LookErr:  errors.New("not found"),
		WantCode: CodeNoClaude,
		Want:     ErrNoClaude,
	}, { // Test 3: Not a repository.
		In:       nil,
		RepoErr:  ErrNoRepo,
		WantCode: CodeNoRepo,
		Want:     ErrNoRepo,
	}, { // Test 4: Repository mid-operation.
		In:       nil,
		RepoErr:  fmt.Errorf("%w: MERGE_HEAD exists", ErrRepoState),
		WantCode: CodeRepoState,
		Want:     ErrRepoState,
	}, { // Test 5: Child exit code is mirrored.
		In:        []string{"--fixup"},
		ChildCode: 7,
		WantCode:  7,
	}, { // Test 6: Bad wrapper arguments.
		In:       []string{"--claude-bin"},
		WantCode: CodeErr,
		Want:     ErrUsage,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			var gotArgv []string
			cleaned := false
			r := &runner{
				Skill: "skill body",
				LookPath: func(bin string) (string, error) {
					if test.LookErr != nil {
						return "", test.LookErr
					}
					return "/bin/" + bin, nil
				},
				CheckRepo: func() error { return test.RepoErr },
				TempSkill: func(string) (string, func(), error) {
					return "/tmp/skill.md", func() { cleaned = true }, nil
				},
				Start: func(_ string, argv []string) (int, error) {
					gotArgv = argv
					return test.ChildCode, nil
				},
				Out: &out,
			}
			code, err := r.Run(test.In)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error mismatch: got %v, want %v", err, test.Want)
			}
			if code != test.WantCode {
				t.Errorf("code mismatch: got %d, want %d", code, test.WantCode)
			}
			if test.WantOut != "" && !strings.Contains(out.String(), test.WantOut) {
				t.Errorf("output %q missing %q", out.String(), test.WantOut)
			}
			if test.Want != nil || test.WantOut != "" {
				return
			}
			if len(gotArgv) == 0 {
				t.Fatal("claude was not started")
			}
			if !cleaned {
				t.Error("temp skill file was not cleaned up")
			}
			joined := strings.Join(gotArgv, " ")
			if !strings.Contains(joined, "/tmp/skill.md") || !strings.Contains(joined, "/preen") {
				t.Errorf("argv missing skill path or invocation: %q", joined)
			}
		})
	}
}

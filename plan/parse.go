package plan

import (
	"fmt"
	"strconv"
	"strings"
)

// ActionKind is what a typed approval-prompt command asks for.
type ActionKind int

// The actions available at the approval prompt.
const (
	// ActionEdit changes the plan and stays at the prompt.
	ActionEdit ActionKind = iota
	// ActionApply accepts the plan as shown.
	ActionApply
	// ActionAbort leaves without changing anything.
	ActionAbort
	// ActionShow redisplays the plan.
	ActionShow
	// ActionHelp lists the available commands.
	ActionHelp
)

// Action is one parsed command from the approval prompt.
type Action struct {
	// Kind is what the command asks for.
	Kind ActionKind
	// Describe is a short account of the edit, echoed back after it applies.
	Describe string
	// Apply changes the plan. It is set only for ActionEdit.
	Apply func(p *Plan) error
}

// EditHelp lists the commands the approval prompt understands.
const EditHelp = `Commands:
  y, apply              Record the plan as shown.
  n, abort              Leave without changing anything.
  show                  Show the plan again.
  merge N into M        Fold commit N into commit M.
  split N               Break commit N into one commit per file.
  move PATH to N        Reassign a file to commit N.
  reword N SUBJECT      Replace commit N's subject.
  drop PATH             Leave a file uncommitted.
  reorder N,M,...       Resequence the commits.
  ?, help               Show this list.`

// ParseAction reads one command typed at the approval prompt.
//
// The grammar matches how the moves read in English, so "merge 2 into 1" and
// "move api/server.go to 3" work as written, and the filler words are
// optional.
func ParseAction(line string) (Action, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return Action{}, fmt.Errorf("%w: type a command, or ? for the list", ErrUsage)
	}
	verb := strings.ToLower(fields[0])
	args := fields[1:]

	switch verb {
	case "y", "yes", "apply", "done":
		return Action{Kind: ActionApply}, nil
	case "n", "no", "abort", "quit", "q":
		return Action{Kind: ActionAbort}, nil
	case "show", "plan", "p":
		return Action{Kind: ActionShow}, nil
	case "?", "help", "h":
		return Action{Kind: ActionHelp}, nil
	case "merge":
		return parseMerge(args)
	case "split":
		return parseSplit(args)
	case "move":
		return parseMove(args)
	case "reword":
		return parseReword(args, line)
	case "drop":
		return parseDrop(args)
	case "reorder":
		return parseReorder(args)
	default:
		return Action{}, fmt.Errorf("%w: unknown command %q, type ? for the list", ErrUsage, fields[0])
	}
}

// parseMerge reads "merge N into M", where "into" is optional.
func parseMerge(args []string) (Action, error) {
	args = drop(args, "into")
	if len(args) != 2 {
		return Action{}, fmt.Errorf("%w: merge needs two commit numbers, like: merge 2 into 1", ErrUsage)
	}
	from, err := number(args[0])
	if err != nil {
		return Action{}, err
	}
	into, err := number(args[1])
	if err != nil {
		return Action{}, err
	}
	return Action{
		Kind:     ActionEdit,
		Describe: fmt.Sprintf("merged commit %d into %d", from, into),
		Apply:    func(p *Plan) error { return p.MergeInto(from, into) },
	}, nil
}

// parseSplit reads "split N".
func parseSplit(args []string) (Action, error) {
	args = drop(args, "by", "file")
	if len(args) != 1 {
		return Action{}, fmt.Errorf("%w: split needs a commit number, like: split 3", ErrUsage)
	}
	at, err := number(args[0])
	if err != nil {
		return Action{}, err
	}
	return Action{
		Kind:     ActionEdit,
		Describe: fmt.Sprintf("split commit %d by file", at),
		Apply:    func(p *Plan) error { return p.SplitByFile(at) },
	}, nil
}

// parseMove reads "move PATH to N", where "to" is optional.
func parseMove(args []string) (Action, error) {
	args = drop(args, "to")
	if len(args) != 2 {
		return Action{}, fmt.Errorf("%w: move needs a path and a commit number, like: move api/x.go to 2", ErrUsage)
	}
	to, err := number(args[1])
	if err != nil {
		return Action{}, err
	}
	path := args[0]
	return Action{
		Kind:     ActionEdit,
		Describe: fmt.Sprintf("moved %s to commit %d", path, to),
		Apply:    func(p *Plan) error { return p.MovePath(path, to) },
	}, nil
}

// parseReword reads "reword N SUBJECT". The subject is taken from the original
// line so its spacing and punctuation survive intact.
func parseReword(args []string, line string) (Action, error) {
	if len(args) < 2 {
		return Action{}, fmt.Errorf("%w: reword needs a number and a subject, like: reword 1 Add the parser", ErrUsage)
	}
	at, err := number(strings.TrimSuffix(args[0], ":"))
	if err != nil {
		return Action{}, err
	}
	subject := subjectFrom(line, args[0])
	if subject == "" {
		return Action{}, fmt.Errorf("%w: reword needs a subject", ErrUsage)
	}
	return Action{
		Kind:     ActionEdit,
		Describe: fmt.Sprintf("reworded commit %d", at),
		Apply:    func(p *Plan) error { return p.Reword(at, subject) },
	}, nil
}

// subjectFrom recovers the text after the commit number in a reword command,
// keeping the spacing the user typed.
func subjectFrom(line, numberField string) string {
	at := strings.Index(line, numberField)
	if at < 0 {
		return ""
	}
	rest := line[at+len(numberField):]
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), ":"))
}

// parseDrop reads "drop PATH".
func parseDrop(args []string) (Action, error) {
	if len(args) != 1 {
		return Action{}, fmt.Errorf("%w: drop needs one path, like: drop notes.txt", ErrUsage)
	}
	path := args[0]
	return Action{
		Kind:     ActionEdit,
		Describe: fmt.Sprintf("left %s uncommitted", path),
		Apply:    func(p *Plan) error { return p.DropPath(path) },
	}, nil
}

// parseReorder reads "reorder N,M,..." and accepts spaces as separators too.
func parseReorder(args []string) (Action, error) {
	fields := strings.FieldsFunc(strings.Join(args, ","), func(r rune) bool {
		return r == ',' || r == ' '
	})
	if len(fields) < 2 {
		return Action{}, fmt.Errorf("%w: reorder needs the full order, like: reorder 3,1,2", ErrUsage)
	}
	order := make([]int, 0, len(fields))
	for _, field := range fields {
		n, err := number(field)
		if err != nil {
			return Action{}, err
		}
		order = append(order, n)
	}
	return Action{
		Kind:     ActionEdit,
		Describe: fmt.Sprintf("reordered to %v", order),
		Apply:    func(p *Plan) error { return p.Reorder(order) },
	}, nil
}

// number parses a commit number, rejecting anything that is not one.
func number(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not a commit number", ErrUsage, s)
	}
	return n, nil
}

// drop removes filler words from an argument list so the commands read
// naturally without the parser depending on them.
func drop(args []string, filler ...string) []string {
	kept := make([]string, 0, len(args))
	for _, arg := range args {
		skip := false
		for _, word := range filler {
			if strings.EqualFold(arg, word) {
				skip = true
				break
			}
		}
		if !skip {
			kept = append(kept, arg)
		}
	}
	return kept
}

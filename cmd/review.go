package cmd

import (
	"bufio"
	"errors"
	"io"
	"strings"

	"github.com/dcadolph/preen/plan"
)

// review runs the approval prompt, applying edits until the plan is accepted
// or abandoned. It reports whether the plan was approved.
//
// An edit that would leave the plan no longer covering the working tree is
// rejected and the previous plan is kept, so the prompt cannot be used to walk
// the plan into a state that loses work.
func review(env *environment, built *plan.Plan) (bool, error) {
	reader := bufio.NewReader(env.In)
	for {
		env.printf("Apply this plan? %d commit%s [y/n, or ? for edits]: ",
			len(built.Commits), plural(len(built.Commits)))
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				// A closed input is not consent.
				env.println()
				return false, nil
			}
			return false, err
		}
		if strings.TrimSpace(line) == "" {
			continue
		}

		action, err := plan.ParseAction(line)
		if err != nil {
			env.printf("%v\n", err)
			continue
		}
		switch action.Kind {
		case plan.ActionApply:
			return true, nil
		case plan.ActionAbort:
			return false, nil
		case plan.ActionHelp:
			env.println(plan.EditHelp)
			continue
		case plan.ActionShow:
			if err := built.Render(env.Out); err != nil {
				return false, err
			}
			continue
		case plan.ActionEdit:
			applyEdit(env, built, action)
		}
	}
}

// applyEdit runs one edit against a copy of the plan and keeps it only when
// the result still accounts for the whole tree.
func applyEdit(env *environment, built *plan.Plan, action plan.Action) {
	candidate := clonePlan(built)
	if err := action.Apply(candidate); err != nil {
		env.printf("%v\n", err)
		return
	}
	if err := candidate.Revalidate(); err != nil {
		env.printf("that edit would leave the plan incomplete: %v\n", err)
		return
	}
	*built = *candidate
	env.printf("Edit applied: %s\n\n", action.Describe)
	if err := built.Render(env.Out); err != nil {
		env.printf("%v\n", err)
	}
}

// clonePlan returns a deep enough copy that a rejected edit leaves the
// original untouched. The commit and part slices are the only mutable parts.
func clonePlan(src *plan.Plan) *plan.Plan {
	dst := *src
	dst.Commits = make([]plan.Commit, len(src.Commits))
	for i, commit := range src.Commits {
		dst.Commits[i] = commit
		dst.Commits[i].Parts = append([]plan.Part(nil), commit.Parts...)
	}
	dst.Leftover = append([]plan.Part(nil), src.Leftover...)
	return &dst
}

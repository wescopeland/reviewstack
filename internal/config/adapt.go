package config

import "strings"

const localReviewPrompt = "Review uncommitted local changes using {{inputs}}/context.json, {{inputs}}/diff.patch, and {{inputs}}/diff-stat.txt."

// AdaptReviewers adjusts reviewer commands for uncommitted/local review mode.
func AdaptReviewers(reviewers []Reviewer, uncommitted bool) []Reviewer {
	if !uncommitted {
		return reviewers
	}
	out := make([]Reviewer, len(reviewers))
	for i, r := range reviewers {
		r.Args = append([]string(nil), r.Args...)
		switch {
		case strings.HasPrefix(r.ID, "codex-"):
			r.Args = codexUncommittedArgs(r.Args)
		case strings.HasPrefix(r.ID, "claude-"):
			r.Args = claudeUncommittedArgs(r.Args)
		case strings.HasPrefix(r.ID, "cursor-"):
			r.Args = cursorUncommittedArgs(r.Args)
		default:
			r.Args = genericUncommittedArgs(r.Args)
		}
		out[i] = r
	}
	return out
}

func claudeUncommittedArgs(args []string) []string {
	out := append([]string(nil), args...)
	for i, arg := range out {
		if arg == "{{pr}}" {
			out[i] = localReviewPrompt
		}
	}
	return out
}

func cursorUncommittedArgs(args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		s := arg
		s = strings.ReplaceAll(s, "for PR {{pr}} against {{base}}.", "for uncommitted local changes.")
		s = strings.ReplaceAll(s, "Thermonuclear maintainability review for PR {{pr}} against {{base}}.", "Thermonuclear maintainability review of uncommitted local changes.")
		s = strings.ReplaceAll(s, "PR {{pr}}", "uncommitted local changes")
		out[i] = s
	}
	return out
}

func genericUncommittedArgs(args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		if arg == "{{pr}}" {
			out[i] = localReviewPrompt
		} else {
			out[i] = arg
		}
	}
	return out
}

func codexUncommittedArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	afterReview := false
	promptKept := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "review" {
			afterReview = true
			filtered = append(filtered, arg)
			continue
		}
		if !afterReview {
			filtered = append(filtered, arg)
			continue
		}
		if arg == "--base" {
			i++
			continue
		}
		if strings.HasPrefix(arg, "--base=") {
			continue
		}
		if !promptKept && !strings.HasPrefix(arg, "-") {
			filtered = append(filtered, arg)
			promptKept = true
			continue
		}
		if codexReviewFlagTakesValue(arg) && i+1 < len(args) {
			filtered = append(filtered, arg, args[i+1])
			i++
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		filtered = append(filtered, arg)
	}
	for i, a := range filtered {
		if a == "review" {
			return append(append(append([]string{}, filtered[:i+1]...), "--uncommitted"), filtered[i+1:]...)
		}
	}
	return filtered
}

func codexReviewFlagTakesValue(arg string) bool {
	switch arg {
	case "-c", "--config", "--commit", "--title", "--enable", "--disable":
		return true
	default:
		return false
	}
}

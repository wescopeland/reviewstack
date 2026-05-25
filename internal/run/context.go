package run

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Context holds values available for config placeholder expansion.
type Context struct {
	Workspace string
	RunDir    string
	Inputs    string
	Base      string
	Target    string
	PR        int
	Final     string
	// Mode is one of "pr", "uncommitted", or "branch".
	Mode string
}

func NewContext(workspace, runDir, base, target string, pr int) Context {
	return Context{
		Workspace: workspace,
		RunDir:    runDir,
		Inputs:    filepath.Join(runDir, "inputs"),
		Base:      base,
		Target:    target,
		PR:        pr,
		Final:     filepath.Join(runDir, FinalReview),
		Mode:      "pr",
	}
}

func (c Context) ExpandArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		expanded := c.Expand(a)
		if expanded != "" {
			out = append(out, expanded)
		}
	}
	return out
}

func (c Context) Expand(s string) string {
	pr := ""
	if c.PR > 0 {
		pr = strconv.Itoa(c.PR)
	}
	repl := strings.NewReplacer(
		"{{run_dir}}", c.RunDir,
		"{{inputs}}", c.Inputs,
		"{{workspace}}", c.Workspace,
		"{{base}}", c.Base,
		"{{target}}", c.Target,
		"{{pr}}", pr,
		"{{final}}", c.Final,
		"{{target_description}}", c.targetDescription(),
		"{{review_target_flags}}", c.reviewTargetFlags(),
	)
	return repl.Replace(s)
}

func (c Context) targetDescription() string {
	switch c.Mode {
	case "uncommitted":
		return "uncommitted local changes"
	case "branch":
		return fmt.Sprintf("committed changes against %s", c.Base)
	default:
		if c.PR > 0 {
			return fmt.Sprintf("PR %d against %s", c.PR, c.Base)
		}
		return fmt.Sprintf("changes against %s", c.Base)
	}
}

func (c Context) reviewTargetFlags() string {
	return ""
}

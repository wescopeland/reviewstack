package run

import (
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
	}
}

func (c Context) ExpandArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = c.Expand(a)
	}
	return out
}

func (c Context) Expand(s string) string {
	pr := "current"
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
	)
	return repl.Replace(s)
}

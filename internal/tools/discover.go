package tools

import (
	"fmt"
	"os/exec"
	"strings"
)

type Requirement struct {
	Name      string
	Commands  []string
	Required  bool
	Available bool
	UsedBy    []string
}

func Discover(cfgReviewerCommands []string, cfgSynthCommand string, needGH bool) []Requirement {
	reqs := map[string]*Requirement{
		"claude": {Name: "claude", Commands: []string{"claude"}, Required: false},
		"codex":  {Name: "codex", Commands: []string{"codex"}, Required: false},
		"gh":     {Name: "gh", Commands: []string{"gh"}, Required: false},
	}

	for _, cmd := range cfgReviewerCommands {
		base := filepathBase(cmd)
		switch base {
		case "claude":
			reqs["claude"].Required = true
		case "codex":
			reqs["codex"].Required = true
		}
	}
	base := filepathBase(cfgSynthCommand)
	if base == "codex" {
		reqs["codex"].Required = true
	}
	if needGH {
		reqs["gh"].Required = true
	}

	out := make([]Requirement, 0, len(reqs))
	order := []string{"claude", "codex", "gh"}
	for _, key := range order {
		r := reqs[key]
		r.Available = firstAvailable(r.Commands)
		out = append(out, *r)
	}
	return out
}

func Validate(requirements []Requirement) error {
	var missing []string
	for _, r := range requirements {
		if r.Required && !r.Available {
			missing = append(missing, r.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing required CLI tools: %s", strings.Join(missing, ", "))
}

func firstAvailable(commands []string) bool {
	for _, c := range commands {
		if _, err := exec.LookPath(c); err == nil {
			return true
		}
	}
	return false
}

func filepathBase(cmd string) string {
	if i := strings.LastIndex(cmd, "/"); i >= 0 {
		return cmd[i+1:]
	}
	return cmd
}

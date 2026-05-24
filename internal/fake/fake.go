package fake

import (
	"fmt"
	"time"

	"github.com/wescopeland/reviewstack/internal/config"
)

// ReviewerScript returns command and args that simulate reviewer behavior.
func ReviewerScript(id string) (string, []string, error) {
	switch id {
	case "claude-aesthetic":
		return shell, []string{"-c", `
echo "Aesthetic review: prefer simpler abstractions"
sleep 1
echo "- internal/handler.go: extract validation helper"
echo "2 findings"
`}, nil
	case "claude-analytical":
		return shell, []string{"-c", `
echo "Analytical review: edge cases matter" >&2
sleep 2
echo "- internal/handler.go: nil check missing on line 42"
echo "1 finding"
`}, nil
	case "codex-medium":
		return shell, []string{"-c", `
echo "tracing API path" >&2
sleep 1
echo "Medium: schema mismatch in request DTO"
`}, nil
	case "codex-xhigh":
		return shell, []string{"-c", `
echo "inspecting schema" >&2
for i in 1 2 3; do echo "deep trace $i" >&2; sleep 1; done
echo "XHigh: race in concurrent map access"
`}, nil
	case "claude-thermo":
		return shell, []string{"-c", `
sleep 1
echo "# Thermonuclear maintainability review"
echo "- internal/tui/model.go: table row padding could be shared helper"
echo "1 finding"
`}, nil
	default:
		return "", nil, fmt.Errorf("unknown fake reviewer %q", id)
	}
}

func Config() *config.Config {
	cfg := config.Default()
	for i := range cfg.Reviewers {
		cmd, args, err := ReviewerScript(cfg.Reviewers[i].ID)
		if err != nil {
			continue
		}
		cfg.Reviewers[i].Command = cmd
		cfg.Reviewers[i].Args = args
	}
	cfg.Synthesis = config.Synthesis{
		Command: shell,
		Args: []string{"-c", `
cat "$1" | head -n 40 > "$2"
echo "# Synthesized Review" >> "$2"
echo "" >> "$2"
echo "Merged findings from all reviewers." >> "$2"
`, "$1", "$2"},
	}
	return cfg
}

func ConfigWithSlow(id string, delay time.Duration) *config.Config {
	cfg := Config()
	for i := range cfg.Reviewers {
		if cfg.Reviewers[i].ID == id {
			cfg.Reviewers[i].Command = shell
			cfg.Reviewers[i].Args = []string{"-c", fmt.Sprintf(`
sleep %d
echo "slow reviewer done"
`, int(delay.Seconds()))}
		}
	}
	return cfg
}

func ConfigWithCancel(id string) *config.Config {
	cfg := Config()
	for i := range cfg.Reviewers {
		if cfg.Reviewers[i].ID == id {
			cfg.Reviewers[i].Command = shell
			cfg.Reviewers[i].Args = []string{"-c", `
trap 'exit 143' TERM
echo "starting long task" >&2
sleep 60
echo "should not reach"
`}
		}
	}
	return cfg
}

const shell = "sh"

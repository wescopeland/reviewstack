package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigPath = ".reviewstack/config.yaml"
	DefaultBaseRef    = "upstream/master"
	DefaultTargetRef  = "HEAD"
)

const (
	claudeAestheticPrompt  = "/aesthetic-review Review {{target_description}} using the Reviewstack inputs. Ignore any empty local git diff from the slash-command context; the captured diff is the source of truth. Read {{inputs}}/context.json, {{inputs}}/diff.patch, and {{inputs}}/diff-stat.txt, then write a markdown review to stdout with findings, severity, concrete fixes, and file/line references where possible."
	claudeAnalyticalPrompt = "/analytical-review Review {{target_description}} using the Reviewstack inputs. Ignore any empty local git diff from the slash-command context; the captured diff is the source of truth. Read {{inputs}}/context.json, {{inputs}}/diff.patch, and {{inputs}}/diff-stat.txt, then write a markdown review to stdout with findings, severity, concrete fixes, and file/line references where possible."
)

var DefaultReviewerIDs = []string{
	"claude-aesthetic",
	"claude-analytical",
	"codex-medium",
	"codex-xhigh",
	"claude-thermo",
}

type Config struct {
	Reviewers []Reviewer `yaml:"reviewers"`
	Synthesis Synthesis  `yaml:"synthesis"`
}

// Placeholders expanded at runtime: {{run_dir}} {{inputs}} {{workspace}}
// {{base}} {{target}} {{pr}} {{final}} {{target_description}} {{review_target_flags}}

type Reviewer struct {
	ID      string   `yaml:"id"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env,omitempty"`
}

type Synthesis struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if len(c.Reviewers) == 0 {
		return fmt.Errorf("config: at least one reviewer is required")
	}
	seen := make(map[string]struct{}, len(c.Reviewers))
	for _, r := range c.Reviewers {
		if strings.TrimSpace(r.ID) == "" {
			return fmt.Errorf("config: reviewer id is required")
		}
		if strings.TrimSpace(r.Command) == "" {
			return fmt.Errorf("config: reviewer %q: command is required", r.ID)
		}
		if _, ok := seen[r.ID]; ok {
			return fmt.Errorf("config: duplicate reviewer id %q", r.ID)
		}
		seen[r.ID] = struct{}{}
	}
	if strings.TrimSpace(c.Synthesis.Command) == "" {
		return fmt.Errorf("config: synthesis command is required")
	}
	return nil
}

func (c *Config) ReviewerByID(id string) (*Reviewer, error) {
	for i := range c.Reviewers {
		if c.Reviewers[i].ID == id {
			return &c.Reviewers[i], nil
		}
	}
	return nil, fmt.Errorf("reviewer %q not found in config", id)
}

func (c *Config) FilterReviewers(only []string) ([]Reviewer, error) {
	if len(only) == 0 {
		return c.Reviewers, nil
	}
	var out []Reviewer
	for _, id := range only {
		r, err := c.ReviewerByID(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, nil
}

func Default() *Config {
	return &Config{
		Reviewers: []Reviewer{
			{
				ID:      "claude-aesthetic",
				Command: "claude",
				Args:    []string{"--print", "--permission-mode", "dontAsk", claudeAestheticPrompt},
			},
			{
				ID:      "claude-analytical",
				Command: "claude",
				Args:    []string{"--print", "--permission-mode", "dontAsk", claudeAnalyticalPrompt},
			},
			{
				ID:      "codex-medium",
				Command: "codex",
				Args: []string{
					"review",
					"{{review_target_flags}}",
					"Review the changes in {{inputs}}/diff.patch using context from {{inputs}}/context.json and stats from {{inputs}}/diff-stat.txt. Write a markdown review to stdout with findings, severity, and concrete fixes.",
					"-c", `model="gpt-5.5"`, "-c", `model_reasoning_effort="medium"`, "-c", `service_tier="fast"`,
				},
			},
			{
				ID:      "codex-xhigh",
				Command: "codex",
				Args: []string{
					"review",
					"{{review_target_flags}}",
					"Review the changes in {{inputs}}/diff.patch using context from {{inputs}}/context.json and stats from {{inputs}}/diff-stat.txt. Write a markdown review to stdout with findings, severity, and concrete fixes.",
					"-c", `model="gpt-5.5"`, "-c", `model_reasoning_effort="xhigh"`, "-c", `service_tier="fast"`,
				},
			},
			{
				ID:      "claude-thermo",
				Command: "claude",
				Args: []string{
					"--print", "--permission-mode", "dontAsk", "--model", "opus", "--output-format", "text",
					"Thermonuclear maintainability review of {{target_description}}. Read {{inputs}}/diff.patch, {{inputs}}/context.json, and {{inputs}}/diff-stat.txt. Focus on coupling, abstraction depth, naming, and long-term maintainability. Write markdown to stdout.",
				},
			},
		},
		Synthesis: Synthesis{
			Command: "codex",
			Args: []string{
				"exec", "--dangerously-bypass-approvals-and-sandbox", "-C", "{{workspace}}", "-o", "{{final}}",
				"-c", `model="gpt-5.5"`, "-c", `model_reasoning_effort="medium"`, "-c", `service_tier="fast"`, "-",
			},
		},
	}
}

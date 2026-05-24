package doctor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wescopeland/reviewstack/internal/config"
)

type Check struct {
	Name    string
	Status  string // ok, warn, fail
	Detail  string
	FixHint string
}

func Run(w io.Writer) int {
	checks := []Check{
		checkTool("go", "version"),
		checkTool("git", "version"),
		checkTool("gh", "version"),
	}

	cfg, cfgErr := loadConfig()
	checks = append(checks, checkGhAuth())
	if cfgErr != nil {
		checks = append(checks, Check{Name: "config", Status: "fail", Detail: cfgErr.Error()})
	} else {
		checks = append(checks, configuredToolChecks(cfg)...)
		checks = append(checks, configuredAuthChecks(cfg)...)
		if needsClaudeSkills(cfg) {
			checks = append(checks, checkClaudeSkills())
		}
		if usesCommand(cfg, "codex") {
			checks = append(checks, checkCodexMCP())
		}
		checks = append(checks, checkConfig())
	}

	failures := 0
	for _, c := range checks {
		icon := "✓"
		switch c.Status {
		case "fail":
			icon = "✗"
			failures++
		case "warn":
			icon = "!"
		}
		_, _ = fmt.Fprintf(w, "%s %-14s %s\n", icon, c.Name+":", c.Detail)
		if c.FixHint != "" && c.Status != "ok" {
			_, _ = fmt.Fprintf(w, "    → %s\n", c.FixHint)
		}
	}

	_, _ = fmt.Fprintln(w)
	if failures == 0 {
		_, _ = fmt.Fprintln(w, "Ready for a real run:")
		_, _ = fmt.Fprintln(w, "  reviewstack --pr 4914 --base upstream/master")
		_, _ = fmt.Fprintln(w, "  reviewstack --pr 4914 --only claude-aesthetic   # one reviewer first")
	} else {
		_, _ = fmt.Fprintln(w, "Fix failures above, then run: reviewstack --pr <n>")
	}
	return failures
}

func loadConfig() (*config.Config, error) {
	for _, p := range []string{".reviewstack/config.yaml", ".reviewstack/config.yml"} {
		if _, err := os.Stat(p); err == nil {
			return config.Load(p)
		}
	}
	return config.Default(), nil
}

func configuredToolChecks(cfg *config.Config) []Check {
	var checks []Check
	if usesCommand(cfg, "claude") {
		checks = append(checks, checkTool("claude", "-v"))
	}
	if usesCommand(cfg, "codex") {
		checks = append(checks, checkTool("codex", "--version"))
	}
	return checks
}

func configuredAuthChecks(cfg *config.Config) []Check {
	var checks []Check
	if usesCommand(cfg, "codex") {
		checks = append(checks, checkAuth("codex", "codex", "doctor"))
	}
	return checks
}

func usesCommand(cfg *config.Config, name string) bool {
	for _, r := range cfg.Reviewers {
		if filepath.Base(r.Command) == name {
			return true
		}
	}
	return filepath.Base(cfg.Synthesis.Command) == name
}

func needsClaudeSkills(cfg *config.Config) bool {
	for _, r := range cfg.Reviewers {
		if filepath.Base(r.Command) != "claude" {
			continue
		}
		for _, arg := range r.Args {
			if strings.HasPrefix(strings.TrimSpace(arg), "/aesthetic-review") ||
				strings.HasPrefix(strings.TrimSpace(arg), "/analytical-review") {
				return true
			}
		}
	}
	return false
}

func checkTool(name string, args ...string) Check {
	if _, err := exec.LookPath(name); err != nil {
		return Check{
			Name:    name,
			Status:  "fail",
			Detail:  "not found in PATH",
			FixHint: "Install " + name,
		}
	}
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return Check{
			Name:    name,
			Status:  "warn",
			Detail:  strings.TrimSpace(string(out)),
			FixHint: "Check " + name + " installation",
		}
	}
	line := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
	return Check{Name: name, Status: "ok", Detail: line}
}

func checkAuth(name string, cmd string, args ...string) Check {
	if _, err := exec.LookPath(cmd); err != nil {
		return Check{Name: name + " auth", Status: "fail", Detail: "CLI missing"}
	}
	out, err := exec.Command(cmd, args...).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return Check{
			Name:    name + " auth",
			Status:  "fail",
			Detail:  firstLine(text),
			FixHint: authHint(name),
		}
	}
	if strings.Contains(strings.ToLower(text), "not logged in") || strings.Contains(strings.ToLower(text), "login") {
		return Check{
			Name:    name + " auth",
			Status:  "fail",
			Detail:  firstLine(text),
			FixHint: authHint(name),
		}
	}
	return Check{Name: name + " auth", Status: "ok", Detail: firstLine(text)}
}

func checkGhAuth() Check {
	if _, err := exec.LookPath("gh"); err != nil {
		return Check{Name: "gh auth", Status: "warn", Detail: "gh not installed (PR metadata optional)"}
	}
	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	if err != nil {
		return Check{
			Name:    "gh auth",
			Status:  "warn",
			Detail:  firstLine(string(out)),
			FixHint: "Run: gh auth login",
		}
	}
	return Check{Name: "gh auth", Status: "ok", Detail: firstLine(string(out))}
}

func checkClaudeSkills() Check {
	home, err := os.UserHomeDir()
	if err != nil {
		return Check{Name: "claude skills", Status: "warn", Detail: "cannot read home dir"}
	}
	dir := filepath.Join(home, ".claude", "commands")
	needed := []string{"aesthetic-review.md", "analytical-review.md"}
	var missing []string
	for _, f := range needed {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			missing = append(missing, strings.TrimSuffix(f, ".md"))
		}
	}
	if len(missing) > 0 {
		return Check{
			Name:    "claude skills",
			Status:  "fail",
			Detail:  "missing: " + strings.Join(missing, ", "),
			FixHint: "Install slash commands in ~/.claude/commands/",
		}
	}
	return Check{Name: "claude skills", Status: "ok", Detail: "aesthetic + analytical review"}
}

func checkCodexMCP() Check {
	if _, err := exec.LookPath("codex"); err != nil {
		return Check{Name: "codex MCP", Status: "warn", Detail: "codex not installed"}
	}
	out, err := exec.Command("codex", "mcp", "list").CombinedOutput()
	text := string(out)
	if err != nil || !strings.Contains(text, "OAuth") {
		return Check{Name: "codex MCP", Status: "ok", Detail: "no OAuth MCP servers configured"}
	}
	probe, _ := exec.Command(
		"sh", "-c",
		`timeout 8 codex review --uncommitted -c 'model="gpt-5.5"' 2>&1 | head -20`,
	).CombinedOutput()
	if strings.Contains(string(probe), "rmcp::transport::worker") {
		return Check{
			Name:    "codex MCP",
			Status:  "warn",
			Detail:  "OAuth MCP token expired (stderr noise; reviews still run)",
			FixHint: "Run: codex mcp login sentry — or add -c 'mcp_servers.sentry.enabled=false' to codex reviewer args",
		}
	}
	return Check{Name: "codex MCP", Status: "ok", Detail: "OAuth MCP servers healthy"}
}

func checkConfig() Check {
	paths := []string{".reviewstack/config.yaml", ".reviewstack/config.yml"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return Check{Name: "config", Status: "ok", Detail: p}
		}
	}
	return Check{
		Name:    "config",
		Status:  "warn",
		Detail:  "using built-in defaults",
		FixHint: "Copy .reviewstack/config.yaml and tune for your CLIs",
	}
}

func authHint(name string) string {
	switch name {
	case "claude":
		return "Run: claude login (or set ANTHROPIC_API_KEY)"
	case "codex":
		return "Run: codex login"
	default:
		return "Authenticate with " + name
	}
}

func firstLine(s string) string {
	if s == "" {
		return "(no output)"
	}
	return strings.Split(s, "\n")[0]
}

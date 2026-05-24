package cli

import (
	"flag"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var prURLPattern = regexp.MustCompile(`/pull/(\d+)`)

type Options struct {
	PR             int
	PRURL          string
	Base           string
	Target         string
	Only           []string
	SkipSynthesis  bool
	SynthesizeOnly string
	Open           bool
	ConfigPath     string
	DryRun         bool
	NoTUI          bool
	FakeReviewers  bool
	Doctor         bool
	Uncommitted    bool
	NoLauncher     bool
	Rereview       bool
	Mode           ReviewMode
	RunDir         string
}

func ParseArgs(args []string) (*Options, error) {
	fs := flag.NewFlagSet("reviewstack", flag.ContinueOnError)
	fs.SetOutput(flag.CommandLine.Output())

	var prFlag string
	opts := &Options{}
	fs.StringVar(&prFlag, "pr", "", "PR number or GitHub URL")
	fs.StringVar(&opts.Base, "base", "upstream/master", "Base ref for diff")
	fs.StringVar(&opts.Target, "target", "HEAD", "Target ref for diff")
	var only string
	fs.StringVar(&only, "only", "", "Comma-separated reviewer ids to run")
	fs.BoolVar(&opts.SkipSynthesis, "skip-synthesis", false, "Skip final synthesis")
	fs.StringVar(&opts.SynthesizeOnly, "synthesize-only", "", "Synthesize from an existing run directory")
	fs.BoolVar(&opts.Open, "open", false, "Open final report after synthesis")
	fs.StringVar(&opts.ConfigPath, "config", "", "Path to config YAML")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "Print planned commands without running")
	fs.BoolVar(&opts.NoTUI, "no-tui", false, "Non-interactive mode for scripting/CI")
	fs.BoolVar(&opts.FakeReviewers, "fake-reviewers", false, "Use built-in fake reviewer commands")
	fs.BoolVar(&opts.Doctor, "doctor", false, "Check CLI tools and auth, then exit")
	fs.BoolVar(&opts.Uncommitted, "uncommitted", false, "Review local uncommitted changes (no --pr required)")
	fs.BoolVar(&opts.NoLauncher, "no-launcher", false, "Skip interactive mode picker (require --pr or --uncommitted)")
	fs.BoolVar(&opts.Rereview, "rereview", false, "Re-review a PR against your prior GitHub feedback (requires --pr)")
	fs.StringVar(&opts.RunDir, "run-dir", "", "Existing run directory (with --synthesize-only)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if only != "" {
		opts.Only = splitCSV(only)
	}

	if prFlag != "" {
		if n, ok := parsePR(prFlag); ok {
			opts.PR = n
			opts.Mode = ModePR
		} else {
			opts.PRURL = prFlag
			opts.Mode = ModePR
		}
	}

	if opts.Uncommitted {
		opts.Mode = ModeUncommitted
	}

	return opts, nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parsePR(v string) (int, bool) {
	v = strings.TrimSpace(v)
	if n, err := strconv.Atoi(v); err == nil {
		return n, true
	}
	if m := prURLPattern.FindStringSubmatch(v); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, true
		}
	}
	if u, err := url.Parse(v); err == nil {
		if m := prURLPattern.FindStringSubmatch(u.Path); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

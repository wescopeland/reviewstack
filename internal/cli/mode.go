package cli

import "fmt"

type ReviewMode int

const (
	ModeUnset ReviewMode = iota
	ModePR
	ModeUncommitted
	ModeBranch
)

func (m ReviewMode) String() string {
	switch m {
	case ModePR:
		return "pr"
	case ModeUncommitted:
		return "uncommitted"
	case ModeBranch:
		return "branch"
	default:
		return "unset"
	}
}

func (o *Options) HasReviewTarget() bool {
	if o.Doctor || o.SynthesizeOnly != "" {
		return true
	}
	switch o.Mode {
	case ModePR:
		return o.PR > 0 || o.PRURL != ""
	case ModeUncommitted, ModeBranch:
		return true
	default:
		return false
	}
}

func (o *Options) NeedsLauncher() bool {
	if o.NoLauncher || o.Doctor || o.SynthesizeOnly != "" || o.FakeReviewers {
		return false
	}
	return !o.HasReviewTarget()
}

func (o *Options) Validate() error {
	if o.Uncommitted && (o.PR > 0 || o.PRURL != "") {
		return fmt.Errorf("--uncommitted cannot be used with --pr")
	}
	if o.Mode == ModeBranch && (o.PR > 0 || o.PRURL != "") {
		return fmt.Errorf("branch diff mode cannot be used with --pr")
	}

	if !o.Rereview {
		return nil
	}
	if o.Uncommitted {
		return fmt.Errorf("--rereview cannot be used with --uncommitted")
	}
	if o.Mode == ModeBranch {
		return fmt.Errorf("--rereview cannot be used with branch diff mode")
	}
	if o.PR <= 0 && o.PRURL == "" {
		return fmt.Errorf("--rereview requires --pr")
	}
	if o.Mode != ModePR && o.Mode != ModeUnset {
		return fmt.Errorf("--rereview requires PR mode (--pr)")
	}
	return nil
}

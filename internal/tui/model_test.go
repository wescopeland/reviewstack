package tui

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/wescopeland/reviewstack/internal/config"
	"github.com/wescopeland/reviewstack/internal/reviewer"
)

func testModel(t *testing.T, auto bool, onSynth func(context.Context) error) *Model {
	t.Helper()
	dir := t.TempDir()
	reviewers := []config.Reviewer{{ID: "a"}, {ID: "b"}}
	mgr := reviewer.NewManager(nil)
	mgr.Init([]string{"a", "b"}, map[string]reviewer.Paths{
		"a": {Raw: filepath.Join(dir, "a.md"), Log: filepath.Join(dir, "a.err")},
		"b": {Raw: filepath.Join(dir, "b.md"), Log: filepath.Join(dir, "b.err")},
	})
	return New(Config{
		RunDir:         dir,
		Reviewers:      reviewers,
		Manager:        mgr,
		AutoSynthesize: auto,
		OnSynthesize:   onSynth,
	})
}

func settleAll(m *Model) {
	m.cfg.Manager.SetStatusForTest("a", reviewer.StateDone)
	m.cfg.Manager.SetStatusForTest("b", reviewer.StateDone)
	m.refreshStatuses()
}

func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func finishSynthesis(t *testing.T, m *Model, cmd tea.Cmd) *Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected synthesis command")
	}
	updated, _ := m.Update(runCmd(cmd))
	return updated.(*Model)
}

func TestAutoSynthesisStartsWhenAllReviewersSettle(t *testing.T) {
	var calls int32
	m := testModel(t, true, func(context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	settleAll(m)
	m.updateSynthesisPhase()
	if m.synthPhase != synthesisDirty {
		t.Fatalf("phase = %v, want dirty", m.synthPhase)
	}

	m = finishSynthesis(t, m, m.maybeAutoSynthesize())
	if calls != 1 {
		t.Fatalf("synthesis calls = %d, want 1", calls)
	}
	if m.synthPhase != synthesisReady {
		t.Fatalf("phase = %v, want ready", m.synthPhase)
	}
}

func TestAutoSynthesizeFalseDoesNotAutoStart(t *testing.T) {
	var calls int32
	m := testModel(t, false, func(context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	settleAll(m)
	m.updateSynthesisPhase()
	if cmd := m.maybeAutoSynthesize(); cmd != nil {
		t.Fatal("expected no auto synthesis command")
	}
	if calls != 0 {
		t.Fatalf("synthesis calls = %d, want 0", calls)
	}
	if m.synthPhase != synthesisDirty {
		t.Fatalf("phase = %v, want dirty", m.synthPhase)
	}
}

func TestManualSynthKeyStartsSynthesis(t *testing.T) {
	var calls int32
	m := testModel(t, false, func(context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	settleAll(m)
	m.updateSynthesisPhase()
	m = finishSynthesis(t, m, m.requestSynthesis(true))

	if calls != 1 {
		t.Fatalf("synthesis calls = %d, want 1", calls)
	}
	if m.synthPhase != synthesisReady {
		t.Fatalf("phase = %v, want ready", m.synthPhase)
	}
}

func TestRerunMarksSynthesisDirtyAndAutoResynthesizes(t *testing.T) {
	var calls int32
	m := testModel(t, true, func(context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	settleAll(m)
	m.updateSynthesisPhase()
	m = finishSynthesis(t, m, m.maybeAutoSynthesize())
	if calls != 1 {
		t.Fatalf("initial synthesis calls = %d, want 1", calls)
	}

	m.invalidateSynthesis()
	m.cfg.Manager.SetStatusForTest("a", reviewer.StateRunning)
	m.refreshStatuses()
	m.cfg.Manager.SetStatusForTest("a", reviewer.StateDone)
	settleAll(m)
	m.updateSynthesisPhase()

	if m.synthPhase != synthesisDirty {
		t.Fatalf("phase = %v, want dirty after rerun settled", m.synthPhase)
	}

	m = finishSynthesis(t, m, m.maybeAutoSynthesize())
	if calls != 2 {
		t.Fatalf("synthesis calls = %d, want 2", calls)
	}
	if m.synthPhase != synthesisReady {
		t.Fatalf("phase = %v, want ready", m.synthPhase)
	}
}

func TestSynthesisErrorLeavesReportDirty(t *testing.T) {
	synthErr := errors.New("synthesis failed")
	m := testModel(t, true, func(context.Context) error {
		return synthErr
	})

	settleAll(m)
	m.updateSynthesisPhase()
	m = finishSynthesis(t, m, m.maybeAutoSynthesize())

	if m.synthPhase != synthesisDirty {
		t.Fatalf("phase = %v, want dirty", m.synthPhase)
	}
	if m.errMsg != synthErr.Error() {
		t.Fatalf("errMsg = %q, want %q", m.errMsg, synthErr.Error())
	}
}

func TestSettledReviewersIncludeFailedAndKilled(t *testing.T) {
	var calls int32
	m := testModel(t, true, func(context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	m.cfg.Manager.SetStatusForTest("a", reviewer.StateDone)
	m.cfg.Manager.SetStatusForTest("b", reviewer.StateFailed)
	m.refreshStatuses()
	m.updateSynthesisPhase()

	m = finishSynthesis(t, m, m.maybeAutoSynthesize())
	if calls != 1 {
		t.Fatalf("synthesis calls = %d, want 1", calls)
	}
	if !m.allReviewsFinished() {
		t.Fatal("expected all reviewers settled including failed")
	}
}

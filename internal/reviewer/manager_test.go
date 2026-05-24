package reviewer_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wescopeland/reviewstack/internal/config"
	"github.com/wescopeland/reviewstack/internal/reviewer"
)

func TestManagerStartupFailureMarksFailed(t *testing.T) {
	dir := t.TempDir()
	mgr := reviewer.NewManager(nil)
	mgr.Init([]string{"bad"}, map[string]reviewer.Paths{
		"bad": {Raw: "/nonexistent/raw.md", Log: filepath.Join(dir, "log.err")},
	})

	ctx := context.Background()
	r := config.Reviewer{
		ID:      "bad",
		Command: "sh",
		Args:    []string{"-c", "echo ok"},
	}
	if err := mgr.Start(ctx, r, reviewer.RunContext{Workspace: dir, RunDir: dir}); err == nil {
		t.Fatal("expected start error")
	}

	st, ok := mgr.Get("bad")
	if !ok {
		t.Fatal("reviewer not found")
	}
	if st.State != reviewer.StateFailed {
		t.Fatalf("state = %s, want failed", st.State)
	}
}

func TestManagerRunsFakeReviewer(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.md")
	logPath := filepath.Join(dir, "log.err")

	mgr := reviewer.NewManager(nil)
	mgr.Init([]string{"demo"}, map[string]reviewer.Paths{
		"demo": {Raw: raw, Log: logPath},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := config.Reviewer{
		ID:      "demo",
		Command: "sh",
		Args:    []string{"-c", `echo "hello stdout"; echo "hello stderr" >&2`},
	}
	rc := reviewer.RunContext{Workspace: dir, RunDir: dir, Base: "HEAD", Target: "HEAD"}
	if err := mgr.Start(ctx, r, rc); err != nil {
		t.Fatal(err)
	}

	waitUntil(t, func() bool {
		st, ok := mgr.Get("demo")
		return ok && st.State == reviewer.StateDone
	})

	rawData, err := os.ReadFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(rawData), "hello stdout") {
		t.Fatalf("raw = %q", rawData)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(logData), "hello stderr") {
		t.Fatalf("log = %q", logData)
	}
}

func TestManagerFiltersNoisyStderr(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.md")
	logPath := filepath.Join(dir, "log.err")

	var events []string
	var eventsMu sync.Mutex
	mgr := reviewer.NewManager(func(st reviewer.Status) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, st.LastEvent)
	})
	mgr.Init([]string{"codex"}, map[string]reviewer.Paths{
		"codex": {Raw: raw, Log: logPath},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := config.Reviewer{
		ID:      "codex",
		Command: "sh",
		Args:    []string{"-c", `echo "review output"; echo "2026 ERROR rmcp::transport::worker: worker quit with fatal" >&2; echo "exec done" >&2`},
	}
	if err := mgr.Start(ctx, r, reviewer.RunContext{Workspace: dir, RunDir: dir}); err != nil {
		t.Fatal(err)
	}

	waitUntil(t, func() bool {
		st, ok := mgr.Get("codex")
		return ok && st.State == reviewer.StateDone
	})

	eventsMu.Lock()
	gotEvents := append([]string(nil), events...)
	eventsMu.Unlock()

	for _, ev := range gotEvents {
		if strings.Contains(ev, "rmcp") {
			t.Fatalf("LastEvent should ignore rmcp noise, saw %q in events", ev)
		}
	}
	if !containsSlice(gotEvents, "exec done") {
		t.Fatalf("expected exec done in events, got %v", gotEvents)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(logData), "rmcp::transport::worker") {
		t.Fatalf("log should still contain rmcp noise: %q", logData)
	}
}

func TestManagerThrottlesOutputEvents(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.md")
	logPath := filepath.Join(dir, "log.err")

	var events atomic.Int64
	mgr := reviewer.NewManager(func(reviewer.Status) {
		events.Add(1)
	})
	mgr.Init([]string{"noisy"}, map[string]reviewer.Paths{
		"noisy": {Raw: raw, Log: logPath},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := config.Reviewer{
		ID:      "noisy",
		Command: "sh",
		Args:    []string{"-c", `i=0; while [ $i -lt 1000 ]; do echo "noise $i" >&2; i=$((i+1)); done`},
	}
	if err := mgr.Start(ctx, r, reviewer.RunContext{Workspace: dir, RunDir: dir}); err != nil {
		t.Fatal(err)
	}

	waitUntil(t, func() bool {
		st, ok := mgr.Get("noisy")
		return ok && st.State == reviewer.StateDone
	})

	if got := events.Load(); got >= 100 {
		t.Fatalf("expected throttled event count, got %d", got)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("expected noisy output to still be written to log")
	}
}

func TestManagerFailure(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.md")
	logPath := filepath.Join(dir, "log.err")

	mgr := reviewer.NewManager(nil)
	mgr.Init([]string{"fail"}, map[string]reviewer.Paths{
		"fail": {Raw: raw, Log: logPath},
	})

	ctx := context.Background()
	r := config.Reviewer{
		ID:      "fail",
		Command: "sh",
		Args:    []string{"-c", `echo "boom" >&2; exit 2`},
	}
	if err := mgr.Start(ctx, r, reviewer.RunContext{Workspace: dir, RunDir: dir}); err != nil {
		t.Fatal(err)
	}

	waitUntil(t, func() bool {
		st, ok := mgr.Get("fail")
		return ok && st.State == reviewer.StateFailed
	})

	st, _ := mgr.Get("fail")
	if st.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", st.ExitCode)
	}
}

func TestManagerCancelNoOrphans(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.md")
	logPath := filepath.Join(dir, "log.err")

	mgr := reviewer.NewManager(nil)
	mgr.Init([]string{"slow"}, map[string]reviewer.Paths{
		"slow": {Raw: raw, Log: logPath},
	})

	ctx, cancel := context.WithCancel(context.Background())
	r := config.Reviewer{
		ID:      "slow",
		Command: "sh",
		Args:    []string{"-c", `trap 'exit 143' TERM; sleep 30`},
	}
	if err := mgr.Start(ctx, r, reviewer.RunContext{Workspace: dir, RunDir: dir}); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)
	cancel()
	if err := mgr.Kill("slow"); err != nil {
		t.Fatal(err)
	}

	waitUntil(t, func() bool {
		st, ok := mgr.Get("slow")
		return ok && (st.State == reviewer.StateKilled || st.State == reviewer.StateFailed)
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, ok := mgr.Get("slow")
		if ok && st.State != reviewer.StateRunning {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("reviewer still running after cancel")
}

func waitUntil(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func containsSlice(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexSubstring(s, sub))
}

func indexSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

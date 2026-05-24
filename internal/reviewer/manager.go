package reviewer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wescopeland/reviewstack/internal/config"
	"github.com/wescopeland/reviewstack/internal/run"
)

const progressEventInterval = 250 * time.Millisecond

type RunContext struct {
	Workspace string
	RunDir    string
	Base      string
	Target    string
	PR        int
}

type Status struct {
	ID         string
	State      State
	StartedAt  time.Time
	FinishedAt time.Time
	RawBytes   int64
	LogBytes   int64
	LastEvent  string
	LastError  string
	ExitCode   int
}

type Manager struct {
	mu      sync.RWMutex
	status  map[string]*Status
	cmds    map[string]*exec.Cmd
	cancel  map[string]context.CancelFunc
	layout  map[string]Paths
	onEvent func(Status)
}

type Paths struct {
	Raw string
	Log string
}

func NewManager(onEvent func(Status)) *Manager {
	return &Manager{
		status:  make(map[string]*Status),
		cmds:    make(map[string]*exec.Cmd),
		cancel:  make(map[string]context.CancelFunc),
		layout:  make(map[string]Paths),
		onEvent: onEvent,
	}
}

func (m *Manager) Init(ids []string, paths map[string]Paths) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		m.status[id] = &Status{ID: id, State: StatePending}
		if p, ok := paths[id]; ok {
			m.layout[id] = p
		}
	}
}

func (m *Manager) Snapshot() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Status, 0, len(m.status))
	for _, st := range m.status {
		cp := *st
		out = append(out, cp)
	}
	return out
}

func (m *Manager) Get(id string) (Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st, ok := m.status[id]
	if !ok {
		return Status{}, false
	}
	cp := *st
	return cp, true
}

func (m *Manager) Start(parent context.Context, r config.Reviewer, rc RunContext) error {
	m.mu.Lock()
	st, ok := m.status[r.ID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("reviewer %q not initialized", r.ID)
	}
	if st.State == StateRunning {
		m.mu.Unlock()
		return fmt.Errorf("reviewer %q already running", r.ID)
	}
	paths := m.layout[r.ID]
	st.State = StatePending
	st.StartedAt = time.Time{}
	st.FinishedAt = time.Time{}
	st.RawBytes = 0
	st.LogBytes = 0
	st.LastEvent = "starting"
	st.LastError = ""
	st.ExitCode = 0
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(parent)
	runCtx := runContextFrom(rc)
	cmd := exec.CommandContext(ctx, r.Command, runCtx.ExpandArgs(r.Args)...)
	cmd.Dir = rc.Workspace
	if len(r.Env) > 0 {
		cmd.Env = append(os.Environ(), r.Env...)
	}
	setProcessGroup(cmd)

	rawFile, err := os.Create(paths.Raw)
	if err != nil {
		cancel()
		m.finish(r.ID, StateFailed, -1, err.Error())
		return fmt.Errorf("create raw file: %w", err)
	}
	logFile, err := os.Create(paths.Log)
	if err != nil {
		_ = rawFile.Close()
		cancel()
		m.finish(r.ID, StateFailed, -1, err.Error())
		return fmt.Errorf("create log file: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = rawFile.Close()
		_ = logFile.Close()
		cancel()
		m.finish(r.ID, StateFailed, -1, err.Error())
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = rawFile.Close()
		_ = logFile.Close()
		cancel()
		m.finish(r.ID, StateFailed, -1, err.Error())
		return fmt.Errorf("stderr pipe: %w", err)
	}

	m.mu.Lock()
	m.cmds[r.ID] = cmd
	m.cancel[r.ID] = cancel
	st.State = StateRunning
	st.StartedAt = time.Now()
	st.LastEvent = "running"
	m.mu.Unlock()
	m.emit(r.ID)

	if err := cmd.Start(); err != nil {
		_ = rawFile.Close()
		_ = logFile.Close()
		cancel()
		m.finish(r.ID, StateFailed, -1, err.Error())
		return err
	}

	go func() {
		<-ctx.Done()
		killProcessGroup(cmd)
	}()

	var streams sync.WaitGroup
	streams.Add(2)
	go func() {
		defer streams.Done()
		m.streamOutput(r.ID, stdout, rawFile, true)
	}()
	go func() {
		defer streams.Done()
		m.streamOutput(r.ID, stderr, logFile, false)
	}()

	go m.wait(r.ID, cmd, &streams)
	return nil
}

func (m *Manager) wait(id string, cmd *exec.Cmd, streams *sync.WaitGroup) {
	streams.Wait()
	err := cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	m.mu.RLock()
	st := m.status[id]
	state := st.State
	m.mu.RUnlock()

	if state == StateKilled {
		return
	}
	if err != nil {
		msg := err.Error()
		if exitCode != 0 {
			msg = fmt.Sprintf("exit %d", exitCode)
		}
		m.finish(id, StateFailed, exitCode, msg)
		return
	}
	m.finish(id, StateDone, 0, "completed")
}

func (m *Manager) finish(id string, state State, exitCode int, lastEvent string) {
	m.mu.Lock()
	st, ok := m.status[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if st.State == StateKilled && state != StateKilled {
		m.mu.Unlock()
		return
	}
	st.State = state
	st.FinishedAt = time.Now()
	st.ExitCode = exitCode
	st.LastEvent = lastEvent
	if state == StateFailed {
		st.LastError = readLastLine(m.layout[id].Log)
	}
	delete(m.cmds, id)
	delete(m.cancel, id)
	cp := *st
	m.mu.Unlock()
	m.emitStatus(cp)
}

func (m *Manager) streamOutput(id string, r io.Reader, w *os.File, isRaw bool) {
	defer func() { _ = w.Close() }()
	reader := bufio.NewReaderSize(r, 64*1024)
	lastEmit := time.Now().Add(-progressEventInterval)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			_, _ = w.WriteString(line + "\n")
			meaningful := m.recordOutputLine(id, line, isRaw)
			if meaningful && time.Since(lastEmit) >= progressEventInterval {
				lastEmit = time.Now()
				m.emit(id)
			}
		}
		if err != nil {
			if err != io.EOF {
				m.recordStreamError(id, err)
				m.emit(id)
			}
			break
		}
	}
	m.emit(id)
}

func (m *Manager) recordOutputLine(id, line string, isRaw bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.status[id]
	if !ok {
		return false
	}
	lineBytes := int64(len(line) + 1)
	if isRaw {
		st.RawBytes += lineBytes
	} else {
		st.LogBytes += lineBytes
	}
	meaningful := isRaw || !isNoisyStderrLine(line)
	if meaningful {
		st.LastEvent = truncate(line, 60)
	}
	return meaningful
}

func (m *Manager) recordStreamError(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.status[id]
	if !ok {
		return
	}
	st.LastError = err.Error()
	st.LastEvent = truncate(err.Error(), 60)
}

func (m *Manager) Kill(id string) error {
	m.mu.Lock()
	cancel, ok := m.cancel[id]
	cmd := m.cmds[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("reviewer %q is not running", id)
	}
	st := m.status[id]
	st.State = StateKilled
	st.LastEvent = "killed"
	m.mu.Unlock()

	killProcessGroup(cmd)
	cancel()
	m.finish(id, StateKilled, -1, "killed")
	return nil
}

func (m *Manager) KillAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.cancel))
	for id := range m.cancel {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		_ = m.Kill(id)
	}
}

// SetStatusForTest overrides a reviewer's state. It is intended for tests.
func (m *Manager) SetStatusForTest(id string, state State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.status[id]
	if !ok {
		return
	}
	st.State = state
}

func (m *Manager) Reset(id string) error {
	m.mu.Lock()
	st, ok := m.status[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("reviewer %q not found", id)
	}
	next, ok := Transition(st.State, EventReset)
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("reviewer %q cannot reset from %s", id, st.State)
	}
	st.State = next
	st.StartedAt = time.Time{}
	st.FinishedAt = time.Time{}
	st.RawBytes = 0
	st.LogBytes = 0
	st.LastEvent = "pending"
	st.LastError = ""
	st.ExitCode = 0
	cp := *st
	m.mu.Unlock()
	m.emitStatus(cp)
	return nil
}

func (m *Manager) emit(id string) {
	m.mu.RLock()
	st, ok := m.status[id]
	if !ok {
		m.mu.RUnlock()
		return
	}
	cp := *st
	m.mu.RUnlock()
	m.emitStatus(cp)
}

func (m *Manager) emitStatus(st Status) {
	if m.onEvent != nil {
		m.onEvent(st)
	}
}

func runContextFrom(rc RunContext) run.Context {
	return run.NewContext(rc.Workspace, rc.RunDir, rc.Base, rc.Target, rc.PR)
}

func readLastLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return truncate(line, 80)
		}
	}
	return ""
}

func isNoisyStderrLine(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "rmcp::transport::worker") {
		return true
	}
	if strings.Contains(lower, "tokenrefreshfailed") {
		return true
	}
	return false
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 3 {
		div *= unit
		exp++
	}
	val := float64(n) / float64(div)
	suffix := []string{"KB", "MB", "GB"}[exp]
	return fmt.Sprintf("%.0f %s", val, suffix)
}

func Elapsed(st Status, now time.Time) time.Duration {
	if st.StartedAt.IsZero() {
		return 0
	}
	end := st.FinishedAt
	if end.IsZero() && st.State == StateRunning {
		end = now
	}
	if end.IsZero() {
		return 0
	}
	return end.Sub(st.StartedAt)
}

func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

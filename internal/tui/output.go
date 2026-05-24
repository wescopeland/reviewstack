package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxOutputBytes = 24 * 1024
	maxOutputLines = 250
)

func outputCacheKey(path string, size int64, modTime time.Time) string {
	return fmt.Sprintf("%s|%d|%d", path, size, modTime.UnixNano())
}

func (m *Model) outputPath() string {
	id := m.selectedID()
	if id == "" {
		return ""
	}
	if m.viewMode == viewLog {
		return filepath.Join(m.cfg.RunDir, "logs", id+".err")
	}
	return filepath.Join(m.cfg.RunDir, "raw", id+".md")
}

func readOutputFile(path string) (string, string) {
	if path == "" {
		return muted.Render("(no reviewer selected)"), ""
	}

	info, err := os.Stat(path)
	if err != nil {
		return muted.Render("(waiting for output…)"), ""
	}

	key := outputCacheKey(path, info.Size(), info.ModTime())
	content := readOutputTail(path, info.Size())
	if content == "" {
		content = muted.Render("(empty)")
	}
	return content, key
}

func readOutputTail(path string, size int64) string {
	if size == 0 {
		return ""
	}

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	readSize := size
	truncated := false
	if readSize > maxOutputBytes {
		readSize = maxOutputBytes
		truncated = true
		if _, err := f.Seek(size-readSize, io.SeekStart); err != nil {
			return ""
		}
	}

	data, err := io.ReadAll(io.LimitReader(f, readSize))
	if err != nil || len(data) == 0 {
		return ""
	}

	text := string(data)
	if truncated {
		if idx := strings.IndexByte(text, '\n'); idx >= 0 {
			text = text[idx+1:]
		}
	}

	lines := strings.Split(text, "\n")
	if len(lines) > maxOutputLines {
		lines = lines[len(lines)-maxOutputLines:]
		text = muted.Render(fmt.Sprintf("… last %d lines — full file on disk", maxOutputLines)) + "\n\n" + strings.Join(lines, "\n")
	} else if truncated {
		text = muted.Render("… truncated — full file on disk") + "\n\n" + text
	}
	return text
}

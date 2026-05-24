package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadOutputTailSmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("hello\nworld"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readOutputTail(path, 11)
	if got != "hello\nworld" {
		t.Fatalf("got %q", got)
	}
}

func TestReadOutputTailLargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.err")
	line := strings.Repeat("x", 100) + "\n"
	data := strings.Repeat(line, 1000)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got := readOutputTail(path, info.Size())
	if !strings.Contains(got, "last 250 lines") && !strings.Contains(got, "truncated") {
		t.Fatalf("expected truncation notice, got len=%d", len(got))
	}
}

func TestReadOutputFileCachesKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.md")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	content, key := readOutputFile(path)
	if content == "" || key == "" {
		t.Fatal("expected content and cache key")
	}

	content2, key2 := readOutputFile(path)
	if content != content2 || key != key2 {
		t.Fatal("expected stable read")
	}

	if err := os.WriteFile(path, []byte("v2-longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, key3 := readOutputFile(path)
	if key3 == key {
		t.Fatal("expected cache key change after write")
	}
}

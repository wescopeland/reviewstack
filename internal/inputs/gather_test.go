package inputs

import (
	"strings"
	"testing"
)

func TestDiffStatBar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		add, del  int
		wantPlus  bool
		wantMinus bool
	}{
		{10, 0, true, false},
		{0, 10, false, true},
		{5, 5, true, true},
	}
	for _, tc := range tests {
		bar := diffStatBar(tc.add, tc.del)
		if tc.wantPlus && !strings.Contains(bar, "+") {
			t.Fatalf("add=%d del=%d: expected + in %q", tc.add, tc.del, bar)
		}
		if tc.wantMinus && !strings.Contains(bar, "-") {
			t.Fatalf("add=%d del=%d: expected - in %q", tc.add, tc.del, bar)
		}
	}
}

func TestFormatPRDiffStat(t *testing.T) {
	t.Parallel()

	out := formatPRDiffStat(prDiffStatPayload{
		Files: []struct {
			Path      string `json:"path"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		}{{Path: "a.go", Additions: 3, Deletions: 1}},
		Additions:    3,
		Deletions:    1,
		ChangedFiles: 1,
	})
	if !strings.Contains(out, "a.go") {
		t.Fatalf("missing file stat: %s", out)
	}
	if !strings.Contains(out, "1 files changed, 3 insertions(+), 1 deletions(-)") {
		t.Fatalf("missing summary: %s", out)
	}
}

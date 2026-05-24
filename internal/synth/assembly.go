package synth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxRawPromptBytes = 128 * 1024
	maxLogPromptBytes = 32 * 1024
)

const rereviewSynthesisPrompt = `You are producing a re-review report for a pull request that was updated after prior feedback.

You will receive:
- the authenticated reviewer's prior GitHub comments and review summaries
- optional prior reviewstack final-review.md excerpt from an earlier run on this PR
- the current PR diff stats and context
- fresh independent reviewer outputs from the current run

Your job:
1. Compare prior GitHub feedback against the current PR diff and reviewer outputs.
2. Classify each prior comment into: addressed, needs follow-up, or unclear/manual-check.
3. Summarize new findings from the current reviewer stack worth acting on.
4. Be concise, actionable, and cite file/line when available.

Output markdown only. Start with "# Re-review report" as the top heading.
Include these sections:
- ## Prior feedback status (addressed / follow-up / manual-check tables or lists)
- ## New findings
- ## Suggested next steps

Do not post to GitHub. This is a local report only.

`

const synthesisPrompt = `You are the final synthesis reviewer for a pull request.

You will receive multiple independent code review outputs (raw stdout) and their stderr/log traces.
Some tools write useful findings to stderr — treat both as evidence.
Large logs may be truncated and binary/control-heavy content may be omitted before it reaches you.

Your job:
1. Merge duplicate findings across reviewers.
2. Reject speculative, stale, unactionable, unsupported, or not-worth-fixing findings.
3. Keep only legitimate PR feedback worth acting on.
4. For each accepted finding include:
   - severity (blocker, major, minor, nit)
   - file/line when available
   - paste-ready PR comment
   - why it is valid
   - minimal fix direction
   - which reviewers found it
5. Include rejected findings with short reasons.

Output markdown only. Be concise and actionable.

`

type Input struct {
	RunDir    string
	Reviewers []string
	Paths     map[string]Paths
	Rereview  bool
}

type Paths struct {
	Raw string
	Log string
}

func Assemble(in Input) (string, error) {
	var b strings.Builder
	if in.Rereview {
		b.WriteString(rereviewSynthesisPrompt)
	} else {
		b.WriteString(synthesisPrompt)
	}
	b.WriteString("\n---\n\n")

	if in.Rereview {
		writeFileSection(&b, "Re-review Context", "json", filepath.Join(in.RunDir, "inputs", "rereview-context.json"), maxRawPromptBytes)
	}

	writeFileSection(&b, "PR Context", "json", filepath.Join(in.RunDir, "inputs", "context.json"), maxRawPromptBytes)
	writeFileSection(&b, "Diff Stat", "", filepath.Join(in.RunDir, "inputs", "diff-stat.txt"), maxRawPromptBytes)

	for _, id := range in.Reviewers {
		p, ok := in.Paths[id]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "## Reviewer: %s\n\n", id)

		raw, err := readPromptFile(p.Raw, maxRawPromptBytes)
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("read raw %s: %w", p.Raw, err)
		}
		b.WriteString("### Raw output\n\n")
		if raw == "" {
			b.WriteString("_empty_\n\n")
		} else {
			writeFenced(&b, "markdown", raw)
		}

		logData, err := readPromptFile(p.Log, maxLogPromptBytes)
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("read log %s: %w", p.Log, err)
		}
		b.WriteString("### Log / stderr\n\n")
		if logData == "" {
			b.WriteString("_empty_\n\n")
		} else {
			writeFenced(&b, "", logData)
		}
	}

	return b.String(), nil
}

func writeFileSection(b *strings.Builder, label, lang, path string, limit int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	fmt.Fprintf(b, "## %s\n\n", label)
	writeFenced(b, lang, promptText(data, limit))
}

func readPromptFile(path string, limit int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return promptText(data, limit), nil
}

func promptText(data []byte, limit int) string {
	if len(data) == 0 {
		return ""
	}
	data = limitBytes(data, limit)
	text := strings.ToValidUTF8(string(data), "[invalid utf-8]")
	return cleanPromptText(text)
}

func limitBytes(data []byte, limit int) []byte {
	if limit <= 0 || len(data) <= limit {
		return data
	}

	head := limit / 2
	tail := limit - head
	omitted := len(data) - head - tail
	marker := []byte(fmt.Sprintf("\n\n[... omitted %d bytes from middle ...]\n\n", omitted))

	out := make([]byte, 0, head+len(marker)+tail)
	out = append(out, data[:head]...)
	out = append(out, marker...)
	out = append(out, data[len(data)-tail:]...)
	return out
}

func cleanPromptText(text string) string {
	var b strings.Builder
	omittedBinaryLines := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if binaryLike(line) {
			omittedBinaryLines++
			continue
		}
		if omittedBinaryLines > 0 {
			fmt.Fprintf(&b, "[... omitted %d binary/control-heavy lines ...]\n", omittedBinaryLines)
			omittedBinaryLines = 0
		}
		writeCleanLine(&b, line)
	}
	if omittedBinaryLines > 0 {
		fmt.Fprintf(&b, "[... omitted %d binary/control-heavy lines ...]\n", omittedBinaryLines)
	}
	return b.String()
}

func binaryLike(line string) bool {
	if strings.ContainsRune(line, '\x00') {
		return true
	}
	if len(line) < 256 {
		return false
	}

	// Long lines with a high ratio of control characters are usually binary
	// or corrupted tool output, not review text worth sending to the model.
	controlCount := 0
	for _, r := range line {
		switch r {
		case '\n', '\r', '\t':
			continue
		}
		if r < ' ' {
			controlCount++
		}
	}
	return controlCount > 0 && controlCount*10 > len(line)
}

func writeCleanLine(b *strings.Builder, line string) {
	for _, r := range line {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(r)
		case r >= ' ':
			b.WriteRune(r)
		default:
			b.WriteByte('?')
		}
	}
}

func writeFenced(b *strings.Builder, language string, text string) {
	fence := fenceFor(text)
	b.WriteString(fence)
	b.WriteString(language)
	b.WriteByte('\n')
	b.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(fence)
	b.WriteString("\n\n")
}

func fenceFor(text string) string {
	longest := 0
	current := 0
	for _, r := range text {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	if longest < 3 {
		return "```"
	}
	return strings.Repeat("`", longest+1)
}

func WritePrompt(runDir string, content string) (string, error) {
	path := filepath.Join(runDir, "synthesis-prompt.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

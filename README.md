# reviewstack

Run several code reviewers in parallel against a PR or local diff. Bubble Tea TUI for live status, reruns, and a merged final report.

## Install

```bash
go install ./cmd/reviewstack
# or: make install
```

Go 1.26+.

## Usage

```bash
reviewstack                  # mode picker (PR, uncommitted, branch diff)
reviewstack --pr 4914
reviewstack --uncommitted
reviewstack --pr 4914 --only claude-aesthetic
reviewstack --doctor         # check CLIs + config
```

Reviewers run from the **repo root** you invoke from. Outputs go to `.reviewstack/runs/`. When everyone finishes, synthesis writes `final-review.md` (disable with `--skip-synthesis`).

| Flag | |
|------|---|
| `--pr` | PR number or URL |
| `--uncommitted` | staged + unstaged vs HEAD |
| `--base` | base ref (default `upstream/master`) |
| `--only` | comma-separated reviewer ids |
| `--dry-run` | print commands, don't run |
| `--no-tui` | headless / scripting |
| `--fake-reviewers` | built-in fakes for dev (`make run-fake`) |
| `--no-launcher` | skip mode picker; require flags |

Needs `gh` for PR mode, plus whatever your config invokes (`claude`, `codex`, etc.). Copy and edit `.reviewstack/config.yaml` — placeholders: `{{base}}`, `{{pr}}`, `{{target_description}}`, `{{review_target_flags}}`, `{{inputs}}`, `{{workspace}}`, `{{final}}`.

Reviewer ids: `claude-aesthetic`, `claude-analytical`, `codex-medium`, `codex-xhigh`, `claude-thermo`.

## TUI

`Tab` cycles views. `Enter` opens raw output. `r` rerun, `k` kill, `s` synthesize, `o` open final report, `q` quit.

## Development

See [AGENTS.md](AGENTS.md). Quick loop: `make verify-fast`. Before pushing: `make verify`.

## License

MIT

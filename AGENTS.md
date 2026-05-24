# reviewstack — agent guide

Single entry point for validation:

```bash
make verify
```

Run this after every code change before marking work complete.

## Commands

| Command | When to use |
|---------|-------------|
| `make verify` | Full check before finishing (fmt, lint, test, build, smoke) |
| `make verify-fast` | Quick loop while iterating (skips smoke E2E) |
| `make fmt` | Auto-format with gofumpt |
| `make test` | Unit tests with race detector |
| `make smoke` | Headless fake reviewer E2E |
| `make help` | List all targets |

## Project layout

```
cmd/reviewstack/          CLI entrypoint
internal/
  app/                    orchestration
  cli/                    flags
  config/                 YAML config
  fake/                   fake reviewers for dev/test
  inputs/                 PR/diff gathering
  reviewer/               process manager
  run/                    .reviewstack/runs/ layout
  synth/                  synthesis prompt assembly
  tools/                  CLI discovery
  tui/                    Bubble Tea dashboard
.reviewstack/config.yaml  example reviewer config
```

## Conventions

- Go 1.26+, format with **gofumpt** (not plain gofmt)
- Lint with **golangci-lint v2** (`standard` preset in `.golangci.yml`)
- Tests live next to code: `foo_test.go` in same package or `foo_test` external test package
- Use `exec.CommandContext` for subprocesses; never orphan child processes
- Config dir: `.reviewstack/` (no hyphen in tool name)

## Reviewer IDs

- `claude-aesthetic` — `/aesthetic-review`
- `claude-analytical` — `/analytical-review`
- `codex-medium`, `codex-xhigh`, `cursor-thermo`

## Real runs

```bash
reviewstack                                    # mode picker
reviewstack --pr 4914                          # skip picker
reviewstack --uncommitted                      # local changes
reviewstack --pr 4914 --only claude-aesthetic  # one reviewer first
```

Reviewers execute from the **repo root** so git/slash commands work. Outputs go to `.reviewstack/runs/`.

Integration test (included in `make test`):

```bash
REVIEWSTACK_INTEGRATION=1 go test ./internal/app -run TestHeadlessFakeRun
```

## Common failures

| Error | Fix |
|-------|-----|
| `files need formatting` | `make fmt` |
| golangci-lint errcheck on `Close()` | use `_ = f.Close()` |
| missing CLI tools | use `--fake-reviewers` for dev, or install claude/codex/cursor-agent |

## Do not

- Commit `.reviewstack/runs/` artifacts
- Hardcode reviewer CLI flags — use `.reviewstack/config.yaml`
- Skip `make verify` after substantive changes

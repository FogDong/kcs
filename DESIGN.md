# KCS Design Document

## Overview

`kcs` is a CLI tool for managing multiple Kubernetes kubeconfig files and contexts. It scans `~/.kube/`, aggregates all contexts, and provides an interactive fuzzy search interface to switch between them.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         kcs CLI                              │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │   Scanner    │  │    Parser    │  │    Selector      │   │
│  │              │  │              │  │                  │   │
│  │ - Find files │  │ - Parse YAML │  │ - Fuzzy search   │   │
│  │ - Validate   │  │ - Parse JSON │  │ - Interactive UI │   │
│  │   kubeconfig │  │ - Extract    │  │                  │   │
│  │              │  │   contexts   │  │                  │   │
│  └──────────────┘  └──────────────┘  └──────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                    Switcher                           │   │
│  │  - Persistent: symlink ~/.kube/kcs-config → source    │   │
│  │  - Session: write minimal kubeconfig + session link   │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## How It Works

1. **Scan**: Find all kubeconfig files in `~/.kube/` (supports YAML and JSON)
2. **Parse**: Extract contexts using `k8s.io/client-go`
3. **Select**: Interactive fuzzy search with `promptui`
4. **Switch**: Persistent or session switch (see below)

## Switching Modes

### Persistent mode

`kcs` manages `~/.kube/kcs-config` as a symlink pointing to the selected
kubeconfig file, then runs `kubectl config use-context` to set
`current-context` in that file. All shells sharing this file see the change.

### Session mode

`kcs` writes a minimal single-context kubeconfig to
`~/.config/kcs/<context-name>` (created once, reused on subsequent switches to
the same context) and updates a per-shell session symlink at
`$XDG_RUNTIME_DIR/kcs/sessions/<KCS_SESSION>`.

Because `KUBECONFIG` is set to `<session-path>:~/.kube/kcs-config`, kubectl
resolves the session symlink first. Switching sessions in one shell has no
effect on other shells.

### Mode selection

| Condition | Behavior |
|-----------|----------|
| Default (no env, no flags) | Persistent |
| `KCS_DEFAULT_SESSION` set | Session |
| `-s` / `--session` flag | Session (overrides env) |
| `-p` / `--persistent` flag | Persistent (overrides env) |

## Shell Initialization (`kcs init`)

`kcs init` outputs shell exports for use with `eval $(kcs init)`:

- Always pins `KCS_SESSION` to the shell's PID (if unset), so the session path
  is stable for the lifetime of the shell
- Always sets `KUBECONFIG=<session-path>:~/.kube/kcs-config`
- `--session` additionally exports `KCS_DEFAULT_SESSION=1`

The session symlink may not exist initially; kubectl silently skips missing
entries in `KUBECONFIG`, so the fallback to `kcs-config` is automatic.

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `KCS_SESSION` | Session identifier used to compute the session symlink path. Set to the shell PID by `kcs init`. |
| `KCS_DEFAULT_SESSION` | When non-empty, makes session switching the default behavior. Set by `kcs init --session`. |

## Project Structure

```
kcs/
├── main.go                 # Entry point
├── cmd/
│   └── root.go             # CLI commands (root, init)
├── internal/
│   ├── scanner/
│   │   └── scanner.go      # File discovery and validation
│   ├── parser/
│   │   └── parser.go       # Kubeconfig parsing
│   ├── selector/
│   │   └── selector.go     # Interactive selection UI
│   └── switcher/
│       └── switcher.go     # Context switching logic
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── DESIGN.md
└── CLAUDE.md
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/manifoldco/promptui` | Interactive prompts |
| `github.com/sahilm/fuzzy` | Fuzzy string matching |
| `k8s.io/client-go` | Kubeconfig parsing |

## Data Structures

```go
type ContextInfo struct {
    Name           string // Context name
    Cluster        string // Cluster name
    User           string // User/AuthInfo name
    Namespace      string // Default namespace
    SourceFile     string // Full path to kubeconfig file
    SourceFileName string // Filename for display
    IsCurrent      bool   // Current context in source file
}
```

## Error Handling

- Invalid kubeconfig files are skipped with a warning
- No kubeconfig files: exit with helpful message
- No matching contexts: exit with "No contexts match the query"
- User cancellation (Ctrl+C): exit gracefully

## Future Enhancements

- Context favorites/bookmarks
- Recent contexts history
- Namespace switching within a context

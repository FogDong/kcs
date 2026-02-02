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
│  │  - Create symlink ~/.kube/kcs-config → source file    │   │
│  │  - Execute kubectl config use-context                 │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## How It Works

1. **Scan**: Find all kubeconfig files in `~/.kube/` (supports YAML and JSON)
2. **Parse**: Extract contexts using `k8s.io/client-go`
3. **Select**: Interactive fuzzy search with `promptui`
4. **Switch**: Create symlink and run `kubectl config use-context`

### Symlink Approach

`kcs` manages `~/.kube/kcs-config` as a symlink pointing to the selected kubeconfig file. Users set `KUBECONFIG=~/.kube/kcs-config` in their shell, and `kcs` updates the symlink target when switching contexts.

This approach:
- Leaves original `~/.kube/config` untouched
- Avoids merging configs (prevents conflicts)
- Works with any number of kubeconfig files

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

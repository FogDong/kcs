# CLAUDE.md

Development guidelines for `kcs` - Kubernetes Config Switcher.

## Build Commands

```bash
go build -o kcs .           # Build binary
go install .                 # Install to $GOPATH/bin
go test ./...                # Run tests
go test -cover ./...         # Run tests with coverage
```

## Architecture

```
kcs/
├── main.go              # Entry point
├── cmd/
│   └── root.go          # CLI commands and flags
├── internal/
│   ├── scanner/         # Discovers kubeconfig files in ~/.kube
│   ├── parser/          # Parses kubeconfig using client-go
│   ├── selector/        # Interactive TUI with fuzzy search
│   └── switcher/        # Creates symlink, runs kubectl
```

## Key Behaviors

### Context Switching
1. Creates symlink `~/.kube/kcs-config` → selected kubeconfig file
2. Runs `kubectl config use-context --kubeconfig ~/.kube/kcs-config`
3. Original `~/.kube/config` is never modified

### File Discovery
- Scans `~/.kube/` for valid kubeconfig files
- Supports both YAML and JSON formats
- Skips: `.bak`, `.key`, `.crt`, `.pem`, cache files, `kcs-config`

### Fuzzy Search
Searches against: cluster name, filename, namespace

## Common Tasks

### Add a CLI flag
Edit `cmd/root.go`, add flag in `init()`, use in `run()`

### Change display format
Edit `internal/selector/selector.go`, modify the `items` string format

### Change file discovery
Edit `internal/scanner/scanner.go`, modify `Scan()` or `shouldSkip()`

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/manifoldco/promptui` | Interactive prompts |
| `github.com/sahilm/fuzzy` | Fuzzy matching |
| `k8s.io/client-go` | Kubeconfig parsing |

# kcs - Kubernetes Config Switcher

A fast CLI tool to switch between Kubernetes contexts across multiple kubeconfig files.

## Features

- Scans all kubeconfig files in `~/.kube/`
- Interactive fuzzy search to find contexts quickly
- Supports both YAML and JSON kubeconfig formats
- Switches contexts without modifying your kubeconfig files directly
- Works immediately after switching (no shell restart needed)
- Per-shell session contexts that don't affect other terminals

## Installation

### From Source

Requires Go 1.21 or later.

```bash
git clone https://github.com/FogDong/kcs.git
cd kcs
go build -o kcs .
sudo mv kcs /usr/local/bin/
```

### Using Go Install

```bash
go install github.com/FogDong/kcs@latest
```

## Setup

Add to your shell configuration and reload:

```bash
# ~/.zshrc or ~/.bashrc

# Persistent switching by default (all shells share one context):
eval $(kcs init)

# Session switching by default (each shell has its own context):
eval $(kcs init --session)
```

`kcs init` sets `KUBECONFIG` to include both a per-shell session path and
`~/.kube/kcs-config`. If no session context has been set, kubectl falls back
to `kcs-config` automatically.

### With mise

```toml
[env]
_.kcs = {}
```

## Usage

### Interactive Selection

```bash
kcs
```

Shows all available contexts from all kubeconfig files. Use arrow keys to navigate, type to filter.

### Fuzzy Search

```bash
kcs prod
```

Pre-filters contexts matching "prod", then shows interactive selection.

### List All Contexts

```bash
kcs --list
```

Output:
```
[config] prod-cluster (ns: default)
[config] staging-cluster (ns: staging)
[dev-config] dev-cluster (ns: default)
```

### Show Current Context

```bash
kcs --current
```

Output:
```
prod-cluster (kubeconfig: config)
```

## Session Mode

By default, `kcs` updates `~/.kube/kcs-config`, which is shared across all
shells. Session mode lets each shell maintain its own independent context.

| Setup | Default behavior |
|-------|-----------------|
| `eval $(kcs init)` | Updates shared `kcs-config` |
| `eval $(kcs init --session)` | Updates per-shell session context |

You can always override the default with a flag:

| Flag | Effect |
|------|--------|
| `-s`, `--session` | Force update session context for this switch |
| `-p`, `--persistent` | Force update shared `kcs-config` for this switch |

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `KCS_SESSION` | Pins the session identifier (set automatically by `kcs init`) |
| `KCS_DEFAULT_SESSION` | When set, makes session switching the default (set by `kcs init --session`) |

## How It Works

### Persistent mode (`kcs` / `kcs -p`)

1. Creates/updates symlink `~/.kube/kcs-config` → selected kubeconfig file
2. Runs `kubectl config use-context` to set `current-context` in that file

### Session mode (`kcs -s` / `kcs` with `KCS_DEFAULT_SESSION`)

1. Writes a minimal single-context kubeconfig to `~/.config/kcs/<context-name>`
2. Updates a per-shell session symlink in `$XDG_RUNTIME_DIR/kcs/sessions/<id>`
3. Because `KUBECONFIG` lists the session path before `kcs-config`, kubectl picks up
   the session context immediately

## Command Reference

```
kcs [search] [flags]

Arguments:
  [search]    Optional fuzzy search query to filter contexts

Flags:
  -s, --session     Update session context (overrides default persistent behavior)
  -p, --persistent  Update shared kcs-config (overrides KCS_DEFAULT_SESSION)
  -l, --list        List all contexts without interactive selection
  -c, --current     Show current context
  -d, --dir PATH    Custom kubeconfig directory (default: ~/.kube)
  -h, --help        Show help

Subcommands:
  init              Output shell exports for use with eval $(kcs init)
    -s, --session   Also export KCS_DEFAULT_SESSION to make session switching the default
```

## License

MIT

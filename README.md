# kcs - Kubernetes Config Switcher

A fast CLI tool to switch between Kubernetes contexts across multiple kubeconfig files.

## Features

- Scans all kubeconfig files in `~/.kube/`
- Interactive fuzzy search to find contexts quickly
- Supports both YAML and JSON kubeconfig formats
- Leaves your original `~/.kube/config` untouched
- Works immediately after switching (no shell restart needed)

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

After installation, run the init command to set up your shell:

```bash
kcs init
```

This will show you the command to add to your shell configuration:

```bash
# For zsh (~/.zshrc)
echo 'export KUBECONFIG=~/.kube/kcs-config' >> ~/.zshrc
source ~/.zshrc

# For bash (~/.bashrc)
echo 'export KUBECONFIG=~/.kube/kcs-config' >> ~/.bashrc
source ~/.bashrc
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
[config] staging-guc1.tail1beac.ts.net (ns: default)
[config] prod-us-east.example.com (ns: production)
[dev-cluster] dev-context (ns: default)
```

### Show Current Context

```bash
kcs --current
```

Output:
```
staging-guc1.tail1beac.ts.net (kubeconfig: config)
```

## How It Works

1. `kcs` scans `~/.kube/` for all valid kubeconfig files
2. Parses and aggregates all contexts from all files
3. When you select a context, it:
   - Creates a symlink `~/.kube/kcs-config` → selected kubeconfig file
   - Runs `kubectl config use-context` to switch

Your original `~/.kube/config` is never modified.

## Command Reference

```
kcs [search] [flags]

Arguments:
  [search]    Optional fuzzy search query to filter contexts

Flags:
  -l, --list        List all contexts without interactive selection
  -c, --current     Show current context
  -d, --dir PATH    Custom kubeconfig directory (default: ~/.kube)
  -h, --help        Show help
```

## License

MIT

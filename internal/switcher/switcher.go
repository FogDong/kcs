package switcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/FogDong/kcs/internal/parser"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const kcsConfigName = "kcs-config"

// SwitchSession creates a single-context kubeconfig in the user's config directory
// for the given context, creates/updates a session symlink pointing to it, and
// returns the session symlink path. The kubeconfig is deterministic per context
// name so repeated switches reuse the same file.
func SwitchSession(ctx parser.ContextInfo) (string, error) {
	sourceFile, err := filepath.Abs(ctx.SourceFile)
	if err != nil {
		return "", fmt.Errorf("failed to resolve source file path: %w", err)
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user config directory: %w", err)
	}
	kcsConfigDir := filepath.Join(configDir, "kcs")
	if err := os.MkdirAll(kcsConfigDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create kcs config directory: %w", err)
	}

	// Sanitize context name for use in a filename
	safeName := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '-'
		}
		return r
	}, ctx.Name)
	configPath := filepath.Join(kcsConfigDir, safeName)

	_, statErr := os.Stat(configPath)
	if os.IsNotExist(statErr) {
		full, err := clientcmd.LoadFromFile(sourceFile)
		if err != nil {
			return "", fmt.Errorf("failed to load kubeconfig: %w", err)
		}

		context, ok := full.Contexts[ctx.Name]
		if !ok {
			return "", fmt.Errorf("context %q not found in %s", ctx.Name, sourceFile)
		}

		minimal := clientcmdapi.NewConfig()
		minimal.CurrentContext = ctx.Name
		minimal.Contexts[ctx.Name] = context
		if cluster, ok := full.Clusters[context.Cluster]; ok {
			minimal.Clusters[context.Cluster] = cluster
		}
		if user, ok := full.AuthInfos[context.AuthInfo]; ok {
			minimal.AuthInfos[context.AuthInfo] = user
		}

		if err := clientcmd.WriteToFile(*minimal, configPath); err != nil {
			return "", fmt.Errorf("failed to write kubeconfig: %w", err)
		}

		if err := os.Chmod(configPath, 0400); err != nil {
			return "", fmt.Errorf("failed to set kubeconfig read-only: %w", err)
		}
	} else if statErr != nil {
		return "", fmt.Errorf("failed to stat kubeconfig: %w", statErr)
	} else if err := verifySessionKubeconfig(configPath, ctx.Name); err != nil {
		return "", fmt.Errorf("kubeconfig at %s has unexpected state: %w\n\nTo fix, run: rm %s", configPath, err, configPath)
	}

	sessionFile, err := writeSessionSymlink(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to write session symlink: %w", err)
	}

	return sessionFile, nil
}

// SessionPath returns the path to the session symlink for the current process's parent.
// If KCS_SESSION is set and not "1", it is used as the session ID instead of the parent PID.
func SessionPath() string {
	sessionID := os.Getenv("KCS_SESSION")
	if sessionID == "" || sessionID == "1" {
		sessionID = fmt.Sprintf("%d", os.Getppid())
	}
	return filepath.Join(xdgRuntimeDir(), "kcs", "sessions", sessionID)
}

func writeSessionSymlink(kubeconfigPath string) (string, error) {
	sessionFile := SessionPath()
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0700); err != nil {
		return "", err
	}
	_ = os.Remove(sessionFile)
	return sessionFile, os.Symlink(kubeconfigPath, sessionFile)
}

func xdgRuntimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	// Fallback: $TMPDIR is per-user and session-scoped on macOS;
	// on Linux XDG_RUNTIME_DIR is almost always set, but os.TempDir()
	// is a reasonable last resort there too.
	return os.TempDir()
}

func verifySessionKubeconfig(path, contextName string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("could not stat %s: %w", path, err)
	}

	if perm := info.Mode().Perm(); perm != 0400 {
		return fmt.Errorf("permissions are %04o, expected 0400", perm)
	}

	existing, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return fmt.Errorf("could not parse existing file: %w", err)
	}

	if existing.CurrentContext != contextName {
		return fmt.Errorf("current-context is %q, expected %q", existing.CurrentContext, contextName)
	}

	if len(existing.Contexts) != 1 {
		return fmt.Errorf("has %d contexts, expected 1", len(existing.Contexts))
	}

	return nil
}

// Switch updates the symlink and switches to the given context
func Switch(kubeDir string, ctx parser.ContextInfo) error {
	kcsConfigPath := filepath.Join(kubeDir, kcsConfigName)

	// Resolve the source file to its absolute path
	sourceFile, err := filepath.Abs(ctx.SourceFile)
	if err != nil {
		return fmt.Errorf("failed to resolve source file path: %w", err)
	}

	// Check current state of ~/.kube/kcs-config
	info, err := os.Lstat(kcsConfigPath)
	if err == nil {
		// File exists
		if info.Mode()&os.ModeSymlink != 0 {
			// It's a symlink - check if it already points to our target
			currentTarget, _ := os.Readlink(kcsConfigPath)
			if currentTarget != "" {
				absTarget := currentTarget
				if !filepath.IsAbs(currentTarget) {
					absTarget = filepath.Join(kubeDir, currentTarget)
				}
				if absTarget == sourceFile {
					// Already pointing to the right file, just switch context
					return switchContext(kubeDir, ctx.Name)
				}
			}
			// Remove existing symlink
			if err := os.Remove(kcsConfigPath); err != nil {
				return fmt.Errorf("failed to remove existing symlink: %w", err)
			}
		} else {
			// It's a regular file, remove it (shouldn't happen normally)
			if err := os.Remove(kcsConfigPath); err != nil {
				return fmt.Errorf("failed to remove existing kcs-config: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check kcs-config: %w", err)
	}

	// Create symlink to the source file
	if err := os.Symlink(sourceFile, kcsConfigPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return switchContext(kubeDir, ctx.Name)
}

func switchContext(kubeDir, name string) error {
	kcsConfigPath := filepath.Join(kubeDir, kcsConfigName)
	cmd := exec.Command("kubectl", "config", "use-context", name, "--kubeconfig", kcsConfigPath)
	// Suppress kubectl output - we show our own message
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to switch context: %w", err)
	}

	return nil
}

// GetCurrentContext returns the current context name and the kubeconfig file
// it was loaded from, using the standard KUBECONFIG loading rules.
func GetCurrentContext() (string, string, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := rules.Load()
	if err != nil {
		return "", "", fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	if config.CurrentContext == "" {
		return "", "", fmt.Errorf("no current context set")
	}

	// Find which KUBECONFIG file provides the current context.
	var source string
	for _, path := range rules.GetLoadingPrecedence() {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			continue
		}
		cfg, err := clientcmd.LoadFromFile(resolved)
		if err != nil {
			continue
		}
		if _, ok := cfg.Contexts[config.CurrentContext]; ok {
			source = filepath.Base(resolved)
			break
		}
	}

	return config.CurrentContext, source, nil
}

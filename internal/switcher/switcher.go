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

	// If file exists, verify its state and reuse it rather than overwriting
	if _, err := os.Stat(configPath); err != nil {
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
	} else if err := verifySessionKubeconfig(configPath, ctx.Name); err != nil {
		return "", fmt.Errorf("kubeconfig at %s has unexpected state: %w\n\nTo fix, run: rm %s", configPath, err, configPath)
	}

	sessionFile, err := writeSessionSymlink(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to write session symlink: %w", err)
	}

	return sessionFile, nil
}

func writeSessionSymlink(kubeconfigPath string) (string, error) {
	dir := filepath.Join(xdgRuntimeDir(), "kcs", "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	sessionFile := filepath.Join(dir, fmt.Sprintf("%d", os.Getppid()))
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

// GetCurrentContext returns the current context name and kubeconfig file
func GetCurrentContext(kubeDir string) (string, string, error) {
	kcsConfigPath := filepath.Join(kubeDir, kcsConfigName)

	// Check if kcs-config exists
	info, err := os.Lstat(kcsConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("kcs not initialized. Run 'kcs init' first")
		}
		return "", "", err
	}

	// Resolve symlink if needed
	actualPath := kcsConfigPath
	if info.Mode()&os.ModeSymlink != 0 {
		actualPath, err = os.Readlink(kcsConfigPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to read symlink: %w", err)
		}
		// Make absolute if relative
		if !filepath.IsAbs(actualPath) {
			actualPath = filepath.Join(kubeDir, actualPath)
		}
	}

	// Get current context using kubectl
	cmd := exec.Command("kubectl", "config", "current-context", "--kubeconfig", kcsConfigPath)
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get current context: %w", err)
	}

	contextName := string(output)
	// Trim newline
	if len(contextName) > 0 && contextName[len(contextName)-1] == '\n' {
		contextName = contextName[:len(contextName)-1]
	}

	return contextName, filepath.Base(actualPath), nil
}

// GetKcsConfigPath returns the path to kcs-config
func GetKcsConfigPath(kubeDir string) string {
	return filepath.Join(kubeDir, kcsConfigName)
}

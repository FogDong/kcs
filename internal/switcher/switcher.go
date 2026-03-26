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

// SwitchEnvVar creates a kubeconfig in /tmp for the given context and returns its path.
// The path is deterministic per context name so repeated switches reuse the same file.
func SwitchEnvVar(ctx parser.ContextInfo) (string, error) {
	sourceFile, err := filepath.Abs(ctx.SourceFile)
	if err != nil {
		return "", fmt.Errorf("failed to resolve source file path: %w", err)
	}

	// Sanitize context name for use in a filename
	safeName := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '-'
		}
		return r
	}, ctx.Name)
	tmpPath := filepath.Join(os.TempDir(), "kcs-"+safeName)

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

	// If file exists, check its state before overwriting
	if _, err := os.Stat(tmpPath); err == nil {
		if err := verifyEnvVarKubeconfig(tmpPath, ctx.Name); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: overwriting kubeconfig with unexpected state: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Warning: overwriting existing kubeconfig at %s\n", tmpPath)
		}
	}

	// Ensure writable before (re)writing — file may be read-only from a prior run
	_ = os.Chmod(tmpPath, 0600)

	if err := clientcmd.WriteToFile(*minimal, tmpPath); err != nil {
		return "", fmt.Errorf("failed to write temp kubeconfig: %w", err)
	}

	if err := os.Chmod(tmpPath, 0400); err != nil {
		return "", fmt.Errorf("failed to set kubeconfig read-only: %w", err)
	}

	return tmpPath, nil
}

func verifyEnvVarKubeconfig(path, contextName string) error {
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

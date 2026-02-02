package switcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/FogDong/kcs/internal/parser"
)

const kcsConfigName = "kcs-config"

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

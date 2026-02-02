package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// Scan finds all kubeconfig files in the given directory
func Scan(kubeDir string) ([]string, error) {
	entries, err := os.ReadDir(kubeDir)
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(kubeDir, "config")
	var files []string
	seenTargets := make(map[string]bool)

	for _, entry := range entries {
		// Skip directories
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip hidden files (except we want to check them for kubeconfig)
		// Skip known non-kubeconfig files
		if shouldSkip(name) {
			continue
		}

		fullPath := filepath.Join(kubeDir, name)

		// Check if it's a symlink
		linfo, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}

		// If this is the config file and it's a symlink, skip it
		// We'll include the target file instead
		if fullPath == configPath && linfo.Mode()&os.ModeSymlink != 0 {
			continue
		}

		// Resolve symlinks to get actual file
		resolvedPath := fullPath
		if linfo.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(fullPath)
			if err != nil {
				continue
			}
			resolvedPath = resolved
		}

		// Skip if we've already seen this target (avoid duplicates from symlinks)
		if seenTargets[resolvedPath] {
			continue
		}

		// Check if it's actually a file (not directory)
		info, err := os.Stat(resolvedPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			continue
		}

		// Skip files that are too large (likely not kubeconfig)
		if info.Size() > 10*1024*1024 { // 10MB limit
			continue
		}

		// Try to validate it's a kubeconfig by checking content
		if isLikelyKubeconfig(resolvedPath) {
			files = append(files, fullPath) // Use original path for display
			seenTargets[resolvedPath] = true
		}
	}

	return files, nil
}

// shouldSkip returns true if the file should be skipped based on name
func shouldSkip(name string) bool {
	// Skip cache and cert files
	skipPrefixes := []string{
		"cache",
		"http-cache",
	}

	skipSuffixes := []string{
		".crt",
		".key",
		".pem",
		".pub",
		".lock",
		".tmp",
		".bak",
		".backup",
	}

	skipExact := []string{
		".kcs-active",
		"kcs-config",
	}

	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	for _, suffix := range skipSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	for _, exact := range skipExact {
		if name == exact {
			return true
		}
	}

	return false
}

// isLikelyKubeconfig does a quick check if the file looks like a kubeconfig
func isLikelyKubeconfig(path string) bool {
	// Read first 1KB to check for kubeconfig markers
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return false
	}

	content := string(buf[:n])

	// Check for kubeconfig markers (YAML format)
	yamlMarkers := []string{
		"apiVersion:",
		"kind: Config",
		"clusters:",
		"contexts:",
		"users:",
	}

	// Check for kubeconfig markers (JSON format)
	jsonMarkers := []string{
		`"apiVersion"`,
		`"kind"`,
		`"clusters"`,
		`"contexts"`,
		`"users"`,
	}

	matchCount := 0
	for _, marker := range yamlMarkers {
		if strings.Contains(content, marker) {
			matchCount++
		}
	}

	// If not enough YAML markers, check JSON markers
	if matchCount < 2 {
		matchCount = 0
		for _, marker := range jsonMarkers {
			if strings.Contains(content, marker) {
				matchCount++
			}
		}
	}

	// Need at least 2 markers to consider it a kubeconfig
	return matchCount >= 2
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/FogDong/kcs/internal/parser"
	"github.com/FogDong/kcs/internal/scanner"
	"github.com/FogDong/kcs/internal/selector"
	"github.com/FogDong/kcs/internal/switcher"
)

var version = "dev"

var (
	listFlag    bool
	currentFlag bool
	dirFlag     string
	initSession bool
)

var rootCmd = &cobra.Command{
	Use:     "kcs [search]",
	Short:   "Kubernetes Config Switcher",
	Long:    `kcs helps you manage multiple kubeconfig files and contexts in ~/.kube/`,
	Args:    cobra.MaximumNArgs(1),
	Version: version,
	Run:     run,
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Output KUBECONFIG export for use with eval",
	Long:  `Outputs a shell export command to set KUBECONFIG. Use with: eval $(kcs init)`,
	Run:   runInit,
}

func init() {
	rootCmd.Flags().BoolVarP(&listFlag, "list", "l", false, "List all contexts without interactive selection")
	rootCmd.Flags().BoolVarP(&currentFlag, "current", "c", false, "Show current context")
	rootCmd.Flags().StringVarP(&dirFlag, "dir", "d", "", "Custom kubeconfig directory (default: ~/.kube)")
	initCmd.Flags().BoolVarP(&initSession, "session", "s", false, "Output session-scoped KUBECONFIG export")
	rootCmd.AddCommand(initCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func sessionModeEnabled() bool {
	return os.Getenv("KCS_SESSION") != ""
}

type configStatus int

const (
	configOK configStatus = iota
	configUnset
	configNotKCS
	configWrongSession   // KCS_SESSION set, KUBECONFIG has a different session path
	configStaticInSession // KCS_SESSION set, KUBECONFIG has the static kcs-config path
)

func checkConfig(kubeDir string) configStatus {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		return configUnset
	}
	paths := strings.Split(kubeconfig, ":")
	contains := func(s string) bool {
		for _, p := range paths {
			if p == s {
				return true
			}
		}
		return false
	}

	staticPath := kubeDir + "/kcs-config"
	if sessionModeEnabled() {
		if contains(switcher.SessionPath()) {
			return configOK
		}
		if contains(staticPath) {
			return configStaticInSession
		}
		return configWrongSession
	}

	if contains(staticPath) {
		return configOK
	}
	return configNotKCS
}

func printSetupHelp(kubeDir string) {
	switch checkConfig(kubeDir) {
	case configUnset:
		fmt.Fprintln(os.Stderr, "KUBECONFIG is not set. Add to your shell configuration (~/.zshrc or ~/.bashrc):")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  eval $(kcs init)            # all shells share one context")
		fmt.Fprintln(os.Stderr, "  eval $(kcs init --session)  # per-shell session (also set KCS_SESSION=1)")
	case configNotKCS:
		fmt.Fprintln(os.Stderr, "KUBECONFIG is not managed by kcs. Add to your shell configuration (~/.zshrc or ~/.bashrc):")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  eval $(kcs init)            # all shells share one context")
		fmt.Fprintln(os.Stderr, "  eval $(kcs init --session)  # per-shell session (also set KCS_SESSION=1)")
	case configStaticInSession:
		fmt.Fprintf(os.Stderr, "Session mode is enabled (KCS_SESSION=%s) but KUBECONFIG points to the shared config.\n", os.Getenv("KCS_SESSION"))
		fmt.Fprintln(os.Stderr, "Run: eval $(kcs init --session)")
	case configWrongSession:
		fmt.Fprintf(os.Stderr, "Session mode is enabled (KCS_SESSION=%s) but KUBECONFIG does not point to this session.\n", os.Getenv("KCS_SESSION"))
		fmt.Fprintln(os.Stderr, "Run: eval $(kcs init --session)")
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "With mise:")
	fmt.Fprintln(os.Stderr, "  [env]")
	fmt.Fprintln(os.Stderr, "  _.kcs = {}")
}

func run(cmd *cobra.Command, args []string) {
	// Determine kubeconfig directory
	kubeDir := dirFlag
	if kubeDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
			os.Exit(1)
		}
		kubeDir = homeDir + "/.kube"
	}

	// Handle --current flag
	if currentFlag {
		showCurrentContext(kubeDir)
		return
	}

	// Scan for kubeconfig files
	files, err := scanner.Scan(kubeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning kubeconfig files: %v\n", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "No kubeconfig files found in %s\n", kubeDir)
		os.Exit(1)
	}

	// Parse all kubeconfig files
	var allContexts []parser.ContextInfo
	for _, file := range files {
		contexts, err := parser.Parse(file)
		if err != nil {
			// Skip invalid files with warning
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", file, err)
			continue
		}
		allContexts = append(allContexts, contexts...)
	}

	if len(allContexts) == 0 {
		fmt.Fprintf(os.Stderr, "No contexts found in any kubeconfig file\n")
		os.Exit(1)
	}

	// Get search query if provided
	var searchQuery string
	if len(args) > 0 {
		searchQuery = args[0]
	}

	// Handle --list flag
	if listFlag {
		listContexts(allContexts, searchQuery)
		return
	}

	// Require KUBECONFIG to be configured before switching
	if checkConfig(kubeDir) != configOK {
		printSetupHelp(kubeDir)
		os.Exit(1)
	}

	// Interactive selection
	selected, err := selector.Select(allContexts, searchQuery)
	if err != nil {
		if err == selector.ErrUserCancelled {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Switch to selected context
	if sessionModeEnabled() {
		if _, err := switcher.SwitchSession(selected); err != nil {
			fmt.Fprintf(os.Stderr, "Error switching context: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "✓ Switched to %s\n", selected.Cluster)
		return
	}

	if err := switcher.Switch(kubeDir, selected); err != nil {
		fmt.Fprintf(os.Stderr, "Error switching context: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "✓ Switched to %s\n", selected.Cluster)
}

func showCurrentContext(kubeDir string) {
	ctx, file, err := switcher.GetCurrentContext(kubeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%s (kubeconfig: %s)\n", ctx, file)
}

func runInit(cmd *cobra.Command, args []string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	staticKCSPath := filepath.Join(homeDir, ".kube", "kcs-config")

	if sessionModeEnabled() || initSession {
		if !sessionModeEnabled() {
			fmt.Printf("export KCS_SESSION='%d'\n", os.Getppid())
		}
		sessionPath := switcher.SessionPath()
		sessionsDir := filepath.Dir(sessionPath)
		fallback := kubeconfigFallback(staticKCSPath, sessionsDir, homeDir)
		kubeconfig := sessionPath + ":" + fallback
		fmt.Printf("export KUBECONFIG='%s'\n", strings.ReplaceAll(kubeconfig, "'", "'\\''"))
		return
	}

	fmt.Printf("export KUBECONFIG='%s'\n", strings.ReplaceAll(staticKCSPath, "'", "'\\''"))
}

// kubeconfigFallback returns the user's existing KUBECONFIG with kcs-managed paths
// removed, or ~/.kube/config if nothing non-kcs remains.
func kubeconfigFallback(staticKCSPath, sessionsDir, homeDir string) string {
	existing := os.Getenv("KUBECONFIG")
	defaultConfig := filepath.Join(homeDir, ".kube", "config")

	if existing == "" {
		return defaultConfig
	}

	var kept []string
	for _, p := range strings.Split(existing, ":") {
		if p == "" || p == staticKCSPath {
			continue
		}
		if strings.HasPrefix(p, sessionsDir+string(filepath.Separator)) || p == sessionsDir {
			continue
		}
		kept = append(kept, p)
	}

	if len(kept) == 0 {
		return defaultConfig
	}
	return strings.Join(kept, ":")
}

func listContexts(contexts []parser.ContextInfo, searchQuery string) {
	filtered := contexts
	if searchQuery != "" {
		filtered = selector.Filter(contexts, searchQuery)
	}

	if len(filtered) == 0 {
		fmt.Println("No contexts match the query")
		return
	}

	for _, ctx := range filtered {
		ns := ctx.Namespace
		if ns == "" {
			ns = "default"
		}
		fmt.Printf("[%s] %s (ns: %s)\n",
			ctx.SourceFileName, ctx.Cluster, ns)
	}
}

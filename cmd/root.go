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
	listFlag       bool
	currentFlag    bool
	dirFlag        string
	initSession    bool
	persistentFlag bool
	sessionFlag    bool
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
	rootCmd.Flags().BoolVarP(&persistentFlag, "persistent", "p", false, "Update shared kcs-config (overrides KCS_DEFAULT_SESSION)")
	rootCmd.Flags().BoolVarP(&sessionFlag, "session", "s", false, "Update session config (overrides default persistent behavior)")
	rootCmd.MarkFlagsMutuallyExclusive("persistent", "session")
	initCmd.Flags().BoolVarP(&initSession, "session", "s", false, "Also export KCS_DEFAULT_SESSION to make session switching the default")
	rootCmd.AddCommand(initCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// sessionIDSet reports whether KCS_SESSION is set, meaning the session path is pinned.
func sessionIDSet() bool {
	return os.Getenv("KCS_SESSION") != ""
}

// defaultSessionEnabled reports whether session switching is the default behavior.
func defaultSessionEnabled() bool {
	return os.Getenv("KCS_DEFAULT_SESSION") != ""
}

type configStatus int

const (
	configOK configStatus = iota
	configUnset
	configNotKCS
	configMissingSession // has kcs-config but not the session path
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
	hasStatic := contains(staticPath)
	hasSession := contains(switcher.SessionPath())

	if hasStatic && hasSession {
		return configOK
	}
	if hasStatic {
		return configMissingSession
	}
	return configNotKCS
}

func printSetupHelp(kubeDir string) {
	switch checkConfig(kubeDir) {
	case configUnset:
		fmt.Fprintln(os.Stderr, "KUBECONFIG is not set. Add to your shell configuration (~/.zshrc or ~/.bashrc):")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  eval $(kcs init)            # persistent switching by default")
		fmt.Fprintln(os.Stderr, "  eval $(kcs init --session)  # session switching by default")
	case configNotKCS:
		fmt.Fprintln(os.Stderr, "KUBECONFIG is not managed by kcs. Add to your shell configuration (~/.zshrc or ~/.bashrc):")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  eval $(kcs init)            # persistent switching by default")
		fmt.Fprintln(os.Stderr, "  eval $(kcs init --session)  # session switching by default")
	case configMissingSession:
		fmt.Fprintln(os.Stderr, "KUBECONFIG is missing the session path. Re-run:")
		fmt.Fprintln(os.Stderr, "  eval $(kcs init)")
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

	// Determine whether to switch session or persistent config.
	// Explicit flags take priority; otherwise fall back to KCS_DEFAULT_SESSION.
	useSession := !persistentFlag && (sessionFlag || defaultSessionEnabled())

	if useSession {
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

	// Pin KCS_SESSION to the current shell's PID if not already set,
	// so the session path is stable for the lifetime of this shell.
	if !sessionIDSet() {
		fmt.Printf("export KCS_SESSION='%d'\n", os.Getppid())
	}

	if initSession {
		fmt.Printf("export KCS_DEFAULT_SESSION='1'\n")
	}

	sessionPath := switcher.SessionPath()
	kubeconfig := sessionPath + ":" + staticKCSPath
	fmt.Printf("export KUBECONFIG='%s'\n", strings.ReplaceAll(kubeconfig, "'", "'\\''"))
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

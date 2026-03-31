package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/FogDong/kcs/internal/parser"
	"github.com/FogDong/kcs/internal/scanner"
	"github.com/FogDong/kcs/internal/selector"
	"github.com/FogDong/kcs/internal/switcher"
)

var (
	listFlag    bool
	currentFlag bool
	dirFlag     string
	initSession bool
)

var rootCmd = &cobra.Command{
	Use:   "kcs [search]",
	Short: "Kubernetes Config Switcher",
	Long:  `kcs helps you manage multiple kubeconfig files and contexts in ~/.kube/`,
	Args:  cobra.MaximumNArgs(1),
	Run:   run,
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

func isKCSConfigured(kubeDir string) bool {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		return false
	}
	if sessionModeEnabled() {
		return kubeconfig == switcher.SessionPath()
	}
	return kubeconfig == kubeDir+"/kcs-config"
}

func printSetupHelp() {
	fmt.Fprintln(os.Stderr, "KUBECONFIG is not configured for kcs.")
	fmt.Fprintln(os.Stderr, "Add to your shell configuration (~/.zshrc or ~/.bashrc):")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "  eval $(kcs init)            # all shells share one context")
	fmt.Fprintln(os.Stderr, "  eval $(kcs init --session)  # each shell has its own context")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "For session mode, also add: export KCS_SESSION=1")
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
	if !isKCSConfigured(kubeDir) {
		printSetupHelp()
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
	if sessionModeEnabled() || initSession {
		if !sessionModeEnabled() {
			fmt.Printf("export KCS_SESSION='%d'\n", os.Getppid())
		}
		sessionPath := switcher.SessionPath()
		fmt.Printf("export KUBECONFIG='%s'\n", strings.ReplaceAll(sessionPath, "'", "'\\''"))
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("export KUBECONFIG='%s/.kube/kcs-config'\n", homeDir)
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

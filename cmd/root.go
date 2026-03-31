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

var (
	listFlag    bool
	currentFlag bool
	dirFlag     string
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
	Short: "Initialize kcs and show shell configuration",
	Long:  `Shows the command to add to your shell configuration (.zshrc or .bashrc) to use kcs.`,
	Run:   runInit,
}

func init() {
	rootCmd.Flags().BoolVarP(&listFlag, "list", "l", false, "List all contexts without interactive selection")
	rootCmd.Flags().BoolVarP(&currentFlag, "current", "c", false, "Show current context")
	rootCmd.Flags().StringVarP(&dirFlag, "dir", "d", "", "Custom kubeconfig directory (default: ~/.kube)")
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
		sessionPath, err := switcher.SwitchSession(selected)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error switching context: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "✓ Switched to %s\n", selected.Cluster)
		// Build KUBECONFIG with session path first, then existing non-session entries
		// so that tools still work if the session file is missing (e.g. after reboot).
		sessionsDir := filepath.Dir(sessionPath)
		parts := []string{sessionPath}
		for _, p := range filepath.SplitList(os.Getenv("KUBECONFIG")) {
			if p != "" && !strings.HasPrefix(p, sessionsDir+string(filepath.Separator)) {
				parts = append(parts, p)
			}
		}
		if len(parts) == 1 {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				parts = append(parts, filepath.Join(homeDir, ".kube", "config"))
			}
		}
		kubeconfig := strings.Join(parts, string(filepath.ListSeparator))
		fmt.Printf("export KUBECONFIG='%s'\n", strings.ReplaceAll(kubeconfig, "'", "'\\''"))
		return
	}

	if err := switcher.Switch(kubeDir, selected); err != nil {
		fmt.Fprintf(os.Stderr, "Error switching context: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Switched to %s\n", selected.Cluster)
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
	if sessionModeEnabled() {
		fmt.Println("Add to your shell configuration (~/.zshrc or ~/.bashrc):")
		fmt.Println()
		fmt.Println(`  export KCS_SESSION=1`)
		fmt.Println(`  eval "$(kcs)"  # initializes KUBECONFIG to your session symlink`)
		fmt.Println()
		fmt.Println("After initialization, kcs updates the symlink directly—no eval needed.")
		fmt.Println()
		fmt.Println("With mise, add to your mise.toml instead:")
		fmt.Println(`  [env]`)
		fmt.Println(`  _.kcs = {}`)
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	kcsConfigPath := homeDir + "/.kube/kcs-config"
	exportCmd := fmt.Sprintf("export KUBECONFIG=%s", kcsConfigPath)

	fmt.Println("Add the following line to your shell configuration file:")
	fmt.Println()
	fmt.Println("For zsh (~/.zshrc):")
	fmt.Printf("  echo '%s' >> ~/.zshrc\n", exportCmd)
	fmt.Println()
	fmt.Println("For bash (~/.bashrc):")
	fmt.Printf("  echo '%s' >> ~/.bashrc\n", exportCmd)
	fmt.Println()
	fmt.Println("Or manually add this line:")
	fmt.Printf("  %s\n", exportCmd)
	fmt.Println()
	fmt.Println("Then reload your shell or run:")
	fmt.Printf("  source ~/.zshrc  # or source ~/.bashrc\n")
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

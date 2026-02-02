package parser

import (
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"
)

// ContextInfo represents a Kubernetes context with its source file
type ContextInfo struct {
	Name           string // Context name
	Cluster        string // Cluster name
	User           string // User/AuthInfo name
	Namespace      string // Default namespace (if set)
	SourceFile     string // Full path to the kubeconfig file
	SourceFileName string // Just the filename for display
	IsCurrent      bool   // Whether this is the current context in its file
}

// Parse reads a kubeconfig file and returns all contexts
func Parse(path string) ([]ContextInfo, error) {
	config, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return nil, err
	}

	var contexts []ContextInfo
	fileName := filepath.Base(path)

	for name, ctx := range config.Contexts {
		info := ContextInfo{
			Name:           name,
			Cluster:        ctx.Cluster,
			User:           ctx.AuthInfo,
			Namespace:      ctx.Namespace,
			SourceFile:     path,
			SourceFileName: fileName,
			IsCurrent:      name == config.CurrentContext,
		}
		contexts = append(contexts, info)
	}

	return contexts, nil
}

package selector

import (
	"errors"
	"fmt"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/sahilm/fuzzy"
	"github.com/FogDong/kcs/internal/parser"
)

var ErrUserCancelled = errors.New("user cancelled")

// contextList implements fuzzy.Source for fuzzy matching
type contextList []parser.ContextInfo

func (c contextList) String(i int) string {
	ctx := c[i]
	return fmt.Sprintf("%s %s %s", ctx.Cluster, ctx.SourceFileName, ctx.Namespace)
}

func (c contextList) Len() int {
	return len(c)
}

// Filter returns contexts matching the search query using fuzzy matching
func Filter(contexts []parser.ContextInfo, query string) []parser.ContextInfo {
	if query == "" {
		return contexts
	}

	matches := fuzzy.FindFrom(query, contextList(contexts))
	if len(matches) == 0 {
		return nil
	}

	threshold := matches[0].Score / 2
	var result []parser.ContextInfo
	for _, match := range matches {
		if match.Score >= threshold {
			result = append(result, contexts[match.Index])
		}
	}
	return result
}

// Select shows an interactive selection prompt and returns the selected context
func Select(contexts []parser.ContextInfo, searchQuery string) (parser.ContextInfo, error) {
	// Pre-filter if search query provided
	filtered := Filter(contexts, searchQuery)
	if len(filtered) == 0 {
		return parser.ContextInfo{}, errors.New("no contexts match the query")
	}

	// If only one match, select it directly
	if len(filtered) == 1 {
		return filtered[0], nil
	}

	// If the query exactly matches a context name or cluster name, select it directly
	if searchQuery != "" {
		var exactMatches []parser.ContextInfo
		for _, ctx := range filtered {
			if ctx.Name == searchQuery || ctx.Cluster == searchQuery {
				exactMatches = append(exactMatches, ctx)
			}
		}
		if len(exactMatches) == 1 {
			return exactMatches[0], nil
		}
	}

	// Create display items
	items := make([]string, len(filtered))
	for i, ctx := range filtered {
		ns := ctx.Namespace
		if ns == "" {
			ns = "default"
		}
		items[i] = fmt.Sprintf("[%s] %s (ns: %s)",
			ctx.SourceFileName, ctx.Cluster, ns)
	}

	// Setup promptui
	searcher := func(input string, index int) bool {
		ctx := filtered[index]
		searchStr := strings.ToLower(fmt.Sprintf("%s %s %s",
			ctx.Cluster, ctx.SourceFileName, ctx.Namespace))
		input = strings.ToLower(input)
		return strings.Contains(searchStr, input)
	}

	prompt := promptui.Select{
		Label:             "Select a Kubernetes context",
		Items:             items,
		Size:              15,
		Searcher:          searcher,
		StartInSearchMode: len(filtered) > 10,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt || err == promptui.ErrEOF {
			return parser.ContextInfo{}, ErrUserCancelled
		}
		return parser.ContextInfo{}, err
	}

	return filtered[idx], nil
}

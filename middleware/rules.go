package middleware

import (
	"sort"
	"strings"
)

// prefixRule is a wildcard rule "prefix/*" compiled for matching.
type prefixRule struct {
	prefix      string
	replacement string
}

// compilePrefixRules splits rules into exact rules and wildcard rules ordered
// longest prefix first, so overlapping rules resolve deterministically.
func compilePrefixRules(rules map[string]string) (map[string]string, []prefixRule) {
	exact := make(map[string]string)
	var wildcards []prefixRule
	for pattern, replacement := range rules {
		if strings.HasSuffix(pattern, "/*") {
			wildcards = append(wildcards, prefixRule{prefix: pattern[:len(pattern)-2], replacement: replacement})
		} else {
			exact[pattern] = replacement
		}
	}
	sort.Slice(wildcards, func(i, j int) bool {
		if len(wildcards[i].prefix) != len(wildcards[j].prefix) {
			return len(wildcards[i].prefix) > len(wildcards[j].prefix)
		}
		return wildcards[i].prefix < wildcards[j].prefix
	})
	return exact, wildcards
}

// matchPrefixRule returns the first rule whose prefix matches path on a
// segment boundary: "/old" matches "/old" and "/old/...", never "/older".
func matchPrefixRule(rules []prefixRule, path string) (prefixRule, bool) {
	for _, r := range rules {
		if path == r.prefix || strings.HasPrefix(path, r.prefix+"/") || r.prefix == "" {
			return r, true
		}
	}
	return prefixRule{}, false
}

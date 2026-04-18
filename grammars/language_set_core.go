//go:build grammar_set_core

package grammars

var coreLanguageSet = languageSetFromNames(core100LanguageNames)

func languageSetFromNames(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}

func compileTimeLanguageEnabled(name string) bool {
	_, ok := coreLanguageSet[name]
	return ok
}

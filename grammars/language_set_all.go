//go:build !grammar_set_core

package grammars

func compileTimeLanguageEnabled(name string) bool {
	return copyleftLanguageAllowed(name)
}

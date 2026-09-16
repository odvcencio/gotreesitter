//go:build grammar_subset && grammar_subset_authzed

package grammarruntime

func init() {
	registerTokenSourceFactory("authzed", NewAuthzedTokenSourceOrEOF)
}

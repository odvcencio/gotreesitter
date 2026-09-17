//go:build grammar_subset && grammar_subset_json

package grammarruntime

func init() {
	registerTokenSourceFactory("json", NewJSONTokenSourceOrEOF)
}

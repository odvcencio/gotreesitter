//go:build grammar_subset && (grammar_subset_c || grammar_subset_cpp)

package grammarruntime

func init() {
	registerTokenSourceFactory("c", NewCTokenSourceOrEOF)
	registerTokenSourceFactory("cpp", NewCTokenSourceOrEOF)
}

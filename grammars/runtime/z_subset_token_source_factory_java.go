//go:build grammar_subset && grammar_subset_java

package grammarruntime

func init() {
	registerTokenSourceFactory("java", NewJavaTokenSourceOrEOF)
}

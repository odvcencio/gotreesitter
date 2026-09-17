//go:build !grammar_subset || grammar_subset_scala

package grammarruntime

// RegisterScalaSupport registers the scanner support for scala.
func RegisterScalaSupport() {
	RegisterExternalScanner("scala", ScalaExternalScanner{})
}

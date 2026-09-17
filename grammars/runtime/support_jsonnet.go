//go:build !grammar_subset || grammar_subset_jsonnet

package grammarruntime

// RegisterJsonnetSupport registers the scanner support for jsonnet.
func RegisterJsonnetSupport() {
	RegisterExternalScanner("jsonnet", JsonnetExternalScanner{})
}

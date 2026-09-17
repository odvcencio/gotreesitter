//go:build !grammar_subset || grammar_subset_dockerfile

package grammarruntime

// RegisterDockerfileSupport registers the scanner support for dockerfile.
func RegisterDockerfileSupport() {
	RegisterExternalScanner("dockerfile", DockerfileExternalScanner{})
}

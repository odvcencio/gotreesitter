//go:build !grammar_subset || grammar_subset_yaml

package grammarruntime

// RegisterYamlSupport registers the scanner support for yaml.
func RegisterYamlSupport() {
	RegisterExternalScanner("yaml", YamlExternalScanner{})
	RegisterExternalLexStates("yaml", yamlExternalLexStates)
}

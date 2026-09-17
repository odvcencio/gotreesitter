//go:build !grammar_subset || grammar_subset_properties

package grammarruntime

// RegisterPropertiesSupport registers the scanner support for properties.
func RegisterPropertiesSupport() {
	RegisterExternalScanner("properties", PropertiesExternalScanner{})
}

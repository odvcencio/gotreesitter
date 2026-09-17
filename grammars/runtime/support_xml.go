//go:build !grammar_subset || grammar_subset_xml

package grammarruntime

// RegisterXmlSupport registers the scanner support for xml.
func RegisterXmlSupport() {
	RegisterExternalScanner("xml", XMLExternalScanner{})
}

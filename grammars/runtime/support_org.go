//go:build !grammar_subset || grammar_subset_org

package grammarruntime

// RegisterOrgSupport registers the scanner support for org.
func RegisterOrgSupport() {
	RegisterExternalScanner("org", OrgExternalScanner{})
}

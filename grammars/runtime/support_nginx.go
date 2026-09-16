//go:build !grammar_subset || grammar_subset_nginx

package grammarruntime

// RegisterNginxSupport registers the scanner support for nginx.
func RegisterNginxSupport() {
	RegisterExternalScanner("nginx", NginxExternalScanner{})
}

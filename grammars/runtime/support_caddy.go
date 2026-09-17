//go:build !grammar_subset || grammar_subset_caddy

package grammarruntime

// RegisterCaddySupport registers the scanner support for caddy.
func RegisterCaddySupport() {
	RegisterExternalScanner("caddy", CaddyExternalScanner{})
}

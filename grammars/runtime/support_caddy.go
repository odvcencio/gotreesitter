//go:build (!grammar_subset || grammar_subset_caddy) && !gotreesitter_no_copyleft

package grammarruntime

// RegisterCaddySupport registers the scanner support for caddy. See
// caddy_no_copyleft_stub.go for the gotreesitter_no_copyleft build, which
// keeps this function's signature but drops its GPL-3.0-derived body.
func RegisterCaddySupport() {
	RegisterExternalScanner("caddy", CaddyExternalScanner{})
}

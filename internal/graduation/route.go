// Package graduation checks external measurement receipts and default routing.
package graduation

// graduatedGrammars is production routing configuration, not a measurement
// receipt. A nonempty list requires an external matrix to pass VerifyDefaults.
// No language currently satisfies every graduation requirement.
var graduatedGrammars = []string{}

// Allowlist returns an independent copy for the admission switch. Explicit
// parser and process overrides retain their existing precedence.
func Allowlist() map[string]bool {
	result := make(map[string]bool, len(graduatedGrammars))
	for _, grammar := range graduatedGrammars {
		result[grammar] = true
	}
	return result
}

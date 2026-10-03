package incr

// FreshVerifiedMergeOverride builds a custom span-changing frontier under
// the fresh verifier's merge schedule. The result still requires verification;
// broader reuse alternatives would only add work to that discarded attempt.
// A negative override selects the existing exact-cap mechanism.
func FreshVerifiedMergeOverride(override, freshCap int, customStream, spanChanging bool) int {
	if customStream && spanChanging {
		return -freshCap
	}
	return override
}

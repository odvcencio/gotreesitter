package gotreesitter

import "testing"

func TestIncrementalFreshVerifierAdmissionObservability(t *testing.T) {
	for _, mode := range []string{"plain", "logger", "trace", "ambiguity"} {
		t.Run(mode, func(t *testing.T) {
			p := NewParser(&Language{Name: "fresh_verifier"})
			p.SetAdmissionCandidateRoute(true)
			switch mode {
			case "logger":
				p.SetLogger(func(ParserLogType, string) {})
			case "trace":
				p.SetGLRTrace(true)
			case "ambiguity":
				p.SetAmbiguityProfile(&AmbiguityProfile{})
			}
			want := p.admissionCandidateFullParseEligible(nil, true)
			verifier := p.newIncrementalFreshVerifier()
			if got := verifier.admissionCandidateFullParseEligible(nil, true); got != want {
				t.Fatalf("fresh candidate eligibility: caller=%t verifier=%t", want, got)
			}
			if verifier.logger != nil || verifier.glrTrace || verifier.ambiguityProfile != nil {
				t.Fatal("hidden verifier inherited caller observers")
			}
		})
	}
}

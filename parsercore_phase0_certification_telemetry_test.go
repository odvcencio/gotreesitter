//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCompactCertificationTelemetryResetsWithParserReuse(t *testing.T) {
	parser := gts.NewParser(grammars.JsonLanguage())
	parser.SetAdmissionCandidateRoute(true)
	parse := func(source string) gts.ParseRuntime {
		t.Helper()
		before, fallbackBefore := gts.AdmissionCandidateCounters()
		tree, err := parser.Parse([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Release()
		routed, fallback := gts.AdmissionCandidateCounters()
		if routed != before+1 || fallback != fallbackBefore {
			t.Fatalf("JSON compact route declined: %s", gts.AdmissionCandidateLastFallbackReason())
		}
		return tree.ParseRuntime()
	}
	if runtime := parse(`{"a":1}`); runtime.CompactPeakHeaders != 0 || runtime.CompactPeakDerivations != 0 || runtime.CompactMultiHeaderTokens != 0 {
		t.Fatalf("disabled telemetry was populated: %+v", runtime)
	}
	parser.SetCompactCertificationTelemetry(true)
	first := parse(`{"a":[1,2,3],"b":true}`)
	if first.CompactPeakHeaders == 0 || first.CompactPeakDerivations < first.CompactPeakHeaders {
		t.Fatalf("invalid first compact peaks: %+v", first)
	}
	second := parse(`{"a":1}`)
	if second.CompactPeakHeaders == 0 || second.CompactPeakDerivations < second.CompactPeakHeaders {
		t.Fatalf("invalid second compact peaks: %+v", second)
	}
	recovery, err := parser.Parse([]byte("{\"a\":1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime := recovery.ParseRuntime(); runtime.CompactPeakHeaders != 0 || runtime.CompactPeakDerivations != 0 || runtime.CompactMultiHeaderTokens != 0 {
		t.Fatalf("fallback retained compact peaks: %+v", runtime)
	}
	recovery.Release()
	parser.SetCompactCertificationTelemetry(false)
	if runtime := parse(`{"a":1}`); runtime.CompactPeakHeaders != 0 || runtime.CompactPeakDerivations != 0 || runtime.CompactMultiHeaderTokens != 0 {
		t.Fatalf("disabled telemetry retained peaks: %+v", runtime)
	}
}

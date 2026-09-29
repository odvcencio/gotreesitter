//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// TestCompactAuditShrink reduces a real-file decline without changing its
// mechanism or production error class. The trial budget bounds diagnostic work;
// the resulting witness is reduced, not a claim of global minimality.
func TestCompactAuditShrink(t *testing.T) {
	path := os.Getenv("GTS_COMPACT_AUDIT_SHRINK_SOURCE")
	if path == "" {
		t.Skip("set GTS_COMPACT_AUDIT_SHRINK_SOURCE and GTS_COMPACT_AUDIT_LANGUAGE")
	}
	name := os.Getenv("GTS_COMPACT_AUDIT_LANGUAGE")
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatalf("unknown grammar %q", name)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	original := runAdmissionScorecardSource(*entry, source)
	if original.status != scorecardFallback {
		t.Fatalf("source route=%s, want FALLBACK: %s", original.status, original.detail)
	}
	mechanism := regexp.MustCompile(`\[mechanism=([^\]]+)\]`)
	numbers := regexp.MustCompile(`[0-9]+`)
	classify := func(detail string) string {
		if match := mechanism.FindStringSubmatch(detail); len(match) > 1 {
			_, reason, _ := strings.Cut(detail, "]: ")
			reason, _, _ = strings.Cut(reason, " [c-mechanism=")
			return match[1] + ":" + numbers.ReplaceAllString(reason, "#")
		}
		if strings.Contains(detail, "live-link cap exceeded") {
			return "live-link-cap"
		}
		if strings.Contains(detail, "accepted compact root is incomplete or erroneous") {
			return "accepted-root-coverage"
		}
		if strings.Contains(detail, "multiple scheduler owners") {
			return "scheduler-owner"
		}
		return detail
	}
	want := classify(original.detail)
	trials := 0
	keep := func(candidate []byte) bool {
		if len(candidate) == 0 || trials >= 256 {
			return false
		}
		trials++
		row := runAdmissionScorecardSource(*entry, candidate)
		return row.status == scorecardFallback && row.productionHasError == original.productionHasError && classify(row.detail) == want
	}
	parts := strings.SplitAfter(string(source), "\n")
	for width := len(parts) / 2; width >= 1 && trials < 256; {
		changed := false
		for start := 0; start+width <= len(parts) && trials < 256; start++ {
			candidate := append(append([]string(nil), parts[:start]...), parts[start+width:]...)
			if keep([]byte(strings.Join(candidate, ""))) {
				parts = candidate
				changed = true
				break
			}
		}
		if !changed {
			width /= 2
		}
	}
	source = []byte(strings.Join(parts, ""))
	if len(source) <= 1024 {
		for width := len(source) / 2; width >= 1 && trials < 256; {
			changed := false
			for start := 0; start+width <= len(source) && trials < 256; start++ {
				candidate := append(append([]byte(nil), source[:start]...), source[start+width:]...)
				if keep(candidate) {
					source = candidate
					changed = true
					break
				}
			}
			if !changed {
				width /= 2
			}
		}
	}
	row := runAdmissionScorecardSource(*entry, source)
	t.Logf("WITNESS grammar=%s bytes=%d trials=%d production_error=%t detail=%q source=%q", name, len(source), trials, row.productionHasError, row.detail, source)
	if output := os.Getenv("GTS_COMPACT_AUDIT_SHRINK_OUTPUT"); output != "" {
		if err := os.WriteFile(output, source, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompactAuditRegistry(t *testing.T) {
	if os.Getenv("GTS_COMPACT_AUDIT_REGISTRY") != "1" {
		t.Skip("set GTS_COMPACT_AUDIT_REGISTRY=1 to report dispatch capabilities")
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	for _, entry := range grammars.AllLanguages() {
		lang := entry.Language()
		if lang == nil {
			t.Errorf("nil grammar: %s", entry.Name)
			continue
		}
		stateless := lang.ExternalScanner == nil
		if scanner, ok := lang.ExternalScanner.(gts.StatelessExternalScanner); ok {
			stateless = scanner.ExternalScannerIsStateless()
		}
		t.Logf("REGISTRY %s forest=%t scanner=%t stateless=%t token_factory=%t backend=%s", entry.Name, gts.LanguageWantsForest(lang), lang.ExternalScanner != nil, stateless, entry.TokenSourceFactory != nil, grammars.EvaluateParseSupport(entry, lang).Backend)
	}
}

// BenchmarkCompactGraduationFull measures one grammar per process. Run through
// scripts/run_randomized_benchmarks.sh with GTS_COMPACT_AUDIT_LANGUAGE set.
// Both routes use the same generated input and a warmed, reusable parser.
// A declined candidate is measured as fallback overhead, never as compact.
func BenchmarkCompactGraduationFull(b *testing.B) {
	name := os.Getenv("GTS_COMPACT_AUDIT_LANGUAGE")
	if name == "" {
		b.Skip("set GTS_COMPACT_AUDIT_LANGUAGE to one generated grammar")
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		b.Fatalf("unknown grammar %q", name)
	}
	seed := int64(1)
	if shuffle := flag.Lookup("test.shuffle"); shuffle != nil {
		if value, err := strconv.ParseInt(shuffle.Value.String(), 10, 64); err == nil {
			seed = value
		}
	}
	random := rand.New(rand.NewSource(seed))
	sizes := []int{32 * 1024, 137 * 1024, 1024 * 1024}
	for _, index := range random.Perm(len(sizes)) {
		size := sizes[index]
		source, _, err := benchfixtures.GeneratedSource(name, size)
		if err != nil {
			b.Fatal(err)
		}
		for _, routeIndex := range random.Perm(2) {
			candidate := routeIndex == 1
			route := "legacy"
			if candidate {
				route = "candidate"
			}
			b.Run(fmt.Sprintf("%s/%dKiB", route, size/1024), func(b *testing.B) {
				parser := gts.NewParser(entry.Language())
				parser.SetAdmissionCandidateRoute(candidate)
				gts.ResetAdmissionCandidateCounters()
				warm, err := parser.Parse(source)
				if err != nil || warm == nil || warm.RootNode() == nil {
					b.Fatalf("warm parse: %v", err)
				}
				root := warm.RootNode()
				if warm.ParseStopReason() != gts.ParseStopAccepted || root.EndByte() != uint32(len(source)) || root.HasError() {
					b.Fatalf("incomplete clean fixture: stop=%s end=%d bytes=%d error=%t", warm.ParseStopReason(), root.EndByte(), len(source), root.HasError())
				}
				warm.Release()
				_, declines := gts.AdmissionCandidateCounters()
				if candidate && declines > 0 {
					b.Logf("candidate fell back: %s", gts.AdmissionCandidateLastFallbackReason())
				}
				gts.ResetAdmissionCandidateCounters()
				b.SetBytes(int64(len(source)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					tree, err := parser.Parse(source)
					if err != nil || tree == nil {
						b.Fatalf("parse: %v", err)
					}
					tree.Release()
				}
				b.StopTimer()
				routed, declines := gts.AdmissionCandidateCounters()
				b.ReportMetric(float64(declines)/float64(b.N), "declines/op")
				b.ReportMetric(float64(routed)/float64(b.N), "routed/op")
			})
		}
	}
}

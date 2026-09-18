//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"slices"
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestRecoverySymbolPolicyDefaultsAndReuse(t *testing.T) {
	for _, widthSource := range []string{"metadata", "names", "count"} {
		t.Run(widthSource, func(t *testing.T) {
			lang := &Language{SymbolMetadata: []SymbolMetadata{{Visible: false, Named: true}}}
			want := []core.SelectedSymbolPolicy{{Visible: false, Named: true}, {Visible: true}, {Visible: true}}
			switch widthSource {
			case "metadata":
				lang.SymbolMetadata = append(lang.SymbolMetadata, SymbolMetadata{Visible: true}, SymbolMetadata{Visible: true})
			case "names":
				lang.SymbolNames = make([]string, 3)
			case "count":
				lang.SymbolCount = 3
			}
			scheduler := diagnosticParserCoreGenericScheduler{tokenSource: &dfaTokenSource{language: lang}}
			before := diagnosticParserCoreSchedulerFootprintBytes(&scheduler)
			got := scheduler.recoverySymbolPolicy()
			if !slices.Equal(got, want) {
				t.Fatalf("policy = %+v, want %+v", got, want)
			}
			selected, err := buildParserCoreSelectedStorePolicy(&Parser{
				language: lang, hasRootSymbol: true, rootSymbol: 0,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(selected.Symbols, want) {
				t.Fatalf("selected-store policy = %+v, want %+v", selected.Symbols, want)
			}
			after := diagnosticParserCoreSchedulerFootprintBytes(&scheduler)
			if wantBytes := uint64(cap(got)) * uint64(unsafe.Sizeof(core.SelectedSymbolPolicy{})); after-before != wantBytes {
				t.Fatalf("footprint growth = %d, want %d", after-before, wantBytes)
			}
			if allocations := testing.AllocsPerRun(100, func() { scheduler.recoverySymbolPolicy() }); allocations != 0 {
				t.Fatalf("repeated projection allocated %v times", allocations)
			}
			if err := resetDiagnosticParserCoreGenericScheduler(&scheduler); err != nil {
				t.Fatal(err)
			}
			if afterReset := diagnosticParserCoreSchedulerFootprintBytes(&scheduler); afterReset != before {
				t.Fatalf("reset footprint = %d, want %d", afterReset, before)
			}
			// A new parse must read changed metadata, even with the same language pointer.
			lang.SymbolMetadata[0] = SymbolMetadata{Visible: true, Named: false}
			scheduler.tokenSource = &dfaTokenSource{language: lang}
			want[0] = core.SelectedSymbolPolicy{Visible: true}
			if got := scheduler.recoverySymbolPolicy(); !slices.Equal(got, want) {
				t.Fatalf("policy after reset = %+v, want %+v", got, want)
			}
		})
	}
}

func TestRecoverySymbolPolicyEmptyLanguage(t *testing.T) {
	for _, lang := range []*Language{nil, {}} {
		scheduler := diagnosticParserCoreGenericScheduler{tokenSource: &dfaTokenSource{language: lang}}
		if got := scheduler.recoverySymbolPolicy(); len(got) != 0 {
			t.Fatalf("empty language policy = %+v", got)
		}
	}
}

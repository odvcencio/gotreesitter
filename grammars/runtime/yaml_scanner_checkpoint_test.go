//go:build !grammar_subset || grammar_subset_yaml

package grammarruntime

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
	"unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestYAMLCheckpointRestorePreservesIndependentHistories(t *testing.T) {
	scanner := YamlExternalScanner{}
	payload := scanner.Create().(*yamlPayload)
	deep := make([]byte, 10+4*17)
	for i := 0; i < len(deep); i += 2 {
		binary.LittleEndian.PutUint16(deep[i:], uint16(300+i))
	}
	// Negative cursor coordinates and indentation lengths are valid scanner
	// state. Keep their signed representation through every restore.
	binary.LittleEndian.PutUint16(deep[4:], 0xffff)
	binary.LittleEndian.PutUint16(deep[12:], 0xfffe)
	shallow := append([]byte(nil), deep[:14]...)
	header := append([]byte(nil), deep[:10]...)
	for _, checkpoint := range [][]byte{deep, shallow, nil, header, deep, deep[:9], shallow, append(shallow, 1, 2, 3)} {
		scanner.Deserialize(payload, checkpoint)
		fresh := scanner.Create().(*yamlPayload)
		scanner.Deserialize(fresh, checkpoint)
		if !reflect.DeepEqual(payload.state, fresh.state) {
			t.Fatalf("restore after another history differs for %d-byte checkpoint: got %+v want %+v", len(checkpoint), payload.state, fresh.state)
		}
		if payload.state.indents[0] != (yamlIndent{typ: yIndRot, length: -1}) {
			t.Fatal("restore lost the root indentation sentinel")
		}
	}
	checkpoint := append([]byte(nil), deep...)
	scanner.Deserialize(payload, checkpoint)
	clear(checkpoint)
	buf := make([]byte, len(deep))
	if n := scanner.Serialize(payload, buf); n != len(deep) || !bytes.Equal(buf, deep) {
		t.Fatal("restored state aliases its serialized checkpoint")
	}
}

func TestYAMLCheckpointRestoreReusesIndentStorage(t *testing.T) {
	scanner := YamlExternalScanner{}
	payload := scanner.Create()
	deep := make([]byte, 10+4*17)
	scanner.Deserialize(payload, deep)
	if got := testing.AllocsPerRun(100, func() {
		scanner.Deserialize(payload, deep[:14])
		scanner.Deserialize(payload, nil)
		scanner.Deserialize(payload, deep)
	}); got != 0 {
		t.Fatalf("warmed checkpoint restores allocate %g times, want zero", got)
	}
}

// The public value scanner must keep its interface while avoiding token-table
// copies on every interface call. Rebinding must not change an existing value.
func TestYAMLScannerSharesImmutableBindingAndKeepsPayloadsIndependent(t *testing.T) {
	var _ gotreesitter.ExternalScanner = YamlExternalScanner{}
	if got, want := unsafe.Sizeof(YamlExternalScanner{}), unsafe.Sizeof(uintptr(0)); got != want {
		t.Fatalf("scanner value is %d bytes, want one %d-byte binding pointer", got, want)
	}
	lang := Language("yaml")
	bound := YamlExternalScanner{}.ExternalScannerForLanguage(lang).(*YamlExternalScanner)
	copied := *bound
	before := *bound.symbolTable()
	if bound.binding == nil || copied.binding != bound.binding {
		t.Fatal("scanner value copy did not share its immutable binding")
	}
	const offset = 73
	renumbered := &gotreesitter.Language{
		SymbolNames:     make([]string, len(lang.SymbolNames)+offset),
		ExternalSymbols: make([]gotreesitter.Symbol, len(lang.ExternalSymbols)),
	}
	copy(renumbered.SymbolNames[offset:], lang.SymbolNames)
	for i, symbol := range lang.ExternalSymbols {
		renumbered.ExternalSymbols[i] = symbol + offset
	}
	rebound := copied.ExternalScannerForLanguage(renumbered).(*YamlExternalScanner)
	for i, symbol := range before {
		if got := rebound.symbolTable()[i]; got != symbol+offset {
			t.Fatalf("renumbered token %d: got %d want %d", i, got, symbol+offset)
		}
	}
	reboundBefore := *rebound.symbolTable()
	clear(renumbered.ExternalSymbols)
	clear(renumbered.SymbolNames)
	if *bound.symbolTable() != before || *copied.symbolTable() != before || *rebound.symbolTable() != reboundBefore || rebound.binding == bound.binding {
		t.Fatal("rebinding or input mutation changed the original immutable table")
	}
	if got := *(YamlExternalScanner{}).symbolTable(); got != yamlDefaultSymTable {
		t.Fatal("zero-value scanner lost its default symbol mapping")
	}
	a, b := bound.Create().(*yamlPayload), copied.Create().(*yamlPayload)
	checkpoint := make([]byte, 10)
	binary.LittleEndian.PutUint16(checkpoint, 513)
	bound.Deserialize(a, checkpoint)
	if a == b || reflect.DeepEqual(a.state, b.state) {
		t.Fatal("shared immutable binding also shared mutable scanner state")
	}
}

func TestYAMLPreparedValidityPreservesGeneralMapping(t *testing.T) {
	identity := make([]int, yTokCount)
	reversed := make([]int, yTokCount)
	for i := range identity {
		identity[i], reversed[i] = i, yTokCount-1-i
	}
	for _, tc := range []struct {
		name          string
		mapping       []int
		maskLength    int
		enabled, want []int
		wantLength    int
	}{
		{"identity", identity, yTokCount, []int{0, 3, yTokCount - 1}, []int{0, 3, yTokCount - 1}, yTokCount},
		{"empty", identity, 0, nil, nil, yTokCount},
		{"identity-short", identity, 4, []int{0, 3}, []int{0, 3}, yTokCount},
		{"identity-long", identity, yTokCount + 1, []int{0, yTokCount}, []int{0}, yTokCount},
		{"reversed", reversed, yTokCount, []int{0, 3}, []int{yTokCount - 1, yTokCount - 4}, yTokCount},
		{"reversed-short", reversed, 4, []int{0, 3}, []int{yTokCount - 1, yTokCount - 4}, yTokCount},
		{"sparse-duplicates", []int{2, -1, 2, 0, yTokCount}, 6, []int{0, 1, 2, 3, 4, 5}, []int{0, 2}, yTokCount},
		{"unbound", nil, 4, []int{0, 3}, []int{0, 3}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := &yamlExternalScannerBinding{externalToToken: append([]int(nil), tc.mapping...)}
			binding.prepare()
			scanner := YamlExternalScanner{binding: binding}
			mask := make([]bool, tc.maskLength)
			for _, index := range tc.enabled {
				mask[index] = true
			}
			before := make([]bool, len(mask))
			copy(before, mask)
			want := make([]bool, tc.wantLength)
			for _, token := range tc.want {
				want[token] = true
			}
			var scratch [yTokCount]bool
			for i := range scratch {
				scratch[i] = true
			}
			got := scanner.semanticValidity(mask, &scratch)
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(mask, before) {
				t.Fatalf("mapped=%v want=%v; input changed=%t", got, want, !reflect.DeepEqual(mask, before))
			}
			clear(mask)
			for _, enabled := range scanner.semanticValidity(mask, &scratch) {
				if enabled {
					t.Fatal("a later scan retained an earlier enabled token")
				}
			}
			if n := testing.AllocsPerRun(100, func() { scanner.semanticValidity(mask, &scratch) }); n != 0 {
				t.Fatalf("validity mapping allocated %g times", n)
			}
		})
	}
}

func TestYAMLIndentFramesKeepPairsAcrossGrowthAndRestore(t *testing.T) {
	scanner := YamlExternalScanner{}
	first := scanner.Create().(*yamlPayload)
	env := yamlEnv{st: &first.state}
	if env.popInd() || env.curInd() != -1 || env.curIndTyp() != yIndRot || env.prtInd() != -1 {
		t.Fatal("root sentinel changed")
	}
	// These signed values exercise every bit, independent of valid indentation kinds.
	for i := 0; i < 257; i++ {
		env.pushInd(int16(32767-i*251), int16(-32768+i*137))
	}
	checkpoint := make([]byte, 10+4*257)
	if scanner.Serialize(first, checkpoint) != len(checkpoint) {
		t.Fatal("deep history truncated")
	}
	saved := append([]byte(nil), checkpoint...)
	second := scanner.Create().(*yamlPayload)
	scanner.Deserialize(second, checkpoint)
	clear(checkpoint)
	for i := 256; i >= 0; i-- {
		if env.curIndTyp() != int16(32767-i*251) || env.curInd() != int16(-32768+i*137) {
			t.Fatal("indentation type and width separated")
		}
		parent := int16(-1)
		if i > 0 {
			parent = int16(-32768 + (i-1)*137)
		}
		if env.prtInd() != parent || !env.popInd() {
			t.Fatal("parent history changed during pop")
		}
	}
	if env.popInd() {
		t.Fatal("popped root sentinel")
	}
	check := make([]byte, len(saved))
	if scanner.Serialize(second, check) != len(saved) || !bytes.Equal(check, saved) {
		t.Fatal("another payload or input mutation changed the restored history")
	}
	// Preserve the existing wire format, including a complete-entry boundary.
	for n := 0; n <= len(saved); n++ {
		buffer := make([]byte, n)
		written := scanner.Serialize(second, buffer)
		want := 0
		if n >= 10 {
			want = 10 + 4*((n-10)/4)
		}
		if written != want || !bytes.Equal(buffer[:written], saved[:written]) {
			t.Fatalf("checkpoint prefix changed at capacity %d", n)
		}
	}
}

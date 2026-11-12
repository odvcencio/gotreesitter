package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

// RunLexExactOnly writes the exact-lexer record of one grammar. It reads the
// shipped blob at blobPath and the parser.c text with the grammar's own
// tree_sitter/parser.h (parserH may be empty for the standard header), checks
// the blob against the C text, and writes two files below outDir:
//
//	lex_exact/<name>.lexx          the record bytes (internal/lexexact, version 1)
//	<name>_lex_exact_gen.go        embeds the bytes and registers them under the blob's SHA-256
//
// It never writes the blob. When the check fails it writes nothing and returns
// an error that names the first differing state. The grammar name is the blob
// file name without ".bin" unless name is set. If outDir has a "runtime"
// folder (the catalog root "grammars"), the files go there, as for the other
// generated sidecars.
func RunLexExactOnly(parserC, parserH, blobPath, outDir, name string) error {
	blob, err := os.ReadFile(blobPath)
	if err != nil {
		return err
	}
	lang, err := gts.LoadLanguage(blob)
	if err != nil {
		return fmt.Errorf("load blob %s: %w", blobPath, err)
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(blobPath), ".bin")
	}
	rec, stats, err := buildLexExactRecord(parserC, parserH, blob, lang)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	data, err := rec.Encode()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	listDir := outDir
	if info, err := os.Stat(filepath.Join(outDir, "runtime")); err == nil && info.IsDir() {
		outDir = filepath.Join(outDir, "runtime")
	}
	base := safeFileBase(name)
	dataDir := filepath.Join(outDir, "lex_exact")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dataDir, base+".lexx"), data, 0o644); err != nil {
		return err
	}
	code := generateLexExactGo(base, name, rec)
	if err := os.WriteFile(filepath.Join(outDir, base+"_lex_exact_gen.go"), []byte(code), 0o644); err != nil {
		return err
	}
	if err := updateLexExactList(filepath.Join(listDir, lexExactListFile), name); err != nil {
		return err
	}
	fmt.Printf("lexexact %s: %d bytes, main %d states, keyword %d states; %s\n",
		name, len(data), len(rec.Main.Accept), len(rec.Keyword.Accept), stats)
	return nil
}

// lexExactStats counts, over the states of the blob, the places where the
// legacy fields of the blob differ from C. They are reported, not errors: the
// record exists to carry what the blob loses.
type lexExactStats struct {
	eofEdgeDiffers  int // blob EOF edge differs from C's step at end of input
	endAcceptMissed int // C accepts the end token, the blob has no AcceptEOF
}

func (s lexExactStats) String() string {
	return fmt.Sprintf("blob end-of-input edge differs from C in %d states, blob lacks the end accept in %d states",
		s.eofEdgeDiffers, s.endAcceptMissed)
}

// buildLexExactRecord parses parserC, builds the record (lexexact.Build compares
// the blob's ranges with the C text), and compares the state counts and the
// accept symbol of every state with the blob.
func buildLexExactRecord(parserC, parserH string, blob []byte, lang *gts.Language) (*lexexact.Tables, lexExactStats, error) {
	var stats lexExactStats
	src, err := lexexact.Parse(parserC, parserH)
	if err != nil {
		return nil, stats, fmt.Errorf("parse parser.c: %w", err)
	}
	rec, err := lexexact.Build(src, parserC, blobTransitions(lang.LexStates), blobTransitions(lang.KeywordLexStates))
	if err != nil {
		return nil, stats, err
	}
	rec.BlobSHA256 = sha256.Sum256(blob)
	if err := rec.Validate(len(lang.LexStates), len(lang.KeywordLexStates)); err != nil {
		return nil, stats, err
	}
	for _, f := range []struct {
		name   string
		lexer  *lexexact.Lexer
		fn     *lexexact.Func
		states []gts.LexState
	}{
		{"ts_lex", src.Main, &rec.Main, lang.LexStates},
		{"ts_lex_keywords", src.Keyword, &rec.Keyword, lang.KeywordLexStates},
	} {
		if f.lexer == nil {
			if len(f.states) != 0 {
				return nil, stats, fmt.Errorf("%s: the blob has %d states, parser.c has no such function", f.name, len(f.states))
			}
			continue
		}
		for i, st := range f.states {
			want := f.fn.Accept[i]
			var have uint32
			switch {
			case st.AcceptEOF:
				have = 1
			case st.AcceptToken != 0:
				have = uint32(st.AcceptToken) + 1
			}
			if have != want {
				if want == 1 && have == 0 {
					stats.endAcceptMissed++
					continue
				}
				return nil, stats, fmt.Errorf("%s: state %d: blob accepts symbol code %d, parser.c %d", f.name, i, have, want)
			}
			if got, ok := lexexact.UnpackStep(f.fn.OnEOF[i]); st.EOF >= 0 && (!ok || got.Target != st.EOF) || st.EOF < 0 && ok {
				stats.eofEdgeDiffers++
			}
		}
	}
	return rec, stats, nil
}

func blobTransitions(states []gts.LexState) [][]lexexact.Transition {
	out := make([][]lexexact.Transition, len(states))
	for i, s := range states {
		for _, tr := range s.Transitions {
			out[i] = append(out[i], lexexact.Transition{Lo: tr.Lo, Hi: tr.Hi, Next: tr.NextState, Skip: tr.Skip})
		}
	}
	return out
}

// generateLexExactGo returns the text of <name>_lex_exact_gen.go.
func generateLexExactGo(base, name string, rec *lexexact.Tables) string {
	ident := languageRegisterIdentifier(name) + "LexExact"
	var b strings.Builder
	fmt.Fprintf(&b, "//go:build !grammar_subset || grammar_subset_%s\n\n", base)
	b.WriteString("// Code generated by cmd/ts2go -lexexact-only; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "// Source: parser.c sha256:%x\n\n", rec.SourceSHA256)
	b.WriteString("package grammarruntime\n\n")
	b.WriteString("import (\n\t_ \"embed\"\n\n\t\"github.com/odvcencio/gotreesitter/internal/lexexact\"\n)\n\n")
	fmt.Fprintf(&b, "// %s holds the exact-lexer record of the %s blob (internal/lexexact, version %d).\n", ident, name, rec.Version)
	fmt.Fprintf(&b, "//\n//go:embed lex_exact/%s.lexx\nvar %s []byte\n\n", base, ident)
	fmt.Fprintf(&b, "func init() {\n\tlexexact.Register(lexexact.MustHash(%q), %s)\n}\n", fmt.Sprintf("%x", rec.BlobSHA256), ident)
	return b.String()
}

// lexExactListFile names the list of the grammars that have a record. The mode
// owns it: every run adds the grammar it wrote, and nothing else changes it.
// A test of the grammars package compares the list with the records that a
// build registers.
const lexExactListFile = "lex_exact_list.txt"

const lexExactListHeader = "# Grammars that have an exact-lexer record. Written by cmd/ts2go -lexexact-only; do not edit by hand.\n"

// updateLexExactList adds name to the sorted list in path (created if it does
// not exist). It rewrites the file only when the content changes.
func updateLexExactList(path, name string) error {
	names := map[string]bool{name: true}
	if old, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(old), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				names[line] = true
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	text := lexExactListHeader + strings.Join(sorted, "\n") + "\n"
	if old, err := os.ReadFile(path); err == nil && string(old) == text {
		return nil
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

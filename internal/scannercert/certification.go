// Package scannercert supplies registry-driven scanner contract tests.
// It is imported by the root test suite, not by the parser runtime.
package scannercert

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Run checks the capabilities the engine trusts,
// across every registered scanner. Unadvertised capabilities are measured too:
// their failures are work items, rather than permission to enable reuse. No
// scanner acquires a capability merely because a finite sample passes.
//
// Replay compares decisions, token spans, read frontiers, column dependencies,
// and subsequent serialized state, not just Serialize(Deserialize(bytes)). It
// uses fresh and dirty destinations so Deserialize must replace earlier state
// and cannot rely on values left over from an earlier scan.
func Run(t *testing.T, api LexerAPI) {
	corpus := scannerCertificationCorpus(t)
	entries := grammars.AllLanguages()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	for _, entry := range entries {
		t.Run(entry.Name, func(t *testing.T) {
			lang := entry.Language()
			if lang.ExternalScanner == nil {
				return
			}
			scanner := lang.ExternalScanner
			cert := newScannerCertification(scanner, api)
			defer cert.close()
			t.Run("boundaries", func(t *testing.T) {
				cert.fixture = "boundaries"
				// Delimiters and long indentation exercise mutable scanner states
				// even when the grammar's smoke sample contains only identifiers.
				for _, source := range []string{
					"\"text\" 'text' `text` #\"text\"# r#\"text\"# R\"tag(body)tag\"\n",
					"$tag$body$tag$ $$body$$\n",
					"<outer><inner/></outer> <!-- comment -->\n",
					"x\n" + strings.Repeat(" ", 300) + "y\n" + strings.Repeat(" ", 200) + "z\n",
					"/* nested /* comment */ */ {- comment -} /- comment -/\n",
					"{}[]() @x(y) [=[body]=] [[body]]\n0000: 90 90 nop\n",
					"::\n\n  body\n",
				} {
					cert.probeRows([]byte(source), lang.ExternalLexStates, len(lang.ExternalSymbols))
				}
			})
			fixtures := append([]scannerCertificationFixture{{name: "smoke", source: []byte(grammars.ParseSmokeSample(entry.Name))}}, corpus[entry.Name]...)
			for _, fixture := range fixtures {
				t.Run(fixture.name, func(t *testing.T) {
					cert.fixture = fixture.name
					source := fixture.load(t)
					// Probe each grammar-owned valid-symbol row at byte boundaries on
					// the smoke sample. These also exercise scanners not reached by
					// that sample's parser state. Corpus probes use real parse calls.
					if fixture.name == "smoke" {
						cert.probeRows(source, lang.ExternalLexStates, len(lang.ExternalSymbols))
					}
					copyLang := *lang
					copyLang.ExternalScanner = cert
					instrumented := scannerCertificationParse(t, &copyLang, source)
					oracle := scannerCertificationParse(t, lang, source)
					if instrumented != oracle {
						t.Fatalf("certification changed parse: instrumented=%s original=%s", instrumented, oracle)
					}
				})
			}
			for _, kind := range []string{"roundtrip", "replay", "absent-replay", "stateless-state", "failure-mutation", "scan-panic"} {
				if witness, ok := cert.failures[kind]; ok {
					t.Logf("UNCERTIFIED %s: %s", kind, witness)
					if cert.required(kind) {
						t.Errorf("advertised scanner capability failed %s: %s", kind, witness)
					}
				}
			}
			t.Logf("scanner=%T nil-payload=%t stateless=%t checkpoints=%t incremental-reuse=%t failure-preserving=%t failure-retaining=%t scans=%d successes=%d failures=%d absent=%d",
				scanner, cert.nilPayload, cert.stateless, cert.checkpointed, cert.reusable, cert.preserving, cert.retaining, cert.scans, cert.successes, cert.failedScans, cert.absent)
		})
	}
}

type scannerCertificationFixture struct {
	name, path, digest string
	source             []byte
}

func (f scannerCertificationFixture) load(t *testing.T) []byte {
	t.Helper()
	if f.path == "" {
		return f.source
	}
	source, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != f.digest {
		t.Fatalf("%s corpus digest = %s, want %s", f.name, got, f.digest)
	}
	return source
}

func scannerCertificationCorpus(t *testing.T) map[string][]scannerCertificationFixture {
	t.Helper()
	const base = "internal/benchfixtures"
	data, err := os.ReadFile(filepath.Join(base, "real_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			Language, Role, SHA256, Path string
			SourceKey                    string `json:"source_key"`
			CommittedPath                string `json:"committed_path"`
		}
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	corpus := make(map[string][]scannerCertificationFixture)
	// Default CI uses the committed sample for each grammar. The same gate
	// accepts every manifest file in an authenticated external corpus checkout,
	// with one language selected by go test -run for heavier local coverage.
	corpusRoot := os.Getenv("GTS_CORPUS_DIR")
	for _, row := range manifest.Entries {
		if row.CommittedPath == "" && corpusRoot == "" {
			continue
		}
		path := filepath.Join(base, row.CommittedPath)
		if corpusRoot != "" {
			path = filepath.Join(corpusRoot, row.SourceKey, row.Path)
		}
		corpus[row.Language] = append(corpus[row.Language], scannerCertificationFixture{name: "corpus-" + row.Role, path: path, digest: row.SHA256})
	}
	return corpus
}

func scannerCertificationParse(t *testing.T, lang *gts.Language, source []byte) string {
	t.Helper()
	parser := gts.NewParser(lang)
	// Fresh parsing isolates the scanner contract from engine graduation.
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	return inspection.SHA256
}

type scannerCertification struct {
	api LexerAPI
	gts.ExternalScanner
	replayPayload                                                        any
	nilPayload, stateless, checkpointed, reusable, preserving, retaining bool
	fixture                                                              string
	failures                                                             map[string]string
	scans, successes, failedScans, absent                                int
	lastReplay                                                           *scannerCertificationReplayOrigin
	nilPayloadBuffer                                                     []byte
}

type scannerCertificationCall struct {
	source []byte
	offset int
	valid  []bool
}

type scannerCertificationReplayOrigin struct {
	call       scannerCertificationCall
	checkpoint []byte
}

type scannerCertificationPayload struct {
	inner      any
	checkpoint []byte
	hasRestore bool
	calls      []scannerCertificationCall
}

func newScannerCertification(scanner gts.ExternalScanner, api LexerAPI) *scannerCertification {
	c := &scannerCertification{api: api, ExternalScanner: scanner, replayPayload: scanner.Create(), failures: make(map[string]string)}
	c.nilPayload = c.replayPayload == nil
	if c.nilPayload {
		c.nilPayloadBuffer = make([]byte, api.SerializationCapacity)
	}
	c.stateless = scannerCapability(scanner, func(s gts.StatelessExternalScanner) bool { return s.ExternalScannerIsStateless() })
	c.checkpointed = scannerCapability(scanner, func(s gts.CheckpointedExternalScanner) bool { return s.UsesExternalScannerCheckpoints() })
	c.reusable = scannerCapability(scanner, func(s gts.IncrementalReuseExternalScanner) bool { return s.SupportsIncrementalReuse() })
	c.preserving = scannerCapability(scanner, func(s gts.FailurePreservingExternalScanner) bool { return s.PreservesStateOnScanFailure() })
	c.retaining = scannerCapability(scanner, func(s gts.FailureStateRetainingExternalScanner) bool { return s.RetainsStateOnScanFailure() })
	return c
}

func scannerCapability[T any](scanner gts.ExternalScanner, enabled func(T) bool) bool {
	capability, ok := scanner.(T)
	return ok && enabled(capability)
}

func (c *scannerCertification) close() { c.ExternalScanner.Destroy(c.replayPayload) }

func (c *scannerCertification) Create() any {
	return &scannerCertificationPayload{inner: c.ExternalScanner.Create()}
}

func scannerCertificationInner(payload any) any {
	if traced, ok := payload.(*scannerCertificationPayload); ok {
		return traced.inner
	}
	return payload
}

func (c *scannerCertification) Destroy(payload any) {
	c.ExternalScanner.Destroy(scannerCertificationInner(payload))
}

func (c *scannerCertification) Serialize(payload any, buffer []byte) int {
	return c.ExternalScanner.Serialize(scannerCertificationInner(payload), buffer)
}

func (c *scannerCertification) Deserialize(payload any, state []byte) {
	c.ExternalScanner.Deserialize(scannerCertificationInner(payload), state)
	if traced, ok := payload.(*scannerCertificationPayload); ok {
		traced.checkpoint = bytes.Clone(state)
		traced.hasRestore = true
		traced.calls = nil
	}
}

// Instrumentation preserves capabilities that affect fresh GLR parsing as
// well as failure semantics. Stateful branch ownership must still record its
// checkpoints, and transactional scanners must keep the production rollback.
func (c *scannerCertification) UsesExternalScannerCheckpoints() bool { return c.checkpointed }
func (c *scannerCertification) SupportsIncrementalReuse() bool       { return c.reusable }
func (c *scannerCertification) ExternalScannerIsStateless() bool     { return c.stateless }
func (c *scannerCertification) PreservesStateOnScanFailure() bool    { return c.preserving }
func (c *scannerCertification) RetainsStateOnScanFailure() bool      { return c.retaining }

func (c *scannerCertification) required(kind string) bool {
	if kind == "absent-replay" {
		return false // The checkpoint contract reserves zero for absence.
	}
	if kind == "failure-mutation" {
		return c.preserving && !c.retaining
	}
	return c.stateless || c.nilPayload || c.checkpointed || c.reusable
}

func (c *scannerCertification) serialize(payload any) []byte {
	buffer := c.nilPayloadBuffer
	if !c.nilPayload {
		buffer = make([]byte, c.api.SerializationCapacity)
	}
	n := c.Serialize(payload, buffer)
	if n < 0 || n > len(buffer) {
		panic(fmt.Sprintf("scanner serialized %d bytes into %d-byte buffer", n, len(buffer)))
	}
	if c.nilPayload {
		// Still invoke Serialize on every attempt. Empty snapshots need no
		// owned buffer; copy any unexpected non-empty result so a later
		// serialization cannot erase evidence of a false capability claim.
		return bytes.Clone(buffer[:n])
	}
	return buffer[:n]
}

func (c *scannerCertification) Scan(payload any, lexer *gts.ExternalLexer, valid []bool) bool {
	traced, _ := payload.(*scannerCertificationPayload)
	if traced != nil {
		source, pos := c.api.Input(lexer)
		// Keep the scan sequence to reproduce states a lossy serializer cannot
		// reconstruct. Deserialize starts a new sequence at its checkpoint.
		traced.calls = append(traced.calls, scannerCertificationCall{source: source, offset: pos, valid: append([]bool(nil), valid...)})
	}
	inner := scannerCertificationInner(payload)
	origin := c.api.Clone(lexer)
	clone := c.api.Clone(lexer)
	before := c.serialize(payload)
	defer func() {
		source, pos := c.api.Input(origin)
		c.lastReplay = &scannerCertificationReplayOrigin{call: scannerCertificationCall{source: source, offset: pos, valid: append([]bool(nil), valid...)}, checkpoint: before}
	}()
	if c.stateless && len(before) != 0 {
		c.fail("stateless-state", origin, valid, before, "stateless scanner serialized a payload", traced)
	}
	c.ExternalScanner.Deserialize(c.replayPayload, bytes.Clone(before))
	roundtrip := c.serialize(c.replayPayload)
	if !bytes.Equal(before, roundtrip) {
		c.fail("roundtrip", origin, valid, before, fmt.Sprintf("restored=%x", roundtrip), traced)
	}
	accepted, scanPanic := scannerCertificationScan(c.ExternalScanner, inner, lexer, valid)
	if scanPanic != "" {
		c.fail("scan-panic", origin, valid, before, scanPanic, traced)
	}
	after := c.serialize(payload)
	replayed, replayPanic := scannerCertificationScan(c.ExternalScanner, c.replayPayload, clone, valid)
	if replayPanic != "" {
		c.fail("scan-panic", origin, valid, before, "restored: "+replayPanic, traced)
	}
	replayAfter := c.serialize(c.replayPayload)
	c.scans++
	if scanPanic != "" || replayPanic != "" {
		return accepted
	}
	if accepted {
		c.successes++
	} else {
		c.failedScans++
		if !bytes.Equal(before, after) {
			c.fail("failure-mutation", origin, valid, before, fmt.Sprintf("after=%x", after), traced)
		}
	}
	if len(before) == 0 && !c.stateless && !c.nilPayload {
		// CheckpointedExternalScanner explicitly reserves zero for an absent
		// checkpoint. Do not treat this boundary as proof of safe reuse.
		c.absent++
	}
	c.compareReplay(origin, lexer, clone, valid, before, after, replayAfter, accepted, replayed, traced)
	if !c.nilPayload {
		fresh := c.ExternalScanner.Create()
		freshLexer := c.api.Clone(origin)
		c.ExternalScanner.Deserialize(fresh, bytes.Clone(before))
		freshAccepted, fault := scannerCertificationScan(c.ExternalScanner, fresh, freshLexer, valid)
		if fault != "" {
			c.fail("scan-panic", origin, valid, before, "fresh restore: "+fault, traced)
		} else {
			c.compareReplay(origin, lexer, freshLexer, valid, before, after, c.serialize(fresh), accepted, freshAccepted, traced)
		}
		c.ExternalScanner.Destroy(fresh)
	}
	return accepted
}

func (c *scannerCertification) compareReplay(origin, actual, restored *gts.ExternalLexer, valid []bool, before, after, replayAfter []byte, accepted, replayed bool, traced *scannerCertificationPayload) {
	if accepted != replayed || !bytes.Equal(after, replayAfter) ||
		c.api.Observe(actual) != c.api.Observe(restored) {
		kind := "replay"
		if len(before) == 0 && c.checkpointed && !c.stateless && !c.nilPayload {
			kind = "absent-replay"
		}
		c.fail(kind, origin, valid, before, fmt.Sprintf("accepted=%t/%t after=%x/%x lexer=%+v/%+v", accepted, replayed, after, replayAfter,
			c.api.Observe(actual), c.api.Observe(restored)), traced)
	}
}

// A panic in an untrusted scanner is a failed certification with a witness. It
// must not prevent the remaining registry entries from being audited. The test
// still fails every advertised contract, and original corpus parses are never
// protected by this helper.
func scannerCertificationScan(scanner gts.ExternalScanner, payload any, lexer *gts.ExternalLexer, valid []bool) (accepted bool, fault string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fault = fmt.Sprint(recovered)
		}
	}()
	accepted = scanner.Scan(payload, lexer, valid)
	return
}

func (c *scannerCertification) fail(kind string, lexer *gts.ExternalLexer, valid []bool, state []byte, detail string, traced *scannerCertificationPayload) {
	if _, exists := c.failures[kind]; exists {
		return
	}
	source, pos := c.api.Input(lexer)
	if pos > len(source) {
		pos = len(source)
	}
	shrunk := false
	if kind == "failure-mutation" {
		source, pos, valid, shrunk = c.shrinkFailureMutation(source, pos, valid, state)
		if shrunk {
			payload := c.ExternalScanner.Create()
			c.ExternalScanner.Deserialize(payload, state)
			scannerCertificationScan(c.ExternalScanner, payload, c.api.New(source, pos), valid)
			detail = fmt.Sprintf("after=%x", c.serialize(payload))
			c.ExternalScanner.Destroy(payload)
		}
	}
	if (kind == "replay" || kind == "scan-panic" || kind == "roundtrip") && traced != nil {
		if witness, ok := c.shrinkSequence(kind, traced); ok {
			c.failures[kind] = "fixture=" + c.fixture + " shrunk=true " + witness
			return
		}
	}
	end := min(pos+80, len(source))
	var symbols []int
	for i, enabled := range valid {
		if enabled {
			symbols = append(symbols, i)
		}
	}
	c.failures[kind] = fmt.Sprintf("fixture=%s shrunk=%t offset=%d input=%q valid=%v state=%x %s", c.fixture, shrunk, pos, source[pos:end], symbols, state, detail)
}

// Minimize an independently reproducible failed-scan witness. Removing bytes
// before the scan origin recomputes its point, so column-sensitive failures
// retain precisely the prefix they need. The checkpoint remains a reachable
// state from the original fixture; it is never replaced with arbitrary bytes.
func (c *scannerCertification) shrinkFailureMutation(source []byte, pos int, valid []bool, state []byte) ([]byte, int, []bool, bool) {
	reproduces := func(source []byte, pos int, valid []bool) bool {
		payload := c.ExternalScanner.Create()
		defer c.ExternalScanner.Destroy(payload)
		c.ExternalScanner.Deserialize(payload, state)
		before := c.serialize(payload)
		accepted, fault := scannerCertificationScan(c.ExternalScanner, payload, c.api.New(source, pos), valid)
		return fault == "" && !accepted && !bytes.Equal(before, c.serialize(payload))
	}
	if !reproduces(source, pos, valid) {
		return source, pos, valid, false
	}
	source = append([]byte(nil), source...)
	valid = append([]bool(nil), valid...)
	for chunk := max(1, len(source)/2); chunk > 0; chunk /= 2 {
		for start := 0; start+chunk <= len(source); {
			end := start + chunk
			if start < pos && end > pos {
				start++
				continue
			}
			nextPos := pos
			if end <= pos {
				nextPos -= chunk
			}
			next := append(append([]byte(nil), source[:start]...), source[end:]...)
			if reproduces(next, nextPos, valid) {
				source, pos = next, nextPos
			} else {
				start++
			}
		}
	}
	for i, enabled := range valid {
		if enabled {
			valid[i] = false
			if !reproduces(source, pos, valid) {
				valid[i] = true
			}
		}
	}
	return source, pos, valid, true
}

func (c *scannerCertification) probeRows(source []byte, rows [][]bool, count int) {
	sets := append([][]bool{make([]bool, count)}, rows...)
	for i := 0; i < count; i++ {
		row := make([]bool, count)
		row[i] = true
		sets = append(sets, row)
	}
	seen := make(map[string]bool)
	for _, row := range sets {
		key := fmt.Sprint(row)
		if seen[key] {
			continue
		}
		seen[key] = true
		for pos := 0; pos <= len(source); pos++ {
			payload := c.Create()
			lexer := c.api.New(source, pos)
			accepted := c.Scan(payload, lexer, row)
			// An empty snapshot can hide state written by a failed scan. Keep
			// that live payload for a continuation too, so omitted state must
			// reproduce decisions rather than pass a vacuous byte comparison.
			if accepted || (!c.nilPayload && len(c.lastReplay.checkpoint) == 0) {
				observation := c.api.Observe(lexer)
				next := observation.Cursor
				if accepted && observation.Marked {
					next = observation.End
				}
				// The internal lexer can consume the rest of the line before
				// requesting the next external token. Keep the live payload
				// rather than serializing it to seed this continuation.
				if newline := bytes.IndexByte(source[next:], '\n'); newline >= 0 {
					next += newline
				}
				c.Scan(payload, c.api.New(source, next), row)
			}
			c.Destroy(payload)
		}
	}
}

package scannercert

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// ErrNoScanner reports a language without an external scanner.
var ErrNoScanner = errors.New("scannercert: language has no external scanner")

// Certify parses every source of corpus with lang while a recorder stands in
// for lang's external scanner, replays each distinct Scan call from the
// serialized state it began with, and reports how the replays compare.
//
// Certify replaces lang.ExternalScanner for the length of the call and
// restores it before returning, so lang must not be in use elsewhere. The
// parser is pinned to the production route, as cmd/zeroreuse pins it, so
// GTS_ADMISSION_CANDIDATE cannot change which calls the parser makes.
func Certify(lang *gts.Language, corpus *Corpus) (*Report, error) {
	if lang == nil || lang.ExternalScanner == nil {
		return nil, ErrNoScanner
	}
	if _, err := lexerLayout(); err != nil {
		return nil, err
	}
	inner := lang.ExternalScanner
	defer func() { lang.ExternalScanner = inner }()

	want, err := parseAll(lang, corpus.Sources, nil)
	if err != nil {
		return nil, fmt.Errorf("scannercert: %s: %w", corpus.Language, err)
	}
	rec := newRecorder(inner)
	lang.ExternalScanner = rec.install()
	got, err := parseAll(lang, corpus.Sources, rec.begin)
	lang.ExternalScanner = inner
	if err != nil {
		return nil, fmt.Errorf("scannercert: %s: %w", corpus.Language, err)
	}
	divergent, errorTrees := 0, 0
	for i := range got {
		if want[i].digest != got[i].digest {
			divergent++
		}
		if got[i].hasError {
			errorTrees++
		}
	}

	a := rec.analyze(inner, symbolNames{lang: lang})
	verdict := a.verdict()
	if divergent > 0 {
		// The recorder changed the parse, so the calls it saw are not the
		// production parser's. Whatever the checks found describes the
		// recorded parse, not the scanner's use in production.
		verdict = VerdictNotTransparent
	}
	name := corpus.Language
	if name == "" {
		name = lang.Name
	}
	return &Report{
		Schema:      Schema,
		Language:    name,
		Scanner:     fmt.Sprintf("%T", inner),
		Declared:    declared(inner),
		Environment: environment(),
		Corpus: CorpusInfo{
			Path: corpus.Path, SampleSHA256: corpus.SampleSHA256,
			SessionSHA256: corpus.SessionSHA256, ManifestSHA256: corpus.ManifestSHA256,
			Sources: len(corpus.Sources),
		},
		Parses: ParseInfo{
			Sources: len(corpus.Sources), SampleHasError: len(got) > 0 && got[0].hasError,
			ErrorTrees: errorTrees, Transparent: divergent == 0, Divergent: divergent,
		},
		Calls:     a.calls,
		Checks:    a.checks,
		Verdict:   verdict,
		Witnesses: a.witnesses,
	}, nil
}

// parsed is what Certify keeps of one parse: a digest of the whole tree and
// whether the tree contains an error.
type parsed struct {
	digest   string
	hasError bool
}

// parseAll parses each source with one parser. begin, when set, is told the
// index of each source before it is parsed.
func parseAll(lang *gts.Language, sources [][]byte, begin func(int)) ([]parsed, error) {
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	results := make([]parsed, 0, len(sources))
	for i, source := range sources {
		if begin != nil {
			begin(i)
		}
		result, err := parseOne(parser, lang, source)
		if err != nil {
			return nil, fmt.Errorf("parse source %d: %w", i, err)
		}
		results = append(results, result)
	}
	return results, nil
}

// parseOne turns a scanner panic into an error so Certify can restore the
// language's scanner and name the source that failed.
func parseOne(parser *gts.Parser, lang *gts.Language, source []byte) (result parsed, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	tree, err := parser.Parse(source)
	if err != nil {
		return parsed{}, err
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		return parsed{digest: "no tree"}, nil
	}
	inspection, err := benchfixtures.InspectGoTree(root, lang)
	if err != nil {
		return parsed{}, err
	}
	return parsed{digest: inspection.SHA256, hasError: root.HasError()}, nil
}

// declared reports the scanner's own capability claims.
func declared(scanner gts.ExternalScanner) Declared {
	tri := func(present, value bool) string {
		switch {
		case !present:
			return "absent"
		case value:
			return "true"
		}
		return "false"
	}
	var d Declared
	reuse, ok := scanner.(gts.IncrementalReuseExternalScanner)
	d.IncrementalReuse = tri(ok, ok && reuse.SupportsIncrementalReuse())
	checkpointed, ok := scanner.(gts.CheckpointedExternalScanner)
	d.Checkpointed = tri(ok, ok && checkpointed.UsesExternalScannerCheckpoints())
	stateless, ok := scanner.(gts.StatelessExternalScanner)
	d.Stateless = tri(ok, ok && stateless.ExternalScannerIsStateless())
	preserving, ok := scanner.(gts.FailurePreservingExternalScanner)
	d.PreservesStateOnFailure = tri(ok, ok && preserving.PreservesStateOnScanFailure())
	retaining, ok := scanner.(gts.FailureStateRetainingExternalScanner)
	d.RetainsStateOnFailure = tri(ok, ok && retaining.RetainsStateOnScanFailure())
	return d
}

// environment lists the GOT_ and GTS_ variables set now, sorted, without
// their values. The parser reads several of them.
func environment() []string {
	names := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GOT_") || strings.HasPrefix(name, "GTS_") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return slices.Compact(names)
}

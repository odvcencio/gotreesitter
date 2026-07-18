//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"fmt"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// parserCoreFreshFullRunner owns the reusable state for the authenticated,
// fresh UTF-8 compact-parser route. It is deliberately build-tagged and does
// not participate in Parser.parseInternal: callers must opt into this exact
// diagnostic route and handle a fail-closed decline themselves.
//
// The only admitted identity is the certified embedded Go blob and exact Go
// external scanner. The route is fresh UTF-8 only and declines recovery,
// missing/no-lookahead tokens, repetition shifts, extra chains, and any EOF
// frontier other than one accepted head with one exact derivation.
//
// The runner is stateful and not safe for concurrent use. A successful parse
// returns a caller-owned tree. Every call resets the compact arenas before
// acquiring a production DFA token source, so a declined or capped parse
// cannot publish state into the next call.
type parserCoreFreshFullRunner struct {
	lang           *Language
	parser         *Parser
	tables         *parserCoreRootTables
	compact        *core.Core
	options        DiagnosticParserCorePrefixOptions
	scannerScratch []byte
}

func newParserCoreFreshFullRunner(scanner ExternalScanner, options DiagnosticParserCorePrefixOptions) (*parserCoreFreshFullRunner, error) {
	if options.Recovery || options.Retry || options.Incremental || options.IncludedRanges || options.GenericStopAtClosedByte != nil {
		return nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "fresh-full runner declines recovery/retry/incremental/included-range/closed-prefix routes",
		}
	}
	if options.ReceiptMode != DiagnosticParserCoreReceiptSummary {
		return nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "fresh-full runner requires bounded summary receipts",
		}
	}
	if options.MaxDispatches == 0 {
		options.MaxDispatches = 100000
	}
	if options.MaxTokens == 0 {
		options.MaxTokens = 100000
	}
	options.freshSchedulerSession = true

	lang, err := authenticatedParserCoreGoLanguage(scanner)
	if err != nil {
		return nil, err
	}
	parser := NewParser(lang)
	tables, err := newParserCoreRootTables(parser)
	if err != nil {
		return nil, err
	}
	compact, err := core.New(tables, options.Limits)
	if err != nil {
		return nil, err
	}
	return &parserCoreFreshFullRunner{
		lang: lang, parser: parser, tables: tables, compact: compact, options: options,
	}, nil
}

func (r *parserCoreFreshFullRunner) executeSchedulerOpen(source []byte, compact *core.Core, reset bool) (*diagnosticParserCoreGenericScheduler, *dfaTokenSource, error) {
	if r == nil || r.parser == nil || r.lang == nil || r.tables == nil || compact == nil {
		return nil, nil, errors.New("parser-core fresh-full runner is incomplete")
	}
	if reset {
		if err := compact.Reset(); err != nil {
			return nil, nil, err
		}
	}
	tokenSource := r.parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		return nil, nil, errors.New("parser-core fresh-full runner: production DFA unavailable")
	}
	scheduler, err := executeDiagnosticParserCoreGenericSchedulerFromSeed(
		compact, tokenSource, &r.scannerScratch, r.lang.InitialState,
		r.options, diagnosticParserCoreSeedObserver{},
	)
	if err != nil {
		tokenSource.Close()
		return nil, nil, err
	}
	if err := requireParserCoreFreshFullAcceptance(scheduler, len(source)); err != nil {
		tokenSource.Close()
		return nil, nil, err
	}
	return scheduler, tokenSource, nil
}

func requireParserCoreFreshFullAcceptance(scheduler *diagnosticParserCoreGenericScheduler, sourceBytes int) error {
	if scheduler == nil || scheduler.receipt == nil || scheduler.receipt.Acceptance == nil || scheduler.acceptedHead.Node == 0 {
		return errors.New("parser-core fresh-full runner did not accept EOF")
	}
	if sourceBytes < 0 || uint64(sourceBytes) > uint64(^uint32(0)) {
		return errors.New("parser-core fresh-full runner source exceeds uint32 offsets")
	}
	acceptance := scheduler.receipt.Acceptance
	wantEOF := uint32(sourceBytes)
	if acceptance.Token.Symbol != 0 || acceptance.Token.StartByte != wantEOF || acceptance.Token.EndByte != wantEOF ||
		acceptance.Token.Missing || acceptance.Token.NoLookahead || acceptance.Token.ExternalScannerToken ||
		!acceptance.Header.Header.Accepted || acceptance.Header.Header.Paused || acceptance.Header.Header.ExactPaths != 1 ||
		acceptance.Header.Header.ByteOffset != wantEOF || acceptance.Accepts != 1 || acceptance.Work.Accepts != 1 {
		return fmt.Errorf("parser-core fresh-full runner acceptance is not sole exact EOF: token=%+v header=%+v accepts=%d/%d",
			acceptance.Token, acceptance.Header.Header, acceptance.Accepts, acceptance.Work.Accepts)
	}
	return nil
}

func (r *parserCoreFreshFullRunner) executeScheduler(source []byte, compact *core.Core, reset bool) (*diagnosticParserCoreGenericScheduler, error) {
	scheduler, tokenSource, err := r.executeSchedulerOpen(source, compact, reset)
	if tokenSource != nil {
		tokenSource.Close()
	}
	return scheduler, err
}

func (r *parserCoreFreshFullRunner) materialize(source []byte, compact *core.Core, head core.Head) (*Tree, error) {
	if r == nil {
		return nil, errors.New("parser-core fresh-full runner is nil")
	}
	return materializeDiagnosticParserCoreAcceptedTree(compact, head, r.parser, source)
}

func (r *parserCoreFreshFullRunner) parse(source []byte) (*Tree, error) {
	if r == nil || r.compact == nil {
		return nil, errors.New("parser-core fresh-full runner is incomplete")
	}
	scheduler, tokenSource, err := r.executeSchedulerOpen(source, r.compact, true)
	if err != nil {
		return nil, err
	}
	defer tokenSource.Close()
	tree, err := r.materialize(source, r.compact, scheduler.acceptedHead)
	if err != nil {
		return nil, err
	}
	return tree, nil
}

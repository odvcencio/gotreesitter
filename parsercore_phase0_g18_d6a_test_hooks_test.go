//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"

// DiagnosticParserCoreFrontierObserverForTest receives one latest frontier
// snapshot after the producer seals it and before it publishes header state.
// The hook exists only in the focused parser-core test build.
type DiagnosticParserCoreFrontierObserverForTest func([]byte)

// DiagnosticParseWithDropCohortFrontierObserverForTest runs the cached
// candidate runner with certificate recording enabled. It does not enable
// certificate admission or alter the parser route result.
func DiagnosticParseWithDropCohortFrontierObserverForTest(
	parser *Parser,
	source []byte,
	observer DiagnosticParserCoreFrontierObserverForTest,
) (tree *Tree, published int, err error) {
	if parser == nil || parser.language == nil {
		return nil, 0, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "frontier observer requires a parser language",
		}
	}
	probeParser := NewParser(parser.language)
	runner, err := newAdmissionCandidateRunner(probeParser)
	if err != nil {
		return nil, 0, err
	}
	runner.certificateAdmissionEnabled = true
	seedObserver := diagnosticParserCoreSeedObserver{
		frontierPublished: func(scheduler *diagnosticParserCoreGenericScheduler, owner core.SchedulerTransactionToken) error {
			published++
			if observer != nil {
				observer(scheduler.compact.DiagnosticDropCohortFrontierSnapshotOwnedForTest(owner))
			}
			return nil
		},
	}
	// Keep the producer probe on a separate parser. Return the unchanged
	// production result from the caller's parser after the probe completes.
	_, _ = runner.parseWithObserver(source, seedObserver)
	tree, err = parser.Parse(source)
	return tree, published, err
}

// DiagnosticParseCandidateWithDropCohortFrontierModeForTest runs one cached
// candidate probe, then one production fallback with candidate routing held.
// It compares the same candidate route with recording disabled and enabled.
func DiagnosticParseCandidateWithDropCohortFrontierModeForTest(
	parser *Parser,
	source []byte,
	record bool,
	observer DiagnosticParserCoreFrontierObserverForTest,
) (tree *Tree, published int, candidateErr, err error) {
	if parser == nil || parser.language == nil {
		return nil, 0, nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "frontier mode probe requires a parser language",
		}
	}
	runner, err := parser.acquireAdmissionCandidateRunner()
	if err != nil {
		return nil, 0, nil, err
	}
	runner.certificateAdmissionEnabled = record
	seedObserver := diagnosticParserCoreSeedObserver{}
	if record {
		seedObserver.frontierPublished = func(scheduler *diagnosticParserCoreGenericScheduler, owner core.SchedulerTransactionToken) error {
			published++
			if observer != nil {
				observer(scheduler.compact.DiagnosticDropCohortFrontierSnapshotOwnedForTest(owner))
			}
			return nil
		}
	}
	var candidateTree *Tree
	candidateTree, candidateErr = runner.parseWithObserver(source, seedObserver)
	if candidateTree != nil {
		candidateTree.Release()
	}
	parser.admissionRouteSuppressed++
	tree, err = parser.Parse(source)
	parser.admissionRouteSuppressed--
	runner.certificateAdmissionEnabled = false
	return tree, published, candidateErr, err
}

//go:build gts_parsercorephase0

package gotreesitter

func RunStateDependentRelexSchedulerForTest(lang *Language, source []byte) (DiagnosticParserCoreGenericScheduler, error) {
	parser := NewParser(lang)
	runner, err := newAdmissionCandidateRunner(parser)
	if err != nil {
		return DiagnosticParserCoreGenericScheduler{}, err
	}
	runner.options.ReceiptMode = DiagnosticParserCoreReceiptFull
	tokenSource := parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		return DiagnosticParserCoreGenericScheduler{}, nil
	}
	defer tokenSource.Close()
	scheduler, runErr := executeDiagnosticParserCoreGenericSchedulerFromSeed(
		runner.compact,
		tokenSource,
		&runner.scannerScratch,
		lang.InitialState,
		runner.options,
		diagnosticParserCoreSeedObserver{},
	)
	if scheduler == nil || scheduler.receipt == nil {
		return DiagnosticParserCoreGenericScheduler{}, runErr
	}
	return *scheduler.receipt, runErr
}

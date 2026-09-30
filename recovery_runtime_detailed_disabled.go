//go:build !gts_recovery_telemetry

package gotreesitter

const recoveryRuntimeDetailedBuildEnabled = false

func (p *Parser) resetRecoveryRuntimeTelemetryDetailed() {}

func (p *Parser) clearRecoveryRuntimeTelemetryDetailed() {}

func (p *Parser) beginRecoveryRuntimeTelemetryDetailed() {}

func (p *Parser) finishRecoveryRuntimeTelemetryDetailed(*Tree, *ParseRuntime) {}

func (p *Parser) recordRecoveryRuntimeRetryTreeDetailed(*Tree, string, string) {}

func (p *Parser) recordRecoveryRuntimeSelectedTreeDetailed(*Tree) {}

func (p *Parser) recordRecoveryRuntimeCandidateReplacedDetailed(*Tree) {}

func (p *Parser) clearRecoveryRuntimeRetryTreesDetailed() {}

func (p *Parser) cCondenseAndResumeDetailed(
	stacks []glrStack,
	source []byte,
	ts TokenSource,
	tok Token,
	nodeCount *int,
	arena *nodeArena,
	entryScratch *glrEntryScratch,
	gssScratch *gssScratch,
	tmpEntries *[]stackEntry,
	parseScratch *parserScratch,
	trackChildErrors *bool,
	condenseClean ...bool,
) ([]glrStack, bool, Token, ParseStopReason) {
	return p.cCondenseAndResume(stacks, source, ts, tok, nodeCount, arena, entryScratch, gssScratch, tmpEntries, parseScratch, trackChildErrors, condenseClean...)
}

// DebugRecoveryRuntimeAttempts returns no attempt receipt in production builds.
func (p *Parser) DebugRecoveryRuntimeAttempts() RecoveryRuntimeAttempts { return nil }

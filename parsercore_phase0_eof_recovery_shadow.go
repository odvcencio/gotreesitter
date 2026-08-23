//go:build !gts_no_parsercorephase0 && gts_eof_history_census && gts_eof_recovery_shadow

package gotreesitter

import (
	"crypto/sha256"
	"errors"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func (s *diagnosticParserCoreGenericScheduler) censusEOFRecoveryShadow(
	headerIndex int,
	paths []core.Derivation,
	record *EOFAcceptHistoryHead,
) {
	if record == nil {
		return
	}
	receipt := &EOFRecoveryShadowReceipt{AcceptIndex: 1, Kind: "recover_eof"}
	record.RecoveryShadow = receipt
	if s == nil || s.compact == nil || headerIndex < 0 || headerIndex >= len(s.headers) ||
		!s.options.materializationContextSet || s.options.materializationParser == nil {
		receipt.Error = "no materialization context"
		return
	}
	if len(paths) != 1 {
		receipt.Error = "private EOF recovery requires one exact derivation"
		return
	}

	head := s.headers[headerIndex].head
	beforeHeader, err := diagnosticParserCoreHeaderReceipt(s.compact, s.headers[headerIndex])
	if err != nil {
		receipt.Error = err.Error()
		return
	}
	beforeStats, err := s.compact.Stats(head)
	if err != nil {
		receipt.Error = err.Error()
		return
	}
	beforeWork := s.compact.Work()
	defer func() {
		afterHeader, headerErr := diagnosticParserCoreHeaderReceipt(s.compact, s.headers[headerIndex])
		afterStats, statsErr := s.compact.Stats(head)
		receipt.LiveStateUnchanged = headerErr == nil && statsErr == nil &&
			afterHeader == beforeHeader && afterStats == beforeStats && s.compact.Work() == beforeWork
	}()

	shadow, root, forkReceipt, err := core.ForkDiagnosticEOFRecovery(s.compact, head, paths[0].Payloads)
	receipt.Steps = forkReceipt.Steps
	receipt.MaxSteps = forkReceipt.MaxSteps
	receipt.Payloads = forkReceipt.Payloads
	receipt.MaxPayloads = forkReceipt.MaxPayloads
	receipt.SourceFootprintBytes = forkReceipt.SourceFootprintBytes
	receipt.MaxCloneBytes = forkReceipt.MaxCloneBytes
	receipt.StartByte = forkReceipt.StartByte
	receipt.EndByte = forkReceipt.EndByte
	receipt.SubtreesBefore = forkReceipt.SubtreesBefore
	receipt.SubtreesAfter = forkReceipt.SubtreesAfter
	receipt.ChildrenBefore = forkReceipt.ChildrenBefore
	receipt.ChildrenAfter = forkReceipt.ChildrenAfter
	receipt.ExistingArenaPreserved = forkReceipt.ExistingArenaPreserved
	receipt.RootChildrenExact = forkReceipt.RootChildrenExact
	receipt.MutableStorageDisjoint = forkReceipt.MutableStorageDisjoint
	receipt.WorkBefore = forkReceipt.WorkBefore
	receipt.WorkAfter = forkReceipt.WorkAfter
	if err != nil {
		receipt.Error = err.Error()
		return
	}

	tree, err := materializeDiagnosticParserCoreAcceptedSelectionWithEOFRecoveryShadow(
		shadow,
		head,
		[]core.SubtreeID{root},
		s.options.materializationParser,
		s.options.materializationSource,
		nil,
		false,
		true,
		true,
	)
	if err != nil {
		receipt.Error = err.Error()
		return
	}
	defer tree.Release()
	if tree.root == nil {
		receipt.Error = errors.New("private EOF recovery materialized no root").Error()
		return
	}
	rootNode := tree.RootNode()
	receipt.RootSymbol = tree.root.symbol
	receipt.RootNamed = rootNode.IsNamed()
	receipt.RootExtra = rootNode.IsExtra()
	receipt.RootMissing = rootNode.IsMissing()
	receipt.RootIsError = rootNode.IsError()
	receipt.RootHasError = rootNode.HasError()
	receipt.RootDynamicPrecedence = tree.root.dynamicPrecedence
	receipt.RootShape = eofAcceptHistoryTreeShape(s.options.materializationParser.language, rootNode)
	receipt.DeepSHA256 = sha256.Sum256([]byte(receipt.RootShape))
	receipt.ErrorCost = cNodeErrorCostLang(s.options.materializationParser.language, tree.root)
}

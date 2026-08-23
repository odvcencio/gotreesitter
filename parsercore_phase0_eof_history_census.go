//go:build !gts_no_parsercorephase0 && gts_eof_history_census

package gotreesitter

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// EOFAcceptHistoryCandidate is one exact compact path at the authenticated
// EOF frontier, before the scheduler drops a no-action sibling.
type EOFAcceptHistoryCandidate struct {
	FoldIndex      int
	Score          int64
	BranchOrder    uint64
	HasBranchOrder bool
	PayloadCount   int
	Shape          string
	DeepSHA256     [32]byte
	MaterializeErr string
}

// EOFAcceptHistoryHead is one live compact history at the pre-drop frontier.
type EOFAcceptHistoryHead struct {
	HeaderIndex          int
	Accepting            bool
	NoAction             bool
	Header               DiagnosticParserCoreHeaderReceipt
	Candidates           []EOFAcceptHistoryCandidate
	RecoveryShadow       *EOFRecoveryShadowReceipt
	EnumerationTruncated bool
	EnumerationErr       string
}

// EOFRecoveryShadowReceipt records one private, bounded recover_eof fold.
// The G3 build supplies this receipt. Other G2 builds leave it nil.
type EOFRecoveryShadowReceipt struct {
	AcceptIndex            int
	Kind                   string
	Steps                  uint32
	MaxSteps               uint32
	Payloads               uint32
	MaxPayloads            uint32
	SourceFootprintBytes   uint64
	MaxCloneBytes          uint64
	StartByte              uint32
	EndByte                uint32
	SubtreesBefore         uint32
	SubtreesAfter          uint32
	ChildrenBefore         uint32
	ChildrenAfter          uint32
	ExistingArenaPreserved bool
	RootChildrenExact      bool
	RootSymbol             Symbol
	RootNamed              bool
	RootExtra              bool
	RootMissing            bool
	RootIsError            bool
	RootHasError           bool
	RootDynamicPrecedence  int32
	RootShape              string
	DeepSHA256             [32]byte
	ErrorCost              uint32
	MutableStorageDisjoint bool
	LiveStateUnchanged     bool
	WorkBefore             core.Work
	WorkAfter              core.Work
	Error                  string
}

// EOFAcceptHistoryFrontier records all compact histories at one certified
// EOF-sibling boundary. The census does not mutate or retain a live head.
type EOFAcceptHistoryFrontier struct {
	ElectionIndex     int
	Token             Token
	AcceptHeaderIndex int
	Heads             []EOFAcceptHistoryHead
}

var (
	eofAcceptHistoryCensusMu        sync.Mutex
	eofAcceptHistoryCensusFrontiers []EOFAcceptHistoryFrontier
)

// EOFAcceptHistoryCensusBuilt reports whether this binary includes the G2
// diagnostic census.
func EOFAcceptHistoryCensusBuilt() bool { return true }

// EOFAcceptHistoryCensusReset removes all recorded diagnostic frontiers.
func EOFAcceptHistoryCensusReset() {
	eofAcceptHistoryCensusMu.Lock()
	eofAcceptHistoryCensusFrontiers = nil
	eofAcceptHistoryCensusMu.Unlock()
}

// EOFAcceptHistoryCensusSnapshot returns an independent copy of the recorded
// frontiers. Candidate slices and shape strings do not alias the live census.
func EOFAcceptHistoryCensusSnapshot() []EOFAcceptHistoryFrontier {
	eofAcceptHistoryCensusMu.Lock()
	defer eofAcceptHistoryCensusMu.Unlock()
	out := make([]EOFAcceptHistoryFrontier, len(eofAcceptHistoryCensusFrontiers))
	for index, frontier := range eofAcceptHistoryCensusFrontiers {
		out[index] = frontier
		out[index].Heads = make([]EOFAcceptHistoryHead, len(frontier.Heads))
		for headIndex, head := range frontier.Heads {
			out[index].Heads[headIndex] = head
			out[index].Heads[headIndex].Candidates = append([]EOFAcceptHistoryCandidate(nil), head.Candidates...)
			if head.RecoveryShadow != nil {
				shadow := *head.RecoveryShadow
				out[index].Heads[headIndex].RecoveryShadow = &shadow
			}
		}
	}
	return out
}

func (s *diagnosticParserCoreGenericScheduler) censusEOFAcceptHistoryFrontier(
	acceptHeaderIndex int,
	noActionIndices []int,
) {
	record := EOFAcceptHistoryFrontier{
		ElectionIndex:     s.electionIndex,
		Token:             s.token,
		AcceptHeaderIndex: acceptHeaderIndex,
		Heads:             make([]EOFAcceptHistoryHead, len(s.headers)),
	}
	noAction := make(map[int]bool, len(noActionIndices))
	for _, index := range noActionIndices {
		noAction[index] = true
	}
	for index, header := range s.headers {
		headRecord := EOFAcceptHistoryHead{
			HeaderIndex: index,
			Accepting:   index == acceptHeaderIndex,
			NoAction:    noAction[index],
		}
		receipt, err := diagnosticParserCoreHeaderReceipt(s.compact, header)
		if err != nil {
			headRecord.EnumerationErr = err.Error()
			record.Heads[index] = headRecord
			continue
		}
		headRecord.Header = receipt
		paths, err := s.compact.Derivations(header.head)
		if errors.Is(err, core.ErrDerivationEnumerationCap) {
			headRecord.EnumerationTruncated = true
			record.Heads[index] = headRecord
			continue
		}
		if err != nil {
			headRecord.EnumerationErr = err.Error()
			record.Heads[index] = headRecord
			continue
		}
		order := eofAcceptHistoryFoldOrder(paths)
		if headRecord.NoAction {
			s.censusEOFRecoveryShadow(index, paths, &headRecord)
		}
		for foldIndex, pathIndex := range order {
			path := paths[pathIndex]
			candidate := EOFAcceptHistoryCandidate{
				FoldIndex:      foldIndex,
				Score:          path.Score,
				BranchOrder:    path.BranchOrder,
				HasBranchOrder: path.HasBranchOrder,
				PayloadCount:   len(path.Payloads),
			}
			if !s.options.materializationContextSet || s.options.materializationParser == nil {
				candidate.MaterializeErr = "no materialization context"
				headRecord.Candidates = append(headRecord.Candidates, candidate)
				continue
			}
			tree, err := materializeDiagnosticParserCoreAcceptedSelection(
				s.compact,
				header.head,
				path.Payloads,
				s.options.materializationParser,
				s.options.materializationSource,
				nil,
				s.options.materializationForceReplayParseStates,
				false,
			)
			if err != nil {
				candidate.MaterializeErr = err.Error()
				headRecord.Candidates = append(headRecord.Candidates, candidate)
				continue
			}
			candidate.Shape = eofAcceptHistoryTreeShape(s.options.materializationParser.language, tree.RootNode())
			candidate.DeepSHA256 = sha256.Sum256([]byte(candidate.Shape))
			tree.Release()
			headRecord.Candidates = append(headRecord.Candidates, candidate)
		}
		record.Heads[index] = headRecord
	}

	eofAcceptHistoryCensusMu.Lock()
	eofAcceptHistoryCensusFrontiers = append(eofAcceptHistoryCensusFrontiers, record)
	eofAcceptHistoryCensusMu.Unlock()
}

func eofAcceptHistoryFoldOrder(paths []core.Derivation) []int {
	order := make([]int, len(paths))
	for index := range paths {
		order[index] = index
	}
	sort.SliceStable(order, func(left, right int) bool {
		leftPath := paths[order[left]]
		rightPath := paths[order[right]]
		if leftPath.HasBranchOrder != rightPath.HasBranchOrder {
			return !leftPath.HasBranchOrder
		}
		return leftPath.HasBranchOrder && leftPath.BranchOrder < rightPath.BranchOrder
	})
	return order
}

func eofAcceptHistoryTreeShape(language *Language, node *Node) string {
	var builder strings.Builder
	eofAcceptHistoryWriteNode(&builder, language, node, "")
	return builder.String()
}

func eofAcceptHistoryWriteNode(builder *strings.Builder, language *Language, node *Node, field string) {
	if node == nil {
		builder.WriteString("(nil)")
		return
	}
	builder.WriteByte('(')
	if field != "" {
		builder.WriteString(field)
		builder.WriteByte(':')
	}
	builder.WriteString(node.Type(language))
	builder.WriteByte('[')
	builder.WriteString(strconv.FormatUint(uint64(node.StartByte()), 10))
	builder.WriteByte('-')
	builder.WriteString(strconv.FormatUint(uint64(node.EndByte()), 10))
	builder.WriteByte(']')
	if !node.IsNamed() {
		builder.WriteString("!anon")
	}
	if node.IsExtra() {
		builder.WriteString("!extra")
	}
	if node.IsMissing() {
		builder.WriteString("!missing")
	}
	if node.IsError() {
		builder.WriteString("!error")
	}
	if node.HasError() {
		builder.WriteString("!has-error")
	}
	for index := 0; index < node.ChildCount(); index++ {
		builder.WriteByte(' ')
		child := node.Child(index)
		fieldName := ""
		if child != nil {
			fieldName = node.FieldNameForChild(index, language)
		}
		eofAcceptHistoryWriteNode(builder, language, child, fieldName)
	}
	builder.WriteByte(')')
}

func (candidate EOFAcceptHistoryCandidate) String() string {
	if candidate.MaterializeErr != "" {
		return fmt.Sprintf("fold=%d score=%d materialize=%q", candidate.FoldIndex, candidate.Score, candidate.MaterializeErr)
	}
	return fmt.Sprintf("fold=%d score=%d sha256=%x", candidate.FoldIndex, candidate.Score, candidate.DeepSHA256)
}

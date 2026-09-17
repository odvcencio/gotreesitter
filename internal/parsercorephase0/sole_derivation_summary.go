package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
)

type derivationPathSummary struct {
	pathCount    uint64
	minimumScore int64
	maximumScore int64
}

type derivationSummaryNodeState uint8

const (
	derivationSummaryNodeVisiting derivationSummaryNodeState = iota + 1
	derivationSummaryNodeComplete
)

type derivationSummaryNode struct {
	id      NodeID
	state   derivationSummaryNodeState
	summary derivationPathSummary
}

type derivationSummaryLink struct {
	id     LinkID
	record linkRecord
}

type derivationSummaryScratch struct {
	core                     *Core
	nodes                    []derivationSummaryNode
	nodeIndexes              map[NodeID]int
	nodeIndexRetainedEntries int
	links                    []derivationSummaryLink
}

func (s *derivationSummaryScratch) begin(core *Core) {
	s.core = core
	s.nodes = s.nodes[:0]
	s.links = s.links[:0]
	clear(s.nodeIndexes)
}

func (s *derivationSummaryScratch) finish() {
	if len(s.nodeIndexes) > s.nodeIndexRetainedEntries {
		s.nodeIndexRetainedEntries = len(s.nodeIndexes)
	}
	clear(s.nodeIndexes)
	s.nodes = s.nodes[:0]
	s.links = s.links[:0]
	s.core = nil
}

func (s *derivationSummaryScratch) appendNode(id NodeID, state derivationSummaryNodeState) int {
	if s.nodeIndexes == nil {
		s.nodeIndexes = make(map[NodeID]int, 16)
	}
	index := len(s.nodes)
	s.nodes = append(s.nodes, derivationSummaryNode{id: id, state: state})
	s.nodeIndexes[id] = index
	return index
}

// appendNodeLinks validates one adjacency and stores it in insertion order.
// The linear duplicate scan preserves nodeLinks' cycle error without a map.
func (s *derivationSummaryScratch) appendNodeLinks(n nodeRecord) (int, int, error) {
	start := len(s.links)
	id := LinkID(n.firstLink)
	for id != 0 {
		for index := start; index < len(s.links); index++ {
			if s.links[index].id == id {
				return 0, 0, errors.New("parser-core phase zero: adjacency cycle")
			}
		}
		if uint64(id) > uint64(len(s.core.links)) {
			return 0, 0, errors.New("parser-core phase zero: link adjacency out of range")
		}
		link := s.core.links[id-1]
		if err := link.validateShape(); err != nil {
			return 0, 0, err
		}
		s.links = append(s.links, derivationSummaryLink{id: id, record: link})
		if uint64(len(s.links)-start) > uint64(n.linkCount) {
			return 0, 0, errors.New("parser-core phase zero: adjacency exceeds recorded link count")
		}
		id = link.next
	}
	end := len(s.links)
	if uint32(end-start) != n.linkCount {
		return 0, 0, errors.New("parser-core phase zero: adjacency shorter than recorded link count")
	}
	for left, right := start, end-1; left < right; left, right = left+1, right-1 {
		s.links[left], s.links[right] = s.links[right], s.links[left]
	}
	return start, end, nil
}

func (s *derivationSummaryScratch) walk(id NodeID) (derivationPathSummary, bool, error) {
	if index, ok := s.nodeIndexes[id]; ok {
		entry := s.nodes[index]
		if entry.state == derivationSummaryNodeVisiting {
			return derivationPathSummary{}, true, errors.New("parser-core phase zero: graph cycle")
		}
		return entry.summary, true, nil
	}
	n, err := s.core.node(id)
	if err != nil {
		return derivationPathSummary{}, true, err
	}
	if n.linkCount == 0 {
		if n.pathCount != 1 {
			return derivationPathSummary{}, true, errors.New("parser-core phase zero: malformed seed path count")
		}
		summary := derivationPathSummary{pathCount: 1}
		index := s.appendNode(id, derivationSummaryNodeComplete)
		s.nodes[index].summary = summary
		return summary, true, nil
	}
	entryIndex := s.appendNode(id, derivationSummaryNodeVisiting)
	// Validate the entire adjacency before following its insertion order.
	start, end, err := s.appendNodeLinks(*n)
	if err != nil {
		return derivationPathSummary{}, true, err
	}
	var summary derivationPathSummary
	for index := start; index < end; index++ {
		link := s.links[index].record
		prefix, complete, err := s.walk(link.prev)
		if err != nil || !complete {
			return derivationPathSummary{}, complete, err
		}
		// A prefix score may overflow before the later cap check stops enumeration.
		// Replay the original enumerator before inspecting either score extremum.
		if summary.pathCount > s.core.limits.MaxDerivations || prefix.pathCount > s.core.limits.MaxDerivations-summary.pathCount {
			return derivationPathSummary{}, false, nil
		}
		minimum, err := checkedAddScore(prefix.minimumScore, link.scoreDelta)
		if err != nil {
			return derivationPathSummary{}, true, err
		}
		maximum, err := checkedAddScore(prefix.maximumScore, link.scoreDelta)
		if err != nil {
			return derivationPathSummary{}, true, err
		}
		if summary.pathCount == 0 || minimum < summary.minimumScore {
			summary.minimumScore = minimum
		}
		if summary.pathCount == 0 || maximum > summary.maximumScore {
			summary.maximumScore = maximum
		}
		summary.pathCount += prefix.pathCount
	}
	if n.pathCount != math.MaxUint64 && summary.pathCount != n.pathCount {
		return derivationPathSummary{}, true, fmt.Errorf("parser-core phase zero: path-count mismatch: enumerated %d, recorded %d", summary.pathCount, n.pathCount)
	}
	s.nodes[entryIndex].state = derivationSummaryNodeComplete
	s.nodes[entryIndex].summary = summary
	return summary, true, nil
}

// summarizeDerivationPaths validates paths in the same order as derivations.
// A false Boolean requires the original enumerator to resolve cap precedence.
func (c *Core) summarizeDerivationPaths(head NodeID) (derivationPathSummary, bool, error) {
	scratch := &c.derivationSummaryScratch
	scratch.begin(c)
	defer scratch.finish()
	return scratch.walk(head)
}

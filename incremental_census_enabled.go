//go:build gts_incr_census

package gotreesitter

import (
	"sync"

	"github.com/odvcencio/gotreesitter/internal/diag/incrcensus"
)

const incrCensusEnabled = true

// The census is process-scoped and must run without concurrent parser calls.
// Serialization here protects observers from each other, not unrelated parses.
var incrCensusMu sync.Mutex
var incrCensusRecorder *incrcensus.Recorder
var incrCensusNodes map[*Node]uint64

func incrCensusEnter(reason string) int {
	if incrCensusRecorder == nil {
		return 0
	}
	return incrCensusRecorder.Enter(reason)
}
func incrCensusLeave(id int) {
	if id != 0 {
		incrCensusRecorder.Leave(id)
	}
}
func incrCensusDecision(n *Node, reason, outcome string) {
	if incrCensusRecorder != nil {
		incrCensusRecorder.Decision(incrCensusNodes[n], reason, outcome)
	}
}
func incrCensusFallback(reason string) {
	if incrCensusRecorder != nil {
		incrCensusRecorder.Fallback(reason)
	}
}
func incrCensusSwitch(reason string) {
	if incrCensusRecorder != nil {
		incrCensusRecorder.Switch(reason)
	}
}

// DiagnosticObserveIncrementalReuse records one edit operation. Call Tree.Edit
// inside run to include invalidation. The snapshot and final identity walk are
// outside the measured operation. Observation materializes lazy node views;
// use separate unobserved runs for throughput and allocation measurements.
// Only available with -tags gts_incr_census. Do not parse concurrently with it.
func DiagnosticObserveIncrementalReuse(oldTree *Tree, run func() (*Tree, error)) (tree *Tree, report incrcensus.Report, err error) {
	incrCensusMu.Lock()
	defer incrCensusMu.Unlock()
	incrCensusNodes = make(map[*Node]uint64)
	var old []incrcensus.Node
	type item struct {
		node   *Node
		parent uint64
	}
	var stack []item
	if oldTree != nil {
		stack = append(stack, item{node: oldTree.RootNode()})
	}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if v.node == nil {
			continue
		}
		if _, exists := incrCensusNodes[v.node]; exists {
			continue
		}
		id := uint64(len(old) + 1)
		incrCensusNodes[v.node] = id
		old = append(old, incrcensus.Node{ID: id, Parent: v.parent, Start: v.node.StartByte(), End: v.node.EndByte()})
		for i := v.node.ChildCount() - 1; i >= 0; i-- {
			stack = append(stack, item{v.node.Child(i), id})
		}
	}
	recorder := incrcensus.New()
	incrCensusRecorder = recorder
	defer func() { incrCensusRecorder = nil; incrCensusNodes = nil }()
	recorder.Start()
	tree, err = run()
	// Stop timing before walking the returned tree; that walk is diagnostic work.
	recorder.Stop()
	retained := make(map[uint64]bool)
	stack = stack[:0]
	if tree != nil {
		stack = append(stack, item{node: tree.RootNode()})
	}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if v.node == nil {
			continue
		}
		if id := incrCensusNodes[v.node]; id != 0 {
			retained[id] = true
		}
		for i := v.node.ChildCount() - 1; i >= 0; i-- {
			stack = append(stack, item{node: v.node.Child(i)})
		}
	}
	for node, id := range incrCensusNodes {
		old[id-1].Start = node.StartByte()
		old[id-1].End = node.EndByte()
	}
	report = recorder.Finish(old, retained)
	return
}
func incrCensusRejectIf(n *Node, reason string, reject bool) bool {
	outcome := "pass"
	if reject {
		outcome = "reject"
	}
	incrCensusDecision(n, reason, outcome)
	return reject
}
func incrCensusSkipIf(n *Node, reason string, skip bool) bool {
	outcome := "pass"
	if skip {
		outcome = "skip"
	}
	incrCensusDecision(n, reason, outcome)
	return skip
}
func incrCensusBlockedAt(offset uint32, reason string) {
	if incrCensusRecorder != nil {
		incrCensusRecorder.BlockedAt(offset, reason)
	}
}

func incrCensusDispatchReady() {
	if incrCensusRecorder != nil {
		incrCensusRecorder.DispatchReady()
	}
}

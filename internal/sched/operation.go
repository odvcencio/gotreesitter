package sched

// Phase names the work that caused an engine attempt. Nested verification and
// recovery retain their phase even when their parser retries internally.
type Phase uint8

const (
	Initial Phase = iota
	Compact
	Retry
	Fallback
	Verification
	Recovery
	Forest
)

// Work counts an attempt's work, including work whose result was discarded.
// Nodes counts constructed public nodes, including emergency heap roots.
// Bytes counts tracked growth beyond entry baselines and emergency roots.
type Work struct {
	Attempts   uint64
	Tokens     uint64
	Nodes      uint64
	Iterations uint64
	Bytes      uint64
}

func (w *Work) Add(other Work) {
	w.Attempts += other.Attempts
	w.Tokens += other.Tokens
	w.Nodes += other.Nodes
	w.Iterations += other.Iterations
	w.Bytes += other.Bytes
}

// OperationWork keeps total work and its disjoint phase attribution.
type OperationWork struct {
	Total        Work
	Initial      Work
	Compact      Work
	Retry        Work
	Fallback     Work
	Verification Work
	Recovery     Work
	Forest       Work
}

// Operation belongs to one public parse call. Sub-parsers borrow it; they must
// not reset the counters or replenish a configured work limit.
type Operation struct {
	Work             OperationWork
	NodeLimit        uint64
	IterationLimit   uint64
	StoppedReason    string
	MemoryLimit      int64
	MemoryConfigured bool
	RuntimeMemory    RuntimeMemory
	LoopNodesSpent   uint64
	LiveNodes        uint64
	LiveIterations   uint64
	LiveBytes        uint64
}

func (o *Operation) Add(phase Phase, work Work) {
	if o == nil {
		return
	}
	o.Work.Total.Add(work)
	var target *Work
	switch phase {
	case Compact:
		target = &o.Work.Compact
	case Retry:
		target = &o.Work.Retry
	case Fallback:
		target = &o.Work.Fallback
	case Verification:
		target = &o.Work.Verification
	case Recovery:
		target = &o.Work.Recovery
	case Forest:
		target = &o.Work.Forest
	default:
		target = &o.Work.Initial
	}
	target.Add(work)
}

func Remaining(limit, spent uint64) int {
	if spent >= limit {
		return 0
	}
	return int(limit - spent)
}

// RuntimeMemory preserves the process-heap baseline and poll cadence across
// attempts. Small sub-parses borrow a large caller's armed guard.
type RuntimeMemory struct {
	Armed                 bool
	Budget                int64
	Baseline, BaselineSys uint64
	Poll, VolumeAtPoll    uint64
	HardCeiling           int64
}

func (o *Operation) NodesSpent() uint64 {
	return max(o.Work.Total.Nodes, o.LoopNodesSpent) + o.LiveNodes
}
func (o *Operation) IterationsSpent() uint64 { return o.Work.Total.Iterations + o.LiveIterations }

// MemoryExceeded bounds live tracked growth across nested attempts. Released
// attempts remain in Work.Bytes, but their freed storage does not consume the
// memory limit. Retained capacity keeps each engine's existing baseline rule.
func (o *Operation) MemoryExceeded(growth uint64) bool {
	return o != nil && o.MemoryConfigured && o.MemoryLimit > 0 &&
		(o.LiveBytes >= uint64(o.MemoryLimit) || growth >= uint64(o.MemoryLimit)-o.LiveBytes)
}

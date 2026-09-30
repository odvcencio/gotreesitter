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
// Bytes counts tracked arena and scratch growth beyond their entry baselines.
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
	Work           OperationWork
	NodeLimit      uint64
	IterationLimit uint64
	StoppedReason  string
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

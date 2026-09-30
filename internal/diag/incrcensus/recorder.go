// Package incrcensus accounts for incremental reuse decisions in diagnostic builds.
// It has no dependency on the parser and is never imported by production builds.
package incrcensus

import (
	"sort"
	"time"
)

// Bound detailed events independently of parser progress. Every decision still
// contributes to aggregate rows and attribution after the detail buffer fills.
const maxDetailedEvents = 65536

type Node struct {
	ID     uint64 `json:"id"`
	Parent uint64 `json:"parent"`
	Start  uint32 `json:"start"`
	End    uint32 `json:"end"`
}
type Event struct {
	Sequence int    `json:"sequence"`
	Node     uint64 `json:"node,omitempty"`
	Offset   uint32 `json:"offset,omitempty"`
	Reason   string `json:"reason"`
	Outcome  string `json:"outcome"`
	Nanos    int64  `json:"nanos"`
}
type Row struct {
	Reason    string `json:"reason"`
	Nanos     int64  `json:"nanos"`
	Decisions uint64 `json:"decisions"`
	LostNodes uint64 `json:"lost_nodes"`
}
type Report struct {
	Schema        string `json:"schema"`
	EditNanos     int64  `json:"edit_nanos"`
	ObserveNanos  int64  `json:"observe_nanos"`
	OldNodes      uint64 `json:"old_nodes"`
	ReusedNodes   uint64 `json:"reused_nodes"`
	LostNodes     uint64 `json:"lost_nodes"`
	FinalFallback string `json:"final_fallback,omitempty"`
	Rows          []Row  `json:"rows"`
	// Events is a bounded chronological prefix; rows account for every event.
	Events        []Event `json:"events"`
	EventsDropped uint64  `json:"events_dropped,omitempty"`
}
type frame struct {
	reason  string
	started time.Time
}
type Recorder struct {
	report   Report
	frames   []frame
	rows     map[string]*Row
	rejected map[uint64]string
	blocked  map[uint32]string
	started  time.Time
}

func New() *Recorder {
	return &Recorder{report: Report{Schema: "gts-incremental-reuse-census/v1"}, rows: make(map[string]*Row), rejected: make(map[uint64]string), blocked: make(map[uint32]string)}
}
func (r *Recorder) row(reason string) *Row {
	p := r.rows[reason]
	if p == nil {
		p = &Row{Reason: reason}
		r.rows[reason] = p
	}
	return p
}

func (r *Recorder) event(event Event) {
	if len(r.report.Events) == maxDetailedEvents {
		r.report.EventsDropped++
		return
	}
	event.Sequence = len(r.report.Events) + 1
	r.report.Events = append(r.report.Events, event)
}
func (r *Recorder) Start() {
	r.started = time.Now()
	r.frames = append(r.frames, frame{"reparse_and_rebuild", r.started})
}
func (r *Recorder) tick(now time.Time, reason string) int64 {
	if len(r.frames) == 0 {
		return 0
	}
	f := &r.frames[len(r.frames)-1]
	n := now.Sub(f.started).Nanoseconds()
	if reason == "" {
		reason = f.reason
	}
	r.row(reason).Nanos += n
	f.started = now
	return n
}
func (r *Recorder) overhead(start time.Time) {
	now := time.Now()
	r.report.ObserveNanos += now.Sub(start).Nanoseconds()
	if len(r.frames) > 0 {
		r.frames[len(r.frames)-1].started = now
	}
}
func (r *Recorder) Enter(reason string) int {
	now := time.Now()
	r.tick(now, "")
	if reason == "fresh_parse" && r.report.FinalFallback != "" {
		reason = "fresh/" + r.report.FinalFallback
	}
	r.frames = append(r.frames, frame{reason, now})
	id := len(r.frames)
	r.overhead(now)
	return id
}
func (r *Recorder) Leave(id int) {
	now := time.Now()
	if id != len(r.frames) {
		panic("unbalanced incremental census phase")
	}
	r.tick(now, "")
	r.frames = r.frames[:len(r.frames)-1]
	r.overhead(now)
}

// Decision charges exclusive work since the preceding decision to this guard.
// It does not claim that rejecting a node caused all subsequent reparse work.
func (r *Recorder) Decision(node uint64, reason, outcome string) {
	now := time.Now()
	n := r.tick(now, reason)
	r.row(reason).Decisions++
	r.event(Event{Node: node, Reason: reason, Outcome: outcome, Nanos: n})
	if outcome == "reject" && node != 0 {
		r.rejected[node] = reason
	}
	r.overhead(now)
}

// BlockedAt records a position at which the dispatcher did not offer reuse.
// This observer has no evaluated guard timer; parsing time remains in its phase.
func (r *Recorder) BlockedAt(offset uint32, reason string) {
	now := time.Now()
	r.tick(now, "")
	reason = "dispatch/" + reason
	r.blocked[offset] = reason
	if len(r.frames) > 0 {
		f := &r.frames[len(r.frames)-1]
		if f.reason == "reparse_and_rebuild" || len(f.reason) >= 8 && f.reason[:8] == "reparse/" {
			f.reason = "reparse/" + reason
		}
	}
	r.row(reason).Decisions++
	r.event(Event{Offset: offset, Reason: reason, Outcome: "not_offered"})
	r.overhead(now)
}

// DispatchReady ends the interval in which the live frontier barred reuse.
func (r *Recorder) DispatchReady() {
	if len(r.frames) == 0 {
		return
	}
	f := &r.frames[len(r.frames)-1]
	if len(f.reason) >= 8 && f.reason[:8] == "reparse/" {
		r.Switch("reparse_and_rebuild")
	}
}
func (r *Recorder) Fallback(reason string) {
	r.Decision(0, reason, "fallback")
	r.report.FinalFallback = reason
	if len(r.frames) > 0 {
		r.frames[len(r.frames)-1].reason = "fresh/" + reason
	}
}
func (r *Recorder) Switch(reason string) {
	now := time.Now()
	r.tick(now, "")
	if len(r.frames) > 0 {
		r.frames[len(r.frames)-1].reason = reason
	}
	r.overhead(now)
}

// Finish partitions old nodes by identity in the selected result. A rejected
// ancestor supplies a reason only for descendants which really were not reused.
// The nearest rejected ancestor wins. Whole-edit fallbacks override candidate
// reasons only when the selected tree retains none of the old nodes.
func (r *Recorder) Stop() {
	now := time.Now()
	r.tick(now, "")
	r.report.EditNanos = now.Sub(r.started).Nanoseconds()
}

func (r *Recorder) Finish(old []Node, retained map[uint64]bool) Report {
	if len(r.frames) != 1 {
		panic("unfinished incremental census phase")
	}
	r.report.OldNodes = uint64(len(old))
	parents := make(map[uint64]uint64, len(old))
	for _, n := range old {
		parents[n.ID] = n.Parent
		if retained[n.ID] {
			r.report.ReusedNodes++
		}
	}
	for _, n := range old {
		if retained[n.ID] {
			continue
		}
		reason := "not_offered_or_selected"
		if r.report.ReusedNodes == 0 && r.report.FinalFallback != "" {
			reason = r.report.FinalFallback
		} else {
			if reasonForNode := r.rejected[n.ID]; reasonForNode != "" {
				reason = reasonForNode
			} else if blocked := r.blocked[n.Start]; blocked != "" {
				reason = blocked
			} else {
				for id := parents[n.ID]; id != 0; id = parents[id] {
					if v := r.rejected[id]; v != "" {
						reason = v
						break
					}
				}
			}
		}
		r.row(reason).LostNodes++
		r.report.LostNodes++
	}
	for _, v := range r.rows {
		r.report.Rows = append(r.report.Rows, *v)
	}
	sort.Slice(r.report.Rows, func(i, j int) bool {
		return r.report.Rows[i].Nanos > r.report.Rows[j].Nanos || r.report.Rows[i].Nanos == r.report.Rows[j].Nanos && r.report.Rows[i].Reason < r.report.Rows[j].Reason
	})
	return r.report
}

//go:build !gts_parsercorephase0 && !gts_workcount && !gts_diag

package incr

import "sync"

type ReuseObservation struct {
	Parser, Old, Candidate, Fresh     any
	Source                            []byte
	WouldCertify, SmallMode, Bypassed bool
}

var ReuseObserver struct {
	sync.RWMutex
	Call func(ReuseObservation)
}

const ReuseObserverEnabled = false

func ObserveReuse(any, []byte, any, any, any, bool, bool, bool) {}

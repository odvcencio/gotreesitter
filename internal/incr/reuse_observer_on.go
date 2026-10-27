//go:build gts_parsercorephase0 || gts_workcount || gts_diag

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

const ReuseObserverEnabled = true

func ObserveReuse(parser any, source []byte, old, candidate, fresh any, would, small, bypassed bool) {
	ReuseObserver.RLock()
	defer ReuseObserver.RUnlock()
	if fn := ReuseObserver.Call; fn != nil {
		fn(ReuseObservation{parser, old, candidate, fresh, source, would, small, bypassed})
	}
}

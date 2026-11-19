package ccore

// noLedger is the account that views without a request use: it has no limit, so the
// helpers of ledger.go call append and make as the code did before. It is never
// written.
var noLedger Ledger

// lg returns the ledger that pages and arrays claimed through this view are
// reserved on: the request's, or noLedger.
func (s *Store) lg() *Ledger {
	if s.led != nil {
		return s.led
	}
	return &noLedger
}

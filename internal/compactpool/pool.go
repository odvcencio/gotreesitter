// Package compactpool bounds idle compact parsing state owned by a language.
package compactpool

import "sync"

// Pool retains one idle value. Checked-out values have exclusive ownership;
// concurrent callers build independent values when the slot is empty. Unlike
// a global cache, a Pool lives and dies with its owning language.
type Pool struct {
	mu   sync.Mutex
	idle any
}

func (p *Pool) Get() any {
	p.mu.Lock()
	value := p.idle
	p.idle = nil
	p.mu.Unlock()
	return value
}

// Put retains value only if the idle slot is empty. Callers must reset it and
// relinquish every reference before calling Put.
func (p *Pool) Put(value any) {
	p.mu.Lock()
	if p.idle == nil {
		p.idle = value
	}
	p.mu.Unlock()
}

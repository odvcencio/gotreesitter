package sched

import "sync"

// RuntimePool lends an exclusively owned runtime and retains at most one idle
// runtime. Concurrent requests create their own runtimes when the slot is empty;
// returning a burst of them cannot increase idle retention. Keep the pool on the
// language so transient grammars and their runtimes can be collected together.
// Callers must detach request-owned references before returning a runtime.
type RuntimePool struct {
	mu   sync.Mutex
	idle any
}

func (p *RuntimePool) Take() any {
	p.mu.Lock()
	runtime := p.idle
	p.idle = nil
	p.mu.Unlock()
	return runtime
}

func (p *RuntimePool) Put(runtime any) {
	p.mu.Lock()
	if p.idle == nil {
		p.idle = runtime
	}
	p.mu.Unlock()
}

// Inspect reads idle storage while excluding checkout. The visitor must not
// retain the runtime or call back into the pool. It supports storage diagnostics
// without lending a mutable runtime to a second owner.
func (p *RuntimePool) Inspect(visit func(any)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	visit(p.idle)
}

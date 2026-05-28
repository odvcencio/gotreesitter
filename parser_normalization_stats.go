package gotreesitter

import "time"

type normalizationPassCounters struct {
	nodesVisited   uint64
	nodesRewritten uint64
}

type normalizationStats struct {
	passesChecked  uint64
	passesRun      uint64
	nodesVisited   uint64
	nodesRewritten uint64
	nanos          int64
	namedPasses    []normalizationNamedPassStats
}

type normalizationNamedPassStats struct {
	name           string
	checked        uint64
	run            uint64
	nodesVisited   uint64
	nodesRewritten uint64
	nanos          int64
}

func (p *Parser) resetNormalizationStats() {
	if p == nil {
		return
	}
	p.normalizationStats = normalizationStats{}
}

func (p *Parser) runNormalizationPass(enabled func() bool, fn func() normalizationPassCounters) {
	if enabled == nil {
		return
	}
	if p != nil {
		p.normalizationStats.passesChecked++
	}
	if p == nil {
		if enabled() {
			fn()
		}
		return
	}
	run := enabled()
	var counters normalizationPassCounters
	if run {
		p.normalizationStats.passesRun++
		counters = fn()
	}
	p.normalizationStats.nodesVisited += counters.nodesVisited
	p.normalizationStats.nodesRewritten += counters.nodesRewritten
}

func (p *Parser) runNamedNormalizationPass(name string, enabled func() bool, fn func() normalizationPassCounters) {
	if enabled == nil {
		return
	}
	if p == nil {
		if enabled() {
			fn()
		}
		return
	}
	p.normalizationStats.passesChecked++
	pass := p.normalizationStats.namedPass(name)
	pass.checked++
	if !enabled() {
		return
	}

	p.normalizationStats.passesRun++
	pass.run++
	start := time.Now()
	counters := fn()
	elapsed := time.Since(start).Nanoseconds()

	pass = p.normalizationStats.namedPass(name)
	p.normalizationStats.nodesVisited += counters.nodesVisited
	p.normalizationStats.nodesRewritten += counters.nodesRewritten
	p.normalizationStats.nanos += elapsed
	pass.nodesVisited += counters.nodesVisited
	pass.nodesRewritten += counters.nodesRewritten
	pass.nanos += elapsed
}

func (p *Parser) recordNormalizationMetric(name string, checked, run, nodesVisited, nodesRewritten uint64) {
	if p == nil {
		return
	}
	pass := p.normalizationStats.namedPass(name)
	pass.checked += checked
	pass.run += run
	pass.nodesVisited += nodesVisited
	pass.nodesRewritten += nodesRewritten
}

func (s *normalizationStats) namedPass(name string) *normalizationNamedPassStats {
	for i := range s.namedPasses {
		if s.namedPasses[i].name == name {
			return &s.namedPasses[i]
		}
	}
	s.namedPasses = append(s.namedPasses, normalizationNamedPassStats{name: name})
	return &s.namedPasses[len(s.namedPasses)-1]
}

func (p *Parser) copyNormalizationStats(rt *ParseRuntime) {
	if p == nil || rt == nil {
		return
	}
	rt.NormalizationPassesChecked = p.normalizationStats.passesChecked
	rt.NormalizationPassesRun = p.normalizationStats.passesRun
	rt.NormalizationNodesVisited = p.normalizationStats.nodesVisited
	rt.NormalizationNodesRewritten = p.normalizationStats.nodesRewritten
	rt.NormalizationNanos = p.normalizationStats.nanos
	if len(p.normalizationStats.namedPasses) > 0 {
		passes := make([]NormalizationPassRuntime, len(p.normalizationStats.namedPasses))
		for i, pass := range p.normalizationStats.namedPasses {
			passes[i] = NormalizationPassRuntime{
				Name:           pass.name,
				Checked:        pass.checked,
				Run:            pass.run,
				NodesVisited:   pass.nodesVisited,
				NodesRewritten: pass.nodesRewritten,
				Nanos:          pass.nanos,
			}
		}
		rt.NormalizationPasses = &passes
	}
}

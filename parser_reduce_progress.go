package gotreesitter

// reductionProgressGuard bounds reductions that do not shorten the stack.
// A new minimum depth proves progress through an existing reduction chain.
type reductionProgressGuard struct {
	minimumDepth int
	stalledSteps int
}

func (g *reductionProgressGuard) advance(depth int) bool {
	if depth < g.minimumDepth {
		g.minimumDepth = depth
		g.stalledSteps = 0
		return true
	}
	g.stalledSteps++
	return g.stalledSteps <= maxConsecutivePrimaryReduces
}

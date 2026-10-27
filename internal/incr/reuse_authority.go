package incr

// Grant is created only by the internal certified profile loader. The owner
// reference does not keep a language alive beyond its own lifecycle.
type ReuseAuthorityGrant struct{ owner any }

func NewReuseAuthorityGrant(owner any) *ReuseAuthorityGrant {
	return &ReuseAuthorityGrant{owner: owner}
}

func (g *ReuseAuthorityGrant) Matches(owner any) bool { return g != nil && g.owner == owner }

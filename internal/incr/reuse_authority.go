package incr

// Grant is created only by the internal certified profile loader. The owner
// reference does not keep a language alive beyond its own lifecycle.
type ReuseAuthorityGrant struct{ owner any }

func NewReuseAuthorityGrant(owner any) *ReuseAuthorityGrant {
	return &ReuseAuthorityGrant{owner: owner}
}

func (g *ReuseAuthorityGrant) Matches(owner any) bool { return g != nil && g.owner == owner }

// GobEncode deliberately omits the owner: authority belongs only to the
// certified in-memory language instance and cannot be transferred by encoding.
func (g *ReuseAuthorityGrant) GobEncode() ([]byte, error) { return []byte{1}, nil }

// GobDecode never grants authority, including when decoding into a reused value.
func (g *ReuseAuthorityGrant) GobDecode([]byte) error {
	g.owner = nil
	return nil
}

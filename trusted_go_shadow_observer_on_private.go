//go:build gts_parsercorephase0 || gts_workcount || gts_diag

package gotreesitter

import "sync"

type trustedGoReuseShadowEvent struct {
	parser                  *Parser
	source                  []byte
	old, candidate, fresh   *Tree
	wouldCertify, smallMode bool
	bypassed                bool
}

func observeAuthenticatedGoReuse(p *Parser, source []byte, old, tree *Tree) {
	trustedGoReuseShadowObserver.RLock()
	defer trustedGoReuseShadowObserver.RUnlock()
	if fn := trustedGoReuseShadowObserver.call; fn != nil {
		fn(trustedGoReuseShadowEvent{parser: p, source: source, old: old, candidate: tree, wouldCertify: true, smallMode: true, bypassed: true})
	}
}

var trustedGoReuseShadowObserver struct {
	sync.RWMutex
	call func(trustedGoReuseShadowEvent)
}

func observeTrustedGoReuseShadow(p *Parser, source []byte, old, tree, fresh *Tree, would bool) {
	trustedGoReuseShadowObserver.RLock()
	defer trustedGoReuseShadowObserver.RUnlock()
	if fn := trustedGoReuseShadowObserver.call; fn != nil {
		fn(trustedGoReuseShadowEvent{parser: p, source: source, old: old, candidate: tree, fresh: fresh, wouldCertify: would, smallMode: p.reuseCursor.smallFreshReuse})
	}
}

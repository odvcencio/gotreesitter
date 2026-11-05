package parsercorephase0

import "errors"

// SoleHeadPayload returns the payload on an exact single-link frontier.
// It does not walk the stack or interpret the payload's public projection.
func (c *Core) SoleHeadPayload(head Head) (SubtreeID, error) {
	n, nPaths, err := c.nodeWithPathCount(head.Node)
	if err != nil {
		return 0, err
	}
	if n.linkCount != 1 || nPaths != 1 {
		return 0, errors.New("parser-core phase zero: payload requires one exact frontier link")
	}
	if n.firstLink == 0 || uint64(n.firstLink) > uint64(c.links.Len()) {
		return 0, errors.New("parser-core phase zero: frontier link is invalid")
	}
	link := c.links.At(int(n.firstLink - 1))
	if link.next != 0 || link.prev == 0 || uint64(link.prev) > uint64(c.nodes.Len()) ||
		link.payload == 0 || uint64(link.payload) > uint64(c.subtrees.Len()) {
		return 0, errors.New("parser-core phase zero: frontier payload is invalid")
	}
	return link.payload, nil
}

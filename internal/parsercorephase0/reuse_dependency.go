package parsercorephase0

import "errors"

// SoleHeadPayload returns the payload on an exact single-link frontier.
// It does not walk the stack or interpret the payload's public projection.
func (c *Core) SoleHeadPayload(head Head) (SubtreeID, error) {
	n, err := c.node(head.Node)
	if err != nil {
		return 0, err
	}
	if n.linkCount != 1 || n.pathCount != 1 {
		return 0, errors.New("parser-core phase zero: payload requires one exact frontier link")
	}
	if n.firstLink == 0 || uint64(n.firstLink) > uint64(len(c.links)) {
		return 0, errors.New("parser-core phase zero: frontier link is invalid")
	}
	link := c.links[n.firstLink-1]
	if link.next != 0 || link.prev == 0 || uint64(link.prev) > uint64(len(c.nodes)) ||
		link.payload == 0 || uint64(link.payload) > uint64(len(c.subtrees)) {
		return 0, errors.New("parser-core phase zero: frontier payload is invalid")
	}
	return link.payload, nil
}

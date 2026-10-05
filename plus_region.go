package gowemf

// PlusRegion contains a preorder, index-linked binary tree. Terminal nodes use
// -1 child indexes; Path is populated only for RegionNodeDataTypePath. The flat
// representation avoids recursive parser calls on hostile trees.
type PlusRegion struct{ Nodes []PlusRegionNode }
type PlusRegionNode struct {
	Type        uint32
	Left, Right int
	Rect        Box
	Path        PlusPath
}
type regionSlot struct {
	parent int
	right  bool
	depth  uint32
}

func (c *cursor) plusRegion() PlusRegion {
	n := uint64(c.dword()) + 1
	if c.err != nil {
		return PlusRegion{}
	}
	// 256 bytes/node conservatively covers node structures and the traversal
	// stack on both supported word sizes; nested path allocations are separate
	// charges against the same remaining budget.
	if n > c.limits.MaxElements || n > c.limits.MaxObjectBytes/256 || n > uint64(int(^uint(0)>>1))/256 {
		c.err = failure(c.pos, "EMF+ region node budget", ErrLimit)
		return PlusRegion{}
	}
	if n > uint64(len(c.b)-c.pos)/4 {
		c.bad("EMF+ region node count")
		return PlusRegion{}
	}
	remaining := c.limits.MaxObjectBytes - n*256
	region := PlusRegion{Nodes: make([]PlusRegionNode, int(n))}
	stack := make([]regionSlot, 1, int(n))
	stack[0] = regionSlot{parent: -1, depth: 1}
	for i := range region.Nodes {
		if len(stack) == 0 {
			c.bad("extra EMF+ region nodes")
			break
		}
		slot := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if slot.depth > c.limits.MaxNesting {
			c.err = failure(c.pos, "EMF+ region nesting", ErrLimit)
			break
		}
		if slot.parent >= 0 {
			if slot.right {
				region.Nodes[slot.parent].Right = i
			} else {
				region.Nodes[slot.parent].Left = i
			}
		}
		node := &region.Nodes[i]
		node.Type = c.dword()
		node.Left, node.Right = -1, -1
		switch node.Type {
		case 1, 2, 3, 4, 5:
			if slot.depth >= c.limits.MaxNesting {
				c.err = failure(c.pos, "EMF+ region nesting", ErrLimit)
				break
			}
			if len(stack)+2 > len(region.Nodes)-i-1 {
				c.bad("missing EMF+ region children")
				break
			}
			stack = append(stack, regionSlot{i, true, slot.depth + 1}, regionSlot{i, false, slot.depth + 1})
		case 0x10000000:
			node.Rect = c.plusBox(false)
		case 0x10000001:
			length := c.long()
			if length < 0 {
				c.bad("negative region path size")
				break
			}
			raw := c.take(uint64(length))
			if c.err != nil {
				break
			}
			if uint64(len(raw)) > remaining || remaining == 0 {
				c.err = failure(c.pos, "EMF+ region path budget", ErrLimit)
				break
			}
			limits := c.limits
			limits.MaxObjectBytes = remaining
			v, err := DecodePlusObject(3, raw, limits)
			if err != nil {
				c.err = err
				break
			}
			node.Path = v.(PlusPath)
			if node.Path.Points.relative != nil {
				cost := uint64(len(node.Path.Points.relative))*16 + uint64(len(node.Path.Types))
				if cost > remaining {
					c.err = failure(c.pos, "EMF+ region path budget", ErrLimit)
					break
				}
				remaining -= cost
			}
		case 0x10000002, 0x10000003:
		default:
			c.unsupported()
		}
		if c.err != nil {
			break
		}
	}
	if c.err == nil && len(stack) != 0 {
		c.bad("incomplete EMF+ region tree")
	}
	return region
}

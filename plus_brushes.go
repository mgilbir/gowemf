package gowemf

type PlusTextureBrush struct {
	Flags, WrapMode uint32
	Transform       Matrix
	Image           *PlusImage
}
type PlusPathGradient struct {
	Flags, WrapMode, CenterColor uint32
	Center                       Point
	SurroundingColors            Integers
	BoundaryPoints               Points
	BoundaryPath                 *PlusPath
	Transform                    Matrix
	PresetPositions              []float64
	PresetColors                 Integers
	Blend                        PlusBlendFactors
	FocusScale                   Point
}

func (c *cursor) textureBrush() *PlusTextureBrush {
	t := &PlusTextureBrush{Flags: c.dword(), WrapMode: c.dword(), Transform: Identity()}
	if t.Flags & ^uint32(0x182) != 0 {
		c.unsupported()
		return t
	}
	if t.WrapMode > 4 {
		c.bad("texture wrap mode")
	}
	if t.Flags&2 != 0 {
		t.Transform = c.matrix()
	}
	if c.err == nil && len(c.b)-c.pos > 3 {
		if c.dword()>>12 != 0xdbc01 {
			c.bad("texture image signature")
		}
		image := c.plusImage()
		t.Image = &image
	}
	return t
}

func (c *cursor) pathGradient() *PlusPathGradient {
	g := &PlusPathGradient{Flags: c.dword(), WrapMode: c.dword(), CenterColor: c.dword(), Center: Point{c.float(), c.float()}, Transform: Identity()}
	if g.Flags & ^uint32(0xcf) != 0 {
		c.unsupported()
		return g
	}
	if g.WrapMode > 4 {
		c.bad("path gradient wrap mode")
	}
	if g.Flags&12 == 12 {
		c.bad("conflicting path-gradient blend patterns")
	}
	g.SurroundingColors = c.ints(uint64(c.dword()), 4)
	if g.Flags&1 != 0 {
		n := c.long()
		if n < 0 {
			c.bad("negative boundary path size")
			return g
		}
		raw := c.take(uint64(n))
		if c.err != nil {
			return g
		}
		remaining := c.limits.MaxObjectBytes - c.allocated
		if remaining == 0 || uint64(len(raw)) > remaining {
			c.err = failure(c.base+c.pos, "boundary path budget", ErrLimit)
			return g
		}
		limits := c.limits
		limits.MaxObjectBytes = remaining
		v, err := DecodePlusObject(3, raw, limits)
		if err != nil {
			if pe, ok := err.(*ParseError); ok {
				c.err = &ParseError{c.base + c.pos - len(raw) + pe.Offset, pe.Field, pe.Err}
			} else {
				c.err = err
			}
			return g
		}
		path := v.(PlusPath)
		if path.Points.relative != nil {
			if !c.allocation(uint64(len(path.Points.relative))*16+uint64(len(path.Types)), 1) {
				return g
			}
		}
		g.BoundaryPath = &path
	} else {
		n := c.long()
		if n < 0 {
			c.bad("negative boundary point count")
			return g
		}
		g.BoundaryPoints = c.points(uint64(n), 1)
	}
	if g.Flags&2 != 0 {
		g.Transform = c.matrix()
	}
	if g.Flags&4 != 0 {
		n := uint64(c.dword())
		g.PresetPositions = c.floats(n)
		g.PresetColors = c.ints(n, 4)
		c.proportions(g.PresetPositions)
	}
	if g.Flags&8 != 0 {
		g.Blend = c.blendFactors()
	}
	if g.Flags&0x40 != 0 {
		if c.dword() != 2 {
			c.bad("focus scale count")
		}
		g.FocusScale = Point{c.float(), c.float()}
		if g.FocusScale.X <= 0 || g.FocusScale.X >= 1 || g.FocusScale.Y <= 0 || g.FocusScale.Y >= 1 {
			c.bad("focus scale range")
		}
	}
	return g
}

package gowemf

func (s *streamState) wmfObjects(cmd *Command) error {
	r := cmd.Source
	typ := r.Type & 255
	ref := func(id uint32, kind uint8) error {
		if uint64(id) >= uint64(len(s.kinds)) || s.kinds[id] != kind {
			return malformed(r.Offset, "WMF object reference type")
		}
		return nil
	}
	switch typ {
	case 0x2a, 0x2b, 0x2c:
		return ref(cmd.Body.(Value).Value, kindRegion)
	case 0x28, 0x29:
		v := cmd.Body.(RegionPaint)
		if err := ref(v.Region, kindRegion); err != nil {
			return err
		}
		return ref(v.Brush, kindBrush)
	case 0x34:
		id := cmd.Body.(Value).Value
		if err := ref(id, kindPalette); err != nil {
			return err
		}
		s.paletteSelection = paletteReference{id, s.generations[id], true}
	case 0x36, 0x37, 0x39:
		selected := s.paletteSelection
		if !selected.selected {
			return malformed(r.Offset, "WMF palette update without selection")
		}
		if err := ref(selected.index, kindPalette); err != nil {
			return err
		}
		if s.generations[selected.index] != selected.generation {
			return malformed(r.Offset, "stale WMF palette selection")
		}
		v := cmd.Body.(Palette)
		v.Handle = selected.index
		if typ == 0x39 {
			s.paletteCounts[selected.index] = v.Count
		} else if uint64(v.Start)+uint64(v.Count) > uint64(s.paletteCounts[selected.index]) {
			return malformed(r.Offset, "WMF palette entry range")
		}
		cmd.Body = v
	}
	return nil
}

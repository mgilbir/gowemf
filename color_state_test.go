package gowemf

import (
	"testing"
)

func TestStreamColorStateSnapshots(t *testing.T) {
	create := colorSpaceRecord(false).Raw
	data := emfFixture(create, emfRecord(EMRSetColorSpace, longs(1)), emfRecord(EMRSetICMMode, longs(2)), emfRecord(EMRSaveDC, nil), emfRecord(EMRSetICMMode, longs(1)), emfRecord(EMRRestoreDC, longs(-1)), emfRecord(EMRDeleteColorSpace, longs(1)))
	var on, savedOff, restored, deleted *ColorPlaybackState
	if _, err := Stream(data, StreamOptions{}, func(c Command) error {
		switch c.Source.Type {
		case EMRSetICMMode:
			if c.Body.(Value).Value == 2 {
				on = c.ColorState
			} else {
				savedOff = c.ColorState
			}
		case EMRRestoreDC:
			restored = c.ColorState
		case EMRDeleteColorSpace:
			deleted = c.ColorState
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if on == nil || on.ICMMode != 2 || on.Source.Type != ColorCalibratedRGB || on.Source.Gamma[0] != 1 || savedOff.ICMMode != 1 || restored.ICMMode != 2 || restored.Source.Type != ColorCalibratedRGB || !deleted.Source.IsSRGB() {
		t.Fatal(on, savedOff, restored, deleted)
	}
	// Retained earlier commands must not be mutated by later selections/deletes.
	if on.Source.IsSRGB() || on.ICMMode != 2 {
		t.Fatal("color state snapshots were mutable")
	}
}

func TestStreamProfileAndProofState(t *testing.T) {
	profile := append(longs(1, 2, 0), []byte{'p', 0}...)
	proof := append(longs(1, 1, 2, 0), []byte{'q', 0}...)
	data := emfFixture(emfRecord(EMRSetICMProfileA, profile), emfRecord(EMRColorMatchToTargetW, proof), emfRecord(EMRColorMatchToTargetW, longs(2, 0, 0, 0)))
	seen := 0
	if _, err := Stream(data, StreamOptions{}, func(c Command) error {
		if c.Source.Type == EMRSetICMProfileA {
			if c.ColorState.OutputProfile == nil || c.ColorState.OutputProfile.Name[0] != 'p' {
				t.Fatal(c.ColorState)
			}
		}
		if c.Source.Type == EMRColorMatchToTargetW {
			seen++
			if (c.ColorState.Proof != nil) != (seen == 1) {
				t.Fatal(c.ColorState)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != 2 {
		t.Fatal(seen)
	}
}

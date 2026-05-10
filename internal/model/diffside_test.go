package model

import "testing"

// NewAppState must seed DiffCursor.Side to RIGHT so context-line Enter on
// a fresh PR posts to the after column (matching GitHub web's default).
func TestNewAppState_DiffCursorSideRight(t *testing.T) {
	s := NewAppState()
	if s.DiffCursor.Side != DiffSideRight {
		t.Fatalf("NewAppState DiffCursor.Side = %q, want %q", s.DiffCursor.Side, DiffSideRight)
	}
}

func TestDiffSideOpposite(t *testing.T) {
	if got := DiffSideRight.Opposite(); got != DiffSideLeft {
		t.Errorf("RIGHT.Opposite = %q, want LEFT", got)
	}
	if got := DiffSideLeft.Opposite(); got != DiffSideRight {
		t.Errorf("LEFT.Opposite = %q, want RIGHT", got)
	}
}

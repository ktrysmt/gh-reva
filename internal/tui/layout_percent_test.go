package tui

import "testing"

// TestSplitColumnWidths_DefaultPercent pins the new built-in default of
// 35% for the Comments column. Replaces the previous fixed `right = 57`
// at total ≥ 130 with a percentage that scales with the terminal width.
func TestSplitColumnWidths_DefaultPercent(t *testing.T) {
	cases := []struct {
		total            int
		wantLeft         int
		wantRight        int
		wantMidAtLeast   int
	}{
		{130, 42, 45, 25},
		{160, 42, 56, 25},
		{200, 42, 70, 25},
	}
	for _, c := range cases {
		l, mid, r := splitColumnWidths(c.total, false, defaultCommentsWidthPercent)
		if l != c.wantLeft {
			t.Errorf("total=%d: left=%d want %d", c.total, l, c.wantLeft)
		}
		if r != c.wantRight {
			t.Errorf("total=%d: right=%d want %d", c.total, r, c.wantRight)
		}
		if mid < c.wantMidAtLeast {
			t.Errorf("total=%d: mid=%d must be ≥ %d", c.total, mid, c.wantMidAtLeast)
		}
		if l+mid+r != c.total {
			t.Errorf("total=%d: sum=%d must equal total", c.total, l+mid+r)
		}
	}
}

// TestSplitColumnWidths_PercentOverride pins the customization path: a
// 50% override at total=200 places Comments at 100, Files at 42, Diff
// at the remainder.
func TestSplitColumnWidths_PercentOverride(t *testing.T) {
	l, mid, r := splitColumnWidths(200, false, 50)
	if r != 100 {
		t.Errorf("right=%d want 100 (50%% of 200)", r)
	}
	if l != 42 {
		t.Errorf("left=%d want 42 (Files stays fixed at the wide-terminal default)", l)
	}
	if mid != 200-42-100 {
		t.Errorf("mid=%d want %d", mid, 200-42-100)
	}
}

// TestSplitColumnWidths_PercentClampOnNarrowTerminal pins that even
// with a high percent, Diff never collapses below the existing mid-25
// floor (the readable-Diff floor that 80 ≤ total < 130 already
// honors). The override is best-effort, not a guarantee.
func TestSplitColumnWidths_PercentClampOnNarrowTerminal(t *testing.T) {
	l, mid, r := splitColumnWidths(100, false, 60)
	if mid < 25 {
		t.Errorf("mid=%d must stay ≥ 25 floor even under aggressive percent; l=%d r=%d", mid, l, r)
	}
	if l+mid+r != 100 {
		t.Errorf("sum=%d must equal total", l+mid+r)
	}
}

// TestSplitColumnWidths_HiddenIgnoresPercent pins that the Comments-
// hidden branch always returns right=0 regardless of percent.
func TestSplitColumnWidths_HiddenIgnoresPercent(t *testing.T) {
	for _, pct := range []int{20, 35, 50, 70} {
		_, _, r := splitColumnWidths(160, true, pct)
		if r != 0 {
			t.Errorf("pct=%d: hidden right=%d must be 0", pct, r)
		}
	}
}

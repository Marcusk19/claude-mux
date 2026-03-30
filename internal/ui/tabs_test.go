package ui

import (
	"strings"
	"testing"
)

func TestTabOrder(t *testing.T) {
	t.Run("Global is first tab (default)", func(t *testing.T) {
		if TabGlobal != 0 {
			t.Errorf("TabGlobal = %d, want 0 (should be default)", TabGlobal)
		}
	})

	t.Run("CC is last tab", func(t *testing.T) {
		if TabCC != Tab(len(tabNames)-1) {
			t.Errorf("TabCC = %d, want %d (should be last)", TabCC, len(tabNames)-1)
		}
	})

	t.Run("tab count matches tabNames", func(t *testing.T) {
		expected := 4
		if len(tabNames) != expected {
			t.Errorf("len(tabNames) = %d, want %d", len(tabNames), expected)
		}
	})

	t.Run("tabNames order matches constants", func(t *testing.T) {
		want := []string{"Global", "Local", "Worktrees", "CC"}
		for i, name := range want {
			if tabNames[i] != name {
				t.Errorf("tabNames[%d] = %q, want %q", i, tabNames[i], name)
			}
		}
	})
}

func TestRenderTabBar(t *testing.T) {
	t.Run("contains all tab names", func(t *testing.T) {
		bar := renderTabBar(TabGlobal, 80)
		for _, name := range tabNames {
			if !strings.Contains(bar, name) {
				t.Errorf("tab bar missing %q", name)
			}
		}
	})

	t.Run("tab names appear in order", func(t *testing.T) {
		bar := renderTabBar(TabGlobal, 80)
		lastIdx := -1
		for _, name := range tabNames {
			idx := strings.Index(bar, name)
			if idx < 0 {
				t.Fatalf("tab bar missing %q", name)
			}
			if idx <= lastIdx {
				t.Errorf("%q (pos %d) should appear after previous tab (pos %d)", name, idx, lastIdx)
			}
			lastIdx = idx
		}
	})

	t.Run("active tab styling uses bold", func(t *testing.T) {
		// Verify that the active style is configured with Bold
		if !activeTabStyle.GetBold() {
			t.Error("activeTabStyle should be bold")
		}
		if inactiveTabStyle.GetBold() {
			t.Error("inactiveTabStyle should not be bold")
		}
	})
}

func TestDefaultActiveTab(t *testing.T) {
	// The zero value of Tab should be TabGlobal, meaning new Models
	// default to the Global tab without explicit assignment.
	var defaultTab Tab
	if defaultTab != TabGlobal {
		t.Errorf("zero-value Tab = %d (%q), want TabGlobal (%d)", defaultTab, tabNames[defaultTab], TabGlobal)
	}
}

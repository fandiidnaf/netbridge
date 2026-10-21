package updater

import (
	"testing"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest   string
		current  string
		expected bool
	}{
		{"1.0.2", "1.0.1", true},
		{"1.1.0", "1.0.9", true},
		{"2.0.0", "1.9.9", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.0.1", false},
		{"1.0.1", "1.0.1", false},
		{"1.0.10", "1.0.9", true},
	}

	for _, tt := range tests {
		result := IsNewer(tt.latest, tt.current)
		if result != tt.expected {
			t.Errorf("IsNewer(%q, %q) = %v; want %v", tt.latest, tt.current, result, tt.expected)
		}
	}
}

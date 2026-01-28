// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import "testing"

func TestDeriveShortRegion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Standard European regions
		{"DE1 to DE", "DE1", "DE"},
		{"GRA7 to GRA", "GRA7", "GRA"},
		{"GRA9 to GRA", "GRA9", "GRA"},
		{"GRA11 to GRA", "GRA11", "GRA"},
		{"BHS5 to BHS", "BHS5", "BHS"},
		{"UK1 to UK", "UK1", "UK"},
		{"WAW1 to WAW", "WAW1", "WAW"},
		{"SBG5 to SBG", "SBG5", "SBG"},

		// US regions with hyphenated format
		{"US-EAST-VA-1 to US-EAST-VA", "US-EAST-VA-1", "US-EAST-VA"},

		// Already short format (no change)
		{"DE unchanged", "DE", "DE"},
		{"GRA unchanged", "GRA", "GRA"},
		{"BHS unchanged", "BHS", "BHS"},
		{"UK unchanged", "UK", "UK"},

		// Edge cases
		{"empty string", "", ""},
		{"single digit suffix", "A1", "A"},
		{"double digit suffix", "A12", "A"},
		{"triple digit suffix", "A123", "A"},

		// Hypothetical future regions
		{"GRA15 to GRA", "GRA15", "GRA"},
		{"DE2 to DE", "DE2", "DE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveShortRegion(tt.input)
			if result != tt.expected {
				t.Errorf("DeriveShortRegion(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

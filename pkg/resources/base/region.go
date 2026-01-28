// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import "regexp"

// regionSuffixPattern matches trailing digits with optional hyphen prefix.
// Examples: "1" in "DE1", "7" in "GRA7", "-1" in "US-EAST-VA-1"
var regionSuffixPattern = regexp.MustCompile(`-?\d+$`)

// DeriveShortRegion converts an OpenStack region code to an OVH Cloud region code.
//
// OVH uses two different region naming conventions:
//   - OpenStack APIs (compute, network): Long codes like DE1, GRA7, BHS5
//   - OVH Cloud APIs (storage, database): Short codes like DE, GRA, BHS
//
// This function strips the trailing availability zone number to derive the short form.
//
// Examples:
//   - DE1 → DE
//   - GRA7, GRA9, GRA11 → GRA
//   - BHS5 → BHS
//   - UK1 → UK
//   - WAW1 → WAW
//   - SBG5 → SBG
//   - US-EAST-VA-1 → US-EAST-VA
//   - DE → DE (already short, unchanged)
func DeriveShortRegion(region string) string {
	if region == "" {
		return ""
	}
	return regionSuffixPattern.ReplaceAllString(region, "")
}

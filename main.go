// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/platform-engineering-labs/formae/pkg/plugin/sdk"

func main() {
	sdk.RunWithManifest(&Plugin{}, sdk.RunConfig{})
}

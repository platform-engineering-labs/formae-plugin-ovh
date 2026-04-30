// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/platform-engineering-labs/formae/pkg/plugin/sdk"
)

func main() {
	pluginStartupLog("main: entry pid=%d args=%v", os.Getpid(), os.Args)
	pluginStartupLog("main: env FORMAE_AGENT_NODE=%q FORMAE_PLUGIN_NODE=%q FORMAE_REGISTRAR_PORT=%q FORMAE_NETWORK_COOKIE_set=%v FORMAE_VERSION=%q",
		os.Getenv("FORMAE_AGENT_NODE"),
		os.Getenv("FORMAE_PLUGIN_NODE"),
		os.Getenv("FORMAE_REGISTRAR_PORT"),
		os.Getenv("FORMAE_NETWORK_COOKIE") != "",
		os.Getenv("FORMAE_VERSION"))
	pluginStartupLog("main: calling sdk.RunWithManifest")
	sdk.RunWithManifest(&Plugin{}, sdk.RunConfig{})
	pluginStartupLog("main: sdk.RunWithManifest returned (plugin shutting down)")
}

// pluginStartupLog writes to stderr with a timestamp so the test harness's
// meta.Port stderr capture surfaces these lines. Used to diagnose readiness
// issues during the conformance OOB-delete phase, where the plugin process
// is spawned freshly and must announce itself within 30s.
func pluginStartupLog(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[ovh-startup %s] ", time.Now().UTC().Format(time.RFC3339Nano))
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintln(os.Stderr)
}

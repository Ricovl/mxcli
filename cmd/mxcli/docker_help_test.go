// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1629: `mxcli docker --help` listed "Mendix 11.6.1 or later"
// as a requirement of the whole command group. The floor is docker build's —
// a Portable App Distribution needs 11.6.1, and Build refuses anything older —
// while docker check has none: measured on a 10.24.18 project, it reports
// "The app contains: 0 errors." An agent takes the help's word for it, so a
// group-wide floor is a check it never tries.
func TestDockerHelp_VersionFloorIsBuildsNotChecks(t *testing.T) {
	for _, line := range strings.Split(dockerCmd.Long, "\n") {
		if strings.Contains(line, "11.6.1") && !strings.Contains(line, "build") {
			t.Errorf("docker --help states the 11.6.1 floor without scoping it to build: %q", line)
		}
	}
	for _, want := range []string{
		"Mendix 10",     // check has no version floor
		"exit status 4", // what a version-mismatched update-widgets looks like
	} {
		if !strings.Contains(dockerCheckCmd.Long, want) {
			t.Errorf("docker check --help does not mention %q", want)
		}
	}
}

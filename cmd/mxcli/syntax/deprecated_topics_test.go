// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// `mxcli syntax <topic> --deprecated` lists the entries filed under a topic,
// so every deprecated spelling must be filed under at least one topic, and
// every topic it names must be a registered syntax path. An entry added
// without a topic, or a topic renamed under it, would silently drop the
// spelling from every topic's list (ako/mxcli#714 decision 3).
func TestDeprecatedTopicsResolve(t *testing.T) {
	for _, e := range deprecation.All() {
		ts := e.Topics()
		if len(ts) == 0 {
			t.Errorf("%s (%s) is filed under no syntax topic: add it to topics in mdl/deprecation/topics.go", e.Code, e.Old)
		}
		for _, p := range ts {
			if !registeredPaths[p] {
				t.Errorf("%s is filed under %q, which is not a registered syntax path", e.Code, p)
			}
		}
	}
}

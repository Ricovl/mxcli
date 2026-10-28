// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
)

// mendixlabs/mxcli#1498: the check that a reference into a Protected (add-on)
// module names a Usable entity (CE1406, CE5400) needs the entity's stored
// export level, which the read dropped. Both members, so a read that returns a
// constant cannot pass.
func TestEntityFromGen_ReadsExportLevel(t *testing.T) {
	for _, lvl := range []string{"API", "Hidden"} {
		e := genDm.NewEntity()
		e.SetName("Request")
		e.SetExportLevel(lvl)
		if got := entityFromGen(e).ExportLevel; got != lvl {
			t.Errorf("stored ExportLevel %q read back as %q", lvl, got)
		}
	}
}

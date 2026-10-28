// SPDX-License-Identifier: Apache-2.0

package depfloor_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// floors are the lowest versions go.mod may require. Each one is a version a
// customer's dependency scan blocked a release on. The scan reads the module
// graph, not the binary, so a module counts even when no mxcli build compiles
// it.
var floors = []struct {
	module, min, why string
}{
	{"github.com/klauspost/compress", "v1.20.2", "GHSA-fpmh-frq8-7rj8 (zstd DecodeAll crash), fixed in v1.20.2"},
	{"go.mongodb.org/mongo-driver/v2", "v2.9.2", "CVE-2026-81521, fixed in v2.8.2, regressed in v2.9.0 and v2.9.1, restored in v2.9.2"},
}

// TestGoModKeepsSecurityFloors fails if go.mod no longer requires a floored
// module at or above its floor.
//
// klauspost/compress is the case this exists for. mxcli imports none of its
// packages; it arrives through mongo-driver/v2, which declares v1.19.2. A
// hand-written require line is removed by the next `go mod tidy`, which is how
// the v1.20.3 floor from ako/mxcli#1493 was lost in ako/mxcli#1497 without any
// build or test noticing. floor.go keeps the requirement alive across tidy.
func TestGoModKeepsSecurityFloors(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	required := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "require "))
		if len(f) >= 2 && strings.HasPrefix(f[1], "v") {
			required[f[0]] = f[1]
		}
	}
	for _, fl := range floors {
		got, ok := required[fl.module]
		switch {
		case !ok:
			t.Errorf("go.mod does not require %s, so the module graph falls back to whatever a dependency declares.\n"+
				"Floor %s: %s.\n"+
				"Keep its import in internal/depfloor/floor.go and run `go mod tidy`; a hand-written require line does not survive tidy.",
				fl.module, fl.min, fl.why)
		case less(got, fl.min):
			t.Errorf("go.mod requires %s %s, below the floor %s: %s", fl.module, got, fl.min, fl.why)
		}
	}
}

// less reports whether semantic version a sorts before b. It compares only
// vMAJOR.MINOR.PATCH, which is all the floors use; a pre-release suffix sorts
// as its release.
func less(a, b string) bool {
	pa, pb := parts(a), parts(b)
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func parts(v string) [3]int {
	var p [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, s := range strings.SplitN(v, ".", 3) {
		p[i], _ = strconv.Atoi(s)
	}
	return p
}

func TestLess(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v1.19.2", "v1.20.2", true},
		{"v1.20.2", "v1.20.2", false},
		{"v1.20.3", "v1.20.2", false},
		{"v1.9.0", "v1.10.0", true},
		{"v2.9.2-rc1", "v2.9.2", false},
	} {
		if got := less(c.a, c.b); got != c.want {
			t.Errorf("less(%s, %s) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

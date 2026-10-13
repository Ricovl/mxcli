// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// javadeps.go resolves a project's managed Java (JAR) dependencies.
//
// mxcli-formula1 findings #12: `ALTER MODULE X ADD JAR DEPENDENCY (…)` writes the
// coordinate to the model and `list jar dependencies` reports it — but the jar
// never reaches the classpath, and the only symptom is a runtime SQLException
// about a missing driver, long after the model looked right.
//
// The cause is not a bad write. Measured on 11.12.1: a full `mxbuild
// --target=deploy` produces a `deployment/build.gradle` with no dependencies
// block and downloads nothing, while `mx sync-java-dependencies <project.mpr>`
// fetches the jar into `vendorlib/`. Resolution is a separate step that Studio
// Pro runs for you when you edit Module Settings, and that nothing was running
// headless.

// SyncJavaDependencies runs `mx sync-java-dependencies` against projectPath,
// which downloads every declared managed dependency into the project's
// vendorlib/. It needs network access (Maven) and the mx binary.
//
// mxPath may be empty, in which case mx is resolved from the version's cache.
func SyncJavaDependencies(projectPath, mxPath, version string, w io.Writer) error {
	if mxPath == "" {
		mxPath = CachedMxPath(version)
	}
	if mxPath == "" {
		// No mx for this exact version. Fall back to any available one — a
		// newer mx reads an older project fine — but say so, because a
		// version mismatch is otherwise invisible until mx complains about
		// the mpr format and the message reads as a corrupt project.
		if resolved, err := ResolveMxForVersion("", version); err == nil && resolved != "" {
			mxPath = resolved
			if w != nil {
				fmt.Fprintf(w, "  Note: no mx for Mendix %s; using %s. Run 'mxcli setup mxbuild --version %s' if this misbehaves.\n",
					version, mxPath, version)
			}
		}
	}
	if mxPath == "" {
		return fmt.Errorf("mx not found for Mendix %s (needed to resolve managed Java dependencies); run 'mxcli setup mxbuild --version %s'", version, version)
	}

	cmd := exec.Command(mxPath, "sync-java-dependencies", projectPath)
	cmd.Dir = filepath.Dir(projectPath)
	PrepareMxCommand(cmd) // FreeType LD_PRELOAD workaround

	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	// mx runs Gradle; a daemon it starts keeps the pipe open after mx exits
	// (#1396). mx's own exit status is the result — ErrWaitDelay only says the
	// daemon outlived it.
	cmd.WaitDelay = orphanPipeWaitDelay
	if err := cmd.Run(); err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return fmt.Errorf("mx sync-java-dependencies failed: %w\n%s", err, lastLines(out.String(), 20))
	}
	if w != nil {
		if s := strings.TrimSpace(out.String()); s != "" {
			fmt.Fprintln(w, "  "+strings.ReplaceAll(s, "\n", "\n  "))
		}
	}
	return nil
}

// UnvendoredJarDependencies returns the coordinates ("group:artifact:version")
// the sync would vendor that have no matching jar in the project's vendorlib/,
// so a caller can sync only when something is actually missing.
//
// Only the resolved version of each artifact counts (see
// resolveJarDependencies): checking every declaration made a coordinate that
// Gradle had dropped "missing" forever, with no sync able to supply it (#1450).
//
// The vendored file name is `<artifact>-<version>.jar`, which is what
// `mx sync-java-dependencies` writes.
func UnvendoredJarDependencies(projectDir string, deps []JarDependencyRef) []string {
	var missing []string
	for _, d := range resolveJarDependencies(deps) {
		jar := filepath.Join(projectDir, "vendorlib", fmt.Sprintf("%s-%s.jar", d.Artifact, d.Version))
		if _, err := os.Stat(jar); err != nil {
			missing = append(missing, fmt.Sprintf("%s:%s:%s", d.Group, d.Artifact, d.Version))
		}
	}
	return missing
}

// JarDependencyRef is the coordinate of a managed Java dependency, decoupled
// from the model types so this package does not depend on the executor.
type JarDependencyRef struct {
	Group    string
	Artifact string
	Version  string
}

// resolveJarDependencies collapses the declarations to the one version of each
// group:artifact that `mx sync-java-dependencies` vendors, in first-declared
// order. Incomplete coordinates are dropped: they can be neither checked nor
// resolved.
//
// The sync runs Gradle over every module's declarations at once, and Gradle's
// conflict resolution keeps the newest version of an artifact declared more
// than once. Measured in #1450: ExcelImporter declaring commons-lang3 3.18.0 and
// CommunityCommons 3.20.0 leaves vendorlib/ holding commons-lang3-3.20.0.jar
// only, and a sync deletes a 3.18.0 jar placed there by hand.
func resolveJarDependencies(deps []JarDependencyRef) []JarDependencyRef {
	var out []JarDependencyRef
	index := map[string]int{} // group:artifact -> position in out
	for _, d := range deps {
		if d.Artifact == "" || d.Version == "" {
			continue
		}
		key := d.Group + ":" + d.Artifact
		if i, ok := index[key]; ok {
			if compareJarVersions(d.Version, out[i].Version) > 0 {
				out[i] = d
			}
			continue
		}
		index[key] = len(out)
		out = append(out, d)
	}
	return out
}

// SupersededJarDependencies lists the declarations the sync will not vendor
// because another module declares a newer version of the same artifact, as
// "group:artifact:version (resolved to newer)". They are not missing, but a
// module that needs exactly that version runs against the newer one, which is
// worth saying when nothing else in the output would.
func SupersededJarDependencies(deps []JarDependencyRef) []string {
	winner := map[string]string{}
	for _, d := range resolveJarDependencies(deps) {
		winner[d.Group+":"+d.Artifact] = d.Version
	}
	var out []string
	seen := map[string]bool{}
	for _, d := range deps {
		if d.Artifact == "" || d.Version == "" {
			continue
		}
		coord := fmt.Sprintf("%s:%s:%s", d.Group, d.Artifact, d.Version)
		w := winner[d.Group+":"+d.Artifact]
		if w == d.Version || seen[coord] {
			continue
		}
		seen[coord] = true
		out = append(out, fmt.Sprintf("%s (resolved to %s)", coord, w))
	}
	return out
}

// jarVersionSpecial ranks the qualifiers Gradle gives a meaning; any other
// non-numeric part ranks 0 and compares lexically against its peers.
var jarVersionSpecial = map[string]int{
	"dev": -1, "rc": 1, "snapshot": 2, "final": 3, "ga": 4, "release": 5, "sp": 6,
}

// compareJarVersions orders two versions the way Gradle's conflict resolution
// does: split on . - _ + and at digit/letter boundaries; numeric parts compare
// numerically and outrank non-numeric ones; a longer version wins when its
// extra part is numeric and loses when it is a qualifier (1.0-beta < 1.0).
// A string comparison gets the common case wrong: "3.9.0" > "3.20.0".
func compareJarVersions(a, b string) int {
	pa, pb := splitJarVersion(a), splitJarVersion(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if c := compareJarVersionPart(pa[i], pb[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(pa) > len(pb):
		if isJarVersionNumber(pa[len(pb)]) {
			return 1
		}
		return -1
	case len(pb) > len(pa):
		if isJarVersionNumber(pb[len(pa)]) {
			return -1
		}
		return 1
	}
	return 0
}

func compareJarVersionPart(x, y string) int {
	if x == y {
		return 0
	}
	xn, yn := isJarVersionNumber(x), isJarVersionNumber(y)
	switch {
	case xn && yn:
		x, y = strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
		if len(x) != len(y) {
			if len(x) < len(y) {
				return -1
			}
			return 1
		}
		return strings.Compare(x, y)
	case xn:
		return 1
	case yn:
		return -1
	}
	sx, xok := jarVersionSpecial[strings.ToLower(x)]
	sy, yok := jarVersionSpecial[strings.ToLower(y)]
	if xok || yok {
		if sx != sy {
			if sx < sy {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(x, y)
}

func splitJarVersion(v string) []string {
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	prevDigit := false
	for i, r := range v {
		switch {
		case r == '.' || r == '-' || r == '_' || r == '+':
			flush()
			continue
		case i > 0 && cur.Len() > 0 && (r >= '0' && r <= '9') != prevDigit:
			flush()
		}
		prevDigit = r >= '0' && r <= '9'
		cur.WriteRune(r)
	}
	flush()
	return parts
}

func isJarVersionNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

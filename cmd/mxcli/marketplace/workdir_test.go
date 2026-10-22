// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Path lengths that decide whether mx can build a reference project at all.
//
// mx refuses to write a file whose absolute path is 260 characters or longer,
// on every platform: Mendix's own FileSystem.OpenNewFileForWriting throws
// PathTooLongException, so Linux fails exactly as Windows does. Measured with
// mx 11.12.1 on Linux: create-project of the blank template succeeds under a
// 77-character directory and fails under a 78-character one, and module-import
// of a package carrying a 200-character entry succeeds at 259 and fails at 260
// with the reported "The specified file name or path is too long".
const (
	// blankTemplateDeepest is the longest relative path `mx create-project`
	// writes for the 11.12.1 blank app (NanoflowCommons 6.0.0's node_modules).
	blankTemplateDeepest = len("javascriptsource/nanoflowcommons/actions/node_modules/@react-native-async-storage/async-storage/macos/RNCAsyncStorage.xcodeproj/xcshareddata/xcschemes/RNCAsyncStorage-macOS.xcscheme")
	// nanoflowCommons7Deepest is the longest path NanoflowCommons 7.x bundles,
	// taken from the npm packages its package.json pins
	// (react-native-nitro-geolocation 1.4.3).
	nanoflowCommons7Deepest = len("javascriptsource/nanoflowcommons/actions/node_modules/react-native-nitro-geolocation/nitrogen/generated/android/kotlin/com/margelo/nitro/nitrogeolocation/Func_void_std__vector_ReverseGeocodedAddress_.kt")
)

// TestWorkRoot_FitsWhatTheReportedTempDirDidNot is mendixlabs/mxcli#1486
// (ako/mxcli#1343):
//
//	Extracting module files into C:\Users\<user>\AppData\Local\Temp\mxdiff1185592798\target-ref
//	The specified file name or path is too long, or a component of the specified path is too long.
//
// The work directory came from os.MkdirTemp("", ...), i.e. %TEMP%, which on
// Windows is deep inside the user profile. Under it, NanoflowCommons 7.x does
// not fit for any user name at all. The fix puts the work directory under the
// shortest root the user owns, and names what lives in it in one letter.
func TestWorkRoot_FitsWhatTheReportedTempDirDidNot(t *testing.T) {
	const (
		user    = "GlennCorvers"
		winTemp = `C:\Users\` + user + `\AppData\Local\Temp`
		winHome = `C:\Users\` + user
	)

	// The reported layout, and that it cannot fit. The random suffix of
	// os.MkdirTemp is up to ten digits.
	reported := winTemp + `\mxdiff1185592798\target-ref`
	if n := len(reported) + 1 + nanoflowCommons7Deepest; n < mxMaxPath {
		t.Fatalf("control: the reported layout should not fit NanoflowCommons 7 (%d < %d)", n, mxMaxPath)
	}

	root := workRoot("windows", winTemp, winHome)
	if want := winHome + `\.mxcli\t`; root != want {
		t.Fatalf("workRoot = %q, want %q", root, want)
	}
	// The deepest reference-project directory now: root\<prefix><10 digits>\<slot>.
	deepest := root + `\mxd1185592798\` + "t"
	if n := len(deepest) + 1 + nanoflowCommons7Deepest; n >= mxMaxPath {
		t.Errorf("NanoflowCommons 7 under %s is %d characters; mx refuses %d and more", deepest, n, mxMaxPath)
	}
	if n := len(deepest) + 1 + blankTemplateDeepest; n >= mxMaxPath {
		t.Errorf("the blank template under %s is %d characters; mx refuses %d and more", deepest, n, mxMaxPath)
	}
}

// TestWorkRoot_PrefersTheShortestRootTheUserOwns: the rule is "shortest wins",
// among roots private to the user — never a directory at the drive root, whose
// children inherit Modify for every authenticated user on Windows, and which
// would let another account swap the package between download and import.
func TestWorkRoot_PrefersTheShortestRootTheUserOwns(t *testing.T) {
	for _, tc := range []struct {
		name, goos, temp, home, want string
	}{
		{"windows default temp", "windows", `C:\Users\GlennCorvers\AppData\Local\Temp`, `C:\Users\GlennCorvers`, `C:\Users\GlennCorvers\.mxcli\t`},
		// The reporter's workaround keeps working: TMP is honoured when shorter.
		{"windows TMP set short", "windows", `C:\mxt`, `C:\Users\GlennCorvers`, `C:\mxt`},
		{"windows no home", "windows", `C:\Users\GlennCorvers\AppData\Local\Temp`, "", `C:\Users\GlennCorvers\AppData\Local\Temp`},
		{"macOS per-user TMPDIR", "darwin", "/var/folders/zz/zyxvpxvq6csfxvn_n0000000000000/T/", "/Users/glenn", "/tmp"},
		{"linux", "linux", "/tmp", "/home/glenn", "/tmp"},
		{"linux TMPDIR set long", "linux", "/run/user/1000/some/long/tmp", "/home/glenn", "/tmp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := workRoot(tc.goos, tc.temp, tc.home); got != tc.want {
				t.Errorf("workRoot(%s, %q, %q) = %q, want %q", tc.goos, tc.temp, tc.home, got, tc.want)
			}
		})
	}
}

// TestNewWorkDir_IsCreatedAndPrivate: the directory exists, sits under the
// chosen root, and on POSIX is the 0700 os.MkdirTemp makes.
func TestNewWorkDir_IsCreatedAndPrivate(t *testing.T) {
	dir, err := NewWorkDir("mxt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		t.Fatalf("work dir not created: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(dir), "mxt") {
		t.Errorf("work dir %s does not carry its prefix", dir)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o700 {
		t.Errorf("work dir mode = %v, want 0700", fi.Mode().Perm())
	}
}

// TestMxFailure_ExplainsAPathTooLong: mx's own message names neither the limit
// nor that the directory is mxcli's choice, so on its own it reads as a broken
// install. The hint says which directory, how long, and how to move it.
func TestMxFailure_ExplainsAPathTooLong(t *testing.T) {
	workDir := `C:\Users\GlennCorvers\AppData\Local\Temp\mxdiff1185592798\target-ref`
	out := []byte("Extracting module files into " + workDir + "\n" +
		"The specified file name or path is too long, or a component of the specified path is too long.\n")
	err := mxFailure("module-import", errors.New("exit status 208"), out, workDir)
	msg := err.Error()
	for _, want := range []string{"mx module-import failed", "exit status 208",
		"The specified file name or path is too long", workDir, "68 characters", "260", "TMP"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q; got:\n%s", want, msg)
		}
	}

	// Control: any other mx failure is passed through without the hint.
	other := mxFailure("module-import", errors.New("exit status 173"),
		[]byte("Project already contains a module with the name"), workDir)
	if strings.Contains(other.Error(), "TMP") {
		t.Errorf("hint attached to an unrelated failure:\n%s", other)
	}
}

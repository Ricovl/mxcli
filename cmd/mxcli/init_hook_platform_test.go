// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bootstrap script names the release asset to download from uname. Under
// Git Bash, MSYS2 and Cygwin `uname -s` is MINGW64_NT-10.0-26200 and friends,
// not "windows", and the Windows assets carry an .exe suffix
// (mendixlabs/mxcli#1680). These tests stand in a fake uname and a curl that
// records the URL it was asked for.

// fakeUname writes a uname reporting kernel name sys and machine arch.
func fakeUname(t *testing.T, bin, sys, arch string) {
	t.Helper()
	body := "#!/bin/sh\ncase \"$1\" in\n  -s) echo '" + sys + "' ;;\n  -m) echo '" + arch + "' ;;\n  *) echo '" + sys + "' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "uname"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// urlRecordingCurl replaces the env's curl with one that also logs its URL.
func urlRecordingCurl(t *testing.T, e bootEnv) {
	t.Helper()
	curl := "#!/bin/sh\nout=\nurl=\nwhile [ $# -gt 0 ]; do case \"$1\" in -o) out=$2; shift ;; -*) ;; *) url=$1 ;; esac; shift; done\n" +
		"echo \"curl $out $url\" >> \"$BOOT_LOG\"\n" +
		"printf '#!/bin/sh\\nif [ \"$1\" = \"--version\" ]; then echo \"mxcli version nightly-20991231-abcdef0\"; exit 0; fi\\necho \"nightly $*\" >> \"$BOOT_LOG\"\\n' > \"$out\"\n"
	if err := os.WriteFile(filepath.Join(e.bin, "curl"), []byte(curl), 0o755); err != nil {
		t.Fatal(err)
	}
}

// downloadedURL is the URL the fake curl was asked for, or "".
func (e bootEnv) downloadedURL(t *testing.T) string {
	t.Helper()
	data, _ := os.ReadFile(e.log)
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) == 3 && f[0] == "curl" {
			return f[2]
		}
	}
	return ""
}

func TestBootstrapScript_DownloadsTheAssetForThePlatform(t *testing.T) {
	requireSh(t)
	const base = "https://github.com/mendixlabs/mxcli/releases/download/nightly/"
	cases := []struct {
		name, sys, arch, asset, binary string
	}{
		{"git bash", "MINGW64_NT-10.0-26200", "x86_64", "mxcli-windows-amd64.exe", "mxcli.exe"},
		{"git bash 32-bit shell", "MINGW32_NT-10.0-26200", "x86_64", "mxcli-windows-amd64.exe", "mxcli.exe"},
		{"msys2", "MSYS_NT-10.0-26200", "x86_64", "mxcli-windows-amd64.exe", "mxcli.exe"},
		{"cygwin", "CYGWIN_NT-10.0-26200", "x86_64", "mxcli-windows-amd64.exe", "mxcli.exe"},
		{"windows arm64", "MINGW64_NT-10.0-26200", "aarch64", "mxcli-windows-arm64.exe", "mxcli.exe"},
		// Controls: the platforms that already worked keep their asset and ./mxcli.
		{"linux", "Linux", "x86_64", "mxcli-linux-amd64", "mxcli"},
		{"macos", "Darwin", "arm64", "mxcli-darwin-arm64", "mxcli"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newBootEnv(t, "")
			urlRecordingCurl(t, e)
			fakeUname(t, e.bin, c.sys, c.arch)

			e.run(t)
			if got := e.downloadedURL(t); got != base+c.asset {
				t.Errorf("downloaded %q, want %q", got, base+c.asset)
			}
			if _, err := os.Stat(filepath.Join(e.project, c.binary)); err != nil {
				t.Errorf("./%s not installed: %v", c.binary, err)
			}
			// The rest of the script must run the binary it installed.
			data, _ := os.ReadFile(e.log)
			if !strings.Contains(string(data), "nightly run --local --setup --ensure-db") {
				t.Errorf("downloaded binary did not provision the app; log:\n%s", data)
			}
		})
	}
}

// `mxcli new` on Windows leaves the Linux build as ./mxcli, which Git Bash does
// not treat as executable. It must not survive beside ./mxcli.exe, where it
// would shadow it for `./mxcli` in Git Bash.
func TestBootstrapScript_WindowsReplacesNonExecutableProjectBinary(t *testing.T) {
	requireSh(t)
	e := newBootEnv(t, "")
	urlRecordingCurl(t, e)
	fakeUname(t, e.bin, "MINGW64_NT-10.0-26200", "x86_64")
	linuxBuild := filepath.Join(e.project, "mxcli")
	if err := os.WriteFile(linuxBuild, []byte("\x7fELF"), 0o644); err != nil {
		t.Fatal(err)
	}

	e.run(t)
	if _, err := os.Stat(filepath.Join(e.project, "mxcli.exe")); err != nil {
		t.Errorf("./mxcli.exe not installed: %v", err)
	}
	if _, err := os.Stat(linuxBuild); !os.IsNotExist(err) {
		t.Errorf("the non-executable ./mxcli was left beside ./mxcli.exe (stat err %v)", err)
	}
}

// mxcli on PATH under Windows is linked in as ./mxcli.exe, so cmd and
// PowerShell can run it too.
func TestBootstrapScript_WindowsLinksPathBinaryAsExe(t *testing.T) {
	requireSh(t)
	e := newBootEnv(t, "")
	fakeUname(t, e.bin, "MINGW64_NT-10.0-26200", "x86_64")
	fakeMxcli(t, filepath.Join(e.bin, "mxcli"), "v0.26.0")

	e.run(t)
	if e.downloaded(t) {
		t.Error("downloaded although mxcli is on PATH")
	}
	if _, err := os.Stat(filepath.Join(e.project, "mxcli.exe")); err != nil {
		t.Errorf("./mxcli.exe not linked in: %v", err)
	}
	data, _ := os.ReadFile(e.log)
	if !strings.Contains(string(data), "v0.26.0 run --local --setup --ensure-db") {
		t.Errorf("the linked binary did not provision the app; log:\n%s", data)
	}
}

// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// ako/mxcli#1340 (mirror of mendixlabs/mxcli#1483), verbatim: "`mxcli run
// --local` sometimes stops for more than 10 minutes, without any output, at:
// `Resolving 4 managed Java dependency/dependencies (...)...` ... The same
// happened in the first build of `mxcli new` (step 6)." and: "Each `mx
// sync-java-dependencies` ... leaves a Gradle 8.5 daemon ... running for hours.
// When the hang occurred, an older daemon from a previous run was alive and
// idle ... With `GRADLE_OPTS=-Dorg.gradle.daemon=false`, the sync finishes in
// about 12 seconds and leaves no daemon behind; I have not seen the hang since."
//
// A one-shot mx/mxbuild run gains nothing from a daemon — it exits when its
// one job is done — and a daemon is what an earlier run left behind to hang
// the next one. Every one-shot that runs Gradle gets the reporter's setting.

// cmdEnv is the environment cmd will run with, last value winning per key.
func cmdEnv(cmd *exec.Cmd) map[string]string {
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	m := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

func TestOneShotMxCommands_RunGradleWithoutADaemon(t *testing.T) {
	// The user's own GRADLE_OPTS (a proxy, here — needed to reach Maven at all
	// behind one) must survive, and so must PrepareMxCommand's LD_PRELOAD.
	t.Setenv("GRADLE_OPTS", "-Xmx2g -Dhttps.proxyHost=proxy.example")
	t.Setenv("LD_PRELOAD", "/opt/custom/libpreload.so")

	for name, cmd := range map[string]*exec.Cmd{
		"mx sync-java-dependencies (run --local, sync-java-deps)": syncJavaDepsCommand("/opt/mx/mx", "/work/App.mpr"),
		"mxbuild settle build (mxcli new step 6)":                 settleCommand("/opt/mx/mxbuild", "/jdk", "/work/App.mpr"),
	} {
		env := cmdEnv(cmd)
		want := "-Xmx2g -Dhttps.proxyHost=proxy.example -Dorg.gradle.daemon=false"
		if got := env["GRADLE_OPTS"]; got != want {
			t.Errorf("%s: GRADLE_OPTS = %q, want %q — without it the run attaches to (or leaves) a "+
				"Gradle daemon, and a stale one hangs the step silently (ako/mxcli#1340)", name, got, want)
		}
		if !strings.Contains(env["LD_PRELOAD"], "/opt/custom/libpreload.so") {
			t.Errorf("%s: LD_PRELOAD = %q, the caller's preload was dropped", name, env["LD_PRELOAD"])
		}
	}
}

func TestWithGradleNoDaemon(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		want string // GRADLE_OPTS the child sees
	}{
		{"absent", []string{"PATH=/bin"}, "-Dorg.gradle.daemon=false"},
		{"empty", []string{"GRADLE_OPTS="}, "-Dorg.gradle.daemon=false"},
		{"appended", []string{"GRADLE_OPTS=-Xmx1g"}, "-Xmx1g -Dorg.gradle.daemon=false"},
		// The user asked for a daemon explicitly: their call, not ours.
		{"explicit choice kept", []string{"GRADLE_OPTS=-Dorg.gradle.daemon=true"}, "-Dorg.gradle.daemon=true"},
		// exec keeps the last of duplicate keys, so that is the one to extend.
		{"duplicate key", []string{"GRADLE_OPTS=-Xold", "GRADLE_OPTS=-Xnew"}, "-Xnew -Dorg.gradle.daemon=false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := append([]string(nil), tc.env...)
			got := cmdEnv(&exec.Cmd{Env: withGradleNoDaemon(in)})["GRADLE_OPTS"]
			if got != tc.want {
				t.Errorf("GRADLE_OPTS = %q, want %q", got, tc.want)
			}
			if strings.Join(in, "\x00") != strings.Join(tc.env, "\x00") {
				t.Errorf("the input environment was mutated: %v", in)
			}
		})
	}
}

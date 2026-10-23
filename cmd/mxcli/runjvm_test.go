// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#131 follow-up: `alter settings configuration 'Default'
// (ExtraJvmParameters: '-Dhttps.proxyHost=…')` is what Studio Pro passes to the
// runtime JVM, and `run --local` ignored it — the proxy the issue was filed for
// was set in the model and absent from every local run, silently.

func jvmCfg(name, extra string, heap int) *model.ServerConfiguration {
	return &model.ServerConfiguration{Name: name, ExtraJvmParameters: extra, MaxJavaHeapSize: heap}
}

func TestConfigurationJVMArgs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cfg   *model.ServerConfiguration
		want  []string
		error string
	}{
		{name: "nothing set", cfg: jvmCfg("Default", "", 0), want: nil},
		{name: "as reported", cfg: jvmCfg("Default", "-Dhttps.proxyHost=127.0.0.1 -Dhttps.proxyPort=29758", 0),
			want: []string{"-Dhttps.proxyHost=127.0.0.1", "-Dhttps.proxyPort=29758"}},
		// Java heap ▸ Maximum size is in MB; 0 is Studio Pro's "Default".
		{name: "heap", cfg: jvmCfg("Default", "", 2048), want: []string{"-Xmx2048m"}},
		// The heap goes first, so an -Xmx among the parameters is the one that wins.
		{name: "heap and parameters", cfg: jvmCfg("Default", "-Xss2m", 512), want: []string{"-Xmx512m", "-Xss2m"}},
		{name: "quoted value with a space", cfg: jvmCfg("Default", `-Dapp.greeting="hello world"  -Dx='a b'`, 0),
			want: []string{"-Dapp.greeting=hello world", "-Dx=a b"}},
		{name: "unbalanced quote", cfg: jvmCfg("Default", `-Dapp.greeting="hello`, 0), error: "unbalanced quote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := configurationJVMArgs(tc.cfg)
			if tc.error != "" {
				if err == nil || !strings.Contains(err.Error(), tc.error) {
					t.Fatalf("want an error containing %q, got %v (args %v)", tc.error, err, got)
				}
				// Name the configuration and the property, so the fix is obvious.
				if !strings.Contains(err.Error(), "Default") || !strings.Contains(err.Error(), "ExtraJvmParameters") {
					t.Errorf("error does not say where the value lives: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("args = %q, want %q", got, tc.want)
			}
		})
	}
}

// The JVM parameters come from the same configuration as the constants — the
// one the run represents — not from whichever is first.
func TestResolveConstantChain_CarriesTheConfigurationsJVMArgs(t *testing.T) {
	ps := settingsWith(
		jvmCfg("Production", "-Dhttps.proxyHost=prod.example.invalid", 0),
		jvmCfg("Default", "-Dhttps.proxyHost=127.0.0.1", 1024),
	)
	chain, err := resolveConstantChain(ps, "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"-Xmx1024m", "-Dhttps.proxyHost=127.0.0.1"}; !slices.Equal(chain.JVMArgs, want) {
		t.Errorf("JVMArgs = %q, want Default's %q", chain.JVMArgs, want)
	}

	chain, err = resolveConstantChain(ps, "Production", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"-Dhttps.proxyHost=prod.example.invalid"}; !slices.Equal(chain.JVMArgs, want) {
		t.Errorf("JVMArgs = %q, want Production's %q", chain.JVMArgs, want)
	}
}

// A value the run cannot split is refused rather than passed on mangled: the JVM
// would then fail to start, or start with a property cut in half.
func TestResolveConstantChain_RefusesUnsplittableJVMParameters(t *testing.T) {
	ps := settingsWith(jvmCfg("Default", `-Dx="open`, 0))
	if _, err := resolveConstantChain(ps, "", nil, nil, nil); err == nil {
		t.Fatal("an unbalanced quote in ExtraJvmParameters was accepted")
	}
}

// The run says what it passes, so silence stops meaning "your proxy is in
// effect". A -D value is not printed: the parameters are also where a trust-store
// password goes, and run output lands in CI logs. `describe settings` shows them.
func TestReportJVMArgs(t *testing.T) {
	var buf bytes.Buffer
	reportJVMArgs(&buf, constantChain{
		Configuration: "Default",
		JVMArgs:       []string{"-Xmx2048m", "-Djavax.net.ssl.trustStorePassword=s3cr3t", "-Dhttps.proxyPort=29758"},
	})
	out := buf.String()
	for _, want := range []string{`configuration "Default"`, "-Xmx2048m", "-Djavax.net.ssl.trustStorePassword=…", "-Dhttps.proxyPort=…"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "s3cr3t") || strings.Contains(out, "29758") {
		t.Errorf("report printed a -D value:\n%s", out)
	}

	buf.Reset()
	reportJVMArgs(&buf, constantChain{Configuration: "Default"})
	if buf.Len() != 0 {
		t.Errorf("nothing to pass, but the report said %q", buf.String())
	}
}

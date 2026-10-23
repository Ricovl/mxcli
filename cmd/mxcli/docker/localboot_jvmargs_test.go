// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"slices"
	"testing"
)

// mendixlabs/mxcli#131 follow-up: a configuration's Extra JVM parameters (and its
// Java heap maximum) are what Studio Pro passes to the runtime JVM. `run --local`
// started the JVM with a fixed argument list, so a proxy set in the model with
// `alter settings configuration 'Default' (ExtraJvmParameters: …)` was silently
// absent from a local run.
func TestJVMArgs_CarriesConfigurationParameters(t *testing.T) {
	o := testLocalOpts()
	o.JVMArgs = []string{"-Xmx2048m", "-Dhttps.proxyHost=127.0.0.1", "-Dhttps.proxyPort=29758"}
	args := o.jvmArgs()

	jar := slices.Index(args, "-jar")
	if jar < 0 {
		t.Fatalf("no -jar in %v", args)
	}
	for _, want := range o.JVMArgs {
		i := slices.Index(args, want)
		if i < 0 {
			t.Errorf("jvmArgs missing %q; got %v", want, args)
			continue
		}
		// After -jar they would be the runtime's arguments, not the JVM's.
		if i > jar {
			t.Errorf("%q comes after -jar, so the JVM never sees it: %v", want, args)
		}
	}
	// After mxcli's own -D flags, so a parameter the configuration sets
	// deliberately wins (the JVM takes the last -D for a property).
	if slices.Index(args, "-Dhttps.proxyHost=127.0.0.1") < slices.Index(args, "-Dmendix.live-preview=enabled") {
		t.Errorf("configuration parameters precede mxcli's defaults: %v", args)
	}
}

// `mxcli test --local` boots through LocalAppOptions; it has to run with the same
// JVM parameters as `run --local`, as it already does with the same constants.
func TestLocalAppOptions_ForwardsJVMArgs(t *testing.T) {
	opts := LocalAppOptions{DeployDir: "/tmp/app/deployment", JVMArgs: []string{"-Xmx1024m"}}
	if rt := opts.runtimeOptions("/install/path"); !slices.Equal(rt.JVMArgs, opts.JVMArgs) {
		t.Fatalf("JVMArgs = %v, want %v to reach the runtime", rt.JVMArgs, opts.JVMArgs)
	}
}

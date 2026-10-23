// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// A configuration's Server tab carries JVM options — Java heap ▸ Maximum size
// (MaxJavaHeapSize, in MB; 0 is "Default") and Extra JVM parameters — which Studio
// Pro passes to the runtime JVM when it runs the app. `run --local` started the
// JVM with a fixed argument list, so a proxy set with
//
//	alter settings configuration 'Default' (ExtraJvmParameters: '-Dhttps.proxyHost=127.0.0.1 -Dhttps.proxyPort=8888');
//
// was in the model, round-tripped through `describe settings`, and absent from
// every local run — the case mendixlabs/mxcli#131 was filed for. Same shape as the
// constants in runconstants.go, and resolved from the same configuration.

// configurationJVMArgs returns a configuration's JVM options as arguments: the
// heap maximum first, so an -Xmx among the parameters is the one that wins, then
// the parameters split the way a shell would split them.
func configurationJVMArgs(cfg *model.ServerConfiguration) ([]string, error) {
	var args []string
	if cfg.MaxJavaHeapSize > 0 {
		args = append(args, fmt.Sprintf("-Xmx%dm", cfg.MaxJavaHeapSize))
	}
	params, err := splitJVMParameters(cfg.ExtraJvmParameters)
	if err != nil {
		return nil, fmt.Errorf("configuration %q: ExtraJvmParameters: %w\n"+
			"  hint: fix it with `alter settings configuration '%s' (ExtraJvmParameters: '…')`",
			cfg.Name, err, cfg.Name)
	}
	return append(args, params...), nil
}

// splitJVMParameters splits a parameter string on whitespace, keeping a single- or
// double-quoted run together and dropping the quotes, so
// `-Dapp.greeting="hello world"` is one argument. An unbalanced quote is an error:
// passed on, it would cut a property in half or stop the JVM from starting.
func splitJVMParameters(s string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		inArg   bool
		quote   rune
		quoteAt int
	)
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, quoteAt, inArg = r, i, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unbalanced quote at position %d in %q", quoteAt, s)
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}

// reportJVMArgs says which JVM options the run passes, and from which
// configuration, so silence means "none are set" rather than "yours are in
// effect". A -D value is not printed: the parameters are also where a trust-store
// password goes, and run output lands in CI logs; `describe settings` shows them.
func reportJVMArgs(w io.Writer, c constantChain) {
	if len(c.JVMArgs) == 0 {
		return
	}
	shown := make([]string, len(c.JVMArgs))
	for i, a := range c.JVMArgs {
		if name, _, isProp := strings.Cut(a, "="); isProp && strings.HasPrefix(a, "-D") {
			a = name + "=…"
		}
		shown[i] = a
	}
	fmt.Fprintf(w, "Passing %d JVM option(s) from configuration %q: %s\n",
		len(c.JVMArgs), c.Configuration, strings.Join(shown, " "))
}

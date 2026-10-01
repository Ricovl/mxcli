// SPDX-License-Identifier: Apache-2.0

// Package scriptdiff answers "what would `mxcli exec` write?" by running exec.
//
// `mxcli diff` used to answer it a second way: render each statement as MDL,
// render the stored document as MDL, and compare the text. Every place the two
// renderings disagreed without exec disagreeing was a phantom change (`Boolean`
// against the stored `Boolean default false`, `String` against
// `String(unlimited)`, a position, a re-laid-out flow), every document kind it
// had no renderer for was not compared at all (pages, translations, a layout
// repointed), and it never ran a statement, so it neither saw what earlier
// statements create nor refused what exec refuses (ako/mxcli#907, #807, #856).
//
// Here the script is executed — by exec's own code, under its own language
// header — against a scratch copy of the project, and the copy is compared with
// the project unit by unit. The units that differ are, by construction, the
// units exec would write: there is no second verdict to keep in step with the
// first. DESCRIBE renders each one on both sides so the change can be read as
// MDL.
package scriptdiff

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// Options configures a run.
type Options struct {
	// NewBackend opens the scratch copy. It must be a file engine: the script is
	// executed for real, and a backend that writes somewhere other than the
	// copy (Studio Pro over MCP) would apply it.
	NewBackend func() backend.FullBackend
	// ScriptDir is the directory of the script, which relative paths inside it
	// name (exec's SetScriptDir).
	ScriptDir string
	// DescribeLanguage is the MDL the changes are rendered in; nil is describe's
	// default.
	DescribeLanguage *langver.Version
	// ContinueOnError runs every statement as `exec --continue-on-error` does,
	// instead of stopping at the first error.
	ContinueOnError bool
	// Preflight, when set, runs exec's pre-flight checks against the scratch
	// executor before anything is executed, writing its report to w, and
	// returns why exec would refuse the script ("" when it would run it).
	Preflight func(scratch *executor.Executor, w io.Writer) string
}

// Report is what exec would do.
type Report struct {
	// Refused is why exec's pre-flight would refuse the script, writing
	// nothing; "" when it would run it.
	Refused string
	// PreflightOutput is what the pre-flight printed.
	PreflightOutput string
	// ExecErr is the error exec would stop on (or, with ContinueOnError, the
	// error that ended the run). Units still lists what it wrote before it.
	ExecErr error
	// Failures are the statement failures a ContinueOnError run reports.
	Failures string
	// Output is what exec printed.
	Output string
	// Units are the units exec would add, rewrite, move or remove.
	Units []UnitChange
	// Files are the files next to the model it would write or delete.
	Files []FileChange
	// Results render Units as MDL, an entry per document (per entity and
	// association for a domain model).
	Results []executor.DiffResult
}

// Run executes prog against a scratch copy of the project at mprPath and
// reports what it wrote there. The project at mprPath is only read.
func Run(mprPath string, prog *ast.Program, opts Options) (*Report, error) {
	if opts.NewBackend == nil {
		return nil, errors.New("scriptdiff: no backend to open the scratch copy with")
	}
	src, err := filepath.Abs(mprPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(src); err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp("", "mxcli-diff-")
	if err != nil {
		return nil, fmt.Errorf("create scratch folder: %w", err)
	}
	defer os.RemoveAll(scratch)
	copyDir := filepath.Join(scratch, "project")
	if err := copyProject(filepath.Dir(src), copyDir); err != nil {
		return nil, fmt.Errorf("copy the project to %s: %w", copyDir, err)
	}
	copyMpr := filepath.Join(copyDir, filepath.Base(src))

	before, err := TakeSnapshot(copyMpr)
	if err != nil {
		return nil, err
	}

	rep := &Report{}
	var out bytes.Buffer
	x := newExecutor(&out, opts)
	if err := x.Execute(&ast.ConnectStmt{Path: copyMpr}); err != nil {
		x.Close()
		return nil, fmt.Errorf("connect to the scratch copy: %w", err)
	}
	if opts.Preflight != nil {
		var pf bytes.Buffer
		rep.Refused = opts.Preflight(x, &pf)
		rep.PreflightOutput = pf.String()
	}
	if rep.Refused == "" {
		if opts.ContinueOnError {
			var fails bytes.Buffer
			_, rep.ExecErr = x.ExecuteProgramContinueOnError(prog, &fails)
			rep.Failures = fails.String()
		} else {
			rep.ExecErr = x.ExecuteProgram(prog)
		}
		if errors.Is(rep.ExecErr, executor.ErrExit) {
			rep.ExecErr = nil
		}
	}
	_ = x.Execute(&ast.DisconnectStmt{})
	x.Close()
	rep.Output = out.String()

	after, err := TakeSnapshot(copyMpr)
	if err != nil {
		return nil, err
	}
	rep.Units, rep.Files = before.Compare(after)
	if len(rep.Units) > 0 {
		r, err := newRenderer(src, copyMpr, before, after, opts)
		if err != nil {
			return nil, err
		}
		rep.Results = r.render(rep.Units)
		r.close()
	}
	return rep, nil
}

func newExecutor(w io.Writer, opts Options) *executor.Executor {
	x := executor.New(w)
	x.SetBackendFactory(opts.NewBackend)
	if opts.ScriptDir != "" {
		x.SetScriptDir(opts.ScriptDir)
	}
	if opts.DescribeLanguage != nil {
		x.SetDescribeLanguage(*opts.DescribeLanguage)
	}
	return x
}

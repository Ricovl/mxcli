// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// StubThenRealRule is the warning for a script set that declares one flow
// twice with `create or modify` (ako/mxcli#905): a placeholder ("stub") that
// earlier scripts can reference, then the real flow. Run in order, the stub
// rebuilds whatever is stored before the real statement restores it — on
// every run. When the two files end up under different language versions the
// mdl 0 stub still rebuilds the real flow while the mdl 1 real statement may
// be refused, and the project keeps running the placeholder with mx check
// clean. Since #843 a self-recursive flow is created in one statement, so the
// stub is no longer needed.
const StubThenRealRule = "MDL-STUB01"

// setScript is one parsed script of a run over several files.
type setScript struct {
	Path   string
	Source string
	Prog   *ast.Program
}

// flowDeclSite is one `create or modify microflow|nanoflow` in a script set.
type flowDeclSite struct {
	File     string
	Line     int // 1-based; 0 when the statement could not be located in the source
	Nanoflow bool
	Version  langver.Version
}

// flowRedeclaration is a flow that a script set declares more than once with
// `create or modify`, in source order (files in the order given).
type flowRedeclaration struct {
	Flow  string
	Sites []flowDeclSite
}

// files returns the distinct files of the redeclaration, in order.
func (r flowRedeclaration) files() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range r.Sites {
		if !seen[s.File] {
			seen[s.File] = true
			out = append(out, s.File)
		}
	}
	return out
}

// mixedVersions reports whether the sites are written in different language
// versions — the pair #905 found, where the mdl 0 stub rebuilds the real flow
// and the mdl 1 real statement is refused.
func (r flowRedeclaration) mixedVersions() bool {
	for _, s := range r.Sites[1:] {
		if s.Version != r.Sites[0].Version {
			return true
		}
	}
	return false
}

// findFlowRedeclarations returns every flow that two or more `create or
// modify microflow|nanoflow` statements of the set declare, whether in
// different files or twice in one. Microflows and nanoflows share one
// namespace, and Mendix document names are matched case-insensitively here,
// as the executor's lookups do. The result is ordered by first appearance.
func findFlowRedeclarations(scripts []setScript) []flowRedeclaration {
	byKey := map[string]*flowRedeclaration{}
	var order []string
	for _, sc := range scripts {
		if sc.Prog == nil {
			continue
		}
		seenInFile := map[string]int{}
		for _, stmt := range sc.Prog.Statements {
			var name ast.QualifiedName
			nano := false
			switch s := stmt.(type) {
			case *ast.CreateMicroflowStmt:
				if !s.CreateOrModify {
					continue
				}
				name = s.Name
			case *ast.CreateNanoflowStmt:
				if !s.CreateOrModify {
					continue
				}
				name, nano = s.Name, true
			default:
				continue
			}
			key := strings.ToLower(name.String())
			r, ok := byKey[key]
			if !ok {
				r = &flowRedeclaration{Flow: name.String()}
				byKey[key] = r
				order = append(order, key)
			}
			nth := seenInFile[key]
			seenInFile[key] = nth + 1
			r.Sites = append(r.Sites, flowDeclSite{
				File:     sc.Path,
				Line:     flowDeclLine(sc.Source, name, nth),
				Nanoflow: nano,
				Version:  sc.Prog.LanguageVersion,
			})
		}
	}
	var out []flowRedeclaration
	for _, k := range order {
		if r := byKey[k]; len(r.Sites) > 1 {
			out = append(out, *r)
		}
	}
	return out
}

// flowDeclLine finds the line of the nth (0-based) `create or modify|replace
// microflow|nanoflow <name>` in src. The AST carries no statement positions;
// a statement whose keywords are split over lines is not located (0).
func flowDeclLine(src string, name ast.QualifiedName, nth int) int {
	ident := func(s string) string { return `(?:"` + regexp.QuoteMeta(s) + `"|` + regexp.QuoteMeta(s) + `)` }
	re, err := regexp.Compile(`(?i)\bcreate\s+or\s+(?:modify|replace)\s+(?:microflow|nanoflow)\s+` +
		ident(name.Module) + `\s*\.\s*` + ident(name.Name) + `(?:[^\w]|$)`)
	if err != nil {
		return 0
	}
	for i, line := range strings.Split(src, "\n") {
		if re.MatchString(line) {
			if nth == 0 {
				return i + 1
			}
			nth--
		}
	}
	return 0
}

// site names a declaration for a message: file:line.
func (s flowDeclSite) String() string {
	if s.Line == 0 {
		return s.File
	}
	return fmt.Sprintf("%s:%d", s.File, s.Line)
}

// stubThenRealViolations is check's warning for each redeclared flow of a
// script set: both statements named, and the advice to drop the stub.
func stubThenRealViolations(redecls []flowRedeclaration) []linter.Violation {
	var out []linter.Violation
	for _, r := range redecls {
		var sites []string
		for _, s := range r.Sites {
			sites = append(sites, fmt.Sprintf("%s (%s)", s, s.Version))
		}
		msg := fmt.Sprintf("%s is declared by %d `create or modify` statements in this script set: %s. "+
			"Run in order, the first replaces what the last one stored, on every run",
			r.Flow, len(r.Sites), strings.Join(sites, ", "))
		if r.mixedVersions() {
			msg += "; and under different language versions the mdl 0 one rebuilds the flow while the mdl 1 one " +
				"can be refused, leaving the project running the placeholder with mx check clean — at the least give " +
				"them the same header (`mxcli fmt --upgrade -w -p app.mpr` over all the files decides it for them together)"
		}
		msg += ". A self-recursive flow no longer needs a placeholder (#843): drop the stub and keep the real statement"
		mod, doc := r.Flow, r.Flow
		if i := strings.IndexByte(r.Flow, '.'); i > 0 {
			mod, doc = r.Flow[:i], r.Flow[i+1:]
		}
		dt := "microflow"
		if r.Sites[0].Nanoflow {
			dt = "nanoflow"
		}
		out = append(out, linter.Violation{
			RuleID:   StubThenRealRule,
			Severity: linter.SeverityWarning,
			Message:  msg,
			Location: linter.Location{Module: mod, DocumentType: dt, DocumentName: doc},
		})
	}
	return out
}

// fileGroups joins the files that share a redeclared flow into groups
// (transitively), each listed in the order the files were given. A group of
// one file — a flow declared twice in the same file — is not returned: one
// file has one header.
func fileGroups(files []string, redecls []flowRedeclaration) [][]string {
	parent := map[string]string{}
	var find func(string) string
	find = func(f string) string {
		if p, ok := parent[f]; ok && p != f {
			r := find(p)
			parent[f] = r
			return r
		}
		parent[f] = f
		return f
	}
	for _, r := range redecls {
		fs := r.files()
		for _, f := range fs[1:] {
			parent[find(f)] = find(fs[0])
		}
	}
	idx := map[string]int{}
	for i, f := range files {
		idx[f] = i
	}
	byRoot := map[string][]string{}
	for f := range parent {
		root := find(f)
		byRoot[root] = append(byRoot[root], f)
	}
	var out [][]string
	for _, g := range byRoot {
		if len(g) < 2 {
			continue
		}
		sort.Slice(g, func(i, j int) bool { return idx[g[i]] < idx[g[j]] })
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return idx[out[i][0]] < idx[out[j][0]] })
	return out
}

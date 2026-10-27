// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// An XPath constraint cannot reference a constant. Mendix expressions write one
// as `@Module.Constant`, and the same spelling in a constraint reads like it
// should work — but mxbuild rejects it (ako/mxcli#1685). Measured on 11.13.0
// (the retrieve also on 11.15.0), each case alone in a fresh app:
//
//	retrieve … where [Total > @Shop.C_Limit]        CE0161  Integer constant
//	retrieve … where [Name = @Shop.C_Name]          CE0161  String constant
//	retrieve … where [Active = @Shop.C_Flag]        CE0161  Boolean constant
//	retrieve … where [Total > @Shop.Nope]           CE0161  no such constant — not CE1613:
//	                                                        XPath does not resolve the name
//	list view database from … [Total > @Shop.C_Limit]   CE0161
//	access rule … [Total > @Shop.C_Limit]           CE0161  (production security; with
//	                                                        security off mxbuild checks no
//	                                                        access rule at all)
//	user task targeting xpath [Name = @Wf.C_Name]   CE0161  ([Name = 'Reviewer'] clean)
//	declare $Limit Integer = @Shop.C_Limit;
//	retrieve … where [Total > $Limit]               clean   the control
//
// Until the XPath grammar had a rule for it, `@` parsed as a one-word name and
// the statement failed at the module name ("mismatched input 'Shop' expecting
// ']'"), which reads as a gap in MDL rather than a rule of the platform. Now it
// parses, and this says what is actually wrong and what to write instead.
//
// It needs no project: the refusal does not depend on whether the constant
// exists. An offline sync constraint is refused alike; it was not measured
// separately.

const xpathConstantRule = "MDL-XPATH03"

// validateXPathConstants reports every constant referenced inside an XPath
// constraint the script writes.
func validateXPathConstants(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		loc, where := xpathConstantLocation(stmt)
		severity := linter.SeverityError
		if _, _, excluded := excludedFlow(stmt); excluded {
			// Mendix does not validate an excluded document.
			severity = linter.SeverityWarning
		}
		seen := map[string]bool{}
		walkXPathSlots(stmt, func(slot, xpath string) {
			for _, name := range xpathConstantRefs(xpath) {
				if seen[slot+"\x00"+name] {
					continue
				}
				seen[slot+"\x00"+name] = true
				out = append(out, linter.Violation{
					RuleID:   xpathConstantRule,
					Severity: severity,
					Location: loc,
					Message: fmt.Sprintf(
						"%s%s: the XPath constraint uses the constant @%s — an XPath constraint cannot reference a constant, "+
							"and mxbuild rejects it with CE0161 \"Error(s) in XPath constraint.\"",
						slot, where, name),
					Suggestion: xpathConstantSuggestion(slot, name),
				})
			}
		})
	}
	return out
}

// xpathConstantSuggestion is what to write instead. Only a flow has variables;
// the other slots have nowhere to put the constant's value.
func xpathConstantSuggestion(slot, name string) string {
	short := name
	if i := strings.LastIndex(name, "."); i >= 0 {
		short = name[i+1:]
	}
	if slot == "retrieve" {
		return fmt.Sprintf("Assign the constant to a variable first and compare with the variable: "+
			"`declare $%s <its type> = @%s;`, then `$%s` in the constraint.", short, name, short)
	}
	return fmt.Sprintf("A %s has no variable to hold @%s. Write the value into the constraint, or retrieve "+
		"in a microflow, where the constant can be assigned to a variable first (a microflow data source).", slot, name)
}

// xpathConstantLocation names the document a statement writes, for the message.
func xpathConstantLocation(stmt ast.Statement) (linter.Location, string) {
	if g, ok := stmt.(*ast.GrantEntityAccessStmt); ok {
		return linter.Location{Module: g.Entity.Module, DocumentType: "entity", DocumentName: g.Entity.Name},
			" on " + g.Entity.String()
	}
	kind, name, _ := stmtCreateKind(stmt)
	if name == "" {
		return linter.Location{}, ""
	}
	loc := linter.Location{DocumentType: kind, DocumentName: name}
	if i := strings.Index(name, "."); i >= 0 {
		loc.Module, loc.DocumentName = name[:i], name[i+1:]
	}
	return loc, " in " + kind + " " + name
}

// xpathConstantRefs returns the qualified names of the constants a stored XPath
// constraint references — each `@Name.Name` outside a string literal — sorted
// and without duplicates. A literal is data (`'info@shop.example'`), and a
// doubled quote inside one is an escaped quote, not its end.
func xpathConstantRefs(xpath string) []string {
	set := map[string]bool{}
	inString := false
	for i := 0; i < len(xpath); i++ {
		c := xpath[i]
		if c == '\'' {
			if inString && i+1 < len(xpath) && xpath[i+1] == '\'' {
				i++
				continue
			}
			inString = !inString
			continue
		}
		if inString || c != '@' {
			continue
		}
		j := i + 1
		for j < len(xpath) && (isWordByte(xpath[j]) || xpath[j] == '.' || xpath[j] == '"') {
			j++
		}
		if name := strings.Trim(strings.ReplaceAll(xpath[i+1:j], `"`, ""), "."); name != "" {
			set[name] = true
		}
		i = j - 1
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

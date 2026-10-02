// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"strings"
	"testing"
)

// Studio Pro 11.15 replaced ped_check_errors' `documents[]` argument with
// `filters{documentType, documentNamePrefix, severities}` plus `pagination`, and
// made every tool schema additionalProperties-permissive. The old argument is
// therefore accepted and IGNORED: the check runs project-wide, so an error in any
// other document fails the statement being validated (measured live on
// 11.15.0-rc.4: an empty enumeration elsewhere in the module failed an entity
// create). These tests pin both shapes.

// the1115CheckErrors is ped_check_errors' input surface on Studio Pro 11.15.
var the1115CheckErrors = map[string][]string{"ped_check_errors": {"filters", "pagination"}}

// the1114CheckErrors is the surface up to 11.14 (serverInfo.version 1.0.0).
var the1114CheckErrors = map[string][]string{"ped_check_errors": {"documents"}}

func listing(from, to, total, checkID int, units ...string) string {
	return fmt.Sprintf("Listing problems %d-%d (out of %d). Check ID: %d\n%s", from, to, total, checkID, strings.Join(units, "\n"))
}

func unitBlock(name, docType string, problems ...string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "'%s' (%s):", name, docType)
	for _, p := range problems {
		sb.WriteString("\n- [error] " + p + " (at locations: -)")
	}
	return sb.String()
}

func TestCheckDocument_1115_SendsFiltersNotDocuments(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return "No errors found.", false
	})
	f.tools = the1115CheckErrors
	b := &Backend{client: f.connectClient(t)}

	if err := b.checkDocumentNow("Enumerations$Enumeration", "M.Colors"); err != nil {
		t.Fatalf("clean document reported dirty: %v", err)
	}
	call, ok := f.callByName("ped_check_errors")
	if !ok {
		t.Fatal("ped_check_errors was not called")
	}
	if _, has := call.Args["documents"]; has {
		t.Errorf("11.15 ignores `documents` (the check runs unscoped); sent %v", call.Args)
	}
	filters, _ := call.Args["filters"].(map[string]any)
	if filters["documentType"] != "Enumerations$Enumeration" || filters["documentNamePrefix"] != "M.Colors" {
		t.Errorf("filters = %v, want documentType + documentNamePrefix of the checked document", filters)
	}
}

// An error in ANOTHER document must not fail this one. documentNamePrefix is a
// prefix, so 'M.Colors' also matches 'M.ColorsOld' — the unit headers decide.
func TestCheckDocument_1115_IgnoresOtherDocumentsInTheListing(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return listing(1, 2, 2, 4,
			unitBlock("M.ColorsOld", "Enumerations$Enumeration", "Missing enumeration values."),
			unitBlock("M.Colors", "Constants$Constant", "Invalid Integer/Long value: 'x'."),
		), false
	})
	f.tools = the1115CheckErrors
	b := &Backend{client: f.connectClient(t)}

	if err := b.checkDocumentNow("Enumerations$Enumeration", "M.Colors"); err != nil {
		t.Fatalf("errors of other documents failed this one: %v", err)
	}
}

func TestCheckDocument_1115_ReportsTheCheckedDocumentsError(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return listing(1, 2, 2, 4,
			unitBlock("M.Other", "Constants$Constant", "Invalid Integer/Long value: 'x'."),
			unitBlock("M.Colors", "Enumerations$Enumeration", "Missing enumeration values."),
		), false
	})
	f.tools = the1115CheckErrors
	b := &Backend{client: f.connectClient(t)}

	err := b.checkDocumentNow("Enumerations$Enumeration", "M.Colors")
	if err == nil {
		t.Fatal("the checked document's error was not reported")
	}
	if !strings.Contains(err.Error(), "Missing enumeration values.") {
		t.Errorf("error = %v, want the document's own problem", err)
	}
	if strings.Contains(err.Error(), "Invalid Integer") {
		t.Errorf("error = %v, carries another document's problem", err)
	}
}

// A module singleton is listed under the module name.
func TestCheckDocument_1115_DomainModelUnit(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return listing(1, 1, 1, 7,
			unitBlock("MyFirstModule", "DomainModels$DomainModel", "Duplicate name 'E' in module 'MyFirstModule'."),
		), false
	})
	f.tools = the1115CheckErrors
	b := &Backend{client: f.connectClient(t)}

	if err := b.pedCheckErrors("MyFirstModule"); err == nil || !strings.Contains(err.Error(), "Duplicate name") {
		t.Fatalf("domain model error not reported: %v", err)
	}
}

// The first response lists at most 100 problems; the rest must be paged in with
// the check ID — and the filters, which a page request does not inherit
// (measured: a page without them lists only severity 'error' of the whole project).
func TestCheckDocument_1115_FetchesEveryPage(t *testing.T) {
	var others []string
	for i := 0; i < 100; i++ {
		others = append(others, unitBlock(fmt.Sprintf("M.Other%d", i), "Microflows$Microflow", "Something."))
	}
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		pg, _ := args["pagination"].(map[string]any)
		if pg == nil {
			return listing(1, 100, 101, 9, others...), false
		}
		if fmt.Sprint(pg["checkId"]) != "9" || fmt.Sprint(pg["offset"]) != "100" {
			return "unexpected page " + fmt.Sprint(pg), true
		}
		if _, ok := args["filters"]; !ok {
			return "page requested without the filters", true
		}
		return listing(101, 101, 101, 9, unitBlock("M.WF", "Workflows$Workflow", "Duplicate name 'ReviewOrder'.")), false
	})
	f.tools = the1115CheckErrors
	b := &Backend{client: f.connectClient(t)}

	err := b.checkDocumentNow("Workflows$Workflow", "M.WF")
	if err == nil || !strings.Contains(err.Error(), "Duplicate name 'ReviewOrder'") {
		t.Fatalf("an error on the second page was missed: %v", err)
	}
}

// A page window goes stale when the model changes between pages; the tool then
// says to fetch again without pagination.
func TestCheckDocument_1115_StaleWindowRefetches(t *testing.T) {
	first := 0
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		if args["pagination"] == nil {
			first++
			if first == 1 {
				return listing(1, 1, 2, 3, unitBlock("M.Other", "Constants$Constant", "x.")), false
			}
			return "No errors found.", false
		}
		return "The error page window is stale: new checks are available. Fetch errors again without pagination.", false
	})
	f.tools = the1115CheckErrors
	b := &Backend{client: f.connectClient(t)}

	if err := b.checkDocumentNow("Workflows$Workflow", "M.WF"); err != nil {
		t.Fatalf("stale window not re-fetched: %v", err)
	}
	if first != 2 {
		t.Errorf("unpaginated listing fetched %d times, want 2 (initial + after stale)", first)
	}
}

// Pre-11.15 servers (schema additionalProperties:false) reject `filters`, so the
// old form must still be sent to them.
func TestCheckDocument_Pre1115_SendsDocuments(t *testing.T) {
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		if _, ok := args["filters"]; ok {
			return "MCP error -32602: Unrecognized key: \"filters\"", true
		}
		return "'M.WF': - Duplicate name 'ReviewOrder'.", false
	})
	f.tools = the1114CheckErrors
	b := &Backend{client: f.connectClient(t)}

	err := b.checkDocumentNow("Workflows$Workflow", "M.WF")
	if err == nil || !strings.Contains(err.Error(), "Duplicate name") {
		t.Fatalf("old-form error not reported: %v", err)
	}
	call, _ := f.callByName("ped_check_errors")
	docs, _ := call.Args["documents"].([]any)
	if len(docs) != 1 {
		t.Fatalf("documents = %v, want the one checked document", call.Args)
	}
}

// When the tools/list probe could not tell (no surface reported), the old form is
// sent; an 11.15 server then answers project-wide. The listing is still scoped by
// its unit headers, so another document's error cannot fail this one.
func TestCheckDocument_UnknownSurface_ScopesAnUnscopedListing(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return listing(1, 1, 1, 2, unitBlock("M.Other", "Enumerations$Enumeration", "Missing enumeration values.")), false
	})
	b := &Backend{client: f.connectClient(t)}

	if err := b.checkDocumentNow("Workflows$Workflow", "M.WF"); err != nil {
		t.Fatalf("unscoped listing failed an unrelated document: %v", err)
	}
}

// Text that is neither the clean verdict nor a parseable listing stays a failure:
// an unrecognised answer must never read as clean.
func TestCheckDocument_1115_UnrecognisedTextFails(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return "Something went wrong while collecting diagnostics.", false
	})
	f.tools = the1115CheckErrors
	b := &Backend{client: f.connectClient(t)}

	if err := b.checkDocumentNow("Workflows$Workflow", "M.WF"); err == nil {
		t.Fatal("an unrecognised answer was treated as clean")
	}
}

// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func pageBuilderAt(major, minor int) *pageBuilder {
	return &pageBuilder{backend: &mock.MockBackend{
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor}
		},
	}}
}

// The source type exists in 9.x only: mx convert 10.12.19 rewrites every one
// into an XPath source, which is what 10+ builds (mendixlabs/mxcli#1599,
// TestBuildPluggableDataSource_DatabaseOverAssociationOn10).
func TestCustomWidgetDatabaseSourceVersionGate(t *testing.T) {
	if !pageBuilderAt(9, 24).customWidgetDatabaseSourceSupported() {
		t.Error("9.24 does not get the 9.x database source")
	}
	if pageBuilderAt(10, 0).customWidgetDatabaseSourceSupported() {
		t.Error("10.0 got a source type Mendix 10 no longer has")
	}
	if (&pageBuilder{backend: &mock.MockBackend{}}).customWidgetDatabaseSourceSupported() {
		t.Error("an unknown version got the 9.x source")
	}
}

// mendixlabs/mxcli#1399, write half. A 9.x "Database" source on a pluggable
// widget stores constraint rows, not XPath; describe prints the rows as the
// XPath they mean, and the writer has to read exactly those rows back.

// Every literal databaseConstraintXPath renders parses back to the stored row.
func TestDatabaseConstraintsRoundTrip(t *testing.T) {
	rows := []pages.DatabaseConstraint{
		{Attribute: "Mod.E.Flag", Operator: "Equals", Value: "true"},
		{Attribute: "Mod.E.Flag", Operator: "Equals", Value: "false"},
		{Attribute: "Mod.E.Flag", Operator: "NotEquals", Value: "Open"},
		{Attribute: "Mod.E.Flag", Operator: "LessThan", Value: "5"},
		{Attribute: "Mod.E.Flag", Operator: "LessThanOrEquals", Value: "-2.5"},
		{Attribute: "Mod.E.Flag", Operator: "GreaterThan", Value: "[%CurrentDateTime%]"},
		{Attribute: "Mod.E.Flag", Operator: "GreaterThanOrEquals", Value: "it's"},
		{Attribute: "Mod.E.Flag", Operator: "Contains", Value: "a, b"},
		{Attribute: "Mod.E.Flag", Operator: "NotContains", Value: "ab"},
		{Attribute: "Mod.E.Flag", Operator: "Equals", Value: ""},
	}
	for _, row := range rows {
		term, ok := databaseConstraintXPath(row.Attribute, row.Operator, row.Value)
		if !ok {
			t.Fatalf("%+v did not render", row)
		}
		got, err := parseDatabaseConstraints("["+term+"]", "Mod.E")
		if err != nil {
			t.Errorf("%s: %v", term, err)
			continue
		}
		if !reflect.DeepEqual(got, []pages.DatabaseConstraint{row}) {
			t.Errorf("%s parsed to %+v, want %+v", term, got, row)
		}
	}
}

func TestParseDatabaseConstraintsForms(t *testing.T) {
	want := []pages.DatabaseConstraint{
		{Attribute: "NativeReason.NativeReason.IsActive", Operator: "Equals", Value: "true"},
		{Attribute: "NativeReason.NativeReason.ReasonType", Operator: "NotEquals", Value: "Unresolved_appointment"},
	}
	for _, xpath := range []string{
		"[IsActive = true() and ReasonType != 'Unresolved_appointment']",
		"[IsActive = true()] [ReasonType != 'Unresolved_appointment']",
		"[IsActive = true()][ReasonType != 'Unresolved_appointment']",
		// Mendix's own spelling, from its 9 → 10 conversion of these rows.
		"[IsActive and ReasonType!='Unresolved_appointment']",
		"[NativeReason.NativeReason.IsActive = true() and\n ReasonType != 'Unresolved_appointment']",
	} {
		got, err := parseDatabaseConstraints(xpath, "NativeReason.NativeReason")
		if err != nil {
			t.Errorf("%q: %v", xpath, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q parsed to %+v", xpath, got)
		}
	}
	if got, err := parseDatabaseConstraints("", "M.E"); err != nil || got != nil {
		t.Errorf("an empty constraint gave %+v, %v", got, err)
	}
	if got, _ := parseDatabaseConstraints("[not(IsActive)]", "M.E"); len(got) != 1 || got[0].Value != "false" {
		t.Errorf("not(IsActive) parsed to %+v", got)
	}
}

// What a constraint row cannot hold is refused, never approximated: dropping a
// term widens the retrieve.
func TestParseDatabaseConstraintsRefusesXPath(t *testing.T) {
	for _, xpath := range []string{
		"[A = 1 or B = 2]",
		"[M.A_B/M.B/Name = 'x']",
		"[A = $currentUser]",
		"[A = 1 + 2]",
		"[starts-with(A, 'x')]",
	} {
		if got, err := parseDatabaseConstraints(xpath, "M.E"); err == nil {
			t.Errorf("%q was accepted as %+v", xpath, got)
		}
	}
}

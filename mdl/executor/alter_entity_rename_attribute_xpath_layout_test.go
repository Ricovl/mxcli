// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// retrieveRawUnit is a microflow holding one database retrieve, in the shape the
// XPath rename pass reads: the target entity is the sibling "Entity" field.
func retrieveRawUnit(t *testing.T, id, name, entity, constraint string) *types.RawUnit {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "Name", Value: name},
		{Key: "ObjectCollection", Value: bson.D{{Key: "Objects", Value: bson.A{
			bson.D{
				{Key: "$Type", Value: "Microflows$ActionActivity"},
				{Key: "Action", Value: bson.D{
					{Key: "$Type", Value: "Microflows$RetrieveAction"},
					{Key: "RetrieveSource", Value: bson.D{
						{Key: "$Type", Value: "Microflows$DatabaseRetrieveSource"},
						{Key: "Entity", Value: entity},
						{Key: "XpathConstraint", Value: constraint},
					}},
				}},
			},
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &types.RawUnit{ID: model.ID(id), Type: "Microflows$Microflow", Contents: b}
}

// TestAlterEntityRenameAttributeCountsOnlyRealXPathChanges is the executor-level
// regression test for ako/mxcli#1506 (mendixlabs/mxcli#1596). Renaming an
// attribute no XPath names printed "Updated 2 XPath constraint(s) in 2
// document(s)" and wrote both microflows: their only difference was the line
// break Studio Pro stores between predicate groups, which the rewrite dropped.
//
// ACT_Uses is the control — it does name the attribute, so exactly one
// constraint in one document is the right report, and its write proves the
// mock records writes at all.
func TestAlterEntityRenameAttributeCountsOnlyRealXPathChanges(t *testing.T) {
	ctx, _, _ := renameAttrTestCtx(t, 0, "Unused", "Score", "Level")
	mb := ctx.Backend.(*mock.MockBackend)

	units := []*types.RawUnit{
		retrieveRawUnit(t, "mf-fetch", "ACT_Fetch", "Sudoku.Game", "[Score = 1]\n[Level = 2]"),
		retrieveRawUnit(t, "mf-other", "ACT_Unrelated", "Other.Thing", "[Label = 'A']\n[Flag != 'B']"),
		retrieveRawUnit(t, "mf-uses", "ACT_Uses", "Sudoku.Game", "[Level = 2]\n[Unused = 'x']"),
	}
	written := map[string]string{}
	mb.ListRawUnitsByTypeFunc = func(string) ([]*types.RawUnit, error) { return units, nil }
	mb.UpdateRawUnitFunc = func(unitID string, contents []byte) error {
		var doc bson.D
		if err := bson.Unmarshal(contents, &doc); err != nil {
			return err
		}
		written[unitID] = xpathConstraintOf(doc)
		return nil
	}

	assertNoError(t, execAlterEntity(ctx, renameAttrStmt("Unused", "Unused2")))

	out := ctx.Output.(interface{ String() string }).String()
	if !strings.Contains(out, "Updated 1 XPath constraint(s) in 1 document(s)") {
		t.Errorf("the report counts constraints the rename did not change, got: %q", out)
	}
	for _, id := range []string{"mf-fetch", "mf-other"} {
		if c, ok := written[id]; ok {
			t.Errorf("%s does not name the attribute but was written, now %q", id, c)
		}
	}
	if got, want := written["mf-uses"], "[Level = 2]\n[Unused2 = 'x']"; got != want {
		t.Errorf("control: ACT_Uses was written as %q, want %q", got, want)
	}
}

// xpathConstraintOf returns the first XPath constraint stored in doc.
func xpathConstraintOf(v any) string {
	switch val := v.(type) {
	case bson.D:
		for _, e := range val {
			if s, ok := e.Value.(string); ok && (e.Key == "XpathConstraint" || e.Key == "XPathConstraint") {
				return s
			}
			if s := xpathConstraintOf(e.Value); s != "" {
				return s
			}
		}
	case bson.A:
		for _, item := range val {
			if s := xpathConstraintOf(item); s != "" {
				return s
			}
		}
	}
	return ""
}

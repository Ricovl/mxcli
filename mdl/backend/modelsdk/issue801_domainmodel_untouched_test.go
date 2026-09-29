// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestIssue801_UpdateDomainModelChangesOnlyTheAlteredElement guards
// ako/mxcli#801: UpdateDomainModel — the write behind ALTER ASSOCIATION, CREATE
// OR MODIFY ASSOCIATION, RENAME and `mxcli layout` — must change the element the
// statement names and nothing else in the unit.
//
// It rebuilt every entity and association from the semantic model through
// entityToGen / assocToGen, and those converters write constants for what the
// semantic model does not carry: ExportLevel "Hidden" on the entity, on each of
// its attributes and on each association, and an access rule whose
// XPathConstraintCaption and Documentation are "". A stored raw carry does not
// help, because a property the rebuild SETS is dirty and wins over the raw bytes.
// So one statement turned every API-exported entity in the module Hidden — which
// silently removes it from the module's public surface when the module is
// exported — and rewrote every entity's access rules.
//
// THE FIXTURE HAS TO BE STUDIO PRO-AUTHORED. mxcli writes "Hidden" and an empty
// caption itself, so on mxcli-authored content the rebuild reproduces the stored
// value and the defect is invisible. ako/TestApp's WorkflowCommons (a marketplace
// module) exports entities and attributes as API; PedApp exports nothing.
//
// CONTROL: the NoChange case writes the semantic model back unmodified. It must
// leave every element byte-identical — before the fix it did not, which is the
// "churns every entity" half of the report on its own.
func TestIssue801_UpdateDomainModelChangesOnlyTheAlteredElement(t *testing.T) {
	src := filepath.Join("..", "..", "..", "testdata", "testapp", "TestApp")
	if _, err := os.Stat(filepath.Join(src, "TestApp.mpr")); err != nil {
		t.Skipf("TestApp submodule not initialised (git submodule update --init testdata/testapp): %v", err)
	}

	for _, tc := range []struct {
		name string
		// allow is the set of top-level keys the altered element may change.
		allow []string
		// mutate applies one statement's worth of change and returns the $ID of
		// the one element it touched ("" for the control).
		mutate func(t *testing.T, dm *domainmodel.DomainModel, apiEntity model.ID) model.ID
	}{
		{
			name: "NoChange",
			mutate: func(*testing.T, *domainmodel.DomainModel, model.ID) model.ID {
				return ""
			},
		},
		{
			name:  "SetAssociationComment",
			allow: []string{"Documentation"},
			mutate: func(t *testing.T, dm *domainmodel.DomainModel, _ model.ID) model.ID {
				a := dm.Associations[0]
				a.Documentation = "issue 801"
				return a.ID
			},
		},
		{
			name:  "RenameAssociation",
			allow: []string{"Name"},
			mutate: func(t *testing.T, dm *domainmodel.DomainModel, _ model.ID) model.ID {
				a := dm.Associations[0]
				a.Name = "Issue801_" + a.Name
				return a.ID
			},
		},
		{
			// `mxcli layout` moves entities on the canvas. Moving the API entity
			// itself checks the ALTERED element keeps its ExportLevel too.
			name:  "LayoutMovesAPIEntity",
			allow: []string{"Location"},
			mutate: func(t *testing.T, dm *domainmodel.DomainModel, apiEntity model.ID) model.ID {
				for _, e := range dm.Entities {
					if e.ID == apiEntity {
						e.Location.X += 40
						return e.ID
					}
				}
				t.Fatalf("API entity %s not in the semantic model", apiEntity)
				return ""
			},
		},
		{
			// RENAME ENTITY of an API entity: the name changes, and so do the
			// member names in its access rules, which embed it (the executor's
			// repointEntitySelfRefs does the same); the export level stays.
			name:  "RenameAPIEntity",
			allow: []string{"Name", "AccessRules", "ValidationRules"},
			mutate: func(t *testing.T, dm *domainmodel.DomainModel, apiEntity model.ID) model.ID {
				for _, e := range dm.Entities {
					if e.ID == apiEntity {
						mod := strings.SplitN(firstMemberName(e), ".", 2)[0]
						oldPrefix := mod + "." + e.Name + "."
						e.Name = "Issue801" + e.Name
						newPrefix := mod + "." + e.Name + "."
						for _, ar := range e.AccessRules {
							for _, ma := range ar.MemberAccesses {
								if strings.HasPrefix(ma.AttributeName, oldPrefix) {
									ma.AttributeName = newPrefix + strings.TrimPrefix(ma.AttributeName, oldPrefix)
								}
							}
						}
						return e.ID
					}
				}
				t.Fatalf("API entity %s not in the semantic model", apiEntity)
				return ""
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyTestAppFixture(t, src)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}

			dmID, apiEntity := domainModelWithAPIEntity(t, b)
			exportFirstAttribute(t, b, dmID, apiEntity)
			before := domainModelElementBytes(t, b, dmID)

			dm := reloadDomainModel(t, b, dmID)
			touched := tc.mutate(t, dm, apiEntity)
			if err := b.UpdateDomainModel(dm); err != nil {
				t.Fatalf("UpdateDomainModel: %v", err)
			}
			if err := b.Disconnect(); err != nil {
				t.Fatalf("disconnect: %v", err)
			}

			b2 := New()
			if err := b2.Connect(proj); err != nil {
				t.Fatalf("reconnect: %v", err)
			}
			t.Cleanup(func() { _ = b2.Disconnect() })
			after := domainModelElementBytes(t, b2, dmID)

			ids := make([]string, 0, len(before))
			for id := range before {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			changed := 0
			for _, id := range ids {
				was := before[id]
				now, ok := after[id]
				if !ok {
					t.Errorf("%s is gone from the unit after the write", was.label)
					continue
				}
				if id == string(touched) {
					if bytes.Equal(was.raw, now.raw) {
						t.Errorf("%s: the altered element is unchanged — the write did not land", was.label)
					}
					if was.exportLevel != now.exportLevel {
						t.Errorf("%s (the altered element): ExportLevel %q -> %q", was.label, was.exportLevel, now.exportLevel)
					}
					for aid, lvl := range was.attrExportLevels {
						if got := now.attrExportLevels[aid]; got != lvl {
							t.Errorf("%s (the altered element): attribute %s ExportLevel %q -> %q", was.label, aid, lvl, got)
						}
					}
					assertMemberAccessKeys(t, was.label, now.raw)
					allowed := map[string]bool{}
					for _, k := range tc.allow {
						allowed[k] = true
					}
					for _, k := range changedKeys(t, was.raw, now.raw) {
						if !allowed[k] {
							t.Errorf("%s (the altered element): %s changed; the statement only changes %v", was.label, k, tc.allow)
						}
					}
					continue
				}
				if !bytes.Equal(was.raw, now.raw) {
					changed++
					if changed <= 8 {
						t.Errorf("%s was not named by the statement and changed: keys %v (ExportLevel %q -> %q)",
							was.label, changedKeys(t, was.raw, now.raw), was.exportLevel, now.exportLevel)
					}
				}
			}
			if changed > 8 {
				t.Errorf("... and %d more untouched elements changed (%d of %d in all)", changed-8, changed, len(before))
			}
		})
	}
}

// elementBytes is one top-level domain-model element as stored.
type elementBytes struct {
	label            string
	raw              []byte
	exportLevel      string
	attrExportLevels map[string]string
}

// domainModelElementBytes snapshots every entity, association and cross-module
// association of a domain model unit, keyed by $ID.
func domainModelElementBytes(t *testing.T, b *Backend, dmID model.ID) map[string]elementBytes {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	out := map[string]elementBytes{}
	for _, el := range gdm.EntitiesItems() {
		ge, ok := el.(*genDm.Entity)
		if !ok {
			continue
		}
		rec := elementBytes{
			label:            "entity " + ge.Name(),
			raw:              append([]byte(nil), ge.Raw()...),
			exportLevel:      ge.ExportLevel(),
			attrExportLevels: map[string]string{},
		}
		for _, ael := range ge.AttributesItems() {
			if a, ok := ael.(*genDm.Attribute); ok {
				rec.attrExportLevels[string(a.ID())] = a.ExportLevel()
			}
		}
		out[string(ge.ID())] = rec
	}
	for _, el := range gdm.AssociationsItems() {
		if a, ok := el.(*genDm.Association); ok {
			out[string(a.ID())] = elementBytes{"association " + a.Name(), append([]byte(nil), a.Raw()...), a.ExportLevel(), nil}
		}
	}
	for _, el := range gdm.CrossAssociationsItems() {
		if a, ok := el.(*genDm.CrossAssociation); ok {
			out[string(a.ID())] = elementBytes{"cross-association " + a.Name(), append([]byte(nil), a.Raw()...), a.ExportLevel(), nil}
		}
	}
	return out
}

// changedKeys names the top-level keys that differ, so a failure says WHAT moved
// (ExportLevel, AccessRules, ...) rather than only that something did. A key whose
// canonical form is equal but whose bytes are not is reported as "key($IDs)": only
// element $IDs moved inside it.
func changedKeys(t *testing.T, a, b []byte) []string {
	t.Helper()
	var da, db bson.D
	if err := bson.Unmarshal(a, &da); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := bson.Unmarshal(b, &db); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wrap := func(v any) []byte {
		raw, err := bson.Marshal(bson.D{{Key: "v", Value: v}})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return raw
	}
	digest := func(raw []byte) string {
		d, err := canon.Digest(raw)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}
		return d
	}
	ma, mb := map[string]any{}, map[string]any{}
	for _, e := range da {
		ma[e.Key] = e.Value
	}
	for _, e := range db {
		mb[e.Key] = e.Value
	}
	var out []string
	for k, va := range ma {
		vb, ok := mb[k]
		if !ok {
			out = append(out, k+"(removed)")
			continue
		}
		ra, rb := wrap(va), wrap(vb)
		switch {
		case bytes.Equal(ra, rb):
		case digest(ra) != digest(rb):
			out = append(out, k)
		default:
			out = append(out, k+"($IDs)")
		}
	}
	for k := range mb {
		if _, ok := ma[k]; !ok {
			out = append(out, k+"(added)")
		}
	}
	sort.Strings(out)
	return out
}

// domainModelWithAPIEntity finds the domain model holding an entity Studio Pro
// exported as API, and returns it with that entity's $ID.
func domainModelWithAPIEntity(t *testing.T, b *Backend) (model.ID, model.ID) {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		gdm, err := b.loadDomainModelGen(d.ID)
		if err != nil || len(gdm.AssociationsItems()) == 0 {
			continue
		}
		for _, el := range gdm.EntitiesItems() {
			if ge, ok := el.(*genDm.Entity); ok && ge.ExportLevel() == "API" && len(ge.AttributesItems()) > 0 {
				return d.ID, model.ID(ge.ID())
			}
		}
	}
	t.Fatal("no domain model in TestApp holds an API-exported entity with attributes and an association")
	return "", ""
}

// copyTestAppFixture copies the TestApp project file and its mprcontents (the
// part a domain-model write reads and writes) into a temp dir.
func copyTestAppFixture(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(filepath.Join(dst, "mprcontents"), os.DirFS(filepath.Join(src, "mprcontents"))); err != nil {
		t.Fatalf("copy mprcontents: %v", err)
	}
	mpr, err := os.ReadFile(filepath.Join(src, "TestApp.mpr"))
	if err != nil {
		t.Fatalf("read TestApp.mpr: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "TestApp.mpr"), mpr, 0o644); err != nil {
		t.Fatalf("write TestApp.mpr: %v", err)
	}
	return filepath.Join(dst, "TestApp.mpr")
}

// firstMemberName returns a qualified member name from the entity's access
// rules, to learn the module prefix without another lookup.
func firstMemberName(e *domainmodel.Entity) string {
	for _, ar := range e.AccessRules {
		for _, ma := range ar.MemberAccesses {
			if ma.AttributeName != "" {
				return ma.AttributeName
			}
		}
	}
	return "WorkflowCommons.x"
}

// exportFirstAttribute sets the API entity's first attribute to ExportLevel API
// before the measurement. TestApp exports entities but no attribute, and the
// attribute carry has to be seen to work: attributeToGen resets the level to
// Hidden exactly as entityToGen does for the entity. The export level is not an
// identity, so setting it through mxcli does not void the Studio Pro-authored
// premise the rest of the test rests on.
func exportFirstAttribute(t *testing.T, b *Backend, dmID, entityID model.ID) {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	ge := findGenEntity(gdm, entityID)
	if ge == nil {
		t.Fatalf("entity %s not found", entityID)
	}
	a, ok := ge.AttributesItems()[0].(*genDm.Attribute)
	if !ok {
		t.Fatalf("first attribute of %s is %T", ge.Name(), ge.AttributesItems()[0])
	}
	a.SetExportLevel("API")
	if err := b.persistDM(dmID, gdm); err != nil {
		t.Fatalf("persist API attribute: %v", err)
	}
	gdm, err = b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := findGenEntity(gdm, entityID).AttributesItems()[0].(*genDm.Attribute).ExportLevel(); got != "API" {
		t.Fatalf("precondition: attribute ExportLevel is %q after setting it to API", got)
	}
}

// TestIssue801_OtherEntityRebuildsKeepExportLevel covers the other two callers of
// the lossy converter. UpdateDomainModel is where #801 was reported, but
// entityToGen resets the export level wherever it rebuilds an existing entity:
// UpdateEntity (every ALTER ENTITY and CREATE OR MODIFY ENTITY) and MoveEntity.
// Each case changes one thing about an API entity and requires its export level,
// its API attribute's, and — where the statement leaves them alone — its access
// rules to come through.
func TestIssue801_OtherEntityRebuildsKeepExportLevel(t *testing.T) {
	src := filepath.Join("..", "..", "..", "testdata", "testapp", "TestApp")
	if _, err := os.Stat(filepath.Join(src, "TestApp.mpr")); err != nil {
		t.Skipf("TestApp submodule not initialised (git submodule update --init testdata/testapp): %v", err)
	}
	for _, tc := range []struct {
		name string
		// write performs the rebuild and returns the domain model the entity
		// lives in afterwards.
		write          func(t *testing.T, b *Backend, dmID model.ID, e *domainmodel.Entity) model.ID
		keepAccessRule bool
	}{
		{
			name: "UpdateEntity",
			write: func(t *testing.T, b *Backend, dmID model.ID, e *domainmodel.Entity) model.ID {
				e.Documentation = "issue 801"
				if err := b.UpdateEntity(dmID, e); err != nil {
					t.Fatalf("UpdateEntity: %v", err)
				}
				return dmID
			},
			keepAccessRule: true,
		},
		{
			name: "MoveEntity",
			write: func(t *testing.T, b *Backend, dmID model.ID, e *domainmodel.Entity) model.ID {
				srcMod, dstMod, dstDM := moveTarget(t, b, dmID)
				if _, err := b.MoveEntity(e, dmID, dstDM, srcMod, dstMod); err != nil {
					t.Fatalf("MoveEntity: %v", err)
				}
				return dstDM
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyTestAppFixture(t, src)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}
			dmID, entityID := domainModelWithAPIEntity(t, b)
			exportFirstAttribute(t, b, dmID, entityID)
			was := domainModelElementBytes(t, b, dmID)[string(entityID)]

			var ent *domainmodel.Entity
			for _, e := range reloadDomainModel(t, b, dmID).Entities {
				if e.ID == entityID {
					ent = e
				}
			}
			newDM := tc.write(t, b, dmID, ent)
			if err := b.Disconnect(); err != nil {
				t.Fatalf("disconnect: %v", err)
			}
			b2 := New()
			if err := b2.Connect(proj); err != nil {
				t.Fatalf("reconnect: %v", err)
			}
			t.Cleanup(func() { _ = b2.Disconnect() })
			now, ok := domainModelElementBytes(t, b2, newDM)[string(entityID)]
			if !ok {
				t.Fatalf("entity %s not found in the target domain model after the write", entityID)
			}
			if now.exportLevel != was.exportLevel {
				t.Errorf("%s: ExportLevel %q -> %q", was.label, was.exportLevel, now.exportLevel)
			}
			for aid, lvl := range was.attrExportLevels {
				if got := now.attrExportLevels[aid]; got != lvl {
					t.Errorf("%s: attribute %s ExportLevel %q -> %q", was.label, aid, lvl, got)
				}
			}
			assertMemberAccessKeys(t, was.label, now.raw)
			if tc.keepAccessRule {
				for _, k := range changedKeys(t, was.raw, now.raw) {
					if k != "Documentation" {
						t.Errorf("%s: %s changed; the statement only changes Documentation", was.label, k)
					}
				}
			}
		})
	}
}

// moveTarget picks another module with a domain model to move an entity into.
func moveTarget(t *testing.T, b *Backend, fromDM model.ID) (srcModule, dstModule string, dstDM model.ID) {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		if d.ID == fromDM {
			srcModule = b.moduleNameFor(d.ID)
		}
	}
	for _, d := range dms {
		if d.ID == fromDM {
			continue
		}
		if _, err := b.loadDomainModelGen(d.ID); err != nil {
			continue
		}
		if name := b.moduleNameFor(d.ID); name != "" && name != "System" {
			return srcModule, name, d.ID
		}
	}
	t.Fatal("no second domain model to move into")
	return "", "", ""
}

// assertMemberAccessKeys requires every member access of a stored entity to carry
// both reference keys, Attribute and Association, as Studio Pro writes them (the
// unused one as ""). A rebuilt rule omitted the unused key, so a rule that had to
// be rebuilt still differed from Studio Pro's form in every member.
func assertMemberAccessKeys(t *testing.T, label string, entityRaw []byte) {
	t.Helper()
	var ent struct {
		AccessRules bson.A `bson:"AccessRules"`
	}
	if err := bson.Unmarshal(entityRaw, &ent); err != nil {
		t.Fatalf("unmarshal %s: %v", label, err)
	}
	for _, r := range ent.AccessRules {
		rule, ok := r.(bson.D)
		if !ok {
			continue
		}
		for _, f := range rule {
			if f.Key != "MemberAccesses" {
				continue
			}
			members, _ := f.Value.(bson.A)
			for _, m := range members {
				ma, ok := m.(bson.D)
				if !ok {
					continue
				}
				have := map[string]bool{}
				for _, mf := range ma {
					have[mf.Key] = true
				}
				if !have["Attribute"] || !have["Association"] {
					t.Errorf("%s: member access %v lacks the Attribute or Association key Studio Pro always writes", label, ma)
					return
				}
			}
		}
	}
}

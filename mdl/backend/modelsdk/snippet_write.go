// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func init() {
	// A snippet always emits its Parameters and Variables arrays (empty = marker 3).
	codec.RegisterTypeDefaults("Forms$Snippet", codec.TypeDefaults{
		MandatoryLists: []string{"Parameters", "Variables"},
	})
}

// encodeSnippet builds and serializes a Forms$Snippet for a project of this
// version. Snippet.Variables shares the Forms$Page floor (10.17.0), and a
// snippet never populates the list — so the only thing that could reach a
// pre-10.17 project is the empty marker the defaults registry adds. Both
// CreateSnippet and UpdateSnippet go through here so the guard cannot be applied
// on one path and forgotten on the other.
//
// carry, when non-nil, runs against the built document before it is encoded —
// UpdateSnippet uses it for carryStoredSnippetHeader, as UpdatePage does.
func encodeSnippet(snippet *pages.Snippet, pv *types.ProjectVersion, carry func(*genPg.Snippet)) ([]byte, error) {
	g, err := snippetToGen(snippet)
	if err != nil {
		return nil, err
	}
	if carry != nil {
		carry(g)
	}
	g.SetID(element.ID(snippet.ID))
	contents, err := docEncoder("Forms$Snippet", pv).Encode(g)
	if err != nil {
		return nil, err
	}
	if err := canon.BareAttributeRefError(fmt.Sprintf("snippet %q", snippet.Name), contents); err != nil { // see encodePage
		return nil, err
	}
	return contents, nil
}

// CreateSnippet inserts a new Forms$Snippet document — a reusable widget tree with
// its own parameters (entity-typed) and a flat Widgets list (no layout call).
func (b *Backend) CreateSnippet(snippet *pages.Snippet) error {
	// A pluggable widget's children are serialized while the executor builds the
	// page, before this call; drain any failure so an unsupported construct fails
	// the statement instead of silently landing as a widget with the piece missing.
	if err := takeChildSerializeErr(); err != nil {
		return fmt.Errorf("CreateSnippet: %w", err)
	}
	if snippet == nil {
		return fmt.Errorf("CreateSnippet: nil snippet")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateSnippet: not connected for writing")
	}
	if snippet.ID == "" {
		snippet.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodeSnippet(snippet, b.ProjectVersion(), nil)
	if err != nil {
		return fmt.Errorf("CreateSnippet: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(snippet.ID), string(snippet.ContainerID), "Documents", "Forms$Snippet", contents); err != nil {
		return fmt.Errorf("CreateSnippet: insert: %w", err)
	}
	return nil
}

// UpdateSnippet rewrites a snippet document (CREATE OR REPLACE).
func (b *Backend) UpdateSnippet(snippet *pages.Snippet) error {
	// A pluggable widget's children are serialized while the executor builds the
	// page, before this call; drain any failure so an unsupported construct fails
	// the statement instead of silently landing as a widget with the piece missing.
	if err := takeChildSerializeErr(); err != nil {
		return fmt.Errorf("UpdateSnippet: %w", err)
	}
	if snippet == nil {
		return fmt.Errorf("UpdateSnippet: nil snippet")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateSnippet: not connected for writing")
	}
	contents, err := encodeSnippet(snippet, b.ProjectVersion(), func(g *genPg.Snippet) {
		b.carryStoredSnippetHeader(snippet.ID, g)
	})
	if err != nil {
		return fmt.Errorf("UpdateSnippet: encode: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(snippet.ID), contents); err != nil {
		return fmt.Errorf("UpdateSnippet: update: %w", err)
	}
	return nil
}

// carryStoredSnippetHeader copies the four header properties off the stored
// unit onto a rebuilt snippet: Type, ExportLevel and the canvas. None has an MDL
// spelling, so a value nobody asked to change must not change — the same
// reasoning as carryStoredPageHeader (#541), which this mirrors. Type is the one
// that matters most: it says whether the snippet is for web or native pages, and
// a rewrite must not decide that on the author's behalf.
//
// A missing or unreadable stored unit leaves the defaults in place.
func (b *Backend) carryStoredSnippetHeader(id model.ID, g *genPg.Snippet) {
	if b.reader == nil || id == "" {
		return
	}
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		return
	}
	var stored bson.D
	if err := bson.Unmarshal(raw, &stored); err != nil {
		return
	}
	if v := bsonnav.DGetString(stored, "Type"); v != "" {
		g.SetType(v)
	}
	if v := bsonnav.DGetString(stored, "ExportLevel"); v != "" {
		g.SetExportLevel(v)
	}
	// Width-agnostic, as for pages: Studio Pro stores the canvas as int64.
	if v := bsonInt(bsonnav.DGet(stored, "CanvasWidth")); v > 0 {
		g.SetCanvasWidth(int32(v))
	}
	if v := bsonInt(bsonnav.DGet(stored, "CanvasHeight")); v > 0 {
		g.SetCanvasHeight(int32(v))
	}
}

// DeleteSnippet removes the snippet unit.
func (b *Backend) DeleteSnippet(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteSnippet: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// snippetToGen builds the gen Snippet (header + parameters + widget tree).
func snippetToGen(s *pages.Snippet) (*genPg.Snippet, error) {
	out := genPg.NewSnippet()
	out.SetName(s.Name)
	out.SetDocumentation(s.Documentation)
	// Carry the stored exclusion: hardcoding false silently un-excluded the
	// document on every rewrite (#914).
	out.SetExcluded(s.Excluded)
	// A new snippet's header is what Studio Pro writes (Web, Hidden, 800 × 600
	// on every snippet of the Blank template); a rewrite carries the stored one
	// instead — see carryStoredSnippetHeader. Type was "" here, which is not a
	// member of PagesType (Native | Web) (ako/mxcli#705).
	out.SetExportLevel("Hidden")
	out.SetCanvasWidth(800)
	out.SetCanvasHeight(600)
	out.SetType("Web")

	for _, p := range s.Parameters {
		out.AddParameters(snippetParameterToGen(p))
	}
	for _, w := range s.Widgets {
		wg, err := widgetToGen(w)
		if err != nil {
			return nil, err
		}
		out.AddWidgets(wg)
	}
	return out, nil
}

// snippetParameterToGen builds a Forms$SnippetParameter. Its ParameterType is
// the same polymorphic DataTypes$DataType a page parameter carries, so it goes
// through the same builder: p.Type holds a primitive's BSON $Type when the
// parameter is primitive, and is empty for an entity parameter.
//
// It used to build a DataTypes$ObjectType unconditionally, so a primitive-typed
// parameter was written pointing at an entity named "" (mendixlabs/mxcli#1028).
func snippetParameterToGen(p *pages.SnippetParameter) *genPg.SnippetParameter {
	gp := genPg.NewSnippetParameter()
	if p.ID != "" {
		gp.SetID(element.ID(p.ID))
	}
	assignID(gp)
	gp.SetName(p.Name)
	gp.SetParameterType(paramTypeToGen(p.Type, p.EntityName))
	return gp
}

// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/menumutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// OpenNavigationProfileForMutation opens the navigation document's raw unit
// and returns the menu splice over one web profile (ALTER NAVIGATION).
func (b *Backend) OpenNavigationProfileForMutation(profileName string) (backend.NavigationProfileMutator, error) {
	if b.writer == nil {
		return nil, fmt.Errorf("OpenNavigationProfileForMutation: not connected for writing")
	}
	nav, err := b.GetNavigation()
	if err != nil {
		return nil, err
	}
	if nav == nil {
		return nil, fmt.Errorf("the project has no navigation document")
	}
	doc, err := b.rawUnitDoc(nav.ID)
	if err != nil {
		return nil, fmt.Errorf("OpenNavigationProfileForMutation: %w", err)
	}
	return menumutator.NewProfile(doc, string(nav.ID), profileName, codecMenuDeps{b: b})
}

// OpenMenuDocumentForMutation opens a menu document's raw unit and returns the
// menu splice over its items (ALTER MENU).
func (b *Backend) OpenMenuDocumentForMutation(id model.ID) (backend.MenuMutator, error) {
	if b.writer == nil {
		return nil, fmt.Errorf("OpenMenuDocumentForMutation: not connected for writing")
	}
	doc, err := b.rawUnitDoc(id)
	if err != nil {
		return nil, fmt.Errorf("OpenMenuDocumentForMutation: %w", err)
	}
	return menumutator.NewMenuDocument(doc, string(id), codecMenuDeps{b: b})
}

func (b *Backend) rawUnitDoc(id model.ID) (bson.D, error) {
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		return nil, fmt.Errorf("load unit: %w", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return d, nil
}

// codecMenuDeps builds new menu nodes with the writers `create navigation`
// uses, so an inserted item has exactly the property set a created one has
// (Studio Pro's, measured: $ID, $Type, Action, AlternativeText, Caption, Icon,
// Items), in both a profile and a menu document, whose items are the same
// element.
type codecMenuDeps struct{ b *Backend }

var _ menumutator.Deps = codecMenuDeps{}

func (d codecMenuDeps) BuildMenuItem(spec types.NavMenuItemSpec) (bson.D, error) {
	return navMenuItemBson(spec)
}

func (d codecMenuDeps) BuildMenuAction(spec types.NavMenuItemSpec) (bson.D, error) {
	return navMenuAction(spec)
}

func (d codecMenuDeps) BuildMenuIcon(spec types.NavMenuItemSpec) any {
	return navMenuIconBson(spec)
}

// SaveUnit writes the splice as a patch: canon elides an unchanged unit and
// guards storage GUIDs as for every write, but does not re-pair the element
// $IDs the splice kept, nor carry back a translation it left out.
func (d codecMenuDeps) SaveUnit(unitID string, contents []byte) error {
	return d.b.writer.UpdateRawUnitPatch(unitID, contents)
}

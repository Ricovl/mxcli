// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genSet "github.com/mendixlabs/mxcli/modelsdk/gen/settings"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// decodeRaw decodes a unit document that did NOT come from this project's
// storage: diff-local hands over a historical unit read back with `git show`,
// so there is no unit to look up. The codec decodes bytes, not maps, so the map
// is re-marshalled first — safe for the same reason as ParseMicroflowFromRaw:
// the result is only rendered, never written back.
func decodeRaw(raw map[string]any) (element.Element, []byte, error) {
	data, err := bson.Marshal(raw)
	if err != nil {
		return nil, nil, err
	}
	elem, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(data))
	if err != nil {
		return nil, nil, err
	}
	return elem, data, nil
}

// ParseProjectSettingsFromRaw converts a Settings$ProjectSettings document to
// the semantic settings, the way GetProjectSettings does for the stored one —
// RawParts included, since DESCRIBE SETTINGS prints only the properties the
// document stores. diff-local renders both revisions of the settings unit with
// it; without it the unit had no renderer, and every change to it counted as
// modified and printed nothing (mendixlabs/mxcli#1602, ako/mxcli#1512).
func (b *Backend) ParseProjectSettingsFromRaw(raw map[string]any) (*model.ProjectSettings, error) {
	elem, data, err := decodeRaw(raw)
	if err != nil {
		return nil, fmt.Errorf("decode project settings: %w", err)
	}
	g, ok := elem.(*genSet.ProjectSettings)
	if !ok {
		return nil, fmt.Errorf("not a project settings document: %T", elem)
	}
	ps := projectSettingsFromGen(g)
	ps.RawParts = settingsRawParts(data)
	return ps, nil
}

// ParseDomainModelFromRaw converts a DomainModels$DomainModel document to the
// semantic domain model, under the same read-only reasoning. diff-local reads
// each entity's access rules from it, so a GRANT renders the way DESCRIBE
// ENTITY prints it (mendixlabs/mxcli#1630, ako/mxcli#1562).
func (b *Backend) ParseDomainModelFromRaw(raw map[string]any, unitID, containerID model.ID) (*domainmodel.DomainModel, error) {
	elem, _, err := decodeRaw(raw)
	if err != nil {
		return nil, fmt.Errorf("decode domain model: %w", err)
	}
	g, ok := elem.(*genDm.DomainModel)
	if !ok {
		return nil, fmt.Errorf("not a domain model document: %T", elem)
	}
	dm := domainModelFromGen(g, containerID)
	if unitID != "" {
		dm.ID = unitID
	}
	return dm, nil
}

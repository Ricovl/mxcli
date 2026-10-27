// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ParseDomainModelFromRaw is ParseMicroflowFromRaw for a domain model, under
// the same read-only reasoning: diff-local renders the result and throws it
// away, so the map round-trip cannot reach storage. It goes through the codec
// rather than reading keys off the map because the keys are storage names — an
// access rule's roles are "AllowedModuleRoles", not the SDK's "ModuleRoles"
// (mendixlabs/mxcli#1630).
func (b *Backend) ParseDomainModelFromRaw(raw map[string]any, unitID, containerID model.ID) *domainmodel.DomainModel {
	data, err := bson.Marshal(raw)
	if err != nil {
		return nil
	}
	elem, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(data))
	if err != nil {
		return nil
	}
	dm, ok := elem.(*genDm.DomainModel)
	if !ok {
		return nil
	}
	out := domainModelFromGen(dm, containerID)
	if unitID != "" {
		out.ID = unitID
	}
	return out
}

// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// importMappingEntityError refuses an import-from-mapping activity without a
// result entity. importXmlActionToGen stores the entity as the activity's
// VariableType.Entity, and an empty one cannot be loaded: Mendix rebuilds it
// into an EntityIdentifier as it reads the unit and `mx check` dies with
// ArgumentNullException in EntityType.set_EntityId before validating anything
// (mendixlabs/mxcli#1418). The builder refuses a mapping it cannot resolve;
// this is the last line, for every path that reaches the encoder. Loop bodies
// are walked, as their activities are written the same way.
func importMappingEntityError(obj microflows.MicroflowObject) error {
	switch o := obj.(type) {
	case *microflows.ActionActivity:
		ix, ok := o.Action.(*microflows.ImportXmlAction)
		if !ok {
			return nil
		}
		if ix.ResultHandling == nil || ix.ResultHandling.ResultEntityID == "" {
			mapping := "(none)"
			if ix.ResultHandling != nil && ix.ResultHandling.MappingID != "" {
				mapping = string(ix.ResultHandling.MappingID)
			}
			return fmt.Errorf("refusing to write import from mapping %s: the activity has no result entity, "+
				"and Mendix cannot load a project holding one (EntityType.set_EntityId: value cannot be null). "+
				"The import mapping must exist, with a root entity, before the flow that calls it is written", mapping)
		}
	case *microflows.LoopedActivity:
		if o.ObjectCollection != nil {
			for _, inner := range o.ObjectCollection.Objects {
				if err := importMappingEntityError(inner); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// flowImportMappingEntityError applies importMappingEntityError to a flow's objects.
func flowImportMappingEntityError(oc *microflows.MicroflowObjectCollection) error {
	if oc == nil {
		return nil
	}
	for _, obj := range oc.Objects {
		if err := importMappingEntityError(obj); err != nil {
			return err
		}
	}
	return nil
}

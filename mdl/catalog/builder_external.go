// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// buildExternalEntities populates the external_entities catalog table
// from domain model entities that have an OData remote entity source.
func (b *Builder) buildExternalEntities() error {
	domainModels, err := b.cachedDomainModels()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO external_entities_data (Id, Name, QualifiedName, ModuleName,
			ServiceName, EntitySet, RemoteName,
			Countable, Creatable, Deletable, Updatable, AttributeCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, dm := range domainModels {
		if !b.own(string(dm.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(dm.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		for _, entity := range dm.Entities {
			// BOTH OData sources are external entities. Cataloguing only the
			// entity-set-backed one hid every type-sourced entity — the derived,
			// abstract, contained and action parameter/return types that
			// CREATE EXTERNAL ENTITIES imports with no entity set of their own.
			//
			// That is not merely an under-count. contract_entities.UsedByExternalEntity
			// is filled by joining this table on RemoteName, so for exactly those
			// entities the column was STRUCTURALLY always empty, whether or not
			// the import had worked. It read as "this contract entity is not
			// linked to anything", which is what sent mendixlabs/mxcli#1020
			// looking for a linkage bug in the MPR that was never there.
			if !isODataEntitySource(entity.Source) {
				continue
			}

			qualifiedName := moduleName + "." + entity.Name

			boolToInt := func(b bool) int {
				if b {
					return 1
				}
				return 0
			}

			_, err := stmt.Exec(
				string(entity.ID),
				entity.Name,
				qualifiedName,
				moduleName,
				entity.RemoteServiceName,
				entity.RemoteEntitySet,
				entity.RemoteEntityName,
				boolToInt(entity.Countable),
				boolToInt(entity.Creatable),
				boolToInt(entity.Deletable),
				boolToInt(entity.Updatable),
				len(entity.Attributes),
				projectID, snapshotID,
			)
			if err != nil {
				return err
			}
			count++
		}
	}
	b.disown()

	b.report("External Entities", count)
	return nil
}

// buildExternalActions populates the external_actions catalog table
// by scanning all microflows and nanoflows for CallExternalAction activities.
func (b *Builder) buildExternalActions() error {
	mfs, nfs, err := b.externalActionFlows()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO external_actions_data (Id, ServiceName, ActionName, ModuleName,
			UsageCount, CallerNames, ParameterNames,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	// Collect unique actions: key = service + "|" + action name
	type actionInfo struct {
		service    string
		actionName string
		module     string
		params     []string
		callers    []string
		count      int
	}
	actionMap := make(map[string]*actionInfo)

	extractActions := func(oc *microflows.MicroflowObjectCollection, flowModule, flowName string) {
		if oc == nil {
			return
		}
		for _, obj := range oc.Objects {
			act, ok := obj.(*microflows.ActionActivity)
			if !ok || act.Action == nil {
				continue
			}
			cea, ok := act.Action.(*microflows.CallExternalAction)
			if !ok {
				continue
			}

			key := cea.ConsumedODataService + "|" + cea.Name
			info, exists := actionMap[key]
			if !exists {
				var params []string
				for _, pm := range cea.ParameterMappings {
					params = append(params, pm.ParameterName)
				}
				info = &actionInfo{
					service:    cea.ConsumedODataService,
					actionName: cea.Name,
					module:     flowModule,
					params:     params,
				}
				actionMap[key] = info
			}
			info.count++
			caller := flowModule + "." + flowName
			// Avoid duplicate caller entries
			found := false
			for _, c := range info.callers {
				if c == caller {
					found = true
					break
				}
			}
			if !found {
				info.callers = append(info.callers, caller)
			}
			// Merge parameter names from different call sites
			if len(cea.ParameterMappings) > len(info.params) {
				info.params = nil
				for _, pm := range cea.ParameterMappings {
					info.params = append(info.params, pm.ParameterName)
				}
			}
		}
	}

	for _, mf := range mfs {
		modID := b.hierarchy.findModuleID(mf.ContainerID)
		modName := b.hierarchy.getModuleName(modID)
		extractActions(mf.ObjectCollection, modName, mf.Name)
	}
	for _, nf := range nfs {
		modID := b.hierarchy.findModuleID(nf.ContainerID)
		modName := b.hierarchy.getModuleName(modID)
		extractActions(nf.ObjectCollection, modName, nf.Name)
	}

	for _, info := range actionMap {
		syntheticID := fmt.Sprintf("%x", sha256.Sum256([]byte(info.service+"|"+info.actionName)))[:32]

		_, err := stmt.Exec(
			syntheticID,
			info.service,
			info.actionName,
			info.module,
			info.count,
			strings.Join(info.callers, ", "),
			strings.Join(info.params, ", "),
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
	}

	b.report("External Actions", len(actionMap))
	return nil
}

// callExternalActionType is the storage name of a call-external-action action.
const callExternalActionType = "Microflows$CallExternalAction"

// externalActionFlows lists the flows that may call an external action. The
// table aggregates over every flow, so it is rebuilt every time; an
// incremental build decodes only the flows whose stored bytes name the action
// type, keeping their order, instead of every flow in the project.
func (b *Builder) externalActionFlows() ([]*microflows.Microflow, []*microflows.Nanoflow, error) {
	if b.list == b.reader || b.scopedReader == nil {
		mfs, err := b.cachedMicroflows()
		if err != nil {
			return nil, nil, err
		}
		nfs, err := b.cachedNanoflows()
		return mfs, nfs, err
	}
	calling := map[string]bool{}
	for _, typeName := range []string{"Microflows$Microflow", "Microflows$Nanoflow"} {
		units, err := b.reader.ListRawUnitsByType(typeName)
		if err != nil {
			return nil, nil, err
		}
		for _, u := range units {
			if bytes.Contains(u.Contents, []byte(callExternalActionType)) {
				calling[string(u.ID)] = true
			}
		}
	}
	view := b.scopedReader(func(id string) bool { return calling[id] })
	mfs, err := view.ListMicroflows()
	if err != nil {
		return nil, nil, err
	}
	nfs, err := view.ListNanoflows()
	return mfs, nfs, err
}

// isODataEntitySource reports whether an entity's Source marks it as consumed
// from an OData service.
//
// Mendix stores two, and the difference is whether the contract gave the type
// an entity set:
//
//	Rest$ODataRemoteEntitySource  entity-set-backed, persistable
//	Rest$ODataEntityTypeSource    no entity set — derived, abstract, contained,
//	                              or an action's parameter/return type
//
// Both are external entities and both are written by CREATE EXTERNAL ENTITIES
// (see applyExternalEntityFields), so anything asking "is this external?" has to
// accept both. The domain-model writer already does.
func isODataEntitySource(source string) bool {
	return source == "Rest$ODataRemoteEntitySource" || source == "Rest$ODataEntityTypeSource"
}

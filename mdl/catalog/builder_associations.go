// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// deleteBehaviorColumns returns (behaviour, error message) for one end of an
// association. A nil end -- an association whose DeleteBehavior element is
// missing -- reads as "", not as the default: the catalog says what is stored.
func deleteBehaviorColumns(db *domainmodel.DeleteBehavior) (string, string) {
	if db == nil {
		return "", ""
	}
	return string(db.Type), db.ErrorMessage
}

func (b *Builder) buildAssociations() error {
	domainModels, err := b.cachedDomainModels()
	if err != nil {
		return err
	}

	// Build entity ID -> qualified name lookup (reuse already-parsed domain models).
	moduleNames := make(map[model.ID]string)
	entityNames := make(map[model.ID]string)
	for _, dm := range domainModels {
		modID := b.hierarchy.findModuleID(dm.ContainerID)
		modName := b.hierarchy.getModuleName(modID)
		moduleNames[dm.ContainerID] = modName
		for _, entity := range dm.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO associations_data (Id, Name, QualifiedName, ModuleName,
			FromEntity, ToEntity, AssociationType, Owner, StorageFormat, Description,
			ToDeleteBehavior, ToDeleteErrorMessage, FromDeleteBehavior, FromDeleteErrorMessage,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, dm := range domainModels {
		modName := moduleNames[dm.ContainerID]

		for _, assoc := range dm.Associations {
			from := entityNames[assoc.ParentID]
			if from == "" {
				from = string(assoc.ParentID)
			}
			to := entityNames[assoc.ChildID]
			if to == "" {
				to = string(assoc.ChildID)
			}
			// Mendix's pointer names are inverted relative to MDL (CLAUDE.md):
			// the Child* delete behaviour belongs to the TO end, Parent* to FROM.
			toDB, toMsg := deleteBehaviorColumns(assoc.ChildDeleteBehavior)
			fromDB, fromMsg := deleteBehaviorColumns(assoc.ParentDeleteBehavior)
			_, err := stmt.Exec(
				string(assoc.ID),
				assoc.Name,
				modName+"."+assoc.Name,
				modName,
				from,
				to,
				string(assoc.Type),
				string(assoc.Owner),
				string(assoc.StorageFormat),
				assoc.Documentation,
				toDB, toMsg, fromDB, fromMsg,
				projectID, snapshotID,
			)
			if err != nil {
				return err
			}
			count++
		}

		for _, ca := range dm.CrossAssociations {
			from := entityNames[ca.ParentID]
			if from == "" {
				from = string(ca.ParentID)
			}
			// Same inversion as above; ChildRef is the TO end by qualified name.
			toDB, toMsg := deleteBehaviorColumns(ca.ChildDeleteBehavior)
			fromDB, fromMsg := deleteBehaviorColumns(ca.ParentDeleteBehavior)
			_, err := stmt.Exec(
				string(ca.ID),
				ca.Name,
				modName+"."+ca.Name,
				modName,
				from,
				ca.ChildRef,
				string(ca.Type),
				string(ca.Owner),
				string(ca.StorageFormat),
				ca.Documentation,
				toDB, toMsg, fromDB, fromMsg,
				projectID, snapshotID,
			)
			if err != nil {
				return err
			}
			count++
		}
	}

	b.report("Associations", count)
	return nil
}

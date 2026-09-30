// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// StoredCommitEvents answers, for `mxcli fmt --upgrade -p app.mpr`, what each
// Commit activity of a stored flow does with events (ako/mxcli#873): a bare
// `commit $X;` means WITH events since #895, and a flow an older mxcli stored
// from the same script commits without them. The upgrade pins the stored flag
// onto the script so that re-running it keeps what is stored.
type StoredCommitEvents struct {
	b backend.FullBackend
}

// NewStoredCommitEvents reads stored flows from a connected backend.
func NewStoredCommitEvents(b backend.FullBackend) *StoredCommitEvents {
	return &StoredCommitEvents{b: b}
}

// CommitEvents returns, per committed variable, the WithEvents flag of each
// Commit activity in the stored microflow (the nanoflow when nanoflow is set)
// named qualifiedName, loop bodies included. found is false when the project
// has no such flow.
func (s *StoredCommitEvents) CommitEvents(nanoflow bool, qualifiedName string) (map[string][]bool, bool) {
	objs, found := s.flowObjects(nanoflow, qualifiedName)
	if !found {
		return nil, false
	}
	events := map[string][]bool{}
	var walk func(*microflows.MicroflowObjectCollection)
	walk = func(oc *microflows.MicroflowObjectCollection) {
		if oc == nil {
			return
		}
		for _, o := range oc.Objects {
			switch a := o.(type) {
			case *microflows.ActionActivity:
				if c, ok := a.Action.(*microflows.CommitObjectsAction); ok {
					events[c.CommitVariable] = append(events[c.CommitVariable], c.WithEvents)
				}
			case *microflows.LoopedActivity:
				walk(a.ObjectCollection)
			}
		}
	}
	walk(objs)
	return events, true
}

// flowObjects reads the stored flow's object collection.
func (s *StoredCommitEvents) flowObjects(nanoflow bool, qualifiedName string) (*microflows.MicroflowObjectCollection, bool) {
	kind := "microflow"
	if nanoflow {
		kind = "nanoflow"
	}
	if raw, err := s.b.GetRawUnitByName(kind, qualifiedName); err == nil && raw != nil && len(raw.Contents) > 0 {
		if mf, err := s.b.ParseMicroflowBSON(raw.Contents, model.ID(raw.ID), ""); err == nil && mf != nil {
			return mf.ObjectCollection, true
		}
	}
	// The lookup by name failed: find the flow by its module and name.
	moduleName, name, ok := strings.Cut(qualifiedName, ".")
	if !ok {
		return nil, false
	}
	module, err := s.b.GetModuleByName(moduleName)
	if err != nil || module == nil {
		return nil, false
	}
	h, err := NewContainerHierarchyFromBackend(s.b)
	if err != nil {
		return nil, false
	}
	if nanoflow {
		nfs, err := s.b.ListNanoflows()
		if err != nil {
			return nil, false
		}
		for _, nf := range nfs {
			if nf != nil && nf.Name == name && h.FindModuleID(nf.ContainerID) == module.ID {
				return nf.ObjectCollection, true
			}
		}
		return nil, false
	}
	mfs, err := s.b.ListMicroflows()
	if err != nil {
		return nil, false
	}
	for _, mf := range mfs {
		if mf != nil && mf.Name == name && h.FindModuleID(mf.ContainerID) == module.ID {
			return mf.ObjectCollection, true
		}
	}
	return nil, false
}

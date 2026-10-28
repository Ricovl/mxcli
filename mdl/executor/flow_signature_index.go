// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/gob"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/model"
)

// The flow signature index: what check reads of every stored microflow and
// nanoflow — name, excluded flag, allowed roles, parameters and return entity —
// and, for MDL-SEC22, a microflow's ApplyEntityAccess and the flows it calls —
// stamped with the unit file's size and mtime and persisted in
// .mxcli/flow-signatures. A flow whose file still matches its stamp is not
// decoded again; on a 23k-unit app decoding every flow's signature twice was a
// third of check.

const flowIndexFormat = 2

// indexedFlow is one flow's index entry.
type indexedFlow struct {
	Kind     string // "microflow" or "nanoflow"
	Name     string
	Excluded bool
	Roles    []string
	Sig      flowSignature
	EA       bool          // microflows: ApplyEntityAccess
	Calls    []indexedCall // microflows: called microflows and rules
	Size     int64
	MtimeNs  int64

	containerID model.ID // from the current stamp, never persisted
}

type indexedCall struct{ Kind, Name string }

type persistedFlowIndex struct {
	Format int
	Flows  map[string]indexedFlow
}

// storedFlowIndex returns every stored flow, decoding only the ones whose unit
// file changed since the index recorded it. ok is false when the backend has
// no unit stamps (MPR v1), and callers then list the flows as before.
func storedFlowIndex(ctx *ExecContext) (flows []indexedFlow, ok bool) {
	b, isStamper := ctx.Backend.(unitStampBackend)
	if !isStamper {
		return nil, false
	}
	stamps, err := b.UnitStamps()
	if err != nil || len(stamps) == 0 {
		return nil, false
	}

	var known map[string]indexedFlow
	if ctx.Cache != nil && ctx.Cache.flowIndex != nil {
		known = ctx.Cache.flowIndex
	} else {
		known = loadFlowIndex(ctx)
	}

	next := make(map[string]indexedFlow, len(known))
	var stale []string
	for _, s := range stamps {
		kind := flowKindOf(s.Type)
		if kind == "" {
			continue
		}
		e, have := known[s.ID]
		if !have || s.MtimeNs == -1 || e.Size != s.Size || e.MtimeNs != s.MtimeNs {
			stale = append(stale, s.ID)
			continue
		}
		e.containerID = model.ID(s.ContainerID)
		next[s.ID] = e
	}
	if len(stale) > 0 {
		fresh, err := decodeFlowEntries(ctx, stale)
		if err != nil {
			return nil, false
		}
		byID := map[string]indexedFlow{}
		for id, e := range fresh {
			byID[id] = e
		}
		for _, s := range stamps {
			if e, ok := byID[s.ID]; ok {
				e.Size, e.MtimeNs, e.containerID = s.Size, s.MtimeNs, model.ID(s.ContainerID)
				next[s.ID] = e
			}
		}
		saveFlowIndex(ctx, next)
	}
	if ctx.Cache != nil {
		ctx.Cache.flowIndex = next
	}
	flows = make([]indexedFlow, 0, len(next))
	for _, e := range next {
		flows = append(flows, e)
	}
	return flows, true
}

func flowKindOf(unitType string) string {
	switch unitType {
	case "Microflows$Microflow":
		return "microflow"
	case "Microflows$Nanoflow":
		return "nanoflow"
	}
	return ""
}

// decodeFlowEntries builds the entries of the given flows: one at a time when
// few changed, through the body-stripped listing when many did (a first run).
func decodeFlowEntries(ctx *ExecContext, ids []string) (map[string]indexedFlow, error) {
	out := map[string]indexedFlow{}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	roles := func(ids []model.ID) []string {
		r := make([]string, len(ids))
		for i, id := range ids {
			r[i] = string(id)
		}
		return r
	}
	if len(ids) > 200 {
		mfs, err := microflowSignatures(ctx.Backend)
		if err != nil {
			return nil, err
		}
		for _, mf := range mfs {
			if mf != nil && want[string(mf.ID)] {
				out[string(mf.ID)] = indexedFlow{Kind: "microflow", Name: mf.Name, Excluded: mf.Excluded,
					Roles: roles(mf.AllowedModuleRoles), Sig: *sdkFlowSignature(mf.Parameters, mf.ReturnType)}
			}
		}
		nfs, err := nanoflowSignatures(ctx.Backend)
		if err != nil {
			return nil, err
		}
		for _, nf := range nfs {
			if nf != nil && want[string(nf.ID)] {
				out[string(nf.ID)] = indexedFlow{Kind: "nanoflow", Name: nf.Name, Excluded: nf.Excluded,
					Roles: roles(nf.AllowedModuleRoles), Sig: *sdkFlowSignature(nf.Parameters, nf.ReturnType)}
			}
		}
		return out, addEntityAccess(ctx, out, true)
	}
	for _, id := range ids {
		if mf, err := ctx.Backend.GetMicroflow(model.ID(id)); err == nil && mf != nil {
			out[id] = indexedFlow{Kind: "microflow", Name: mf.Name, Excluded: mf.Excluded,
				Roles: roles(mf.AllowedModuleRoles), Sig: *sdkFlowSignature(mf.Parameters, mf.ReturnType)}
			continue
		}
		if nf, err := ctx.Backend.GetNanoflow(model.ID(id)); err == nil && nf != nil {
			out[id] = indexedFlow{Kind: "nanoflow", Name: nf.Name, Excluded: nf.Excluded,
				Roles: roles(nf.AllowedModuleRoles), Sig: *sdkFlowSignature(nf.Parameters, nf.ReturnType)}
		}
	}
	return out, addEntityAccess(ctx, out, false)
}

// addEntityAccess fills EA and Calls of the microflow entries from their raw
// BSON: through one listing when many changed, else unit by unit.
func addEntityAccess(ctx *ExecContext, out map[string]indexedFlow, many bool) error {
	set := func(id string, contents []byte) {
		e, ok := out[id]
		if !ok || e.Kind != "microflow" {
			return
		}
		ea, calls := rawEntityAccess(contents)
		e.EA, e.Calls = ea, nil
		for _, c := range calls {
			e.Calls = append(e.Calls, indexedCall{c.kind, c.name})
		}
		out[id] = e
	}
	if many {
		units, err := ctx.Backend.ListRawUnitsByType("Microflows$Microflow")
		if err != nil {
			return err
		}
		for _, u := range units {
			if u.Type == "Microflows$Microflow" {
				set(string(u.ID), u.Contents)
			}
		}
		return nil
	}
	for id, e := range out {
		if e.Kind != "microflow" {
			continue
		}
		b, err := ctx.Backend.GetRawUnitBytes(model.ID(id))
		if err != nil {
			return err
		}
		set(id, b)
	}
	return nil
}

// flowIndexPath persists only into an existing .mxcli directory.
func flowIndexPath(ctx *ExecContext) string {
	if ctx.MprPath == "" {
		return ""
	}
	dir := filepath.Join(filepath.Dir(ctx.MprPath), ".mxcli")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ""
	}
	return filepath.Join(dir, "flow-signatures")
}

func loadFlowIndex(ctx *ExecContext) map[string]indexedFlow {
	path := flowIndexPath(ctx)
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var p persistedFlowIndex
	if gob.NewDecoder(f).Decode(&p) != nil || p.Format != flowIndexFormat {
		return nil
	}
	return p.Flows
}

// saveFlowIndex writes via temp file and rename; failure only costs a decode.
func saveFlowIndex(ctx *ExecContext, flows map[string]indexedFlow) {
	path := flowIndexPath(ctx)
	if path == "" {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "flow-signatures-*")
	if err != nil {
		return
	}
	err = gob.NewEncoder(tmp).Encode(persistedFlowIndex{Format: flowIndexFormat, Flows: flows})
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
	}
}

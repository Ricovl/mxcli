// SPDX-License-Identifier: Apache-2.0

package mpr

// InvalidateUnit is InvalidateCache for a write to one unit: on MPR v2 it
// re-reads that unit's row and file into the unit cache and the name index
// instead of dropping them, so the next lookup does not rebuild either from
// every unit. Derived caches that a changed unit can affect are dropped.
func (r *Reader) InvalidateUnit(unitID string) {
	blob := uuidToBlob(unitID)
	if r.version != MPRVersionV2 || !r.unitCacheValid || blob == nil {
		r.InvalidateCache()
		return
	}
	id := blobToUUID(blob)
	var containerID []byte
	var containment, rowHash string
	err := r.db.QueryRow(`SELECT ContainerID, ContainmentName, COALESCE(ContentsHash, '') FROM Unit WHERE UnitID = ?`, blob).Scan(&containerID, &containment, &rowHash)

	idx := -1
	for i := range r.unitCache {
		if r.unitCache[i].ID == id {
			idx = i
			break
		}
	}
	oldType := ""
	var old *cachedUnit
	if idx >= 0 {
		oldType = r.unitCache[idx].Type
		o := r.unitCache[idx]
		old = &o
	}

	var cu cachedUnit
	present := err == nil
	if present {
		// Through the read path, so the unit store keeps the new copy under
		// the row's new hash.
		if store := r.unitStore(); store != nil {
			store.noteRowHash(id, rowHash)
		}
		r.contentMu.Lock()
		delete(r.contentCache, id)
		r.contentMu.Unlock()
		contents, rerr := r.readUnit(id, false)
		if rerr != nil {
			r.InvalidateCache()
			return
		}
		e := unitIndexEntryOf(contents)
		r.v2TypesMu.Lock()
		r.learnUnit(contentsHash(contents), e)
		r.v2TypesMu.Unlock()
		cu = cachedUnit{ID: id, ContainerID: blobToUUID(containerID), ContainmentName: containment, Type: e.Type, Name: e.Name, Excluded: e.Excluded}
		if held, ok := r.overlaid(id); ok {
			h := unitIndexEntryOf(held)
			cu.Type, cu.Name, cu.Excluded = h.Type, h.Name, h.Excluded
		}
	}
	switch {
	case present && idx >= 0:
		r.unitCache[idx] = cu
	case present:
		r.unitCache = append(r.unitCache, cu)
	case idx >= 0:
		r.unitCache = append(r.unitCache[:idx], r.unitCache[idx+1:]...)
	}

	var now *cachedUnit
	if present {
		now = &cu
	}
	r.patchNameIndex(old, now)
	r.decodedMu.Lock()
	delete(r.decoded, oldType)
	delete(r.decoded, cu.Type)
	r.decodedMu.Unlock()
	r.contentMu.Lock()
	delete(r.contentCache, id)
	r.contentMu.Unlock()
}

// patchNameIndex moves one unit's entries in the name index from old to now
// (either nil for an insert or a delete). A module or folder changes every
// qualified name below it, so the index is dropped and rebuilt on next use.
func (r *Reader) patchNameIndex(old, now *cachedUnit) {
	r.nameIndexMu.Lock()
	defer r.nameIndexMu.Unlock()
	if !r.nameIndexBuilt {
		return
	}
	structural := func(cu *cachedUnit) bool {
		return cu != nil && (cu.Type == "Projects$ModuleImpl" || cu.Type == "Projects$Module" || cu.Type == "Projects$Folder")
	}
	if structural(old) || structural(now) {
		r.nameIndex, r.nameIndexBuilt = nil, false
		r.moduleNameByID, r.containerParent, r.folderIDs = nil, nil, nil
		r.headersByType, r.headerByID = nil, nil
		return
	}
	key := func(cu *cachedUnit) string {
		qn := cu.Name
		if m := ResolveModuleName(cu.ContainerID, r.moduleNameByID, r.containerParent); m != "" {
			qn = m + "." + cu.Name
		}
		return cu.Type + "\x00" + qn
	}
	info := func(cu *cachedUnit) UnitHeaderInfo {
		return UnitHeaderInfo{ID: cu.ID, ContainerID: cu.ContainerID, Type: cu.Type, Name: cu.Name, Excluded: cu.Excluded}
	}
	entry := func(cu *cachedUnit) nameIndexEntry {
		return nameIndexEntry{ID: cu.ID, ContainerID: cu.ContainerID, Type: cu.Type}
	}

	// Headers by type: replaced in place on an update, so the listing order
	// stays what a fresh build gives.
	replaced := false
	if old != nil {
		list := r.headersByType[old.Type]
		for i := range list {
			if list[i].ID != old.ID {
				continue
			}
			if now != nil && now.Type == old.Type {
				list[i], replaced = info(now), true
			} else {
				r.headersByType[old.Type] = append(list[:i:i], list[i+1:]...)
			}
			break
		}
		delete(r.headerByID, old.ID)
		delete(r.containerParent, old.ID)
		if old.Name != "" {
			k := key(old)
			entries := r.nameIndex[k]
			for i := range entries {
				if entries[i].ID == old.ID {
					entries = append(entries[:i:i], entries[i+1:]...)
					break
				}
			}
			if len(entries) == 0 {
				delete(r.nameIndex, k)
			} else {
				r.nameIndex[k] = entries
			}
		}
	}
	if now != nil {
		if !replaced {
			r.headersByType[now.Type] = append(r.headersByType[now.Type], info(now))
		}
		r.headerByID[now.ID] = info(now)
		r.containerParent[now.ID] = now.ContainerID
		if now.Name != "" {
			k := key(now)
			r.nameIndex[k] = append(r.nameIndex[k], entry(now))
		}
	}
}

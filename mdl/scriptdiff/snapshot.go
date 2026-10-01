// SPDX-License-Identifier: Apache-2.0

package scriptdiff

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// unit is one unit of a project as stored: its bytes and its place in the
// project tree. The place is a row of the .mpr's Unit table, not part of the
// unit's contents, so a statement that only moves a document to another folder
// rewrites no unit bytes and is visible here alone.
type unit struct {
	Type, Name             string
	Container, Containment string
	Raw                    []byte
}

// Snapshot is a project's units by ID, and the hashes of the files next to it.
type Snapshot struct {
	units map[string]unit
	files map[string][sha256.Size]byte // path relative to the project folder
}

// TakeSnapshot reads every unit of the project at mprPath, and hashes every
// file in its folder that is not part of the model.
func TakeSnapshot(mprPath string) (*Snapshot, error) {
	r, err := mmpr.Open(mprPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer r.Close()
	list, err := r.ListUnits()
	if err != nil {
		return nil, fmt.Errorf("list units: %w", err)
	}
	s := &Snapshot{units: make(map[string]unit, len(list)), files: map[string][sha256.Size]byte{}}
	for _, u := range list {
		b, err := r.GetRawUnitBytes(u.ID)
		if err != nil {
			return nil, fmt.Errorf("read unit %s: %w", u.ID, err)
		}
		typ, name := typeAndName(b)
		if typ == "" {
			typ = u.Type
		}
		s.units[u.ID] = unit{Type: typ, Name: name, Container: u.ContainerID, Containment: u.ContainmentName, Raw: b}
	}

	dir := filepath.Dir(mprPath)
	mprBase := filepath.Base(mprPath)
	err = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		top := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		// The model itself is compared as units above. .mxcli holds mxcli's own
		// caches (the catalog, widget definitions), and the widget docs are
		// generated from them alongside; any run of mxcli may refresh both, so
		// they are not what a script writes.
		if top == "mprcontents" || top == ".mxcli" || strings.HasPrefix(top, mprBase) ||
			isWidgetDocs(filepath.ToSlash(rel)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		var sum [sha256.Size]byte
		copy(sum[:], h.Sum(nil))
		s.files[filepath.ToSlash(rel)] = sum
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}
	return s, nil
}

// isWidgetDocs reports whether rel is the folder of generated widget docs
// (executor.WidgetDocsDirs) or inside it.
func isWidgetDocs(rel string) bool {
	for _, d := range []string{".claude/skills/widgets", ".ai-context/skills/widgets"} {
		if rel == d || strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}

// typeAndName reads a unit's $Type and Name without decoding the whole unit.
func typeAndName(b []byte) (string, string) {
	raw := bson.Raw(b)
	if raw.Validate() != nil {
		return "", ""
	}
	typ, _ := raw.Lookup("$Type").StringValueOK()
	name, _ := raw.Lookup("Name").StringValueOK()
	return typ, name
}

// ChangeKind is what exec does to a unit or a file.
type ChangeKind string

const (
	Added    ChangeKind = "added"
	Modified ChangeKind = "modified"
	Removed  ChangeKind = "removed"
)

// UnitChange is one unit exec writes.
type UnitChange struct {
	ID   string
	Kind ChangeKind
	// Type is the unit's storage $Type, e.g. Microflows$Microflow.
	Type string
	// Name is the unit's qualified name where it has one (Module.Name, the
	// module name for a domain model or module security, a folder path for a
	// folder), else its plain name.
	Name string
	// Moved is set when the unit is placed in another container: the folder
	// path it is moved to.
	Moved string
	// Rewritten is false for a unit that is only moved.
	Rewritten bool
	// IDChurnOnly marks a rewrite whose canonical form is unchanged: only
	// element $IDs differ. Exec's write elision (ADR-0008) should never let one
	// through, so one here is a defect worth reporting, not a change of meaning.
	IDChurnOnly bool

	before, after []byte
}

// FileChange is one file next to the model that exec writes or deletes.
type FileChange struct {
	Path string
	Kind ChangeKind
}

// Compare lists what changed from s to o: the units and files a run wrote.
func (s *Snapshot) Compare(o *Snapshot) ([]UnitChange, []FileChange) {
	var units []UnitChange
	for id, a := range s.units {
		b, ok := o.units[id]
		if !ok {
			units = append(units, UnitChange{ID: id, Kind: Removed, Type: a.Type, Name: s.qualifiedName(id), before: a.Raw})
			continue
		}
		rewritten := !bytes.Equal(a.Raw, b.Raw)
		moved := a.Container != b.Container || a.Containment != b.Containment
		if !rewritten && !moved {
			continue
		}
		c := UnitChange{ID: id, Kind: Modified, Type: b.Type, Name: o.qualifiedName(id), Rewritten: rewritten, before: a.Raw, after: b.Raw}
		if moved {
			c.Moved = o.folderPath(b.Container)
			if c.Moved == "" {
				c.Moved = "(module root)"
			}
		}
		if rewritten {
			if eq, err := canon.Equal(a.Raw, b.Raw); err == nil && eq {
				c.IDChurnOnly = true
			}
		}
		units = append(units, c)
	}
	for id, b := range o.units {
		if _, ok := s.units[id]; !ok {
			units = append(units, UnitChange{ID: id, Kind: Added, Type: b.Type, Name: o.qualifiedName(id), Rewritten: true, after: b.Raw})
		}
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].Name != units[j].Name {
			return units[i].Name < units[j].Name
		}
		if units[i].Type != units[j].Type {
			return units[i].Type < units[j].Type
		}
		return units[i].ID < units[j].ID
	})

	var files []FileChange
	for p, b := range o.files {
		if a, ok := s.files[p]; !ok {
			files = append(files, FileChange{Path: p, Kind: Added})
		} else if a != b {
			files = append(files, FileChange{Path: p, Kind: Modified})
		}
	}
	for p := range s.files {
		if _, ok := o.files[p]; !ok {
			files = append(files, FileChange{Path: p, Kind: Removed})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return units, files
}

const moduleType = "Projects$ModuleImpl"

// moduleOf returns the name of the module a unit sits in, "" for a
// project-level unit.
func (s *Snapshot) moduleOf(id string) string {
	for range 64 { // a cycle in a corrupt tree must not hang diff
		u, ok := s.units[id]
		if !ok {
			return ""
		}
		if u.Type == moduleType {
			return u.Name
		}
		if u.Container == id {
			return ""
		}
		id = u.Container
	}
	return ""
}

// folderPath is the path of folder names from a unit's module down to it,
// "Module/A/B" for a folder, "Module" for a module.
func (s *Snapshot) folderPath(id string) string {
	var parts []string
	for range 64 {
		u, ok := s.units[id]
		if !ok || u.Container == id {
			break
		}
		parts = append([]string{u.Name}, parts...)
		if u.Type == moduleType {
			break
		}
		id = u.Container
	}
	return strings.Join(parts, "/")
}

// qualifiedName names a unit the way MDL does.
func (s *Snapshot) qualifiedName(id string) string {
	u := s.units[id]
	switch u.Type {
	case moduleType:
		return u.Name
	case "Projects$Folder":
		return s.folderPath(id)
	}
	mod := s.moduleOf(id)
	switch {
	case u.Name == "":
		return mod
	case mod == "":
		return u.Name
	}
	return mod + "." + u.Name
}

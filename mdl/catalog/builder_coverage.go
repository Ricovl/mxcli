// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Lookup sites, and the unit types whose change can alter what each one
// answers: a name a member lookup probes can only start or stop resolving
// when a domain model or an enumeration changes, whatever unit happens to
// hold a name with that prefix (an icon collection's `Mod.Icons.pencil` is
// no member).
const (
	siteMemberRefs = "member refs"
	siteAssocEnds  = "association ends"
	siteEntityByID = "entity by ID"
	siteFlowByID   = "flow by ID"
)

var siteHolderTypes = map[string][]string{
	siteMemberRefs: {"DomainModels$DomainModel", "Enumerations$Enumeration"},
	siteAssocEnds:  {"DomainModels$DomainModel"},
	siteEntityByID: {"DomainModels$DomainModel"},
	siteFlowByID:   {"Microflows$Microflow", "Microflows$Nanoflow", "Microflows$Rule"},
}

// qualifiedName is what a lookup of a model name can be: identifiers joined
// by dots. A path (`../datasource`) or a sentence is not one.
var qualifiedName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)+$`)

// Ref coverage (ako/mxcli#1804).
//
// An incremental build rebuilds a document when a unit it names in refs
// changes. That is only correct if refs records every other document a pass
// reads while writing the document's rows: a read refs does not cover leaves
// stale rows behind. Those reads happen in a few lookups — the member names
// a unit's strings resolve against, the ends of the association a retrieve
// follows, the names of entities and flows found by ID — and each of them
// reports what it looked up, found or not (a name that is missing today
// changes the rows when it appears). With tracing on, DependencyCoverage
// then asks of every read what the plan asks of a change: would a change to
// the unit holding that name rebuild the reader?

// SetTraceDependencies records, during the next Build, every name a pass
// looks up for a document. It is for tests and diagnosis: DependencyCoverage
// reports the result.
func (b *Builder) SetTraceDependencies(on bool) {
	b.deps = nil
	if on {
		b.deps = map[string]map[string]string{}
	}
}

// dependsOn records that the document being built read name, at site.
func (b *Builder) dependsOn(site, name string) {
	if b.deps == nil || !qualifiedName.MatchString(name) {
		return
	}
	owner := b.currentOwner()
	if owner == "" || owner == skippedOwner {
		return // a project-wide row is written again by every build
	}
	reads := b.deps[owner]
	if reads == nil {
		reads = map[string]string{}
		b.deps[owner] = reads
	}
	if _, seen := reads[name]; !seen {
		reads[name] = site
	}
}

// probe is dependsOn for one site, for the lookups that take a callback.
func (b *Builder) probe(site string) func(string) {
	if b.deps == nil {
		return nil
	}
	return func(name string) { b.dependsOn(site, name) }
}

// DependencyCoverage is how many of the traced reads the plan covers.
type DependencyCoverage struct {
	Reads     int // reads of a name some unit holds, in another unit
	Covered   int // ... that a change to that unit would rebuild the reader for
	Self      int // reads of the reader's own unit
	Unowned   int // reads of a name no unit holds (a missing name; the build's check covers its appearance)
	Uncovered []UncoveredRead
}

// UncoveredRead is a read the plan would not follow.
type UncoveredRead struct {
	Reader string // the reading document's qualified name (or unit ID)
	Name   string // what it read
	Holder string // a qualified name of the unit that holds it
	Site   string // which lookup read it
}

// DependencyCoverage checks the reads traced during the last Build against
// the catalog it built.
func (b *Builder) DependencyCoverage() (*DependencyCoverage, error) {
	g, err := loadRefGraph(b.catalog.db.Query)
	if err != nil {
		return nil, err
	}
	// holder is the unit holding name, or the nearest member-prefix that a
	// unit holds (`M.E.Attr` is held by M.E's domain model), among the units
	// whose change can alter what the site's lookup answers. A name no
	// unit holds (a System entity) has no holder.
	holder := func(site, name string) string {
		types := siteHolderTypes[site]
		for n := name; n != ""; {
			for _, u := range g.unitOf[n] {
				t, isUnit := b.unitTypes[u]
				if isUnit && (len(types) == 0 || slices.Contains(types, t)) {
					return u
				}
			}
			i := strings.LastIndexByte(n, '.')
			if i < 0 {
				return ""
			}
			n = n[:i]
		}
		return ""
	}
	label := func(unit string) string {
		if names := g.namesOf[unit]; len(names) > 0 {
			sorted := append([]string(nil), names...)
			sort.Strings(sorted)
			return sorted[0]
		}
		return unit
	}
	cov := &DependencyCoverage{}
	rebuilds := map[string]map[string]bool{} // holder -> the units a change to it rebuilds
	for reader, reads := range b.deps {
		for name, site := range reads {
			h := holder(site, name)
			switch {
			case h == "":
				cov.Unowned++
				continue
			case h == reader:
				cov.Self++
				continue
			}
			cov.Reads++
			deps, ok := rebuilds[h]
			if !ok {
				deps = g.dependents(g.namesOf[h])
				rebuilds[h] = deps
			}
			if deps[reader] {
				cov.Covered++
				continue
			}
			cov.Uncovered = append(cov.Uncovered, UncoveredRead{label(reader), name, label(h), site})
		}
	}
	sort.Slice(cov.Uncovered, func(i, j int) bool {
		a, c := cov.Uncovered[i], cov.Uncovered[j]
		if a.Site != c.Site {
			return a.Site < c.Site
		}
		if a.Reader != c.Reader {
			return a.Reader < c.Reader
		}
		return a.Name < c.Name
	})
	return cov, nil
}

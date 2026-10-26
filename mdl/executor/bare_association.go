// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `retrieve $C from $O/Ord_Cust` — an association written by its bare name —
// passed check -p --references and exec, and was stored as the association
// ".Ord_Cust", with an empty module: CE1613 "The selected association
// '.Ord_Cust' no longer exists.", and a describe that does not parse
// (ako/mxcli#1690, measured on 11.13.0). The builder joined the AST's empty
// Module to the name.
//
// Which association a bare name means from an object is answered here once,
// as a function of the data its caller hands it (resolveBareAssociation): the
// flow builder feeds it the project as the statements before have left it,
// check --references the project overlaid with the associations the script
// declares before the flow — so the two cannot disagree about the rule
// (docs-wiki/bug-patterns/duplicate-resolver-drift.md).

// generalizationChainFor is generalizationChain against a backend: entityQN
// followed by its generalizations, nearest first, and whether the walk reached
// the root. An incomplete chain is enough to answer "yes", never "no".
func generalizationChainFor(b entityLookupBackend, entityQN string) ([]string, bool) {
	if b == nil || entityQN == "" {
		return nil, false
	}
	var chain []string
	seen := map[string]bool{}
	for currentQN := entityQN; currentQN != ""; {
		if seen[currentQN] {
			return chain, false
		}
		seen[currentQN] = true
		chain = append(chain, currentQN)
		entity, ok := findEntityByQN(b, currentQN)
		if !ok {
			return chain, false
		}
		currentQN = entity.GeneralizationRef
	}
	return chain, true
}

// associationCatalog is every association of a project with its two ends,
// keyed by lower-cased qualified name. unnamed holds the bare names of
// associations whose ends could not be named (an external entity, a dangling
// pointer): a bare name that matches one of them cannot be ruled out.
type associationCatalog struct {
	shapes   map[string]assocShape
	unnamed  map[string]bool
	complete bool // the module list and every domain model were read
}

// projectAssociationCatalog indexes the associations of the project behind b.
// A domain model is kept as two lists — an association to another module (or
// to System) is a CrossAssociation with the far end held by name — and both
// are read, or every association that leaves its module is missed.
func projectAssociationCatalog(modules []*model.Module, dms []*domainmodel.DomainModel) associationCatalog {
	c := associationCatalog{shapes: map[string]assocShape{}, unnamed: map[string]bool{}, complete: true}
	moduleNames := make(map[model.ID]string, len(modules))
	for _, m := range modules {
		if m != nil {
			moduleNames[m.ID] = m.Name
		}
	}
	byID := map[model.ID]string{}
	for _, dm := range dms {
		if dm == nil {
			continue
		}
		if mod := moduleNames[dm.ContainerID]; mod != "" {
			for _, ent := range dm.Entities {
				if ent != nil {
					byID[ent.ID] = mod + "." + ent.Name
				}
			}
		}
	}
	for _, dm := range dms {
		if dm == nil {
			continue
		}
		mod := moduleNames[dm.ContainerID]
		if mod == "" {
			c.complete = false
			continue
		}
		for _, a := range dm.Associations {
			if a == nil {
				continue
			}
			from, to := byID[a.ParentID], byID[a.ChildID]
			if from == "" || to == "" {
				c.unnamed[a.Name] = true
				continue
			}
			qn := mod + "." + a.Name
			c.shapes[strings.ToLower(qn)] = assocShape{qn: qn, from: from, to: to,
				refSet: a.Type == domainmodel.AssociationTypeReferenceSet, owner: sdkOwnerName(a.Owner),
				view: a.Source == domainmodel.OqlViewAssociationSource}
		}
		for _, a := range dm.CrossAssociations {
			if a == nil {
				continue
			}
			from := byID[a.ParentID]
			if from == "" || a.ChildRef == "" {
				c.unnamed[a.Name] = true
				continue
			}
			qn := mod + "." + a.Name
			c.shapes[strings.ToLower(qn)] = assocShape{qn: qn, from: from, to: a.ChildRef,
				refSet: a.Type == domainmodel.AssociationTypeReferenceSet, owner: sdkOwnerName(a.Owner),
				view: a.Source == domainmodel.OqlViewAssociationSource}
		}
	}
	return c
}

// associationCatalogFor reads the catalog through a backend.
func associationCatalogFor(b interface {
	ListModules() ([]*model.Module, error)
	ListDomainModels() ([]*domainmodel.DomainModel, error)
}) associationCatalog {
	if b == nil {
		return associationCatalog{}
	}
	modules, err := b.ListModules()
	if err != nil {
		return associationCatalog{}
	}
	dms, err := b.ListDomainModels()
	if err != nil {
		return associationCatalog{}
	}
	return projectAssociationCatalog(modules, dms)
}

// bareAssocVerdict is the answer to "which association does this bare name
// mean from this entity".
type bareAssocVerdict int

const (
	// bareAssocResolved: exactly one association of the name has the entity,
	// or one of its generalizations, as an end.
	bareAssocResolved bareAssocVerdict = iota
	// bareAssocMissing: none does, the whole chain and every association
	// were read. Refused.
	bareAssocMissing
	// bareAssocAmbiguous: more than one does — two modules may each have an
	// association of the name. Refused: picking one is a guess.
	bareAssocAmbiguous
	// bareAssocUnknown: something could not be read, so no association was
	// established. exec still refuses — the bare name cannot be written, it
	// is stored with an empty module — and check stays silent.
	bareAssocUnknown
)

// resolveBareAssociation picks the association a bare name means when a flow
// follows it from an object: the one of that name with an end anywhere on the
// object's generalization chain (an association is traversable from either
// end, and a specialization inherits its generalizations' associations).
//
// chain is the entity followed by its generalizations; chainComplete whether
// it reached the root. matches lists every association that fits, sorted.
func resolveBareAssociation(name string, chain []string, chainComplete bool, cat associationCatalog) (qn string, matches []string, verdict bareAssocVerdict) {
	onChain := make(map[string]bool, len(chain))
	for _, e := range chain {
		onChain[e] = true
	}
	for _, sh := range cat.shapes {
		if _, short, ok := strings.Cut(sh.qn, "."); !ok || short != name {
			continue
		}
		if onChain[sh.from] || onChain[sh.to] {
			matches = append(matches, sh.qn)
		}
	}
	sort.Strings(matches)
	switch {
	case len(matches) == 1:
		return matches[0], matches, bareAssocResolved
	case len(matches) > 1:
		return "", matches, bareAssocAmbiguous
	case len(chain) == 0 || !chainComplete || !cat.complete || cat.unnamed[name]:
		return "", nil, bareAssocUnknown
	}
	return "", nil, bareAssocMissing
}

// bareRetrieveAssociationRefusal is the message exec refuses a bare-association
// retrieve with, and check reports it with: what the name could not be
// resolved to, and the qualified spelling that would work. entityQN is "" when
// the start variable's entity is not known.
func bareRetrieveAssociationRefusal(variable, start, name, entityQN string, matches []string, v bareAssocVerdict) string {
	stmt := fmt.Sprintf("retrieve $%s from $%s/%s", variable, start, name)
	const stored = "written as it stands it is stored with an empty module, and mxbuild reports CE1613 " +
		"\"The selected association '.%s' no longer exists\""
	switch v {
	case bareAssocAmbiguous:
		return fmt.Sprintf("%s: %q could be any of %s from %s — qualify it with the one you mean (`$%s/%s`); "+stored,
			stmt, name, strings.Join(matches, ", "), entityQN, start, matches[0], name)
	case bareAssocMissing:
		return fmt.Sprintf("%s: no association called %q has %s, or an entity it extends, at either end — "+
			"fix the name, or qualify it as `$%s/Module.%s`; "+stored,
			stmt, name, entityQN, start, name, name)
	}
	why := "the entity $" + start + " holds is not known here"
	if entityQN != "" {
		why = "the model of " + entityQN + " could not be read in full"
	}
	return fmt.Sprintf("%s: the association is not qualified, and %s, so which association it names cannot be "+
		"worked out — qualify it as `$%s/Module.%s`; "+stored, stmt, why, start, name, name)
}

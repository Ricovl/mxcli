// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

// viewSourceEntityErrors reports the entities a view entity's OQL reads from —
// FROM, JOIN and the end of an association-path join — that neither the
// project nor the script has (mendixlabs/mxcli#1472). Before, an unknown
// source only made its columns infer no type, and an untyped column is
// skipped, so `from Administration.NoSuchEntity as a` passed check, exec wrote
// it, and mxbuild (11.13.0) answered CE0174 "Error(s) in OQL query: Cannot
// resolve object name 'Administration.NoSuchEntity'".
//
// The sources are read by extractAliasMap, the same reading the type inference
// uses. Could-not-establish is silence: a module the script creates, a module a
// bulk external-entity import fills, a backend that lists no entities, and a
// System entity the metadata does not list.
func viewSourceEntityErrors(ctx *ExecContext, oql string, sc *scriptContext) []string {
	sources := map[string]bool{}
	for _, qn := range extractAliasMap(oql) {
		sources[qn] = true
	}
	if len(sources) == 0 {
		return nil
	}
	var known map[string]bool
	lowerKnown := map[string]bool{}
	var missing []string
	for qn := range sources {
		mod, name, ok := strings.Cut(qn, ".")
		if !ok || mod == "" || name == "" {
			continue
		}
		if sc.modules[mod] || sc.externalEntityModules[mod] || sc.entities[qn] || sc.indirectEntities[qn] {
			continue
		}
		if strings.EqualFold(mod, systemModuleName) {
			if len(meta.SystemEntities) == 0 {
				continue
			}
			if !isSystemEntityName(name) {
				missing = append(missing, qn)
			}
			continue
		}
		if known == nil {
			known = buildEntityQualifiedNames(ctx)
			for k := range known {
				lowerKnown[strings.ToLower(k)] = true
			}
		}
		if len(known) == 0 {
			return nil // a backend that lists none cannot say one is missing
		}
		if !lowerKnown[strings.ToLower(qn)] {
			missing = append(missing, qn)
		}
	}
	sort.Strings(missing)
	var out []string
	for _, qn := range missing {
		out = append(out, fmt.Sprintf("the query reads from %s, which does not exist — mxbuild reports "+
			"CE0174 \"Error(s) in OQL query: Cannot resolve object name '%s'\"", qn, qn))
	}
	return out
}

// isSystemEntityName reports whether System declares an entity of that name
// (runtime-only rows included: this only decides whether to stay silent).
func isSystemEntityName(name string) bool {
	for _, e := range meta.SystemEntities {
		if strings.EqualFold(e.Name, name) {
			return true
		}
	}
	return false
}

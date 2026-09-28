// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// remoteTypeChangeRefused is the language change for ako/mxcli#764: a mapped
// attribute of an external entity declared with a type the service does not
// publish. The type is the service contract's — Studio Pro offers no way to
// change it, and mx check reports CE6616 against the service's $metadata, not
// against the stored RemoteType — so deriving a RemoteType from the declared
// type would only move the mismatch. Under mdl 1 the statement is refused.
var remoteTypeChangeRefused = langver.Change{
	Code:  "MDL-V1-REMOTETYPE",
	Since: langver.V1,
	Old: "`create or modify external entity` writes a mapped attribute with the declared type and keeps the " +
		"service's RemoteType, which mx check reports as CE6616",
	New: "a refusal that names the attribute and the type the service publishes, with nothing written",
}

// remoteTypeMismatches lists, for each declared attribute that maps a stored
// OData property, a declared type the property's Edm type does not map to. A
// RemoteType outside the Edm primitives (an enumeration type, say) is not
// judged: its mapping is not the Edm table's.
func remoteTypeMismatches(stored *domainmodel.Entity, declared []*domainmodel.Attribute) []string {
	byName := make(map[string]*domainmodel.Attribute, len(stored.Attributes))
	for _, a := range stored.Attributes {
		if a != nil {
			byName[a.Name] = a
		}
	}
	var out []string
	for _, a := range declared {
		if a == nil || a.Type == nil {
			continue
		}
		old, ok := byName[a.Name]
		if !ok || old.RemoteName == "" || !knownEdmPrimitive(old.RemoteType) {
			continue
		}
		want := edmToDomainModelAttrType(&types.EdmProperty{Type: old.RemoteType}, false).GetTypeName()
		if got := a.Type.GetTypeName(); got != want {
			out = append(out, fmt.Sprintf("%s is declared %s, but the service publishes it as %s, which maps to %s",
				a.Name, got, old.RemoteType, want))
		}
	}
	return out
}

// knownEdmPrimitive reports whether t is an Edm type edmToDomainModelAttrType
// maps explicitly, rather than through its String fallback.
func knownEdmPrimitive(t string) bool {
	switch t {
	case "Edm.String", "Edm.Int32", "Edm.Int16", "Edm.Byte", "Edm.SByte", "Edm.Int64",
		"Edm.Decimal", "Edm.Double", "Edm.Single", "Edm.Boolean",
		"Edm.DateTime", "Edm.DateTimeOffset", "Edm.Date", "Edm.Guid", "Edm.Binary":
		return true
	}
	return false
}

// checkRemoteTypes refuses (mdl 1) or warns about (mdl 0) the declared
// attributes whose type the service does not publish.
func checkRemoteTypes(ctx *ExecContext, qualifiedName string, stored *domainmodel.Entity, declared []*domainmodel.Attribute) error {
	mism := remoteTypeMismatches(stored, declared)
	if len(mism) == 0 {
		return nil
	}
	if remoteTypeChangeRefused.Applies(ctx.LanguageVersion) {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"create or modify external entity %s: an external entity's attribute types come from the OData service, "+
				"and this statement declares others:\n  - %s\n"+
				"Nothing was written: mx check would report CE6616 for each. Declare the type the service publishes, "+
				"or leave the attribute out to remove it.",
			qualifiedName, strings.Join(mism, "\n  - ")))
	}
	fmt.Fprintf(ctx.progress(), "Warning [%s]: external entity %s: %s. %s\n",
		remoteTypeChangeRefused.Code, qualifiedName, strings.Join(mism, "; "),
		remoteTypeChangeRefused.Warning(ctx.LanguageVersion))
	return nil
}

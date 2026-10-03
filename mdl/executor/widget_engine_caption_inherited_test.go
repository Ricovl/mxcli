// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A combo box's `CaptionAttribute: FullName` over an entity that extends
// Administration.Account was stored as `<Entity>.FullName` and mxbuild failed
// with CE1613 "The selected attribute … no longer exists". Every other
// attribute mapping in the engine qualifies through resolveAttributePathForEntity,
// which names the DECLARING entity; the caption concatenated the context entity.
func TestResolveMapping_CaptionAttribute_Inherited(t *testing.T) {
	for _, tc := range []struct {
		attr, want string
	}{
		{"FullName", "Administration.Account.FullName"},                  // inherited
		{"IsAvailable", "TaskBoard.Person.IsAvailable"},                  // own (control)
		{"Administration.Account.Email", "Administration.Account.Email"}, // already qualified
	} {
		t.Run(tc.attr, func(t *testing.T) {
			engine := &PluggableWidgetEngine{pageBuilder: inheritancePB("TaskBoard.Person")}
			mapping := PropertyMapping{PropertyKey: "optionsSourceAssociationCaptionAttribute", Source: "CaptionAttribute"}
			w := &ast.WidgetV3{Properties: map[string]any{"CaptionAttribute": tc.attr}}
			ctx, err := engine.resolveMapping(mapping, w)
			if err != nil {
				t.Fatalf("resolveMapping: %v", err)
			}
			if ctx.AttributePath != tc.want {
				t.Errorf("caption attribute path %q, want %q", ctx.AttributePath, tc.want)
			}
		})
	}
}

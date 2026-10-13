// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// describe business event service wrote ServiceName and EventNamePrefix as
// `'%s'`, the quotes added by hand and the value's own quotes left single. A
// service name with an apostrophe — `Bob's orders` — came out as a statement
// that does not parse, so the describe → exec round trip failed on it.
//
// The assertion is that the output re-parses to the same values, under both
// languages describe can write (mdl 0 escapes a backslash, mdl 1 does not),
// not that it contains a particular escape.
func TestDescribeBusinessEventService_QuotedValuesRoundTrip(t *testing.T) {
	const serviceName, prefix = `Bob's orders`, `it's\here`
	for _, lang := range []langver.Version{langver.V0, langver.V1} {
		mod := mkModule("MyModule")
		svc := &model.BusinessEventService{
			BaseElement: model.BaseElement{ID: nextID("bes")},
			ContainerID: mod.ID,
			Name:        "OrderEvents",
			Definition: &model.BusinessEventDefinition{
				ServiceName:     serviceName,
				EventNamePrefix: prefix,
				// A message with an attribute: a service with no message
				// describes as `{ }` and a message with no attribute as
				// `message M () publish;`, both of which the grammar refuses —
				// separate defects, kept out of this test.
				Channels: []*model.BusinessEventChannel{{
					ChannelName: "ch1",
					Messages: []*model.BusinessEventMessage{{MessageName: "OrderCreated",
						Attributes: []*model.BusinessEventAttribute{{AttributeName: "Total", AttributeType: "Decimal"}}}},
				}},
			},
		}
		h := mkHierarchy(mod)
		withContainer(h, svc.ContainerID, mod.ID)
		mb := &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
				return []*model.BusinessEventService{svc}, nil
			},
		}
		ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
		ctx.describeLang = &lang
		assertNoError(t, describeBusinessEventService(ctx, ast.QualifiedName{Module: "MyModule", Name: "OrderEvents"}))

		src := buf.String()
		if lang == langver.V1 {
			src = langver.HeaderFor(langver.V1) + "\n" + src
		}
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("mdl %d: describe output does not parse: %v\n%s", lang, errs, src)
		}
		var stmt *ast.CreateBusinessEventServiceStmt
		for _, s := range prog.Statements {
			if c, ok := s.(*ast.CreateBusinessEventServiceStmt); ok {
				stmt = c
			}
		}
		if stmt == nil {
			t.Fatalf("mdl %d: no business event service in:\n%s", lang, src)
		}
		if stmt.ServiceName != serviceName || stmt.EventNamePrefix != prefix {
			t.Errorf("mdl %d: re-parsed ServiceName %q, EventNamePrefix %q; want %q, %q\n%s",
				lang, stmt.ServiceName, stmt.EventNamePrefix, serviceName, prefix, strings.TrimSpace(src))
		}
	}
}

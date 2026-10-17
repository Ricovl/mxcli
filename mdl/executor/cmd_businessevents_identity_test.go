// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1525: `create or modify business event service` minted a new
// random ChannelName for every message on every run. The ChannelName is the
// channel's identity outside the model — the topic subscribers listen on — so a
// re-run of an unchanged script moved every topic of a deployed app, and could
// never report Unchanged (ADR-0008). A rewrite must carry the stored channel
// (its ChannelName and $ID) and the stored message/attribute/operation $IDs,
// matched by message name and attribute name; only a new message gets a new
// channel.
func TestCreateOrModifyBusinessEventService_CarriesChannelIdentity(t *testing.T) {
	mod := mkModule("M")
	// Studio Pro-shaped identities: none of them equal to anything the executor
	// would mint, so a re-mint cannot pass by coincidence.
	stored := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: nextID("bes")},
		ContainerID: mod.ID,
		Name:        "OrderEvents",
		ExportLevel: "Hidden",
		Definition: &model.BusinessEventDefinition{
			BaseElement: model.BaseElement{ID: "sp-def"},
			ServiceName: "OrderEvents",
			Channels: []*model.BusinessEventChannel{
				{
					BaseElement: model.BaseElement{ID: "sp-ch-created"},
					ChannelName: "1111aaaa2222bbbb3333cccc4444dddd",
					Messages: []*model.BusinessEventMessage{{
						BaseElement: model.BaseElement{ID: "sp-msg-created"},
						MessageName: "OrderCreated",
						Attributes: []*model.BusinessEventAttribute{
							{BaseElement: model.BaseElement{ID: "sp-attr-orderid"}, AttributeName: "OrderId", AttributeType: "Long"},
							{BaseElement: model.BaseElement{ID: "sp-attr-note"}, AttributeName: "Note", AttributeType: "String"},
						},
					}},
				},
				{
					BaseElement: model.BaseElement{ID: "sp-ch-shipped"},
					ChannelName: "5555eeee6666ffff77770000888899990",
					Messages: []*model.BusinessEventMessage{{
						BaseElement: model.BaseElement{ID: "sp-msg-shipped"},
						MessageName: "OrderShipped",
					}},
				},
			},
		},
		OperationImplementations: []*model.ServiceOperation{
			{BaseElement: model.BaseElement{ID: "sp-op-created"}, MessageName: "OrderCreated", Operation: "publish", Entity: "M.PBE_Order"},
			{BaseElement: model.BaseElement{ID: "sp-op-shipped"}, MessageName: "OrderShipped", Operation: "publish", Entity: "M.PBE_Order"},
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, stored.ContainerID, mod.ID)

	var written *model.BusinessEventService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{stored}, nil
		},
		UpdateBusinessEventServiceFunc: func(svc *model.BusinessEventService) error {
			written = svc
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	// Reordered messages, reordered attributes, one new attribute, one new message.
	err := createBusinessEventService(ctx, &ast.CreateBusinessEventServiceStmt{
		Name:           ast.QualifiedName{Module: "M", Name: "OrderEvents"},
		ServiceName:    "OrderEvents",
		CreateOrModify: true,
		Messages: []*ast.BusinessEventMessageDef{
			{MessageName: "OrderShipped", Operation: "publish", Entity: "M.PBE_Order"},
			{MessageName: "OrderCreated", Operation: "publish", Entity: "M.PBE_Order", Attributes: []*ast.BusinessEventAttributeDef{
				{Name: "Note", TypeName: "String"},
				{Name: "OrderId", TypeName: "Long"},
				{Name: "Extra", TypeName: "String"},
			}},
			{MessageName: "OrderCancelled", Operation: "publish", Entity: "M.PBE_Order"},
		},
	})
	assertNoError(t, err)
	if written == nil || written.Definition == nil {
		t.Fatal("UpdateBusinessEventService was not called with a definition")
	}
	if written.Definition.ID != "sp-def" {
		t.Errorf("definition $ID = %q, want the stored sp-def", written.Definition.ID)
	}

	byMsg := map[string]*model.BusinessEventChannel{}
	for _, ch := range written.Definition.Channels {
		if len(ch.Messages) != 1 {
			t.Fatalf("channel %q holds %d messages, want 1", ch.ChannelName, len(ch.Messages))
		}
		byMsg[ch.Messages[0].MessageName] = ch
	}

	created, shipped, cancelled := byMsg["OrderCreated"], byMsg["OrderShipped"], byMsg["OrderCancelled"]
	if created == nil || shipped == nil || cancelled == nil {
		t.Fatalf("missing channels: %+v", byMsg)
	}
	if created.ChannelName != "1111aaaa2222bbbb3333cccc4444dddd" || created.ID != "sp-ch-created" {
		t.Errorf("OrderCreated channel = (%q, %q), want the stored (1111aaaa..., sp-ch-created)", created.ChannelName, created.ID)
	}
	if shipped.ChannelName != "5555eeee6666ffff77770000888899990" || shipped.ID != "sp-ch-shipped" {
		t.Errorf("OrderShipped channel = (%q, %q), want the stored (5555eeee..., sp-ch-shipped)", shipped.ChannelName, shipped.ID)
	}
	if created.Messages[0].ID != "sp-msg-created" || shipped.Messages[0].ID != "sp-msg-shipped" {
		t.Errorf("message $IDs not carried: created=%q shipped=%q", created.Messages[0].ID, shipped.Messages[0].ID)
	}
	attrIDs := map[string]model.ID{}
	for _, a := range created.Messages[0].Attributes {
		attrIDs[a.AttributeName] = a.ID
	}
	if attrIDs["OrderId"] != "sp-attr-orderid" || attrIDs["Note"] != "sp-attr-note" {
		t.Errorf("attribute $IDs not carried by name: %v", attrIDs)
	}
	if attrIDs["Extra"] != "" {
		t.Errorf("a new attribute must not take a stored $ID, got %q", attrIDs["Extra"])
	}

	// A new message gets a channel of its own, never a stored one.
	if cancelled.ChannelName == "" || cancelled.ID != "" || cancelled.Messages[0].ID != "" {
		t.Errorf("new message's channel = (%q, %q, msg %q), want a fresh name and no stored $IDs",
			cancelled.ChannelName, cancelled.ID, cancelled.Messages[0].ID)
	}
	if cancelled.ChannelName == created.ChannelName || cancelled.ChannelName == shipped.ChannelName {
		t.Errorf("new message reused a stored ChannelName %q", cancelled.ChannelName)
	}

	opIDs := map[string]model.ID{}
	for _, op := range written.OperationImplementations {
		opIDs[op.MessageName] = op.ID
	}
	if opIDs["OrderCreated"] != "sp-op-created" || opIDs["OrderShipped"] != "sp-op-shipped" || opIDs["OrderCancelled"] != "" {
		t.Errorf("operation $IDs not carried by message name: %v", opIDs)
	}
}

// A stored channel holding two messages (expressible in Studio Pro, not in MDL,
// which writes one channel per message) must not be handed to both: two channels
// sharing a ChannelName is the duplicate PR #533 crashed on. The first message
// keeps the stored channel; the second gets a fresh one.
func TestCreateOrModifyBusinessEventService_SharedStoredChannelNotDuplicated(t *testing.T) {
	mod := mkModule("M")
	stored := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: nextID("bes")},
		ContainerID: mod.ID,
		Name:        "Evts",
		Definition: &model.BusinessEventDefinition{
			Channels: []*model.BusinessEventChannel{{
				BaseElement: model.BaseElement{ID: "sp-ch"},
				ChannelName: "abcabcabcabcabcabcabcabcabcabcab",
				Messages: []*model.BusinessEventMessage{
					{BaseElement: model.BaseElement{ID: "sp-a"}, MessageName: "A"},
					{BaseElement: model.BaseElement{ID: "sp-b"}, MessageName: "B"},
				},
			}},
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, stored.ContainerID, mod.ID)
	var written *model.BusinessEventService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{stored}, nil
		},
		UpdateBusinessEventServiceFunc: func(svc *model.BusinessEventService) error { written = svc; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, createBusinessEventService(ctx, &ast.CreateBusinessEventServiceStmt{
		Name:           ast.QualifiedName{Module: "M", Name: "Evts"},
		CreateOrModify: true,
		Messages: []*ast.BusinessEventMessageDef{
			{MessageName: "A", Operation: "publish"},
			{MessageName: "B", Operation: "publish"},
		},
	}))
	chs := written.Definition.Channels
	if len(chs) != 2 {
		t.Fatalf("got %d channels, want 2", len(chs))
	}
	if chs[0].ChannelName != "abcabcabcabcabcabcabcabcabcabcab" || chs[0].ID != "sp-ch" {
		t.Errorf("first message lost the stored channel: (%q, %q)", chs[0].ChannelName, chs[0].ID)
	}
	if chs[1].ChannelName == chs[0].ChannelName || chs[1].ID == chs[0].ID {
		t.Errorf("second message duplicated the stored channel: (%q, %q)", chs[1].ChannelName, chs[1].ID)
	}
	if chs[1].Messages[0].ID != "sp-b" {
		t.Errorf("message B's $ID = %q, want the stored sp-b", chs[1].Messages[0].ID)
	}
}

// The same defect measured end to end on a real project: the second run of an
// unchanged script must write nothing, report Unchanged and keep every
// ChannelName. Run 1 is the control — it is a write and says so.
func TestCreateOrModifyBusinessEventService_RerunIsUnchanged(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const svc = `create or modify business event service MyFirstModule.Rerun1525 (ServiceName: 'Rerun1525', EventNamePrefix: '')
{
  message OrderCreated (OrderId: long, Note: string) publish entity MyFirstModule.PBE_Rerun1525;
  message OrderShipped (OrderId: long) publish entity MyFirstModule.PBE_Rerun1525;
};`
	if err := afRun(t, exec, `create persistent entity MyFirstModule.PBE_Rerun1525 (OrderId: Long, Note: String(200));`+"\n"+svc); err != nil {
		t.Fatalf("run 1: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Created business event service") {
		t.Fatalf("run 1 is a create and must say so:\n%s", out.String())
	}
	names := func() map[string]string {
		t.Helper()
		all, err := exec.newExecContext(context.Background()).Backend.ListBusinessEventServices()
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, s := range all {
			if s.Name != "Rerun1525" || s.Definition == nil {
				continue
			}
			for _, ch := range s.Definition.Channels {
				for _, m := range ch.Messages {
					got[m.MessageName] = ch.ChannelName
				}
			}
		}
		return got
	}
	first := names()
	if len(first) != 2 {
		t.Fatalf("channels after run 1: %v", first)
	}

	before := projectFiles(t, dir)
	out.Reset()
	if err := afRun(t, exec, svc); err != nil {
		t.Fatalf("run 2: %v\n%s", err, out.String())
	}
	if second := names(); second["OrderCreated"] != first["OrderCreated"] || second["OrderShipped"] != first["OrderShipped"] {
		t.Errorf("re-run moved the channels: before %v, after %v", first, second)
	}
	if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
		t.Errorf("re-run of an unchanged script wrote %v", changed)
	}
	if got := out.String(); !strings.Contains(got, "Unchanged business event service") {
		t.Errorf("re-run wrote nothing and must report Unchanged, got:\n%s", got)
	}
}

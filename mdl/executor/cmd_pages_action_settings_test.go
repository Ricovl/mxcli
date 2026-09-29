// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#721 L2: DESCRIBE prints a client action's non-default settings as
// `with ( … )`, and the builder carries them onto the action it writes.
package executor

import (
	"context"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func settingsText(s string) map[string]any {
	return map[string]any{
		"$Type": "Texts$Text",
		"Items": []any{int32(3), map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": s}},
	}
}

// The stored shape of WorkflowCommons' "Lock" button (PedApp): a non-blocking
// progress bar with a message, a confirmation, asynchronous, and not disabled
// during execution.
func storedMicroflowActionWithSettings() map[string]any {
	return map[string]any{
		"$Type":                   "Forms$MicroflowAction",
		"DisabledDuringExecution": false,
		"MicroflowSettings": map[string]any{
			"$Type":           "Forms$MicroflowSettings",
			"Asynchronous":    true,
			"FormValidations": "Widget",
			"Microflow":       "M.Lock",
			"ProgressBar":     "NonBlocking",
			"ProgressMessage": settingsText("Please wait"),
			"ConfirmationInfo": map[string]any{
				"$Type":                "Forms$ConfirmationInfo",
				"Question":             settingsText("Lock it? It's final."),
				"ProceedButtonCaption": settingsText("Proceed"),
				"CancelButtonCaption":  settingsText("Cancel"),
			},
		},
	}
}

func TestRenderClientActionMDL_Settings(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	got := renderClientActionMDL(ctx, storedMicroflowActionWithSettings())
	want := "call microflow M.Lock with (DisabledDuringExecution: false, ProgressBar: NonBlocking, " +
		"ProgressMessage: 'Please wait', Confirmation: 'Lock it? It''s final.', ProceedCaption: 'Proceed', " +
		"CancelCaption: 'Cancel', Asynchronous: true, FormValidations: Widget)"
	if got != want {
		t.Errorf("renderClientActionMDL =\n  %s\nwant\n  %s", got, want)
	}
}

// Control: an action holding every default describes exactly as before.
func TestRenderClientActionMDL_DefaultSettingsPrintNothing(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	action := map[string]any{
		"$Type":                   "Forms$MicroflowAction",
		"DisabledDuringExecution": true,
		"MicroflowSettings": map[string]any{
			"$Type": "Forms$MicroflowSettings", "Asynchronous": false, "FormValidations": "All",
			"Microflow": "M.F", "ProgressBar": "None", "ProgressMessage": nil, "ConfirmationInfo": nil,
		},
	}
	if got := renderClientActionMDL(ctx, action); got != "call microflow M.F" {
		t.Errorf("renderClientActionMDL = %q, want no settings", got)
	}
	save := map[string]any{"$Type": "Forms$SaveChangesClientAction", "DisabledDuringExecution": true, "ClosePage": true}
	if got := renderClientActionMDL(ctx, save); got != "save changes close page" {
		t.Errorf("renderClientActionMDL = %q", got)
	}
}

// describe → parse → build: what DESCRIBE prints builds the same settings back.
func TestActionSettings_DescribeParsesBackToTheSameAction(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	mdl := renderClientActionMDL(ctx, storedMicroflowActionWithSettings())
	prog, errs := visitor.Build("create page M.P (Title: 'T', Layout: A.L) {\n" +
		"  actionbutton b1 (Caption: 'Go', Action: " + mdl + ")\n}")
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, mdl)
	}
	a := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Properties["Action"].(*ast.ActionV3)
	pb := &pageBuilder{execCache: &executorCache{createdMicroflows: map[string]*createdMicroflowInfo{
		"M.Lock": {ID: "mf-1"},
	}}}
	built, err := pb.buildClientActionV3(a)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	mf := built.(*pages.MicroflowClientAction)
	if mf.DisabledDuringExecution == nil || *mf.DisabledDuringExecution {
		t.Errorf("DisabledDuringExecution = %v, want false", mf.DisabledDuringExecution)
	}
	if mf.ProgressBar != "NonBlocking" || !mf.Asynchronous || mf.FormValidations != "Widget" {
		t.Errorf("ProgressBar %q Asynchronous %v FormValidations %q", mf.ProgressBar, mf.Asynchronous, mf.FormValidations)
	}
	c := mf.Confirmation
	if c == nil || firstText(c.Question) != "Lock it? It's final." ||
		firstText(c.ProceedCaption) != "Proceed" || firstText(c.CancelCaption) != "Cancel" {
		t.Errorf("confirmation = %+v", c)
	}
	if firstText(mf.ProgressMessage) != "Please wait" {
		t.Errorf("progress message = %+v", mf.ProgressMessage)
	}
}

func TestApplyActionSettings_ConfirmationDefaultsItsCaptions(t *testing.T) {
	q := "Sure?"
	nf := &pages.NanoflowClientAction{NanoflowName: "M.N"}
	if err := applyActionSettings(nf, &ast.ActionSettingsV3{Confirmation: &q}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	c := nf.Confirmation
	if c == nil || firstText(c.ProceedCaption) != "Proceed" || firstText(c.CancelCaption) != "Cancel" {
		t.Errorf("confirmation = %+v, want Studio Pro's Proceed / Cancel", c)
	}
}

func firstText(t interface{ GetTranslation(string) string }) string {
	if t == nil {
		return ""
	}
	return t.GetTranslation("en_US")
}

// A flow-call setting on an action that has none is refused, not dropped.
func TestApplyActionSettings_RefusesFlowSettingsOnOtherActions(t *testing.T) {
	if err := applyActionSettings(&pages.SaveChangesClientAction{}, &ast.ActionSettingsV3{ProgressBar: "Blocking"}); err == nil {
		t.Error("ProgressBar on save changes was accepted")
	}
	f := false
	if err := applyActionSettings(&pages.NoClientAction{}, &ast.ActionSettingsV3{DisabledDuringExecution: &f}); err == nil {
		t.Error("DisabledDuringExecution on nothing was accepted")
	}
}

// A blank progress message is stored as a text with no translations; writing
// an en_US "" in its place rewrote every such button (found on the fixtures).
func TestApplyActionSettings_BlankProgressMessageHasNoTranslations(t *testing.T) {
	blank := ""
	mf := &pages.MicroflowClientAction{MicroflowName: "M.F"}
	if err := applyActionSettings(mf, &ast.ActionSettingsV3{ProgressBar: "Blocking", ProgressMessage: &blank}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if mf.ProgressMessage == nil {
		t.Fatal("ProgressMessage: '' dropped the message; Studio Pro stores an (empty) text")
	}
	if n := len(mf.ProgressMessage.Translations); n != 0 {
		t.Errorf("ProgressMessage has %d translations, want none", n)
	}
}

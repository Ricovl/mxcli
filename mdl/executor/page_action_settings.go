// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// applyActionSettings copies an action's `with ( … )` settings onto the built
// client action (ako/mxcli#721 L2). The visitor has already refused a setting
// the action type does not have; the checks here are for a caller that built
// the AST by hand.
func applyActionSettings(a pages.ClientAction, s *ast.ActionSettingsV3) error {
	if s.DisabledDuringExecution != nil {
		exec := pages.ExecutionOf(a)
		if exec == nil {
			return mdlerrors.NewValidation("DisabledDuringExecution: this action has no execution to disable the widget during")
		}
		v := *s.DisabledDuringExecution
		exec.DisabledDuringExecution = &v
	}
	hasFlowSettings := s.ProgressBar != "" || s.ProgressMessage != nil || s.Confirmation != nil ||
		s.ProceedCaption != nil || s.CancelCaption != nil
	hasMicroflowSettings := s.Asynchronous != nil || s.FormValidations != ""
	var flow *pages.FlowCallSettings
	switch x := a.(type) {
	case *pages.MicroflowClientAction:
		flow = &x.FlowCallSettings
	case *pages.NanoflowClientAction:
		flow = &x.FlowCallSettings
		if hasMicroflowSettings {
			return mdlerrors.NewValidation("Asynchronous and FormValidations are settings of `call microflow` only")
		}
	default:
		if hasFlowSettings || hasMicroflowSettings {
			return mdlerrors.NewValidation("ProgressBar, ProgressMessage and Confirmation are settings of `call microflow` and `call nanoflow` only")
		}
		return nil
	}
	flow.ProgressBar = s.ProgressBar
	if s.ProgressMessage != nil {
		flow.ProgressMessage = authoringText(*s.ProgressMessage)
	}
	if s.Confirmation != nil {
		// Studio Pro's captions for a new confirmation: measured on
		// ako/TestApp, where the one confirmation whose captions were never
		// edited stores exactly these.
		proceed, cancel := "Proceed", "Cancel"
		if s.ProceedCaption != nil {
			proceed = *s.ProceedCaption
		}
		if s.CancelCaption != nil {
			cancel = *s.CancelCaption
		}
		flow.Confirmation = &pages.ConfirmationInfo{
			Question:       authoringText(*s.Confirmation),
			ProceedCaption: authoringText(proceed),
			CancelCaption:  authoringText(cancel),
		}
	} else if s.ProceedCaption != nil || s.CancelCaption != nil {
		return mdlerrors.NewValidation("ProceedCaption and CancelCaption are the buttons of a confirmation; write Confirmation: '<question>' as well")
	}
	if s.Asynchronous != nil {
		flow.Asynchronous = *s.Asynchronous
	}
	flow.FormValidations = s.FormValidations
	return nil
}

// authoringText is s as a text in the project's authoring language; the
// rewrite carries a stored text's other translations (ako/mxcli#705).
//
// The empty string is a text with no translations, not an empty one: that is
// how Studio Pro stores a progress message left blank (all 7 blank ones in PedApp and
// ako/TestApp), and an en_US "" in its place is a rewrite of every such button.
func authoringText(s string) *model.Text {
	if s == "" {
		return &model.Text{}
	}
	return &model.Text{Translations: map[string]string{model.AuthoringLanguage(): s}}
}

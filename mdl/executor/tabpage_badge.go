// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A tab page's badge (mendixlabs/mxcli#1644).
//
// Forms$TabPage.Badge is a client template — the counter Studio Pro shows
// beside a tab's caption, stored in the shape of a dynamic text's Content. MDL
// had no spelling for it: `check` warned MDL-WIDGET07 that `Badge:` would be
// dropped, describe printed nothing, and re-executing describe's output stored
// "Badge": null, so a badge set in Studio Pro was removed by any rewrite of its
// page while `mx check` stayed clean. The spelling is the dynamic text's:
//
//	tabpage tpOpen (Caption: 'Open', Badge: '{1}', BadgeParams: ({1} = OpenCount)) { … }
//
// The parameters resolve against the data container around the tab container,
// as a dynamic text's there would. Translations of the template beyond the
// one MDL writes are carried by canon.CarryTranslations, keyed on the tab
// page's name.
//
// The keys are a tab page's only. They are kept out of staticWidgetKnownProps
// (the union every widget type shares) so `Badge:` on any other widget still
// draws MDL-WIDGET07; the per-type exemption is isTabPageBadgeProp, the same
// arrangement as the submit keys of input_submit_width.go, and this file, not a
// cmd_pages_builder* or describe-output file, holds the reads and the emits.

// tabPageBadgeProps are the badge keys, in canonical spelling and describe
// order.
var tabPageBadgeProps = []string{"Badge", "BadgeParams"}

// tabPageBadgeRule refuses a badge written in a shape the tab page cannot
// store.
const tabPageBadgeRule = "MDL-WIDGET51"

const tabPageBadgeExample = "Badge: '{1}', BadgeParams: ({1} = Attribute)"

// isTabPageBadgeProp reports whether key is a badge property of the MDL widget
// type widgetType.
func isTabPageBadgeProp(widgetType, key string) bool {
	if !strings.EqualFold(widgetType, "tabpage") {
		return false
	}
	for _, p := range tabPageBadgeProps {
		if strings.EqualFold(p, key) {
			return true
		}
	}
	return false
}

// tabPageBadgeOf reads a tab page's badge template and parameters. set is
// false when the statement gives neither; a malformed badge is an error, the
// same one check reports.
func tabPageBadgeOf(w *ast.WidgetV3) (text string, params []ast.ParamAssignmentV3, set bool, err error) {
	rawText, hasText := lookupPropCI(w, "Badge")
	rawParams, hasParams := lookupPropCI(w, "BadgeParams")
	if !hasText && !hasParams {
		return "", nil, false, nil
	}
	if hasText {
		s, ok := rawText.(string)
		if !ok {
			return "", nil, false, fmt.Errorf("tabpage %s: Badge is a text template written in quotes, like a dynamic text's Content (%s)",
				w.Name, tabPageBadgeExample)
		}
		text = s
	}
	if hasParams {
		p, ok := rawParams.([]ast.ParamAssignmentV3)
		if !ok {
			return "", nil, false, fmt.Errorf("tabpage %s: BadgeParams is a parameter list (%s)", w.Name, tabPageBadgeExample)
		}
		params = p
	}
	if !hasText {
		return "", nil, false, fmt.Errorf("tabpage %s: BadgeParams has no Badge template to fill (%s)", w.Name, tabPageBadgeExample)
	}
	return text, params, true, nil
}

// buildTabPageBadge is the badge the builder writes; nil when the statement
// gives none.
func (pb *pageBuilder) buildTabPageBadge(w *ast.WidgetV3) (*pages.ClientTemplate, error) {
	text, params, set, err := tabPageBadgeOf(w)
	if err != nil {
		return nil, mdlerrors.NewValidationf("%v [%s]", err, tabPageBadgeRule)
	}
	if !set {
		return nil, nil
	}
	return &pages.ClientTemplate{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Forms$ClientTemplate"},
		Template: &model.Text{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Texts$Text"},
			Translations: map[string]string{pb.textLang(): text},
		},
		Parameters: pb.buildClientTemplateParams(params),
	}, nil
}

// validateTabPageBadge is the check-time half of the refusal above.
func validateTabPageBadge(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "tabpage") {
		return nil
	}
	if _, _, _, err := tabPageBadgeOf(w); err != nil {
		return []linter.Violation{{
			RuleID:   tabPageBadgeRule,
			Severity: linter.SeverityError,
			Message:  fmt.Sprintf("%s: %v", locationPrefix, err),
		}}
	}
	return nil
}

// readTabPageBadge reads a stored tab page's badge for describe. A stored
// Badge is a client template, or null when none is set.
func readTabPageBadge(ctx *ExecContext, tp map[string]any, w *rawWidget) {
	if _, ok := tp["Badge"].(map[string]any); !ok {
		return
	}
	w.TabBadgeSet = true
	w.TabBadge = extractTextContent(ctx, tp, "Badge")
	w.TabBadgeParams = extractClientTemplateParameters(ctx, tp, "Badge")
}

// describeTabPageBadge appends a stored badge to a tab page's properties. An
// empty template is printed too: the badge is set, and leaving it out would
// make re-executing the description remove it.
func describeTabPageBadge(ctx *ExecContext, props []string, w rawWidget) []string {
	if !w.TabBadgeSet {
		return props
	}
	props = append(props, fmt.Sprintf("Badge: %s", mdlQuote(ctx, w.TabBadge)))
	if len(w.TabBadgeParams) > 0 {
		props = append(props, fmt.Sprintf("BadgeParams: (%s)", strings.Join(formatParametersV3(w.TabBadgeParams), ", ")))
	}
	return props
}

// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/translations"
	"github.com/mendixlabs/mxcli/model"
)

// RequiredCaptionDefaultLanguageRule reports a caption mxbuild requires in the
// project's default language and that has no text there: CE4899 "Empty
// caption. [German, Germany]" (ako/mxcli#944).
//
// QUAL005 cannot see it: it compares the languages texts are translated into
// with each other and never asks which one is the default, so a project whose
// default changed to de_DE after its pages were written in en_US is reported
// as nothing at all — or as a warning among hundreds — while the build fails.
// Which captions mxbuild requires is measured (translations.RequiredCaptionKind:
// tab page captions only), so this rule is an error and flags nothing else.
type RequiredCaptionDefaultLanguageRule struct{}

// NewRequiredCaptionDefaultLanguageRule creates the rule.
func NewRequiredCaptionDefaultLanguageRule() *RequiredCaptionDefaultLanguageRule {
	return &RequiredCaptionDefaultLanguageRule{}
}

func (r *RequiredCaptionDefaultLanguageRule) ID() string { return "QUAL006" }
func (r *RequiredCaptionDefaultLanguageRule) Name() string {
	return "RequiredCaptionMissingDefaultLanguage"
}
func (r *RequiredCaptionDefaultLanguageRule) Category() string { return "quality" }
func (r *RequiredCaptionDefaultLanguageRule) DefaultSeverity() linter.Severity {
	return linter.SeverityError
}

func (r *RequiredCaptionDefaultLanguageRule) Description() string {
	return "Checks that every caption mxbuild requires (a tab page caption) has text in the project's default language (CE4899)"
}

// captionProject is what the rule needs from the lint reader: the stored units
// and the project's default language. The backend behind `mxcli lint` has both;
// a reader without them (a test double, no project) makes the rule silent.
type captionProject interface {
	translations.UnitReader
	GetProjectSettings() (*model.ProjectSettings, error)
}

// Check runs the rule.
func (r *RequiredCaptionDefaultLanguageRule) Check(ctx *linter.LintContext) []linter.Violation {
	p, ok := ctx.Reader().(captionProject)
	if !ok || p == nil {
		return nil
	}
	ps, err := p.GetProjectSettings()
	if err != nil || ps == nil || ps.Language == nil || ps.Language.DefaultLanguageCode == "" {
		return nil
	}
	lang := ps.Language.DefaultLanguageCode
	missing, err := translations.MissingRequiredCaptions(p, lang)
	if err != nil {
		return nil
	}
	return r.violations(ctx, lang, missing)
}

// violations names each missing caption by its document, resolved through the
// catalog's objects view, and applies the module and document filters.
func (r *RequiredCaptionDefaultLanguageRule) violations(ctx *linter.LintContext, lang string, missing []translations.MissingCaption) []linter.Violation {
	var out []linter.Violation
	for _, m := range missing {
		qn, name, module, docType := m.UnitName, m.UnitName, "", "page"
		if db := ctx.CatalogDB(); db != nil {
			row := db.QueryRow(`SELECT QualifiedName, Name, ModuleName, ObjectType FROM objects WHERE Id = ?`, string(m.UnitID))
			var q, n, mod, ot string
			if row.Scan(&q, &n, &mod, &ot) == nil {
				qn, name, module, docType = q, n, mod, strings.ToLower(ot)
			}
		}
		if module != "" && ctx.IsExcluded(module) {
			continue
		}
		if ctx.IsDocumentExcluded(qn) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message: fmt.Sprintf("%s: %s has no text in the default language %s — mxbuild reports CE4899 \"Empty caption\"",
				qn, m.String(), lang),
			Location: linter.Location{
				Module:       module,
				DocumentType: docType,
				DocumentName: name,
				DocumentID:   string(m.UnitID),
			},
			Suggestion: captionSuggestion(lang, m.UnitType, qn, m.OwnerName),
		})
	}
	return out
}

// captionSuggestion names the statement that fixes it: an alter writes the
// authoring language, which is the default. A layout has no alter.
func captionSuggestion(lang, unitType, doc, tab string) string {
	kind := "page"
	switch unitType {
	case "Forms$Snippet":
		kind = "snippet"
	case "Forms$Layout":
		return fmt.Sprintf("Give %s a %s caption in Studio Pro (layout %s)", tab, lang, doc)
	}
	return fmt.Sprintf("Give it a %s text: alter %s %s { set (Caption: '…') on %s; }; writes the default language",
		lang, kind, doc, tab)
}

// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ADR-0013: the import-with-mapping activity's dialog settings are one ( Key:
// value, … ) list after its main operand, `import from mapping M.IMM($Json)
// ( … )`. MDL could not say either before ako/mxcli#1681, so a mapping with an
// input object could not be called at all (mxbuild CE0244) and a describe of
// one dropped the object. New syntax, so an unknown key, a repeated key or a
// value of the wrong shape is an error under every language version (R11).
//
// Keys are the metamodel's (Microflows$ImportMappingCall): Parameter for
// ParameterVariableName, whose value is the variable holding the input object,
// and Commit, whose values are the CommitEnum members.
var importMappingSettingKeys = []string{"Parameter", "Commit"}

var importMappingCommitValues = []string{"Yes", "YesWithoutEvents", "No"}

// importMappingSettingsInto reads the list into stmt. It is lenient — a key or
// value it does not recognise is skipped — because the statement builders have
// no error channel; ExitImportFromMappingStatement reports it.
func importMappingSettingsInto(stmt *ast.ImportFromMappingStmt, settings parser.IImportMappingSettingsContext) {
	sc, ok := settings.(*parser.ImportMappingSettingsContext)
	if !ok || sc == nil {
		return
	}
	for _, s := range sc.AllImportMappingSetting() {
		key, variable, word := importMappingSettingParts(s)
		switch canonicalKey(key, importMappingSettingKeys) {
		case "Parameter":
			if variable != "" {
				stmt.Parameter = strings.TrimPrefix(variable, "$")
			}
		case "Commit":
			stmt.Commit = canonicalKey(word, importMappingCommitValues)
		}
	}
}

// importMappingSettingParts returns a setting's key and its value, as a
// variable ("$Obj") or as a word ("No"); the other is "".
func importMappingSettingParts(s parser.IImportMappingSettingContext) (key, variable, word string) {
	c, ok := s.(*parser.ImportMappingSettingContext)
	if !ok || c == nil {
		return "", "", ""
	}
	keys := c.AllIdentifierOrKeyword()
	if len(keys) == 0 {
		return "", "", ""
	}
	key = identifierOrKeywordText(keys[0])
	if v := c.VARIABLE(); v != nil {
		variable = v.GetText()
	} else if len(keys) > 1 {
		word = identifierOrKeywordText(keys[1])
	}
	return key, variable, word
}

func (b *Builder) ExitImportFromMappingStatement(ctx *parser.ImportFromMappingStatementContext) {
	settings, ok := ctx.ImportMappingSettings().(*parser.ImportMappingSettingsContext)
	if !ok || settings == nil {
		return
	}
	const what = "import from mapping"
	seen := map[string]bool{}
	for _, s := range settings.AllImportMappingSetting() {
		key, variable, word := importMappingSettingParts(s)
		line := s.GetStart().GetLine()
		canonical := canonicalKey(key, importMappingSettingKeys)
		if canonical == "" {
			b.addError(unknownKeyError(line, what, key, importMappingSettingKeys))
			continue
		}
		if seen[canonical] {
			b.addError(fmt.Errorf("line %d: %s sets %s twice", line, what, canonical))
			continue
		}
		seen[canonical] = true
		switch canonical {
		case "Parameter":
			if variable == "" {
				b.addError(fmt.Errorf("line %d: %s: Parameter is a variable holding the mapping's input object, like $Obj, not %s",
					line, what, word))
			}
		case "Commit":
			if canonicalKey(word, importMappingCommitValues) == "" {
				shown := word
				if variable != "" {
					shown = variable
				}
				b.addError(fmt.Errorf("line %d: %s: Commit is Yes, YesWithoutEvents or No, not %s", line, what, shown))
			}
		}
	}
}

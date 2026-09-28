// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// R7 (PROPOSAL_mdl_beta_syntax_freeze.md §3; ADR-0010): MDL is what makes
// sense in a checked-in `.mdl` file applied to a model. A session command —
// one that needs a session or an environment: a connection, an output format,
// a build, a running app — is typed at the REPL or given as a command-line
// flag (`mxcli exec script.mdl -p app.mpr --json`).
//
// The grammar keeps parsing them, because the REPL reads its input with the
// same parser. What changes is a script: under mdl 1 a session command in it
// is refused, under mdl 0 it runs as before and warns. The REPL never
// validates a program (ValidateProgram is `check` and `exec`), so it sees
// neither the warning nor, for a line without a header, the error.

// sessionCommandInScript is the new rejection of a session command in a script.
var sessionCommandInScript = langver.Change{
	Code:  "MDL-V1-SESSION",
	Since: langver.V1,
	Old:   "a session command in a script runs as if typed at the REPL",
	New: "an error: a script holds model statements only; type the command at the REPL, " +
		"or use the command-line flag (`-p app.mpr` to connect, `--json` for the output format)",
}

// ExitUtilityStatement gates the session commands among the utility
// statements (R7). The others — `exit`, `refresh catalog`, `search`, `sql`,
// `import from`, `define fragment`, `show lint rules` — are not session
// commands and are not reported.
func (b *Builder) ExitUtilityStatement(ctx *parser.UtilityStatementContext) {
	cmd := sessionCommand(ctx)
	if cmd == "" {
		return
	}
	if b.gate(sessionCommandInScript, ctx) {
		b.addError(fmt.Errorf("line %d: `%s` is a session command, not a model statement: under %s a script "+
			"holds model statements only. Type it at the REPL, or use the command-line flag "+
			"(`-p app.mpr` to connect, `--json` for the output format)",
			ctx.GetStart().GetLine(), cmd, b.langVersion))
		return
	}
	if n := len(b.langNotes); n > 0 && b.langNotes[n-1].Code == sessionCommandInScript.Code {
		b.langNotes[n-1].Message = fmt.Sprintf("`%s` is a session command: %s", cmd, b.langNotes[n-1].Message)
	}
}

// sessionCommand names the session command ctx holds, or "" when it holds
// another utility statement.
func sessionCommand(ctx *parser.UtilityStatementContext) string {
	switch {
	case ctx.ConnectStatement() != nil:
		return "connect"
	case ctx.DisconnectStatement() != nil:
		return "disconnect"
	case ctx.StatusStatement() != nil:
		return "status"
	case ctx.CheckStatement() != nil:
		return "check"
	case ctx.BuildStatement() != nil:
		return "build"
	case ctx.ExecuteScriptStatement() != nil:
		return "execute script"
	case ctx.ExecuteRuntimeStatement() != nil:
		return "execute runtime"
	case ctx.UseSessionStatement() != nil:
		return "use"
	case ctx.IntrospectApiStatement() != nil:
		return "introspect api"
	case ctx.DebugStatement() != nil:
		return "debug"
	case ctx.SessionSetStatement() != nil:
		set := ctx.SessionSetStatement().(*parser.SessionSetStatementContext)
		if k := set.IdentifierOrKeyword(); k != nil {
			return "set " + strings.ToLower(k.GetText())
		}
		return "set"
	case ctx.LintStatement() != nil:
		// `show lint rules` lists the rules; only running the linter needs
		// a session.
		if l := ctx.LintStatement().(*parser.LintStatementContext); l.SHOW() == nil {
			return "lint"
		}
	case ctx.HelpStatement() != nil:
		// `exit` and `quit` end a script, which is meaningful in one.
		if h := ctx.HelpStatement().(*parser.HelpStatementContext); h.IDENTIFIER() != nil &&
			strings.EqualFold(h.IDENTIFIER().GetText(), "help") {
			return "help"
		}
	}
	return ""
}

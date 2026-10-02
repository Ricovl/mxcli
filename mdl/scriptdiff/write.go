// SPDX-License-Identifier: Apache-2.0

package scriptdiff

import (
	"fmt"
	"io"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/executor"
)

// Counts are the documents a report changes: entities and associations count
// one each, as does every other document.
func (r *Report) Counts() (added, modified, removed int) {
	for _, res := range r.Results {
		switch {
		case res.IsNew:
			added++
		case res.IsDeleted:
			removed++
		default:
			modified++
		}
	}
	return added, modified, removed
}

// Write prints the report: the changes as MDL, the files, how exec ends and a
// summary.
func (r *Report) Write(w io.Writer, opts executor.DiffOptions, showExecOutput bool) {
	if r.PreflightOutput != "" {
		fmt.Fprint(w, r.PreflightOutput)
	}
	if r.Refused != "" {
		fmt.Fprintf(w, "Refused: exec would refuse to run this script, and write nothing.\n%s", r.Refused)
		fmt.Fprintln(w, "\nSummary: 0 new, 0 modified, 0 removed — exec would write nothing")
		return
	}
	if showExecOutput && r.Output != "" {
		fmt.Fprintf(w, "-- exec output --\n%s-- end of exec output --\n\n", r.Output)
	}
	executor.WriteDiffResults(w, r.Results, opts)
	for _, f := range r.Files {
		verdict := map[ChangeKind]string{Added: "New file", Modified: "Modified file", Removed: "Removed file"}[f.Kind]
		fmt.Fprintf(w, "%s: %s\n", verdict, f.Path)
	}
	if r.Failures != "" {
		fmt.Fprintf(w, "\nexec would report these failures and carry on:\n%s", r.Failures)
	}
	if r.ExecErr != nil {
		what := "writing only what is listed above"
		if len(r.Units) == 0 && len(r.Files) == 0 {
			what = "having written nothing"
		}
		fmt.Fprintf(w, "\nRefused: exec would stop at this error, %s: %v\n", what, strings.TrimSpace(r.ExecErr.Error()))
	}
	added, modified, removed := r.Counts()
	summary := fmt.Sprintf("\nSummary: %d new, %d modified, %d removed", added, modified, removed)
	switch {
	case len(r.Units) == 0 && len(r.Files) == 0:
		summary += " — exec would write nothing"
	default:
		summary += fmt.Sprintf(" — exec would write %d unit(s)", len(r.Units))
		if len(r.Files) > 0 {
			summary += fmt.Sprintf(" and %d file(s)", len(r.Files))
		}
	}
	fmt.Fprintln(w, summary)
}

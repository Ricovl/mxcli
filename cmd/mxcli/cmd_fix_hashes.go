// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	mpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// cmd_fix_hashes.go verifies and repairs the MPR v2 ContentsHash index
// (ako/mxcli#972). Unlike its siblings in cmd_fix.go it needs no mx: the index
// is mxcli's own to read and write.

var fixHashesCmd = &cobra.Command{
	Use:   "hashes",
	Short: "Verify (and with --repair, rewrite) the MPR v2 ContentsHash index",
	Long: `Verify that every unit's ContentsHash in the .mpr matches its .mxunit file.

In an MPR v2 project the .mpr indexes each mprcontents/**/*.mxunit file by
base64(SHA-256(file)). mxcli and Studio Pro keep the two in step, but anything
that changes a .mxunit behind their back does not: restoring a unit with
'git checkout', 'git restore' or a merge leaves the index describing bytes that
are no longer on disk. 'mx check' does not notice; Studio Pro uses the index to
decide what changed.

Reported:
  MISMATCH  the file's hash differs from the stored one (repairable)
  MISSING   the .mpr indexes a unit whose file does not exist
  ORPHAN    a .mxunit file that no unit in the .mpr indexes

--repair rewrites every mismatched hash from the file on disk, in one
transaction, and leaves the files untouched — the files are the source of
truth. MISSING and ORPHAN have no correct hash to write and are only reported
('mxcli diag --check-units --fix' removes orphan files). Like every mxcli
write, the repair is refused while Studio Pro has the project open.

Exits 1 when an issue remains after the command (so the verify can gate CI).
An MPR v1 project keeps unit contents inside the .mpr and has no index to
drift; the command says so and exits 0.`,
	Example: `  mxcli fix hashes -p app.mpr
  mxcli fix hashes -p app.mpr --repair`,
	Args:          cobra.NoArgs,
	RunE:          runFixHashes,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	fixHashesCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	_ = fixHashesCmd.MarkFlagRequired("project")
	fixHashesCmd.Flags().Bool("repair", false, "rewrite mismatched ContentsHash values from the files on disk")
	fixCmd.AddCommand(fixHashesCmd)
}

// errHashIssuesRemain makes the command exit 1 after it printed the report.
var errHashIssuesRemain = errors.New("ContentsHash issues remain")

func runFixHashes(cmd *cobra.Command, _ []string) error {
	mprPath, _ := cmd.Flags().GetString("project")
	repair, _ := cmd.Flags().GetBool("repair")
	out := cmd.OutOrStdout()
	if _, err := os.Stat(mprPath); err != nil {
		return fmt.Errorf("project not found: %s", mprPath)
	}
	err := fixHashes(mprPath, repair, out)
	if errors.Is(err, errHashIssuesRemain) {
		cmd.SilenceErrors = true
		os.Exit(1)
	}
	return err
}

// fixHashes is the command without its exit: it prints the report and returns
// errHashIssuesRemain when the index and the files still disagree.
func fixHashes(mprPath string, repair bool, out io.Writer) error {
	var (
		rep      *mpr.ContentsHashReport
		repaired int
		err      error
	)
	if repair {
		w, werr := mpr.NewWriter(mprPath)
		if werr != nil {
			return fmt.Errorf("open project: %w", werr)
		}
		rep, repaired, err = w.RepairContentsHashes()
		_ = w.Close()
	} else {
		r, rerr := mpr.Open(mprPath)
		if rerr != nil {
			return fmt.Errorf("open project: %w", rerr)
		}
		rep, err = r.VerifyContentsHashes()
		_ = r.Close()
	}
	if errors.Is(err, mpr.ErrNotMPRv2) {
		fmt.Fprintf(out, "%s is an MPR v1 project: unit contents live inside the .mpr, so there is no ContentsHash index to verify.\n", filepath.Base(mprPath))
		return nil
	}
	if rep == nil {
		return err
	}

	fmt.Fprintf(out, "Checked %d unit(s) in %s against %d .mxunit file(s).\n", rep.Units, filepath.Base(mprPath), rep.Files)
	contentsDir := filepath.Join(filepath.Dir(mprPath), "mprcontents")
	rel := func(p string) string {
		if r, e := filepath.Rel(contentsDir, p); e == nil {
			return filepath.Join("mprcontents", r)
		}
		return p
	}
	for _, m := range rep.Mismatches {
		fmt.Fprintf(out, "  MISMATCH  %s  %s%s\n            stored %s, file %s\n", m.UnitID, rel(m.Path), unitLabel(m.Path), orNone(m.Stored), m.Actual)
	}
	for _, m := range rep.MissingFiles {
		fmt.Fprintf(out, "  MISSING   %s  %s (indexed, no file)\n", m.UnitID, rel(m.Path))
	}
	for _, f := range rep.OrphanFiles {
		fmt.Fprintf(out, "  ORPHAN    %s%s (file, not indexed)\n", rel(f), unitLabel(f))
	}
	fmt.Fprintf(out, "\n%d mismatch(es), %d missing file(s), %d orphan file(s).\n",
		len(rep.Mismatches), len(rep.MissingFiles), len(rep.OrphanFiles))

	if err != nil { // the repair itself failed (Studio Pro guard, SQLite)
		return err
	}
	remaining := len(rep.MissingFiles) + len(rep.OrphanFiles)
	switch {
	case repair && repaired > 0:
		fmt.Fprintf(out, "Repaired %d ContentsHash value(s) from the files on disk.\n", repaired)
	case !repair && len(rep.Mismatches) > 0:
		remaining += len(rep.Mismatches)
		fmt.Fprintf(out, "Run 'mxcli fix hashes -p %s --repair' to rewrite the mismatched hashes from the files.\n", mprPath)
	case rep.Clean():
		fmt.Fprintln(out, "The ContentsHash index matches every unit file.")
	}
	if len(rep.OrphanFiles) > 0 {
		fmt.Fprintf(out, "Orphan files are not indexed by the .mpr; 'mxcli diag --check-units -p %s --fix' removes them.\n", mprPath)
	}
	if len(rep.MissingFiles) > 0 {
		fmt.Fprintln(out, "Missing files cannot be repaired from here: restore them from version control.")
	}
	if remaining > 0 {
		return errHashIssuesRemain
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// unitLabel names the unit a .mxunit holds (" [Microflows$Microflow Name]"),
// or "" when the file does not read as BSON.
func unitLabel(path string) string {
	b, err := os.ReadFile(path)
	if err != nil || bson.Raw(b).Validate() != nil {
		return ""
	}
	raw := bson.Raw(b)
	typ, _ := raw.Lookup("$Type").StringValueOK()
	name, _ := raw.Lookup("Name").StringValueOK()
	switch {
	case typ == "":
		return ""
	case name == "":
		return " [" + typ + "]"
	}
	return " [" + typ + " " + name + "]"
}

// contentsHashDriftWarning is the one-line warning exec and docker check print
// when the ContentsHash index disagrees with the unit files, or "" when it
// agrees. Measured on testapp (900 units): the whole `mxcli fix hashes` run,
// process start included, takes ~0.2 s, so it is cheap enough to run on every
// exec and check. It never fails the caller: an MPR v1 project or any error
// opening or reading the project yields "".
func contentsHashDriftWarning(mprPath string) string {
	if mprPath == "" {
		return ""
	}
	r, err := mpr.Open(mprPath)
	if err != nil {
		return ""
	}
	defer r.Close()
	rep, err := r.VerifyContentsHashes()
	if err != nil {
		return ""
	}
	n := len(rep.Mismatches) + len(rep.MissingFiles)
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("Warning: %d unit(s) in %s do not match the .mpr's ContentsHash index "+
		"(typically .mxunit files restored with git outside Studio Pro; mx check does not notice). "+
		"Run 'mxcli fix hashes -p %s' for the list, and --repair to fix it.\n",
		n, filepath.Base(mprPath), mprPath)
}

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// The run-local skill is synced into every user project, and its flag table is how
// an agent discovers what `run --local` can do: a flag that is in neither table is
// unreachable in practice. #1292 was `--db-type hsqldb` — the one way to boot on a
// machine with no PostgreSQL — shipping with only the help text and dated proposals
// mentioning it, while users hit "no local PostgreSQL superuser available".
//
// Scoped to the database flags, which decide whether a run can boot at all; the
// set is read from the command, so a new --db-* flag is caught without editing
// this test.
func TestRunDatabaseFlagsAreInBothFlagTables(t *testing.T) {
	var dbFlags []string
	runCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if strings.HasPrefix(f.Name, "db-") {
			dbFlags = append(dbFlags, f.Name)
		}
	})
	if len(dbFlags) == 0 {
		t.Fatal("no --db-* flags found on runCmd; the test is not looking at the right command")
	}

	docs := []string{
		filepath.Join("..", "..", ".claude", "skills", "mendix", "run-local", "SKILL.md"),
		filepath.Join("..", "..", "docs-site", "src", "tools", "run-local.md"),
	}
	for _, doc := range docs {
		body, err := os.ReadFile(doc)
		if err != nil {
			t.Fatalf("read %s: %v", doc, err)
		}
		rows := flagTableRows(string(body))
		for _, name := range dbFlags {
			re := regexp.MustCompile("`--" + regexp.QuoteMeta(name) + "`")
			if !re.MatchString(rows) {
				t.Errorf("%s: flag table has no row for --%s", doc, name)
			}
		}
	}
}

// flagTableRows returns the rows of every Markdown table whose header starts with
// "| Flag |", so a flag mentioned only in prose does not count as documented.
func flagTableRows(md string) string {
	var b strings.Builder
	in := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "| Flag |"):
			in = true
		case in && strings.HasPrefix(trimmed, "|"):
			b.WriteString(line)
			b.WriteByte('\n')
		default:
			in = false
		}
	}
	return b.String()
}

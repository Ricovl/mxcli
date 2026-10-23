// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docker check never downloads MxBuild and never starts a container: it runs
// the mx of an MxBuild already installed or cached, as a local process, and
// stops with "mx not found" when there is none (TestCheck_MissingMxNamesSetup
// NotADownload pins that). Measured: with an empty HOME it fails rather than
// downloading, and with docker and podman replaced by stubs that fail it
// passes having called neither. The docs said otherwise in seven places —
// "auto-downloads mxbuild", "The first time you run this, it will download the
// MxBuild toolchain", "runs Mendix's own mx check tool inside a Docker
// container" — so a fresh clone met an error the docs said could not happen.
//
// The rule held here: a section that talks about docker check and about
// downloading says that check does not download; the command's own help and
// page say it whatever else they discuss.
func TestDockerCheckDocs_SayItDoesNotDownload(t *testing.T) {
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	type doc struct {
		body   string
		help   bool // a command's help: every part of it is about the docker commands
		always bool // the check command's own text: must say it whatever it discusses
	}
	docs := map[string]doc{
		"docker --help":                        {body: dockerCmd.Long, help: true},
		"docker check --help":                  {body: dockerCheckCmd.Long, help: true, always: true},
		"docs-site/src/tools/docker-check.md":  {body: read("docs-site/src/tools/docker-check.md"), always: true},
		"docs-site/src/tools/docker.md":        {body: read("docs-site/src/tools/docker.md")},
		"docs-site/src/tutorial/validation.md": {body: read("docs-site/src/tutorial/validation.md")},
		"README.md":                            {body: read("README.md")},
		"CLAUDE.md":                            {body: read("CLAUDE.md")},
		// Synced into user projects, where agents read them.
		".claude/commands/mendix/validate-project.md":    {body: read(".claude/commands/mendix/validate-project.md")},
		".claude/skills/mendix/docker-workflow/SKILL.md": {body: read(".claude/skills/mendix/docker-workflow/SKILL.md")},
	}
	const says = "does not download"
	download := regexp.MustCompile(`(?i)download`)
	container := regexp.MustCompile(`(?i)inside a docker container`)
	for name, d := range docs {
		if d.always && !strings.Contains(flat(d.body), says) {
			t.Errorf("%s does not say that docker check %s MxBuild", name, says)
		}
		for _, sec := range markdownSections(d.body) {
			sec = flat(sec)
			if !d.help && !strings.Contains(sec, "docker check") {
				continue
			}
			if download.MatchString(sec) && !strings.Contains(sec, says) {
				t.Errorf("%s: a section about docker check mentions downloading without saying check %s it:\n%s", name, says, sec)
			}
			if m := container.FindString(sec); m != "" {
				t.Errorf("%s: says docker check runs %q; it runs mx as a local process:\n%s", name, m, sec)
			}
		}
	}
}

// flat collapses whitespace, so a phrase wrapped across lines still matches.
func flat(s string) string { return strings.Join(strings.Fields(s), " ") }

var markdownHeading = regexp.MustCompile(`^#{1,6} `)

// markdownSections splits s at its headings. A `#` line inside a fenced code
// block is a shell comment, not a heading. Text without headings is one section.
func markdownSections(s string) []string {
	var secs []string
	var cur strings.Builder
	fenced := false
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		} else if !fenced && markdownHeading.MatchString(line) && cur.Len() > 0 {
			secs = append(secs, cur.String())
			cur.Reset()
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	return append(secs, cur.String())
}

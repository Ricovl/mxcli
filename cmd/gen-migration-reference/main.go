// SPDX-License-Identifier: Apache-2.0

// gen-migration-reference regenerates the tables on the docs page "Language
// versions and migration" from the language-change and deprecation registries
// (mdl/migration). With -check it writes nothing and fails when the page is
// stale, the way CI uses it.
//
//	go run ./cmd/gen-migration-reference           # make gen-migration-reference
//	go run ./cmd/gen-migration-reference -check    # make check-migration-reference
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mendixlabs/mxcli/mdl/migration"
)

func main() {
	page := flag.String("page", migration.PagePath, "docs page holding the generated markers")
	check := flag.Bool("check", false, "fail if the page is stale instead of rewriting it")
	flag.Parse()

	old, err := os.ReadFile(*page)
	if err != nil {
		fail(err)
	}
	updated, err := migration.Splice(string(old), migration.GeneratedTables())
	if err != nil {
		fail(fmt.Errorf("%s: %w", *page, err))
	}
	if updated == string(old) {
		return
	}
	if *check {
		fail(fmt.Errorf("%s is stale: the language-change or deprecation registry changed; run `make gen-migration-reference` and commit the page", *page))
	}
	if err := os.WriteFile(*page, []byte(updated), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("updated %s\n", *page)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen-migration-reference:", err)
	os.Exit(1)
}

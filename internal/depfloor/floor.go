// SPDX-License-Identifier: Apache-2.0

//go:build depfloor

// Package depfloor keeps security floors in go.mod for modules mxcli requires
// but never compiles.
//
// A customer's dependency scan reads the module graph, so a vulnerable version
// there blocks a release even when no mxcli build contains the code. Raising
// such a module needs a require line in go.mod, but `go mod tidy` removes a
// requirement no package imports. Tidy considers every build tag, so the
// imports below keep each requirement alive. Nothing sets the depfloor tag,
// so no build compiles this file and the binary is unchanged.
//
// TestGoModKeepsSecurityFloors checks the versions. To raise a floor, run
// `go get <module>@<version>` and then `go mod tidy`. To drop one, remove its
// import here and its row in the test.
package depfloor

import (
	// GHSA-fpmh-frq8-7rj8: zstd DecodeAll crash on crafted input, fixed in
	// v1.20.2. Arrives through go.mongodb.org/mongo-driver/v2, which declares
	// v1.19.2 and which mxcli uses only for its BSON packages.
	_ "github.com/klauspost/compress/zstd"
)

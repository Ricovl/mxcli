// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"

	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: debug_modules <mpr-path>")
		os.Exit(1)
	}

	mprPath := os.Args[1]
	contentsDir := filepath.Join(filepath.Dir(mprPath), "mprcontents")

	db, err := sql.Open("sqlite", mprPath+"?mode=ro")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT UnitID FROM Unit")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	fmt.Println("=== All Module-related Types ===")

	for rows.Next() {
		var unitID []byte
		rows.Scan(&unitID)

		uuid := blobToUUIDSwapped(unitID)
		path := filepath.Join(contentsDir, uuid[0:2], uuid[2:4], uuid+".mxunit")

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var raw map[string]any
		if err := bsoncompat.UnmarshalDocMap(data, &raw); err != nil {
			continue
		}

		typeName, _ := raw["$Type"].(string)
		if typeName == "Projects$Module" || typeName == "Projects$ModuleImpl" || typeName == "Projects$ModuleSettings" {
			name, _ := raw["Name"].(string)
			fmt.Printf("Type: %s\n", typeName)
			fmt.Printf("  Name: %q\n", name)

			// Print full structure for first of each type
			prettyJSON, _ := json.MarshalIndent(raw, "  ", "  ")
			fmt.Printf("  Full: %s\n\n", string(prettyJSON))
		}
	}
}

func blobToUUIDSwapped(blob []byte) string {
	if len(blob) != 16 {
		return ""
	}
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		blob[3], blob[2], blob[1], blob[0],
		blob[5], blob[4],
		blob[7], blob[6],
		blob[8], blob[9],
		blob[10], blob[11], blob[12], blob[13], blob[14], blob[15])
}

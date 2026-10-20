// SPDX-License-Identifier: Apache-2.0

// Example: Debug BSON contents
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"

	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: debug_bson <path-to-mpr-file> [type-filter]")
		os.Exit(1)
	}

	mprPath := os.Args[1]
	typeFilter := ""
	if len(os.Args) > 2 {
		typeFilter = os.Args[2]
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", mprPath))
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query("SELECT UnitID, ContainerID, ContainmentName, Contents FROM Unit")
	if err != nil {
		fmt.Printf("Error querying: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var unitID, containerID []byte
		var containmentName string
		var contents []byte

		rows.Scan(&unitID, &containerID, &containmentName, &contents)

		var raw map[string]any
		if err := bsoncompat.UnmarshalDocMap(contents, &raw); err != nil {
			continue
		}

		typeName, _ := raw["$Type"].(string)

		if typeFilter != "" && typeName != typeFilter {
			continue
		}

		count++
		fmt.Printf("\n=== Unit %d ===\n", count)
		fmt.Printf("UnitID: %x\n", unitID)
		fmt.Printf("ContainerID: %x\n", containerID)
		fmt.Printf("ContainmentName: %s\n", containmentName)
		fmt.Printf("Type: %s\n", typeName)

		// Convert to JSON for readable output
		jsonBytes, _ := json.MarshalIndent(raw, "", "  ")
		if len(jsonBytes) > 5000 {
			fmt.Printf("Content (truncated): %s...\n", jsonBytes[:5000])
		} else {
			fmt.Printf("Content:\n%s\n", jsonBytes)
		}

		if count >= 5 {
			fmt.Println("\n... (showing first 5 matching units)")
			break
		}
	}

	if count == 0 {
		fmt.Println("No matching units found")
	}
}

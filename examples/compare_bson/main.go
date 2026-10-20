// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: compare_bson <mprcontents-path> <uuid>")
		fmt.Println("Example: compare_bson mx-test-projects/test1-go-app/mprcontents 84776610-13b2-48cb-a265-d5e984d63129")
		os.Exit(1)
	}

	contentsDir := os.Args[1]
	uuid := os.Args[2]

	// Build file path
	path := filepath.Join(contentsDir, uuid[0:2], uuid[2:4], uuid+".mxunit")

	fmt.Printf("Reading: %s\n\n", path)

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	var raw map[string]any
	if err := bsoncompat.UnmarshalDocMap(data, &raw); err != nil {
		fmt.Printf("Error unmarshaling BSON: %v\n", err)
		os.Exit(1)
	}

	// Pretty print as JSON to see structure
	prettyJSON, _ := json.MarshalIndent(raw, "", "  ")
	fmt.Println("=== BSON Content (as JSON) ===")
	fmt.Println(string(prettyJSON))

	// Check the type of $ID field
	fmt.Println("\n=== Field Types ===")
	for key, value := range raw {
		fmt.Printf("%s: %T\n", key, value)
		if key == "$ID" {
			fmt.Printf("  $ID value: %v\n", value)
		}
	}
}

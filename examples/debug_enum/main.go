// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"fmt"
	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
	"os"
	"path/filepath"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: debug_enum <mpr-path>")
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
		if typeName == "Enumerations$Enumeration" {
			name, _ := raw["Name"].(string)
			if name == "Enum_DistanceUnit" {
				fmt.Printf("Enum: %s\n", name)

				values := raw["Values"]
				fmt.Printf("  Values type: %T\n", values)
				fmt.Printf("  Values reflect type: %s\n", reflect.TypeOf(values))

				if arr, ok := values.(bson.A); ok {
					fmt.Printf("  Is bson.A: true, len=%d\n", len(arr))
					for i, v := range arr {
						fmt.Printf("    [%d] type: %T\n", i, v)
					}
				} else if arr, ok := values.([]any); ok {
					fmt.Printf("  Is []interface{}: true, len=%d\n", len(arr))
					for i, v := range arr {
						fmt.Printf("    [%d] type: %T\n", i, v)
					}
				} else {
					fmt.Println("  Neither bson.A nor []interface{}")
				}
				break
			}
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

// SPDX-License-Identifier: Apache-2.0

package sql

import (
	dbsql "database/sql"
	"testing"
)

// The external-SQL drivers are wired in by blank import for their registration
// side effect alone — this package calls none of their APIs. A dependency bump
// can therefore change or drop a driver name without breaking the build, and
// the failure would only surface as "unknown driver" at connect time, in front
// of a user with a real database.
//
// Added while bumping github.com/microsoft/go-mssqldb v1.10.0 -> v1.11.2 for a
// customer's blocking dependency scan, which had no coverage to back it.
func TestExternalDriversRegistered(t *testing.T) {
	registered := make(map[string]bool)
	for _, d := range dbsql.Drivers() {
		registered[d] = true
	}

	for _, name := range []string{"sqlserver", "pgx", "oracle"} {
		if !registered[name] {
			t.Errorf("driver %q is not registered; database/sql knows %v", name, dbsql.Drivers())
		}
	}
}

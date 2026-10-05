// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// The fixtures are Studio Pro-authored 11.15 documents: `mx convert` (11.15.0)
// of an 11.14 project made with 40-message-definition-examples.mdl
// (ako/mxcli#987). The collection MsgTest.MD_Order became two
// MessageDefinition2 documents, and IMM_Order's source moved from
// MessageDefinition "MsgTest.MD_Order.OrderMessage" to MessageDefinition2
// "MsgTest.OrderMessage" with no MessageDefinition key. `mx check`: 0 errors.
//
// The round trip is the one users perform: describe -> exec rewrites the same
// document, which must come back semantically unchanged (canon.Equal, element
// $IDs normalised away).
const messageDocumentDeps = `create module MsgTest;
create persistent entity MsgTest.Customer ( FirstName: String(100), LastName: String(100), Address: String(200) );
create persistent entity MsgTest.Order ( OrderId: Long, OrderDate: DateTime, TotalAmount: Decimal );
create persistent entity MsgTest.OrderLine ( Sku: String(50), Quantity: Integer );
create association MsgTest.Order_Customer from MsgTest.Order to MsgTest.Customer;
create association MsgTest.OrderLine_Order from MsgTest.OrderLine to MsgTest.Order;`

func TestMessageDefinitionDocumentRoundTrip(t *testing.T) {
	env := setupTestEnvWithBackend(t, func() backend.FullBackend { return modelsdkbackend.New() })
	defer env.teardown()
	env.requireMinVersion(t, 11, 15)

	if err := env.executeMDL(messageDocumentDeps); err != nil {
		t.Fatalf("deps: %v", err)
	}
	modules := moduleUnitIDs(t, env.projectPath)
	dir := filepath.Join("testdata", "message-definitions-11.15")
	transplant := func(file, typeName string) {
		contents, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		w, err := mmpr.NewWriter(env.projectPath)
		if err != nil {
			t.Fatalf("open writer: %v", err)
		}
		defer w.Close()
		if err := w.InsertUnit(documentID(t, contents), modules["MsgTest"], "Documents", typeName, contents); err != nil {
			t.Fatalf("transplant %s: %v", file, err)
		}
	}
	transplant("MsgTest.OrderMessage.11_15.bson", "MessageDefinitions$MessageDefinition2")
	transplant("MsgTest.CustomerOrders.11_15.bson", "MessageDefinitions$MessageDefinition2")
	transplant("MsgTest.IMM_Order.11_15.bson", "ImportMappings$ImportMapping")
	// The writer above went around the executor's connection; reconnect so the
	// next statements read the transplanted documents.
	if err := env.executor.Execute(&ast.DisconnectStmt{}); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if err := env.executor.Execute(&ast.ConnectStmt{Path: env.projectPath}); err != nil {
		t.Fatalf("reconnect: %v", err)
	}

	cases := []struct{ describe, typeName, name string }{
		{"describe message definition MsgTest.OrderMessage", "MessageDefinitions$MessageDefinition2", "OrderMessage"},
		{"describe message definition MsgTest.CustomerOrders", "MessageDefinitions$MessageDefinition2", "CustomerOrders"},
		{"describe import mapping MsgTest.IMM_Order", "ImportMappings$ImportMapping", "IMM_Order"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, before := storedUnit(t, env.projectPath, tc.typeName, tc.name)
			described, err := env.describeMDL(tc.describe)
			if err != nil {
				t.Fatalf("describe: %v", err)
			}
			if err := env.executeMDL(described); err != nil {
				t.Fatalf("re-executing DESCRIBE output failed: %v\n--- output ---\n%s", err, described)
			}
			_, after := storedUnit(t, env.projectPath, tc.typeName, tc.name)
			equal, err := canon.Equal(before, after)
			if err != nil {
				t.Fatalf("canonical compare: %v", err)
			}
			if !equal {
				t.Errorf("%s does not round-trip: DESCRIBE output rebuilt a different document.\n"+
					"--- DESCRIBE output ---\n%s", tc.name, described)
			}
		})
	}

	// Control: a describe output that changes one exposed name IS seen, so the
	// green subtests above compared something.
	t.Run("control", func(t *testing.T) {
		_, before := storedUnit(t, env.projectPath, "MessageDefinitions$MessageDefinition2", "OrderMessage")
		described, err := env.describeMDL("describe message definition MsgTest.OrderMessage")
		if err != nil {
			t.Fatalf("describe: %v", err)
		}
		changed := strings.Replace(described, "as 'GrandTotal'", "as 'Total'", 1)
		if changed == described {
			t.Fatalf("control needs `as 'GrandTotal'` in the description:\n%s", described)
		}
		if err := env.executeMDL(changed); err != nil {
			t.Fatalf("exec: %v", err)
		}
		_, after := storedUnit(t, env.projectPath, "MessageDefinitions$MessageDefinition2", "OrderMessage")
		if equal, _ := canon.Equal(before, after); equal {
			t.Error("control: a changed exposed name was not seen by the comparison")
		}
	})
}

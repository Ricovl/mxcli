// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1443 (mirror of mendixlabs/mxcli#1545): a document in a folder of
// its own name — one folder per page or microflow is a common layout — was not
// renamed. RENAME PAGE renamed the FOLDER, printed "Renamed page: …", and the
// reference sweep had already pointed every caller at the new name, so mx check
// reported CE1613 "The selected page 'Shop.Order_List' no longer exists".
//
// The document rename looked its target up again by bare name, among every unit
// in the module's containers whatever its $Type, and took the first match. A
// folder is such a unit, and a folder created by a FOLDER clause is inserted
// before the document it holds, so it wins. Each case below arms that trap
// (precondition: the folder is listed before the document) for one of the kinds
// RENAME treats as a document, and the last case is the issue's control: a page
// in a differently named folder, which was always renamed correctly.
func TestRenameDocument_InSameNamedFolder_RenamesTheDocument(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const setup = `create module Shop1443;
create persistent entity Shop1443.Order (Number: String(50));
create page Shop1443.Order_Overview folder 'Order_Overview' (Title: 'Orders', Layout: Atlas_Core.Atlas_Default) {
  dynamictext txtTitle (Content: 'Orders')
};
create microflow Shop1443.ACT_Order_Overview_Open () begin show page Shop1443.Order_Overview(); end;
create microflow Shop1443.Order_Process ($Order: Shop1443.Order) folder 'Order_Process' begin return; end;
create microflow Shop1443.ACT_Order_Process_Call ($Order: Shop1443.Order) begin call microflow Shop1443.Order_Process(Order = $Order); end;
create nanoflow Shop1443.NF_Order () folder 'NF_Order' begin return; end;
create constant Shop1443.C_Order folder 'C_Order' (Type: String, DefaultValue: 'x');
create enumeration Shop1443.E_Status (Open 'Open', Closed 'Closed') folder 'E_Status';
create java action Shop1443.JA_Order folder 'JA_Order' () returns Boolean as $$
return true;
$$;
create workflow Shop1443.WF_Order folder 'WF_Order' parameter $Context: Shop1443.Order begin end workflow;
create page Shop1443.Customer_Overview folder 'Customers' (Title: 'Customers', Layout: Atlas_Core.Atlas_Default) {
  dynamictext txtTitle (Content: 'Customers')
};`
	if err := afRun(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	for _, tc := range []struct {
		kind, keyword, old, new, folder string
	}{
		{"page", "page", "Order_Overview", "Order_List", "Order_Overview"},
		{"microflow", "microflow", "Order_Process", "Order_Handle", "Order_Process"},
		{"nanoflow", "nanoflow", "NF_Order", "NF_Order_Renamed", "NF_Order"},
		{"constant", "constant", "C_Order", "C_Order_Renamed", "C_Order"},
		{"enumeration", "enumeration", "E_Status", "E_Status_Renamed", "E_Status"},
		{"javaaction", "java action", "JA_Order", "JA_Order_Renamed", "JA_Order"},
		{"workflow", "workflow", "WF_Order", "WF_Order_Renamed", "WF_Order"},
		// Control: the folder's name differs from the page's.
		{"page", "page", "Customer_Overview", "Customer_List", "Customers"},
	} {
		t.Run(tc.kind+"_"+tc.old, func(t *testing.T) {
			docID := docIDInModule(t, exec, tc.kind, "Shop1443", tc.old)
			if docID == "" {
				t.Fatalf("precondition: %s Shop1443.%s not found", tc.kind, tc.old)
			}
			folderID := folderIDInModule(t, exec, "Shop1443", tc.folder)
			if folderID == "" {
				t.Fatalf("precondition: folder %q not found", tc.folder)
			}
			if tc.folder == tc.old && unitIndex(t, exec, folderID) > unitIndex(t, exec, docID) {
				t.Fatalf("precondition: folder %q must be listed before the %s it holds, or a name-only lookup finds the document first and this case cannot detect anything", tc.folder, tc.kind)
			}

			out.Reset()
			stmt := "rename " + tc.keyword + " Shop1443." + tc.old + " to " + tc.new + ";"
			if err := afRun(t, exec, stmt); err != nil {
				t.Fatalf("%s: %v\n%s", stmt, err, out.String())
			}

			if got := docIDInModule(t, exec, tc.kind, "Shop1443", tc.new); got != docID {
				t.Errorf("%s Shop1443.%s after the rename: got unit %q, want the renamed document %q\n%s", tc.kind, tc.new, got, docID, out.String())
			}
			if got := docIDInModule(t, exec, tc.kind, "Shop1443", tc.old); got != "" {
				t.Errorf("%s Shop1443.%s still exists after the rename (unit %q)", tc.kind, tc.old, got)
			}
			if got := folderIDInModule(t, exec, "Shop1443", tc.folder); got != folderID {
				t.Errorf("folder %q was renamed by RENAME %s (now unit %q)", tc.folder, strings.ToUpper(tc.keyword), got)
			}
			if got := folderIDInModule(t, exec, "Shop1443", tc.new); got != "" {
				t.Errorf("a folder is now named %q: the rename hit the folder, not the %s", tc.new, tc.kind)
			}
		})
	}

	// The callers point at a document that exists under the new name.
	for _, c := range []struct{ caller, want string }{
		{"Shop1443.ACT_Order_Overview_Open", "Shop1443.Order_List"},
		{"Shop1443.ACT_Order_Process_Call", "Shop1443.Order_Handle"},
	} {
		out.Reset()
		if err := afRun(t, exec, "describe microflow "+c.caller+";"); err != nil {
			t.Fatalf("describe %s: %v", c.caller, err)
		}
		if !strings.Contains(out.String(), c.want) {
			t.Errorf("%s does not reference %s after the rename:\n%s", c.caller, c.want, out.String())
		}
	}
}

// docIDInModule returns the ID of the document of the given RENAME kind named
// name in module, or "".
func docIDInModule(t *testing.T, exec *Executor, kind, module, name string) model.ID {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	invalidateHierarchy(ctx)
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	type doc struct {
		id          model.ID
		containerID model.ID
		name        string
	}
	var docs []doc
	b := ctx.Backend
	switch kind {
	case "page":
		all, err := b.ListPages()
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range all {
			docs = append(docs, doc{d.ID, d.ContainerID, d.Name})
		}
	case "microflow":
		all, err := b.ListMicroflows()
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range all {
			docs = append(docs, doc{d.ID, d.ContainerID, d.Name})
		}
	case "nanoflow":
		all, err := b.ListNanoflows()
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range all {
			docs = append(docs, doc{d.ID, d.ContainerID, d.Name})
		}
	case "constant":
		all, err := b.ListConstants()
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range all {
			docs = append(docs, doc{d.ID, d.ContainerID, d.Name})
		}
	case "enumeration":
		all, err := b.ListEnumerations()
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range all {
			docs = append(docs, doc{d.ID, d.ContainerID, d.Name})
		}
	case "javaaction":
		all, err := b.ListJavaActions()
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range all {
			docs = append(docs, doc{d.ID, d.ContainerID, d.Name})
		}
	case "workflow":
		all, err := b.ListWorkflows()
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range all {
			docs = append(docs, doc{d.ID, d.ContainerID, d.Name})
		}
	default:
		t.Fatalf("docIDInModule: unknown kind %q", kind)
	}
	for _, d := range docs {
		if d.name == name && h.GetModuleName(h.FindModuleID(d.containerID)) == module {
			return d.id
		}
	}
	return ""
}

// folderIDInModule returns the ID of the folder named name anywhere in module.
func folderIDInModule(t *testing.T, exec *Executor, module, name string) model.ID {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	invalidateHierarchy(ctx)
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	folders, err := ctx.Backend.ListFolders()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range folders {
		if f.Name == name && h.GetModuleName(h.FindModuleID(f.ContainerID)) == module {
			return f.ID
		}
	}
	return ""
}

// unitIndex is the position of a unit in the backend's unit listing — the order
// a name-only scan over the units meets them in.
func unitIndex(t *testing.T, exec *Executor, id model.ID) int {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	units, err := ctx.Backend.ListUnits()
	if err != nil {
		t.Fatal(err)
	}
	for i, u := range units {
		if u.ID == id {
			return i
		}
	}
	t.Fatalf("unit %s is not listed", id)
	return -1
}

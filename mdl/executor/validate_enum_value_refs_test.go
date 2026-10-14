// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1472 item 6: `Module.Enum.Key` in an expression was never
// resolved, so a key the enumeration does not have passed check and exec.

func enumValueCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("MyModule")
	status := mkEnumeration(mod.ID, "Status", "Open", "Closed")
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:      func() ([]*types.FolderInfo, error) { return nil, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{status}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx
}

func checkEnumValues(t *testing.T, ctx *ExecContext, src string) ([]error, []string) {
	t.Helper()
	prog := parseMDL(t, src)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	errs := validateEnumValueRefs(ctx, prog, sc)
	return errs, sc.warnings
}

func TestEnumValueRefs_Reported(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"declare default and comparison (the reported shape)": {
			`create microflow MyModule.K6 () returns Boolean
			begin
				declare $p Enumeration(MyModule.Status) = MyModule.Status.NoSuchKey;
				return $p = MyModule.Status.AlsoNoKey;
			end;`,
			"MyModule.Status.NoSuchKey",
		},
		"nanoflow if condition": {
			`create nanoflow MyModule.N ($s: Enumeration(MyModule.Status)) returns Boolean
			begin
				if $s = MyModule.Status.Opne then return true; else return false; end if;
			end;`,
			"MyModule.Status.Opne",
		},
		"enumeration the script creates": {
			`create enumeration MyModule.Color (Red 'Red', Blue 'Blue');
			create microflow MyModule.C () returns Boolean begin
				declare $c Enumeration(MyModule.Color) = MyModule.Color.Green;
				return true;
			end;`,
			"MyModule.Color.Green",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			errs, _ := checkEnumValues(t, enumValueCtx(t), c.src)
			var all []string
			for _, e := range errs {
				all = append(all, e.Error())
			}
			got := strings.Join(all, "\n")
			if !strings.Contains(got, c.want) {
				t.Errorf("got %q, want a finding naming %q", got, c.want)
			}
		})
	}
	// Both keys of the reported shape, not just the first.
	errs, _ := checkEnumValues(t, enumValueCtx(t), `create microflow MyModule.K6 () returns Boolean
		begin
			declare $p Enumeration(MyModule.Status) = MyModule.Status.NoSuchKey;
			return $p = MyModule.Status.AlsoNoKey;
		end;`)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "AlsoNoKey") || !strings.Contains(errs[0].Error(), "Open, Closed") {
		t.Errorf("got %v, want one error naming both keys and the values", errs)
	}
}

func TestEnumValueRefs_Controls(t *testing.T) {
	cases := map[string]string{
		"existing keys": `create microflow MyModule.OK () returns Boolean begin
			declare $p Enumeration(MyModule.Status) = MyModule.Status.Open;
			return $p = MyModule.Status.Closed; end;`,
		"enumeration not in the project": `create microflow MyModule.OK () returns Boolean begin
			return $x = System.WorkflowState.InProgress; end;`,
		"value the script adds": `alter enumeration MyModule.Status add value Pending caption 'Pending';
			create microflow MyModule.OK () returns Boolean begin
			declare $p Enumeration(MyModule.Status) = MyModule.Status.Pending; return true; end;`,
		"enumeration the script creates, existing key": `create enumeration MyModule.Color (Red 'Red', Blue 'Blue');
			create microflow MyModule.C () returns Boolean begin
			declare $c Enumeration(MyModule.Color) = MyModule.Color.Blue; return true; end;`,
		"two-part name is not a key": `create microflow MyModule.OK () returns Boolean begin
			declare $l List of MyModule.Status = empty; return true; end;`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if errs, _ := checkEnumValues(t, enumValueCtx(t), src); len(errs) != 0 {
				t.Errorf("got %v, want none", errs)
			}
		})
	}
	// An excluded microflow is not validated by Mendix: a warning, not an error.
	errs, warns := checkEnumValues(t, enumValueCtx(t), `@excluded
		create microflow MyModule.X () returns Boolean begin
		declare $p Enumeration(MyModule.Status) = MyModule.Status.Nope; return true; end;`)
	if len(errs) != 0 || len(warns) != 1 {
		t.Errorf("excluded: got errors %v, warnings %v; want one warning", errs, warns)
	}
}

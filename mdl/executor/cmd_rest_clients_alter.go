// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// alterRestClient handles ALTER CONSUMED REST SERVICE (mendixlabs/mxcli#1608).
//
// Before it, the only MDL way to add an operation to an existing consumed REST
// service was CREATE OR MODIFY restating every operation, and an operation
// left out was deleted. The edit goes to the backend as a splice of the stored
// unit: an operation no action names is written back as stored, including the
// properties of a Studio Pro-made operation that MDL has no words for.
//
// The actions apply in order, so `drop operation X, add operation X (…)`
// replaces X, and `modify operation X` may follow `add operation X`.
func alterRestClient(ctx *ExecContext, stmt *ast.AlterRestClientStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	if err := checkVersionGates(ctx, stmt); err != nil {
		return err
	}
	svc, err := findRestClient(ctx, stmt.Name)
	if err != nil {
		return err
	}

	// The operations as they will be, in order: the stored ones first, then
	// the added ones.
	type pendingOp struct {
		op     *model.RestClientOperation // what is written
		stored string                     // the stored name; "" for an added operation
		keys   []string                   // MDL properties a modify wrote (stored operations)
	}
	var ops []*pendingOp
	for _, op := range svc.Operations {
		cp := *op
		ops = append(ops, &pendingOp{op: &cp, stored: op.Name})
	}
	find := func(name string) int {
		for i, p := range ops {
			if strings.EqualFold(p.op.Name, name) {
				return i
			}
		}
		return -1
	}
	notFound := func(name string) error {
		var have []string
		for _, p := range ops {
			have = append(have, p.op.Name)
		}
		return mdlerrors.NewNotFoundMsg("operation", name, fmt.Sprintf(
			"consumed rest service %s has no operation %s (it has: %s)", stmt.Name, name, strings.Join(have, ", ")))
	}

	edit := &backend.ConsumedRestServiceEdit{}
	for _, action := range stmt.Actions {
		switch a := action.(type) {
		case *ast.RestClientSetAction:
			if a.BaseUrlSet {
				v := a.BaseUrl
				edit.BaseUrl = &v
			}
			if a.AuthenticationSet {
				edit.SetAuthentication = true
				edit.Authentication = nil
				if a.Authentication != nil {
					if edit.Authentication, err = restAuthFromDef(ctx, stmt.Name, svc.ContainerID, a.Authentication); err != nil {
						return err
					}
				}
			}

		case *ast.RestClientAddOperationAction:
			if find(a.Operation.Name) >= 0 {
				return mdlerrors.NewAlreadyExistsMsg("operation", a.Operation.Name, fmt.Sprintf(
					"consumed rest service %s already has an operation %s — change it with 'modify operation %s ( … )'",
					stmt.Name, a.Operation.Name, a.Operation.Name))
			}
			op, err := buildRestClientOperation(a.Operation)
			if err != nil {
				return fmt.Errorf("operation %q: %w", a.Operation.Name, err)
			}
			ops = append(ops, &pendingOp{op: op})

		case *ast.RestClientModifyOperationAction:
			i := find(a.Operation.Name)
			if i < 0 {
				return notFound(a.Operation.Name)
			}
			if err := applyRestOperationKeys(ops[i].op, a.Operation); err != nil {
				return fmt.Errorf("operation %q: %w", a.Operation.Name, err)
			}
			ops[i].keys = append(ops[i].keys, a.Operation.Keys...)

		case *ast.RestClientDropOperationAction:
			i := find(a.Name)
			if i < 0 {
				return notFound(a.Name)
			}
			ops = append(ops[:i], ops[i+1:]...)

		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unsupported alter action: %T", action))
		}
	}

	kept := map[string]bool{}
	for _, p := range ops {
		switch {
		case p.stored == "":
			edit.Add = append(edit.Add, p.op)
		case len(p.keys) > 0:
			kept[p.stored] = true
			edit.Modify = append(edit.Modify, backend.ConsumedRestOperationEdit{Name: p.stored, Operation: p.op, Keys: p.keys})
		default:
			kept[p.stored] = true
		}
	}
	for _, op := range svc.Operations {
		if !kept[op.Name] {
			edit.Drop = append(edit.Drop, op.Name)
		}
	}

	if err := ctx.Backend.EditConsumedRestService(svc.ID, edit); err != nil {
		return mdlerrors.NewBackend("alter consumed rest service", err)
	}
	ctx.ReportMutation("Altered", "consumed rest service: %s (%d operations)", stmt.Name, len(ops))
	return nil
}

// findRestClient returns the stored consumed REST service named name.
func findRestClient(ctx *ExecContext, name ast.QualifiedName) (*model.ConsumedRestService, error) {
	services, err := ctx.Backend.ListConsumedRestServices()
	if err != nil {
		return nil, mdlerrors.NewBackend("list consumed rest services", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, mdlerrors.NewBackend("build hierarchy", err)
	}
	for _, svc := range services {
		if strings.EqualFold(h.GetModuleName(h.FindModuleID(svc.ContainerID)), name.Module) && strings.EqualFold(svc.Name, name.Name) {
			return svc, nil
		}
	}
	return nil, mdlerrors.NewNotFound("consumed rest service", name.String())
}

// applyRestOperationKeys writes the properties def states onto op, leaving
// every other one as it was: `modify operation X (Timeout: 60)` changes the
// timeout and nothing else.
func applyRestOperationKeys(op *model.RestClientOperation, def *ast.RestOperationDef) error {
	built, err := buildRestClientOperation(def)
	if err != nil {
		return err
	}
	for _, k := range def.Keys {
		switch k {
		case "Method":
			op.HttpMethod = built.HttpMethod
		case "Path":
			op.Path = built.Path
		case "Parameters":
			op.Parameters = built.Parameters
		case "Query":
			op.QueryParameters = built.QueryParameters
		case "Headers":
			op.Headers = built.Headers
		case "Body":
			op.BodyType, op.BodyVariable, op.BodyMappings = built.BodyType, built.BodyVariable, built.BodyMappings
		case "Timeout":
			op.Timeout = built.Timeout
		case "Response":
			op.ResponseType, op.ResponseVariable = built.ResponseType, built.ResponseVariable
			op.ResponseEntity, op.ResponseMappings = built.ResponseEntity, built.ResponseMappings
		}
	}
	return nil
}

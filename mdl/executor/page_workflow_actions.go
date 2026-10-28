// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// The workflow events a widget can carry beside `complete task`: `open user
// task` (Studio Pro's "Show user task page"), `open workflow` ("Show workflow
// admin page") and `call workflow M.WF` ("Call workflow"). Until they had a
// spelling, describe printed no action for a stored one and a rewrite wrote a
// Forms$NoAction in its place — on ako/TestApp, the link button that opens a
// task from WorkflowCommons.TaskInbox (ako/mxcli#832).

// projectAtLeast reports whether the connected project is at least
// major.minor. A project whose version cannot be read counts as current, as
// the other page version guards treat it.
func (pb *pageBuilder) projectAtLeast(major, minor int) bool {
	if pb.backend == nil {
		return true
	}
	pv := pb.backend.ProjectVersion()
	if pv == nil || pv.MajorVersion == 0 {
		return true
	}
	return pv.IsAtLeast(major, minor)
}

// resolveWorkflow checks that a workflow of that qualified name exists.
func (pb *pageBuilder) resolveWorkflow(qualifiedName string) error {
	qualifiedName = unquoteQualifiedName(qualifiedName)
	module, name, ok := strings.Cut(qualifiedName, ".")
	if !ok || module == "" || name == "" {
		return mdlerrors.NewValidationf("invalid workflow name: %s", qualifiedName)
	}
	wfs, err := pb.backend.ListWorkflows()
	if err != nil {
		return mdlerrors.NewBackend("list workflows", err)
	}
	h, err := pb.getHierarchy()
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}
	for _, wf := range wfs {
		if wf.Name == name && h.GetModuleName(h.FindModuleID(wf.ContainerID)) == module {
			return nil
		}
	}
	return mdlerrors.NewNotFound("workflow", qualifiedName)
}

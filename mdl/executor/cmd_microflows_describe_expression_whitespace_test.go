// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Studio Pro stores an expression exactly as typed, and users routinely leave
// a trailing newline (or space) in the expression editor. `describe` used to
// emit the stored text verbatim, so the closing `)` or `;` landed on a line of
// its own:
//
//	change $IteratorSingleMCPTool (SyncState = MCPClient.ENUM_SyncState.Syncing
//	);
//
// The values below are copied from real documents (Evora Factory Management):
// the leading/trailing whitespace is layout, not meaning, and is dropped;
// interior newlines of a multi-line expression are the author's formatting and
// are kept.
func TestDescribeTrimsStoredExpressionWhitespace(t *testing.T) {
	e := newTestExecutor()
	cases := []struct {
		name   string
		action microflows.MicroflowAction
		want   string
	}{
		{
			name: "change member",
			action: &microflows.ChangeObjectAction{
				ChangeVariable: "IteratorSingleMCPTool",
				Changes: []*microflows.MemberChange{
					{AttributeQualifiedName: "MCPClient.SingleMCPTool.SyncState", Value: "MCPClient.ENUM_SyncState.Syncing\n"},
				},
			},
			want: "change $IteratorSingleMCPTool (SyncState = MCPClient.ENUM_SyncState.Syncing);",
		},
		{
			name: "create members, trailing newline and trailing space",
			action: &microflows.CreateObjectAction{
				EntityQualifiedName: "DigitalTwin.TechnicianTicket",
				OutputVariable:      "TechnicianTicket",
				InitialMembers: []*microflows.MemberChange{
					{AttributeQualifiedName: "DigitalTwin.TechnicianTicket.Description", Value: "'Ticket for configuration change:' \n"},
					{AttributeQualifiedName: "DigitalTwin.TechnicianTicket.NeedsApproval", Value: "\ntrue\n"},
				},
			},
			want: "$TechnicianTicket = create DigitalTwin.TechnicianTicket (Description = 'Ticket for configuration change:', NeedsApproval = true);",
		},
		{
			name: "change variable (member path)",
			action: &microflows.ChangeVariableAction{
				VariableName: "$Order/Total",
				Value:        "$Total\r\n",
			},
			want: "change $Order (Total = $Total);",
		},
		{
			name:   "set variable",
			action: &microflows.ChangeVariableAction{VariableName: "Count", Value: "$Count + 1\n"},
			want:   "set $Count = $Count + 1;",
		},
		{
			name: "call microflow argument",
			action: &microflows.MicroflowCallAction{
				MicroflowCall: &microflows.MicroflowCall{
					Microflow: "AgentCommons.Version_Validate_BeforeRun",
					ParameterMappings: []*microflows.MicroflowCallParameterMapping{
						{Parameter: "AgentCommons.Version_Validate_BeforeRun.Version", Argument: "$PageHelper/AgentCommons.PageHelper_Version/AgentCommons.Version\n"},
					},
				},
				UseReturnVariable:  true,
				ResultVariableName: "Valid",
			},
			want: "$Valid = call microflow AgentCommons.Version_Validate_BeforeRun(Version = $PageHelper/AgentCommons.PageHelper_Version/AgentCommons.Version);",
		},
		{
			name: "find keeps interior newlines",
			action: &microflows.ListOperationAction{
				OutputVariable: "Next",
				Operation: &microflows.FindOperation{
					ListVariable: "List",
					Expression:   "$currentObject/IsEnabled\nand $currentObject/IsEdited = false\n",
				},
			},
			want: "$Next = find $List where $currentObject/IsEnabled\nand $currentObject/IsEdited = false;",
		},
		{
			name: "log template parameter",
			action: &microflows.LogMessageAction{
				LogLevel:           "Info",
				LogNodeName:        "'Node'\n",
				TemplateParameters: []string{"$Order/Number\n"},
			},
			want: "log node 'Node' 'Message' with ({1} = $Order/Number);",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := e.formatAction(tc.action, nil, nil)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// SPDX-License-Identifier: Apache-2.0

package meta

// System module read access — who may READ each System entity, as the runtime
// defines it.
//
// The System module's access rules are not in the .mpr: Studio Pro shows them
// read-only and mxbuild writes them into the deployed model. So
// SystemEntities carries no access rules, and anything that reasons about
// "can this role read that entity" had no answer for System at all — a lint
// rule asking it would flag every System entity as unreadable.
//
// MEASURED, not taken from documentation: every row below is read from the
// System domain model in a built `deployment/model/model.mdp` — Mendix
// 11.15.0, security level Production (CheckEverything), an app whose user
// roles each hold exactly one of System.Administrator / System.User. The .mdp
// stores each rule's AllowedUserRoles (user roles, not module roles); a rule
// granted to every user role is recorded here as {System.User,
// System.Administrator}, one granted only to the user roles holding
// System.Administrator (`IsSystemAdministrator` in the .mdp) as
// {System.Administrator}. TestSystemReadAccessMatchesMDP re-checks the table
// against an .mdp given with -mdp. Four System entities the 11.15.0 model has
// and SystemEntities does not (WorkflowSubProcess, WorkflowSubProcessDefinition
// — both System.Administrator only — CdcSnapshotStore, SynchronizationHistory —
// no rule) are left out with them.
//
// Only READ is recorded — a rule counts as granting read when any member of
// it is ReadOnly or ReadWrite. The XPath constraints are abbreviated in the
// comments; RowScoped says only that one exists.

// System module roles.
const (
	SystemRoleUser          = "System.User"
	SystemRoleAdministrator = "System.Administrator"
)

// SystemReadGrant is one System access rule that grants read.
type SystemReadGrant struct {
	ModuleRole string // SystemRoleUser or SystemRoleAdministrator
	RowScoped  bool   // the rule carries an XPath constraint
}

var (
	everyRole = []SystemReadGrant{{SystemRoleUser, false}, {SystemRoleAdministrator, false}}
	adminOnly = []SystemReadGrant{{SystemRoleAdministrator, false}}
	// Administrator reads every row; System.User only the rows the constraint
	// lets through (the current user's own tasks, groups, …).
	adminAllUserScoped = []SystemReadGrant{{SystemRoleAdministrator, false}, {SystemRoleUser, true}}
	everyRoleScoped    = []SystemReadGrant{{SystemRoleUser, true}, {SystemRoleAdministrator, true}}
	noRule             = []SystemReadGrant{}
)

// systemReadAccess is keyed by qualified entity name. An entity with an empty
// slice has NO access rule: nobody reads it directly, only through a
// specialization of their own (System.FileDocument, System.Image).
var systemReadAccess = map[string][]SystemReadGrant{
	// [id = '[%CurrentUser%]'] — for EVERY role, Administrator included. Other
	// users' ROWS come through a specialization's rules (Administration.Account);
	// their System.User MEMBERS only where that rule grants them, or for users
	// whose roles the user role may manage. Measured on 11.15.0 (ChipCoV9): a
	// list over System.User showed a ServiceManager every row with Name blank
	// but their own, and an Administrator (Manage All Roles) every name — the
	// Account rules of both grant no System.User member. SEC010 applies this.
	"System.User": everyRoleScoped,
	// [System.UserRoles = '[%CurrentUser%]' or System.grantableRoles[reversed()]/…/System.UserRoles = '[%CurrentUser%]']
	"System.UserRole": everyRoleScoped,
	// [System.owner='[%CurrentUser%]']
	"System.PrivateFileDocument": everyRoleScoped,

	"System.Language":                   everyRole,
	"System.TimeZone":                   everyRole,
	"System.HttpMessage":                everyRole,
	"System.HttpHeader":                 everyRole,
	"System.HttpRequest":                everyRole,
	"System.HttpResponse":               everyRole,
	"System.Paging":                     everyRole,
	"System.ODataResponse":              everyRole,
	"System.ConsumedODataConfiguration": everyRole,
	"System.WorkflowJumpToDetails":      everyRole,
	"System.WorkflowCurrentActivity":    everyRole,
	"System.WorkflowActivityDetails":    everyRole,
	"System.WorkflowRecord":             everyRole,
	"System.WorkflowActivityRecord":     everyRole,
	"System.WorkflowEvent":              everyRole,

	"System.XASInstance":                adminOnly,
	"System.Session":                    adminOnly,
	"System.ScheduledEventInformation":  adminOnly,
	"System.TokenInformation":           adminOnly,
	"System.SynchronizationError":       adminOnly,
	"System.SynchronizationErrorFile":   adminOnly,
	"System.ProcessedQueueTask":         adminOnly,
	"System.QueuedTask":                 adminOnly,
	"System.TaskQueueToken":             adminOnly,
	"System.WorkflowDefinition":         adminOnly,
	"System.WorkflowUserTaskDefinition": adminOnly,

	// [System.WorkflowUserTask_Workflow/System.WorkflowUserTask[State = 'InProgress' and (TargetUsers | TargetGroups/…/User | Assignees = '[%CurrentUser%]')]]
	"System.Workflow": adminAllUserScoped,
	// [State = 'InProgress' and (TargetUsers | TargetGroups/…/User | Assignees = '[%CurrentUser%]') and Workflow[State != 'Incompatible' and != 'Failed']]
	"System.WorkflowUserTask": adminAllUserScoped,
	// the same, through System.WorkflowUserTaskOutcome_WorkflowUserTask
	"System.WorkflowUserTaskOutcome": adminAllUserScoped,
	// [System.WorkflowEndedUserTask_Assignees = '[%CurrentUser%]']
	"System.WorkflowEndedUserTask": adminAllUserScoped,
	// the same, through …_WorkflowEndedUserTask
	"System.WorkflowEndedUserTaskOutcome": adminAllUserScoped,
	// [System.WorkflowGroup_User = '[%CurrentUser%]']
	"System.WorkflowGroup": adminAllUserScoped,

	"System.FileDocument":                    noRule,
	"System.Image":                           noRule,
	"System.Thumbnail":                       noRule,
	"System.Error":                           noRule,
	"System.SoapFault":                       noRule,
	"System.UserReportInfo":                  noRule,
	"System.BackgroundJob":                   noRule,
	"System.AutoCommitEntry":                 noRule,
	"System.UnreferencedFile":                noRule,
	"System.OfflineCreatedGuids":             noRule,
	"System.OfflineSynchronizationHistory":   noRule,
	"System.ChangeHash":                      noRule,
	"System.WorkflowVersion":                 noRule,
	"System.WorkflowActivity":                noRule,
	"System.WorkflowActivityUserTaskOutcome": noRule,
}

// SystemReadAccess returns the read-granting access rules of a System entity
// (qualified name) and whether the entity was measured at all. ok == false
// means "unknown", which a caller must not read as "nobody may read it".
func SystemReadAccess(entityQN string) (grants []SystemReadGrant, ok bool) {
	grants, ok = systemReadAccess[entityQN]
	return grants, ok
}

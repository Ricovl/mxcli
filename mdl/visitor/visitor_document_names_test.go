// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R10: every statement that names a renamed document type takes the Studio Pro
// name, reports the old one as its registered alias, and builds the same
// statement from both (ako/mxcli#755).
func TestDocumentTypeNamesFollowStudioPro(t *testing.T) {
	cases := []struct {
		code, old, canonical string
	}{
		// consumed rest service
		{deprecation.ConsumedRestService,
			"create rest client M.Api (BaseUrl: 'https://x', Authentication: none) { };",
			"create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) { };"},
		{deprecation.ConsumedRestService,
			"create or modify rest client M.Api (BaseUrl: 'https://x', Authentication: none) { };",
			"create or modify consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) { };"},
		{deprecation.ConsumedRestService, "drop rest client if exists M.Api;", "drop consumed rest service if exists M.Api;"},
		{deprecation.ConsumedRestService, "describe rest client M.Api;", "describe consumed rest service M.Api;"},
		{deprecation.ConsumedRestService, "list rest clients in M;", "list consumed rest services in M;"},
		{deprecation.ConsumedRestService, "move rest client M.Api to folder 'Int';", "move consumed rest service M.Api to folder 'Int';"},
		// consumed odata service
		{deprecation.ConsumedODataService,
			"create odata client M.Crm (ODataVersion: OData4, MetadataUrl: 'https://x/$metadata');",
			"create consumed odata service M.Crm (ODataVersion: OData4, MetadataUrl: 'https://x/$metadata');"},
		{deprecation.ConsumedODataService, "alter odata client M.Crm set ( Version: '2' );", "alter consumed odata service M.Crm set ( Version: '2' );"},
		{deprecation.ConsumedODataService, "drop odata client M.Crm;", "drop consumed odata service M.Crm;"},
		{deprecation.ConsumedODataService, "describe odata client M.Crm;", "describe consumed odata service M.Crm;"},
		{deprecation.ConsumedODataService, "list odata clients;", "list consumed odata services;"},
		{deprecation.ConsumedODataService, "move odata client M.Crm to N;", "move consumed odata service M.Crm to N;"},
		{deprecation.ConsumedODataService,
			"create external entity M.Cust from odata client M.Crm (EntitySet: 'Customers', RemoteName: 'Customer') (Name: String(100));",
			"create external entity M.Cust from consumed odata service M.Crm (EntitySet: 'Customers', RemoteName: 'Customer') (Name: String(100));"},
		// published odata service
		{deprecation.PublishedODataService,
			"create odata service M.Api (Path: 'odata/v1', Namespace: 'M');",
			"create published odata service M.Api (Path: 'odata/v1', Namespace: 'M');"},
		{deprecation.PublishedODataService, "alter odata service M.Api set ( Version: '2' );", "alter published odata service M.Api set ( Version: '2' );"},
		{deprecation.PublishedODataService, "drop odata service M.Api;", "drop published odata service M.Api;"},
		{deprecation.PublishedODataService, "describe odata service M.Api;", "describe published odata service M.Api;"},
		{deprecation.PublishedODataService, "list odata services in M;", "list published odata services in M;"},
		{deprecation.PublishedODataService, "grant access on odata service M.Api to M.User;", "grant access on published odata service M.Api to M.User;"},
		{deprecation.PublishedODataService, "revoke access on odata service M.Api from M.User;", "revoke access on published odata service M.Api from M.User;"},
		// task queue
		{deprecation.TaskQueue, "create queue M.Jobs (Parallelism: 2);", "create task queue M.Jobs (Parallelism: 2);"},
		{deprecation.TaskQueue, "create or modify queue M.Jobs folder 'Q' (Parallelism: 2);", "create or modify task queue M.Jobs folder 'Q' (Parallelism: 2);"},
		{deprecation.TaskQueue, "drop queue M.Jobs;", "drop task queue M.Jobs;"},
		{deprecation.TaskQueue, "describe queue M.Jobs;", "describe task queue M.Jobs;"},
		{deprecation.TaskQueue, "list queues in M;", "list task queues in M;"},
		{deprecation.TaskQueue, "move queue M.Jobs to folder 'Q';", "move task queue M.Jobs to folder 'Q';"},
		// app security
		{deprecation.AppSecurity, "alter project security level production;", "alter app security level production;"},
		{deprecation.AppSecurity, "alter project security guest access on role Guest;", "alter app security guest access on role Guest;"},
		{deprecation.AppSecurity, "alter project security strict mode on;", "alter app security strict mode on;"},
		// settings runtime
		{deprecation.SettingsRuntime, "alter settings model ( BcryptCost: 11, HashAlgorithm: 'BCrypt' );", "alter settings runtime ( BcryptCost: 11, HashAlgorithm: 'BCrypt' );"},
		{deprecation.SettingsRuntime, "ALTER SETTINGS MODEL (BcryptCost: 11);", "ALTER SETTINGS RUNTIME (BcryptCost: 11);"},
	}
	for _, c := range cases {
		t.Run(c.old, func(t *testing.T) {
			old := mustBuild(t, c.old)
			if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{c.code}) {
				t.Errorf("%q recorded %v, want [%s]", c.old, got, c.code)
			}
			canon := mustBuild(t, c.canonical)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("%q recorded %v, want none", c.canonical, got)
			}
			if len(old.Statements) != 1 {
				t.Fatalf("%q built %d statements, want 1", c.old, len(old.Statements))
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("old and canonical build different statements:\n old:   %#v\n canon: %#v",
					old.Statements, canon.Statements)
			}
		})
	}
}

// Words that happen to be the same tokens but are not the document type name
// are not aliases: a call's `in queue` option, a published REST service and
// the catalog's session state.
func TestDocumentTypeNamesLeaveOtherUsesAlone(t *testing.T) {
	for _, src := range []string{
		"create microflow M.F () begin call microflow M.G() in queue M.Jobs; end;",
		"drop published rest service M.Api;",
		"list consumed mcp services;",
		// `project security` after `show` is R6's MDL-DEPR090 (the whole
		// phrase becomes `describe app security`), not R10's name alias.
		"describe app security;",
		"alter settings workflows ( UserEntity: 'System.User' );",
	} {
		t.Run(src, func(t *testing.T) {
			if got := deprecationCodes(mustBuild(t, src)); len(got) != 0 {
				t.Errorf("recorded %v, want none", got)
			}
		})
	}
}

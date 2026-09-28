// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R10 (ako/mxcli#755): the old document type names are rewritten to Studio
// Pro's, the whole name at once, in the letter case the script uses.
func TestUpgrade_DocumentTypeNames(t *testing.T) {
	src := "CREATE REST CLIENT M.Api (BaseUrl: 'https://x', Authentication: none) { };\n" +
		"list odata clients;\nlist odata services in M;\n" +
		"grant access on odata service M.Api to M.User;\n" +
		"create or modify queue M.Jobs (Parallelism: 2);\nlist queues;\n" +
		"alter project security demo users off;\n" +
		"alter settings model ( BcryptCost: 11 );\n" +
		"create microflow M.F () begin call microflow M.G() in queue M.Jobs; end;\n"
	want := "CREATE CONSUMED REST SERVICE M.Api (BaseUrl: 'https://x', Authentication: none) { };\n" +
		"list consumed odata services;\nlist published odata services in M;\n" +
		"grant access on published odata service M.Api to M.User;\n" +
		"create or modify task queue M.Jobs (Parallelism: 2);\nlist task queues;\n" +
		"alter app security ( EnableDemoUsers: false );\n" +
		"alter settings runtime ( BcryptCost: 11 );\n" +
		// `in queue` is a call option, not the document type name.
		"create microflow M.F () begin call microflow M.G() in queue M.Jobs; end;\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	for code, n := range map[string]int{
		deprecation.ConsumedRestService: 1, deprecation.ConsumedODataService: 1,
		deprecation.PublishedODataService: 2, deprecation.TaskQueue: 2,
		deprecation.AppSecurity: 1, deprecation.SettingsRuntime: 1, deprecation.AppSecurityClause: 1,
	} {
		if res.Rewritten[code] != n {
			t.Errorf("Rewritten[%s] = %d, want %d (all: %v)", code, res.Rewritten[code], n, res.Rewritten)
		}
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}

// The rest of R10 (ako/mxcli#755): `ai model` and a JSON structure's `sample`;
// an agent's `Model:` key and a page `snippet` are not the names.
func TestUpgrade_AIModelAndJSONSample(t *testing.T) {
	src := "CREATE MODEL M.Gpt (Provider: MxCloudGenAI, Key: @M.ApiKey);\n" +
		"list models in M;\nmove model M.Gpt to folder 'AI';\n" +
		"create agent M.A (UsageType: Task, Model: M.Gpt, SystemPrompt: 'x');\n" +
		"create json structure M.J Snippet '{\"a\": 1}';\n" +
		"drop snippet M.S;\n"
	want := "CREATE AI MODEL M.Gpt (Provider: MxCloudGenAI, Key: @M.ApiKey);\n" +
		"list ai models in M;\nmove ai model M.Gpt to folder 'AI';\n" +
		"create agent M.A (UsageType: Task, Model: M.Gpt, SystemPrompt: 'x');\n" +
		"create json structure M.J Sample '{\"a\": 1}';\n" +
		"drop snippet M.S;\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	if res.Rewritten[deprecation.AIModel] != 3 || res.Rewritten[deprecation.JSONStructureSample] != 1 {
		t.Errorf("Rewritten = %v", res.Rewritten)
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}

// R10 (ako/mxcli#755): each `alter app security` clause becomes its property
// list, in the letter case of the script.
func TestUpgrade_AppSecurityClauses(t *testing.T) {
	src := "ALTER APP SECURITY LEVEL PRODUCTION;\nalter app security guest access on role Guest;\n" +
		"alter project security strict mode off;\nalter app security demo users on;\n" +
		"alter app security guest access off;\n"
	want := "ALTER APP SECURITY ( SecurityLevel: PRODUCTION );\n" +
		"alter app security ( EnableGuestAccess: true, GuestUserRole: Guest );\n" +
		"alter app security ( StrictMode: false );\nalter app security ( EnableDemoUsers: true );\n" +
		"alter app security ( EnableGuestAccess: false );\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	if res.Rewritten[deprecation.AppSecurityClause] != 5 {
		t.Errorf("Rewritten = %v", res.Rewritten)
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}

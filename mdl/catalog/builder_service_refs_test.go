// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"reflect"
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// A published OData service names microflows the platform runs for it — a
// resource's read microflow (`ReadMode: microflow X`), its insert/update/delete
// microflows, a microflow published as an OData action, and the custom
// authentication microflow (`Authentication: (microflow X)`). The catalog
// recorded none of them as an edge, so `mxcli refs` said "no references found",
// `impact` agreed, and QUAL004 advised "Remove if unused" on the read
// implementations behind six published services and the one microflow all six
// authenticate through — 35 of one backend's 43 findings (ako/mxcli#1639).
//
// The same gap, same mechanism — the builder holds the name and drops it — was
// open on three siblings: a published REST service's authentication microflow,
// a business event service's handler microflow, and a consumed OData service's
// configuration / headers / error-handling microflows.

// serviceRefsFixture runs the real collecting halves — the service builders —
// and the real emitting half over the given services, and returns the catalog.
func serviceRefsFixture(t *testing.T, r *mock.MockBackend) *Catalog {
	t.Helper()
	cat, err := New()
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}
	t.Cleanup(func() { cat.Close() })

	tx, err := cat.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// Every service list the builders read; the ones a test does not set are
	// empty rather than the mock's "not configured" error.
	if r.ListPublishedODataServicesFunc == nil {
		r.ListPublishedODataServicesFunc = func() ([]*model.PublishedODataService, error) { return nil, nil }
	}
	if r.ListPublishedRestServicesFunc == nil {
		r.ListPublishedRestServicesFunc = func() ([]*model.PublishedRestService, error) { return nil, nil }
	}
	if r.ListBusinessEventServicesFunc == nil {
		r.ListBusinessEventServicesFunc = func() ([]*model.BusinessEventService, error) { return nil, nil }
	}
	if r.ListConsumedODataServicesFunc == nil {
		r.ListConsumedODataServicesFunc = func() ([]*model.ConsumedODataService, error) { return nil, nil }
	}

	b := &Builder{
		catalog:  cat,
		reader:   r,
		snapshot: &Snapshot{ID: "snap-1"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{restRefsModuleID: true},
			moduleNames:     map[model.ID]string{restRefsModuleID: "Sales"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		tx: wrapTx(tx),
	}
	for _, step := range []struct {
		name  string
		build func() error
	}{
		{"buildODataClients", b.buildODataClients},
		{"buildODataServices", b.buildODataServices},
		{"buildBusinessEventServices", b.buildBusinessEventServices},
		{"buildPublishedRestServices", b.buildPublishedRestServices},
	} {
		if err := step.build(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		t.Fatalf("prepare refs: %v", err)
	}
	projectID, snapshotID := b.snapshotMeta()
	b.extractPublishedRestRefs(stmt, projectID, snapshotID)
	b.extractServiceMicroflowRefs(stmt, projectID, snapshotID)
	stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return cat
}

// edgeRows returns "SourceType SourceName -> TargetName kind" for every refs
// row with the given source type, sorted.
func edgeRows(t *testing.T, cat *Catalog, sourceType string) []string {
	t.Helper()
	var out []string
	for _, row := range queryRows(t, cat, `SELECT SourceName, TargetType, TargetName, RefKind, ModuleName FROM refs WHERE SourceType = '`+sourceType+`'`) {
		out = append(out, row[0].(string)+" -> "+row[1].(string)+" "+row[2].(string)+" "+row[3].(string)+" ["+row[4].(string)+"]")
	}
	sort.Strings(out)
	return out
}

func publishedOData() *model.PublishedODataService {
	svc := &model.PublishedODataService{
		ContainerID:         restRefsModuleID,
		Name:                "LiveAPI",
		AuthenticationTypes: []string{"Microflow"},
		AuthMicroflow:       "Sales.AuthenticateApiClient",
		EntitySets: []*model.PublishedEntitySet{
			{
				ExposedName: "Rows",
				ReadMode:    "CallMicroflow:Sales.Read_Rows",
				InsertMode:  "CallMicroflow:Sales.Insert_Row",
				UpdateMode:  "ChangeFromDatabase",
				DeleteMode:  "NotSupported",
			},
			// The same read microflow behind a second resource is one edge,
			// and a microflow mode with no microflow named is none.
			{ExposedName: "AllRows", ReadMode: "CallMicroflow:Sales.Read_Rows", UpdateMode: "CallMicroflow"},
			// A database-backed resource names no microflow.
			{ExposedName: "Plain", ReadMode: "ReadFromDatabase", InsertMode: "ChangeFromDatabase"},
		},
		Microflows: []*model.PublishedMicroflow{{Microflow: "Sales.OD_DoThing", ExposedName: "DoThing"}},
	}
	svc.ID = "odata-1"
	return svc
}

// TestPublishedODataServiceEmitsPublishEdges is the reported half of
// ako/mxcli#1639: ReadMode and Authentication, plus the change modes and the
// published actions, which are the same property family on the same document.
func TestPublishedODataServiceEmitsPublishEdges(t *testing.T) {
	cat := serviceRefsFixture(t, &mock.MockBackend{
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) {
			return []*model.PublishedODataService{publishedOData()}, nil
		},
	})

	got := edgeRows(t, cat, RefObjectPublishedODataService)
	want := []string{
		"Sales.LiveAPI -> MICROFLOW Sales.AuthenticateApiClient publish [Sales]",
		"Sales.LiveAPI -> MICROFLOW Sales.Insert_Row publish [Sales]",
		"Sales.LiveAPI -> MICROFLOW Sales.OD_DoThing publish [Sales]",
		"Sales.LiveAPI -> MICROFLOW Sales.Read_Rows publish [Sales]",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("published OData edges:\n got %v\nwant %v", got, want)
	}

	// The edge joins back to the service row, so "who runs this microflow" is
	// one join from the service's path and version.
	joined := queryRows(t, cat, `
		SELECT COUNT(*) FROM refs r
		JOIN odata_services_data s ON s.Id = r.SourceId
		WHERE r.SourceType = 'PUBLISHED_ODATA_SERVICE'`)
	if joined[0][0].(int64) != int64(len(want)) {
		t.Errorf("the edges do not join to odata_services_data on SourceId; got %v of %d", joined[0][0], len(want))
	}
}

// TestPublishedRestServiceAuthenticationMicroflowEmitsPublishEdge: the
// operation edges already existed; the service-level authentication microflow,
// which every authenticated request runs, had none.
func TestPublishedRestServiceAuthenticationMicroflowEmitsPublishEdge(t *testing.T) {
	svc := oneService(&model.PublishedRestOperation{HTTPMethod: "GET", Path: "{id}", Microflow: "Sales.GetOrder"})
	svc.AuthenticationTypes = []string{"Microflow"}
	svc.AuthenticationMicroflow = "Sales.AuthenticateRestClient"
	cat := serviceRefsFixture(t, &mock.MockBackend{
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) {
			return []*model.PublishedRestService{svc}, nil
		},
	})

	got := edgeRows(t, cat, RefObjectPublishedRestService)
	want := []string{"Sales.OrdersApi -> MICROFLOW Sales.AuthenticateRestClient publish [Sales]"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("published REST service edges:\n got %v\nwant %v", got, want)
	}
	// CONTROL: the operation edge is still there, under its own source type.
	if ops := edgeRows(t, cat, RefObjectPublishedRestOperation); len(ops) != 1 {
		t.Errorf("control: want the one operation edge beside it, got %v", ops)
	}
}

// TestBusinessEventServiceHandlerEmitsEventEdge: a subscribe operation's
// microflow runs whenever the event arrives. business_events_data recorded it
// as SubscribeMicroflow and no edge followed.
func TestBusinessEventServiceHandlerEmitsEventEdge(t *testing.T) {
	svc := &model.BusinessEventService{
		ContainerID: restRefsModuleID,
		Name:        "CustomerEvents",
		OperationImplementations: []*model.ServiceOperation{
			{MessageName: "CustomerChanged", Operation: "subscribe", Entity: "Sales.CustomerChanged", Microflow: "Sales.OnCustomerChanged"},
			// A publish operation binds an entity and no microflow.
			{MessageName: "OrderPlaced", Operation: "publish", Entity: "Sales.OrderPlaced"},
		},
	}
	svc.ID = "bes-1"
	cat := serviceRefsFixture(t, &mock.MockBackend{
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{svc}, nil
		},
	})

	got := edgeRows(t, cat, RefObjectBusinessEventService)
	want := []string{"Sales.CustomerEvents -> MICROFLOW Sales.OnCustomerChanged event [Sales]"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("business event service edges:\n got %v\nwant %v", got, want)
	}
}

// TestConsumedODataServiceMicroflowsEmitCallEdges: the runtime calls a consumed
// OData service's configuration, headers and error-handling microflows on every
// request it makes through the service.
func TestConsumedODataServiceMicroflowsEmitCallEdges(t *testing.T) {
	svc := &model.ConsumedODataService{
		ContainerID:            restRefsModuleID,
		Name:                   "TripPin",
		ConfigurationMicroflow: "Sales.ConfigureTripPin",
		HeadersMicroflow:       "Sales.TripPinHeaders",
		ErrorHandlingMicroflow: "Sales.TripPinErrors",
	}
	svc.ID = "codata-1"
	cat := serviceRefsFixture(t, &mock.MockBackend{
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
	})

	got := edgeRows(t, cat, RefObjectConsumedODataService)
	want := []string{
		"Sales.TripPin -> MICROFLOW Sales.ConfigureTripPin call [Sales]",
		"Sales.TripPin -> MICROFLOW Sales.TripPinErrors call [Sales]",
		"Sales.TripPin -> MICROFLOW Sales.TripPinHeaders call [Sales]",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("consumed OData service edges:\n got %v\nwant %v", got, want)
	}
}

// TestServiceMicroflowsAreNotDead is the consumer-facing symptom.
// GRAPH_DEAD_ASSETS is kind-agnostic, as are `refs` and (since ako/mxcli#1484)
// QUAL004, so the edge alone clears all three. The unreferenced microflow is the
// control: without it, a fix that emptied the view would pass.
func TestServiceMicroflowsAreNotDead(t *testing.T) {
	cat := serviceRefsFixture(t, &mock.MockBackend{
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) {
			return []*model.PublishedODataService{publishedOData()}, nil
		},
	})
	for _, mf := range []string{"Read_Rows", "AuthenticateApiClient", "Orphan"} {
		if _, err := cat.db.Exec(
			`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType)
			 VALUES (?, ?, ?, 'Sales', 'MICROFLOW')`,
			"mf-"+mf, mf, "Sales."+mf); err != nil {
			t.Fatalf("insert microflow: %v", err)
		}
	}

	dead := map[string]bool{}
	for _, row := range queryRows(t, cat, `SELECT QualifiedName FROM graph_dead_assets`) {
		dead[row[0].(string)] = true
	}
	if !dead["Sales.Orphan"] {
		t.Fatal("control failed: a microflow nothing references is not reported dead, " +
			"so this test cannot detect the bug")
	}
	for _, live := range []string{"Sales.Read_Rows", "Sales.AuthenticateApiClient"} {
		if dead[live] {
			t.Errorf("%s runs behind a published OData service and is reported dead (ako/mxcli#1639)", live)
		}
	}
}

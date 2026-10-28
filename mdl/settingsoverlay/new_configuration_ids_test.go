// SPDX-License-Identifier: Apache-2.0

package settingsoverlay

import (
	"strconv"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
)

// collectIDs returns every $ID in a stored document tree, with the path it sits at.
func collectIDs(v any, path string, into map[string]any) {
	if m, ok := AsMap(v); ok {
		for k, child := range m {
			if k == "$ID" {
				into[path] = child
				continue
			}
			collectIDs(child, path+"/"+k, into)
		}
		return
	}
	if arr, ok := v.(bson.A); ok {
		for i, el := range arr {
			collectIDs(el, path+"/"+strconv.Itoa(i), into)
		}
	}
}

// A new configuration copies a sibling as its shape template. On Mendix 11.12+
// that sibling carries a nested OpenTelemetry part whose elements have $IDs of
// their own; copying them made the unit hold the same id twice, and the write
// path refused it ("element id(s) are used more than once") — so `create
// configuration` failed on every 11.12+ project whose configuration had one.
func TestConfigurations_NewConfigurationMintsFreshNestedIDs(t *testing.T) {
	otel := func() bson.M {
		return bson.M{
			"$ID":      "otel",
			"$Type":    "Settings$OpenTelemetryConfiguration",
			"Enabled":  false,
			"Endpoint": "http://localhost:4318",
			"Logs":     bson.M{"$ID": "otel-logs", "$Type": "Settings$OpenTelemetryLogsConfiguration", "Enabled": true},
			"Traces":   bson.M{"$ID": "otel-traces", "$Type": "Settings$OpenTelemetryTracesConfiguration", "Enabled": true},
		}
	}
	part := map[string]any{
		"Configurations": bson.A{
			int32(3),
			bson.M{"$ID": "id-default", "Name": "Default", "OpenTelemetry": otel()},
		},
	}
	cs := &model.ConfigurationSettings{Configurations: []*model.ServerConfiguration{
		{Name: "Default"},
		{Name: "Staging"},
	}}

	cfgs := ArrayElements(Configurations(cs, part)["Configurations"])
	if len(cfgs) != 2 {
		t.Fatalf("got %d configurations, want 2", len(cfgs))
	}
	seen := map[any]string{}
	for _, c := range cfgs {
		ids := map[string]any{}
		collectIDs(c, c["Name"].(string), ids)
		for path, id := range ids {
			key := idKey(id)
			if other, dup := seen[key]; dup {
				t.Errorf("$ID used twice: %s and %s", other, path)
			}
			seen[key] = path
		}
	}

	added, stored := cfgs[1], cfgs[0]
	if added["Name"] != "Staging" || stored["Name"] != "Default" {
		t.Fatalf("unexpected order: %v, %v", added["Name"], stored["Name"])
	}
	// The values are still inherited — only the identities are new.
	o, ok := AsMap(added["OpenTelemetry"])
	if !ok || o["Endpoint"] != "http://localhost:4318" || o["$Type"] != "Settings$OpenTelemetryConfiguration" {
		t.Errorf("OpenTelemetry not inherited from the sibling: %#v", added["OpenTelemetry"])
	}
	if logs, ok := AsMap(o["Logs"]); !ok || logs["Enabled"] != true {
		t.Errorf("OpenTelemetry.Logs not inherited: %#v", o["Logs"])
	}
	// And the sibling used as the template keeps its own.
	so, _ := AsMap(stored["OpenTelemetry"])
	if so["$ID"] != "otel" {
		t.Errorf("template sibling's OpenTelemetry $ID changed to %#v", so["$ID"])
	}
	if sl, _ := AsMap(so["Logs"]); sl["$ID"] != "otel-logs" {
		t.Errorf("template sibling's Logs $ID changed to %#v", sl["$ID"])
	}
}

// idKey makes an $ID comparable whatever its stored form.
func idKey(id any) any {
	if b, ok := id.(bson.Binary); ok {
		return string(b.Data)
	}
	return id
}

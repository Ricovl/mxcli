// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// serviceMicroflowRef is one edge from an integration service document to a
// microflow the platform runs on the service's behalf. The source is the
// service itself (its catalog id and qualified name), so the edge joins back to
// the service's own table — odata_services_data, published_rest_services_data,
// business_event_services_data or odata_clients_data — on SourceId.
//
// Collected while the service builders catalogue the services, and emitted by
// buildReferences, like scheduledEventRefs and publishedRestRefs.
type serviceMicroflowRef struct {
	sourceType, sourceID, sourceName, moduleName, microflow, kind string
}

// addServiceMicroflowRefs records one edge per distinct microflow the service
// names. An empty name is a real shape (a mode or a method chosen before its
// microflow is) and is skipped: an edge to "" would collide with every other
// unnamed target. A microflow named twice by one service — the same read
// microflow behind two resources — is one edge, as a page's widgets are.
func (b *Builder) addServiceMicroflowRefs(sourceType string, sourceID model.ID, sourceName, moduleName, kind string, microflows ...string) {
	seen := map[string]bool{}
	for _, mf := range microflows {
		mf = strings.TrimSpace(mf)
		if mf == "" || seen[mf] {
			continue
		}
		seen[mf] = true
		b.serviceMicroflowRefs = append(b.serviceMicroflowRefs, serviceMicroflowRef{
			sourceType: sourceType,
			sourceID:   string(sourceID),
			sourceName: sourceName,
			moduleName: moduleName,
			microflow:  mf,
			kind:       kind,
		})
	}
}

// publishedODataMicroflows returns every microflow a published OData service
// runs: the custom authentication microflow, each resource's read and
// insert/update/delete microflows, and each microflow published as an action.
func publishedODataMicroflows(svc *model.PublishedODataService) []string {
	out := []string{svc.AuthMicroflow}
	for _, es := range svc.EntitySets {
		for _, mode := range []string{es.ReadMode, es.InsertMode, es.UpdateMode, es.DeleteMode} {
			out = append(out, odataModeMicroflow(mode))
		}
	}
	for _, pm := range svc.Microflows {
		out = append(out, pm.Microflow)
	}
	return out
}

// odataModeMicroflow returns the microflow of a published resource's read or
// change mode, or "" when the mode names none. The reader spells a
// microflow-backed mode "CallMicroflow:Module.Name" (parseODataModeRaw), and a
// database-backed or unsupported one by a bare word.
func odataModeMicroflow(mode string) string {
	if mf, ok := strings.CutPrefix(mode, "CallMicroflow:"); ok {
		return mf
	}
	return ""
}

// extractServiceMicroflowRefs emits the edges the service builders collected.
func (b *Builder) extractServiceMicroflowRefs(stmt rowStmt, projectID, snapshotID string) int {
	count := 0
	for _, r := range b.serviceMicroflowRefs {
		b.unit = r.sourceID // the service document is its unit
		if _, err := stmt.Exec(
			r.sourceType, r.sourceID, r.sourceName,
			RefObjectMicroflow, "", r.microflow,
			r.kind, r.moduleName, projectID, snapshotID,
		); err == nil {
			count++
		}
	}
	return count
}

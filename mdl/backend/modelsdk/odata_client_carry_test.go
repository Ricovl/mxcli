// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
)

// Running `create or modify consumed odata service` over a Studio Pro-authored
// client (ako/TestApp: Clients.OrderODataClient, Odata.Bug1073, Mendix 11.14.0)
// rewrote UseQuerySegment true -> false, deleted the Icon, the catalog, proxy and
// microflow keys Studio Pro stores empty, the HttpConfiguration's
// CustomLocationTemplate, and re-marked the empty lists (#743). The model holds
// none of these, so the rewrite carries them from the stored document.

// storedStudioProClient is Clients.OrderODataClient as ako/TestApp stores it,
// less its $metadata.
func storedStudioProClient(t *testing.T) []byte {
	t.Helper()
	id := func(b byte) bson.Binary {
		return bson.Binary{Subtype: 0, Data: []byte{b, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}}
	}
	doc := bson.D{
		{Key: "$ID", Value: id(1)},
		{Key: "$Type", Value: "Rest$ConsumedODataService"},
		{Key: "ApplicationId", Value: ""},
		{Key: "CatalogUrl", Value: ""},
		{Key: "ConfigurationEntityMicroflow", Value: ""},
		{Key: "Description", Value: ""},
		{Key: "Documentation", Value: ""},
		{Key: "EndpointId", Value: ""},
		{Key: "EnvironmentType", Value: "Unknown"},
		{Key: "ErrorHandlingMicroflow", Value: "Clients.HandleError"},
		{Key: "Excluded", Value: false},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "HeaderListMicroflow", Value: ""},
		{Key: "HttpConfiguration", Value: bson.D{
			{Key: "$ID", Value: id(2)},
			{Key: "$Type", Value: "Microflows$HttpConfiguration"},
			{Key: "ClientCertificate", Value: ""},
			{Key: "CustomLocation", Value: "@Clients.OrderODataClient_Location"},
			{Key: "CustomLocationTemplate", Value: nil},
			{Key: "HttpAuthenticationPassword", Value: "'1'"},
			{Key: "HttpAuthenticationUserName", Value: "'MxAdmin'"},
			{Key: "HttpHeaderEntries", Value: bson.A{int32(3)}},
			{Key: "HttpMethod", Value: "Post"},
			{Key: "OverrideLocation", Value: false},
			{Key: "UseHttpAuthentication", Value: true},
		}},
		{Key: "Icon", Value: bson.Binary{Subtype: 0, Data: []byte{0x89, 'P', 'N', 'G'}}},
		{Key: "LastUpdated", Value: ""},
		{Key: "MetadataHash", Value: "713d"},
		{Key: "MetadataReferences", Value: bson.A{int32(3)}},
		{Key: "MetadataUrl", Value: "file:///tmp/$metadata.xml"},
		{Key: "MinimumMxVersion", Value: ""},
		{Key: "Name", Value: "OrderODataClient"},
		{Key: "ODataVersion", Value: "OData4"},
		{Key: "ProxyHost", Value: ""},
		{Key: "ProxyPassword", Value: ""},
		{Key: "ProxyPort", Value: ""},
		{Key: "ProxyType", Value: "DefaultProxy"},
		{Key: "ProxyUsername", Value: ""},
		{Key: "RecommendedMxVersion", Value: ""},
		{Key: "ServiceName", Value: "OrderODataClient"},
		{Key: "TimeoutExpression", Value: "300"},
		{Key: "UseQuerySegment", Value: true},
		{Key: "Validated", Value: false},
		{Key: "ValidatedEntities", Value: bson.A{int32(1)}},
		{Key: "Version", Value: ""},
	}
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// readStoredClient is the model the reader makes of that document.
func readStoredClient() *model.ConsumedODataService {
	return &model.ConsumedODataService{
		BaseElement:            model.BaseElement{ID: "01000000-0000-0000-0000-000000000000"},
		Name:                   "OrderODataClient",
		ServiceName:            "OrderODataClient",
		ODataVersion:           "OData4",
		MetadataUrl:            "file:///tmp/$metadata.xml",
		MetadataHash:           "713d",
		TimeoutExpression:      "300",
		ProxyType:              "DefaultProxy",
		EnvironmentType:        "Unknown",
		ErrorHandlingMicroflow: "Clients.HandleError",
		HttpConfiguration: &model.HttpConfiguration{
			UseAuthentication: true,
			Username:          "'MxAdmin'",
			Password:          "'1'",
			HttpMethod:        "Post",
			CustomLocation:    "@Clients.OrderODataClient_Location",
		},
	}
}

func rewriteClient(t *testing.T, svc *model.ConsumedODataService) bson.Raw {
	t.Helper()
	fresh, err := (&codec.Encoder{}).Encode(consumedODataServiceToGen(svc, "ConfigurationEntityMicroflow", "HeaderListMicroflow"))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := carryStoredConsumedODataService(storedStudioProClient(t), fresh,
		consumedODataOptionalKeys("ConfigurationEntityMicroflow", "HeaderListMicroflow"))
	if err != nil {
		t.Fatalf("carry: %v", err)
	}
	return out
}

func TestConsumedODataServiceRewrite_CarriesWhatTheModelDoesNotHold(t *testing.T) {
	got := rewriteClient(t, readStoredClient())
	stored := bson.Raw(storedStudioProClient(t))
	for _, path := range [][]string{
		{"UseQuerySegment"}, {"Icon"}, {"ExportLevel"},
		{"ApplicationId"}, {"EndpointId"}, {"CatalogUrl"}, {"ConfigurationEntityMicroflow"}, {"HeaderListMicroflow"},
		{"ProxyHost"}, {"ProxyPort"}, {"ProxyUsername"}, {"ProxyPassword"},
		{"MetadataReferences"}, {"ValidatedEntities"},
		{"HttpConfiguration", "CustomLocationTemplate"}, {"HttpConfiguration", "HttpHeaderEntries"},
	} {
		want := stored.Lookup(path...)
		v, err := got.LookupErr(path...)
		if err != nil {
			t.Errorf("%v: dropped (stored %v)", path, want)
			continue
		}
		if !v.Equal(want) {
			t.Errorf("%v = %v, stored %v", path, v, want)
		}
	}
}

// What the model does hold is the statement's: a changed value, a cleared
// microflow and a declared header are written, not the stored ones.
func TestConsumedODataServiceRewrite_ModelValuesWin(t *testing.T) {
	svc := readStoredClient()
	svc.TimeoutExpression = "60"
	svc.ErrorHandlingMicroflow = ""
	svc.HttpConfiguration.HeaderEntries = []*model.HttpHeaderEntry{{Key: "X-Trace", Value: "'1'"}}
	got := rewriteClient(t, svc)

	if s, _ := got.Lookup("TimeoutExpression").StringValueOK(); s != "60" {
		t.Errorf("TimeoutExpression = %q, want the declared 60", s)
	}
	if s, ok := got.Lookup("ErrorHandlingMicroflow").StringValueOK(); !ok || s != "" {
		t.Errorf("a cleared ErrorHandlingMicroflow = %v, want \"\"", got.Lookup("ErrorHandlingMicroflow"))
	}
	vals, err := got.Lookup("HttpConfiguration", "HttpHeaderEntries").Array().Values()
	if err != nil || len(vals) != 2 {
		t.Fatalf("HttpHeaderEntries = %v, want [3, entry]", got.Lookup("HttpConfiguration", "HttpHeaderEntries"))
	}
	if m, _ := vals[0].Int32OK(); m != 3 {
		t.Errorf("header marker = %v, want 3", vals[0])
	}
	if k, _ := vals[1].Document().Lookup("Key").StringValueOK(); k != "X-Trace" {
		t.Errorf("header = %v, want X-Trace", vals[1])
	}
}

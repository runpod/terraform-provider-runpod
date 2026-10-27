package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	runpod "github.com/runpod/go-sdk/v2"
)

func TestConfigureSendsTheProviderUserAgentToEveryHost(t *testing.T) {
	// The v2 client and the deprecated GraphQL shim must identify the provider
	// and the Terraform release: 1.x sent Terraform/RunPodProvider to the
	// legacy hosts, and the plain SDK string says nothing about Terraform at
	// all. The shim keeps the 1.x value first, so filters that match it still
	// match.
	ctx := context.Background()

	var mu sync.Mutex
	agents := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		agents[r.URL.Path] = r.Header.Get("User-Agent")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	p := New("2.0.0-test")()
	var schemaResp provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("provider schema type = %T, want an object", schemaResp.Schema.Type().TerraformType(ctx))
	}
	raw := providerConfig(t, objectType, map[string]tftypes.Value{
		"api_key":     tftypes.NewValue(tftypes.String, "test-key"),
		"base_url":    tftypes.NewValue(tftypes.String, srv.URL),
		"graphql_url": tftypes.NewValue(tftypes.String, srv.URL+"/graphql"),
		"rest_v1_url": tftypes.NewValue(tftypes.String, srv.URL+"/v1"),
	})

	var configureResp provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{
		TerraformVersion: "1.16.3",
		Config:           tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
	}, &configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatal(configureResp.Diagnostics)
	}
	configured, ok := configureResp.ResourceData.(*clients)
	if !ok {
		t.Fatalf("provider data = %T, want *clients", configureResp.ResourceData)
	}

	resp, err := configured.API.Account.GetSshKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if _, err := configured.Legacy.Query(ctx, "query { myself { id } }", nil); err != nil {
		t.Fatal(err)
	}

	v2 := "terraform-provider-runpod/2.0.0-test (terraform 1.16.3; " + runtime.Version() + ") runpod-go-sdk/" + runpod.Version
	shim := "Terraform/RunPodProvider " + v2
	mu.Lock()
	defer mu.Unlock()
	// rest_v1_url is set above and must route nothing: the provider has no
	// REST v1 client.
	for route := range agents {
		if strings.HasPrefix(route, "/v1") {
			t.Errorf("a request reached the REST v1 path %s", route)
		}
	}
	for _, tc := range []struct{ route, host, want string }{
		{"/v2/account/ssh-keys", "REST v2", v2},
		{"/graphql", "GraphQL", shim},
	} {
		if got, called := agents[tc.route]; !called {
			t.Errorf("%s: %s was never called", tc.host, tc.route)
		} else if got != tc.want {
			t.Errorf("%s User-Agent = %q, want %q", tc.host, got, tc.want)
		}
	}
}

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// newTestLegacyClient builds a shim client for a test, failing the test rather
// than the assertion when the configuration is rejected.
func newTestLegacyClient(t *testing.T, config legacy.Config) *legacy.Client {
	t.Helper()
	client, err := legacy.New(config)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestStringOrEnvPrefersConfigurationThenEachVariableInOrder(t *testing.T) {
	t.Setenv("RUNPOD_TEST_FIRST", "first")
	t.Setenv("RUNPOD_TEST_SECOND", "second")

	if got := stringOrEnv(types.StringValue("configured"), "fallback", "RUNPOD_TEST_FIRST"); got != "configured" {
		t.Errorf("configured value = %q, want it to win over the environment", got)
	}
	if got := stringOrEnv(types.StringNull(), "fallback", "RUNPOD_TEST_FIRST", "RUNPOD_TEST_SECOND"); got != "first" {
		t.Errorf("= %q, want the first variable that is set", got)
	}
	if got := stringOrEnv(types.StringUnknown(), "fallback", "RUNPOD_TEST_UNSET", "RUNPOD_TEST_SECOND"); got != "second" {
		t.Errorf("= %q, want the next variable when the first is unset", got)
	}
	if got := stringOrEnv(types.StringNull(), "fallback", "RUNPOD_TEST_UNSET"); got != "fallback" {
		t.Errorf("= %q, want the fallback when nothing is set", got)
	}

	t.Setenv("RUNPOD_TEST_FIRST", "")
	if got := stringOrEnv(types.StringNull(), "fallback", "RUNPOD_TEST_FIRST", "RUNPOD_TEST_SECOND"); got != "second" {
		t.Errorf("= %q, want an empty variable skipped", got)
	}
}

func TestNewLegacyClientHonoursTheRestV1EnvironmentChain(t *testing.T) {
	// RUNPOD_BASE_URL is the 1.x name for the REST v1 host: a 1.x
	// configuration pinned to staging with it must not fall through to
	// production. `task test` loads .env, so clear the higher-priority
	// variable rather than assuming the environment is empty.
	t.Setenv("RUNPOD_API_KEY", "test-key")
	t.Setenv("RUNPOD_REST_V1_URL", "")
	t.Setenv("RUNPOD_BASE_URL", "https://staging.example.com/v1")
	if got := legacyClient(t, runpodProviderModel{}).RestV1URL(); got != "https://staging.example.com/v1" {
		t.Errorf("rest v1 url = %q, want the RUNPOD_BASE_URL host", got)
	}

	t.Setenv("RUNPOD_REST_V1_URL", "https://rest-v1.example.com/v1")
	if got := legacyClient(t, runpodProviderModel{}).RestV1URL(); got != "https://rest-v1.example.com/v1" {
		t.Errorf("rest v1 url = %q, want RUNPOD_REST_V1_URL to win over RUNPOD_BASE_URL", got)
	}

	configured := legacyClient(t, runpodProviderModel{RestV1URL: types.StringValue("https://pinned.example.com/v1")})
	if got := configured.RestV1URL(); got != "https://pinned.example.com/v1" {
		t.Errorf("rest v1 url = %q, want the provider attribute to win over both variables", got)
	}
}

// legacyClient runs newLegacyClient for a test that is asserting on the
// resolved endpoints, not on the constructor's error.
func legacyClient(t *testing.T, config runpodProviderModel) *legacy.Client {
	t.Helper()
	client, err := newLegacyClient(config, userAgent("test", "1.16.3"))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNewLegacyClientFallsBackToProductionEndpoints(t *testing.T) {
	t.Setenv("RUNPOD_API_KEY", "test-key")
	t.Setenv("RUNPOD_REST_V1_URL", "")
	t.Setenv("RUNPOD_BASE_URL", "")
	t.Setenv("RUNPOD_GRAPHQL_URL", "")

	client := legacyClient(t, runpodProviderModel{})
	if client.RestV1URL() != legacy.DefaultRestV1URL {
		t.Errorf("rest v1 url = %q, want %q", client.RestV1URL(), legacy.DefaultRestV1URL)
	}
	if client.GraphQLURL() != legacy.DefaultGraphQLURL {
		t.Errorf("graphql url = %q, want %q", client.GraphQLURL(), legacy.DefaultGraphQLURL)
	}
}

// providerConfig builds a provider configuration where every attribute is null
// except the ones named, so the test survives a new provider attribute.
func providerConfig(t *testing.T, objectType tftypes.Object, set map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	values := map[string]tftypes.Value{}
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	for name, value := range set {
		if _, ok := values[name]; !ok {
			t.Fatalf("provider schema has no %q attribute", name)
		}
		values[name] = value
	}
	return tftypes.NewValue(objectType, values)
}

func TestConfigureAcceptsAnUnknownApiKeyAndUsesTheEnvironment(t *testing.T) {
	// api_key = var.key is unknown at plan time. ValueString() on an unknown
	// yields "", which as WithAPIKey("") would override the environment and
	// fail the client with "API key required".
	ctx := context.Background()
	t.Setenv("RUNPOD_API_KEY", "env-key")

	p := New("test")()
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
		"api_key": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})

	var configureResp provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
	}, &configureResp)

	if configureResp.Diagnostics.HasError() {
		t.Fatalf("Configure failed on an unknown api_key: %v", configureResp.Diagnostics)
	}
	configured, ok := configureResp.ResourceData.(*clients)
	if !ok {
		t.Fatalf("provider data = %T, want *clients", configureResp.ResourceData)
	}
	if configured.API == nil || configured.Legacy == nil {
		t.Fatalf("provider data carries API=%v Legacy=%v, want both", configured.API, configured.Legacy)
	}
}

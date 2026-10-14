package provider

import (
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// Plumbing for the deprecated compatibility shims: runpod_machine and
// runpod_user on GraphQL, and the three runpod_billing_*
// data sources and the deprecated attributes on runpod_pod, runpod_template
// and runpod_serverless_endpoint on REST v1. They are the only things in this
// provider that do not go through the generated v2 SDK, because their routes
// do not exist in v2 yet. Everything in this file goes away with them.

const machineDeprecation = "Machines have no REST v2 route yet; this calls the GraphQL API " +
	"and will be removed when a /v2/machines surface exists."

const userDeprecation = "There is no REST v2 account identity route yet; this data source reads the " +
	"account through the GraphQL myself query (REST v1 /user is gone from the v1 host) and will be " +
	"removed when a /v2 account surface exists. Only id and email have a GraphQL source; the other " +
	"attributes read null."

// legacyUserAgentPrefix is the whole User-Agent 1.x sent to these hosts. The
// shims lead with it, so filters on the retiring hosts that match it still
// see 2.0.0, and follow it with the v2 client's User-Agent, so 2.0.0 is not
// mistaken for 1.x.
const legacyUserAgentPrefix = "Terraform/RunPodProvider"

// newLegacyClient resolves the legacy endpoints from the provider
// configuration, then the documented environment variables in order, then the
// production defaults.
//
// RUNPOD_BASE_URL is the 1.x name for the REST v1 host and is still honoured
// after RUNPOD_REST_V1_URL: a 1.x configuration pinned to a staging REST v1
// host set that variable, and dropping it would silently send those reads to
// production. It does not apply to the v2 client, which reads
// RUNPOD_API_BASE_URL.
func newLegacyClient(config runpodProviderModel, userAgent string) (*legacy.Client, error) {
	return legacy.New(legacy.Config{
		APIKey:     stringOrEnv(config.APIKey, "", "RUNPOD_API_KEY"),
		GraphQLURL: stringOrEnv(config.GraphQLURL, legacy.DefaultGraphQLURL, "RUNPOD_GRAPHQL_URL"),
		RestV1URL:  stringOrEnv(config.RestV1URL, legacy.DefaultRestV1URL, "RUNPOD_REST_V1_URL", "RUNPOD_BASE_URL"),
		UserAgent:  legacyUserAgentPrefix + " " + userAgent,
	})
}

// stringOrEnv returns the configured attribute, else the first environment
// variable that is set and non-empty, else fallback.
func stringOrEnv(attribute types.String, fallback string, variables ...string) string {
	if !attribute.IsNull() && !attribute.IsUnknown() {
		return attribute.ValueString()
	}
	for _, variable := range variables {
		if value := os.Getenv(variable); value != "" {
			return value
		}
	}
	return fallback
}

// configuredLegacyClient is configuredClient's counterpart for the shims; the
// nil-provider-data case is documented there.
func configuredLegacyClient(providerData any, diags *diag.Diagnostics) *legacy.Client {
	if providerData == nil {
		return nil
	}
	configured, ok := providerData.(*clients)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *clients, got %T", providerData))
		return nil
	}
	return configured.Legacy
}

// GraphQL and REST v1 have no generated types, so responses arrive as
// map[string]any. These read one field into a framework value: absent, null,
// or unexpectedly typed fields become null rather than an error, the same
// tolerance types.StringPointerValue gives the v2 mappings.
//
// The Or variants keep a planned value the response does not carry, which an
// optional-and-computed attribute needs: a machine only accepts `name` on
// create, so every other configured value would otherwise be dropped. An
// unknown plan value still has to become null: state cannot hold unknown.

func jsonString(object map[string]any, key string) types.String {
	return jsonStringOr(object, key, types.StringNull())
}

func jsonStringOr(object map[string]any, key string, planned types.String) types.String {
	if value, ok := object[key].(string); ok {
		return types.StringValue(value)
	}
	if planned.IsUnknown() {
		return types.StringNull()
	}
	return planned
}

func jsonInt64(object map[string]any, key string) types.Int64 {
	return jsonInt64Or(object, key, types.Int64Null())
}

func jsonInt64Or(object map[string]any, key string, planned types.Int64) types.Int64 {
	// encoding/json decodes every JSON number into float64.
	if value, ok := object[key].(float64); ok {
		return types.Int64Value(int64(value))
	}
	if planned.IsUnknown() {
		return types.Int64Null()
	}
	return planned
}

func jsonFloat64(object map[string]any, key string) types.Float64 {
	return jsonFloat64Or(object, key, types.Float64Null())
}

func jsonFloat64Or(object map[string]any, key string, planned types.Float64) types.Float64 {
	if value, ok := object[key].(float64); ok {
		return types.Float64Value(value)
	}
	if planned.IsUnknown() {
		return types.Float64Null()
	}
	return planned
}

func jsonBool(object map[string]any, key string) types.Bool {
	return jsonBoolOr(object, key, types.BoolNull())
}

func jsonBoolOr(object map[string]any, key string, planned types.Bool) types.Bool {
	if value, ok := object[key].(bool); ok {
		return types.BoolValue(value)
	}
	if planned.IsUnknown() {
		return types.BoolNull()
	}
	return planned
}

// jsonNestedString reads a field out of a nested object, e.g. gpuType { id }.
func jsonNestedString(object map[string]any, key, nestedKey string) types.String {
	return jsonNestedStringOr(object, key, nestedKey, types.StringNull())
}

func jsonNestedStringOr(object map[string]any, key, nestedKey string, planned types.String) types.String {
	nested, ok := object[key].(map[string]any)
	if !ok {
		if planned.IsUnknown() {
			return types.StringNull()
		}
		return planned
	}
	return jsonStringOr(nested, nestedKey, planned)
}

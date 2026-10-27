package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	runpod "github.com/runpod/go-sdk/v2"
)

// Helpers for the shim apply tests, which drive the resources' real Create
// and Update rather than their mapping helpers, because what they pin down is
// the order of the calls: which model the shim write is built from, and
// whether the state is written before a shim failure is reported. The tests
// themselves live with the resource they exercise, in
// legacy_shim_apply_<resource>_test.go.
//
// One httptest server stands in for both hosts: the v2 SDK asks for /v2/…
// and the GraphQL shim posts to its own path, so a single handler sees the
// whole sequence.

func shimTestClient(t *testing.T, url string) *runpod.Client {
	t.Helper()
	client, err := runpod.New(runpod.WithAPIKey("test-key"), runpod.WithBaseURL(url))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// shimResourceSchema constructs a resource's schema and the all-null object
// value plan, config and state start from, the way providerConfig builds a
// provider configuration in legacy_client_test.go.
func shimResourceSchema(ctx context.Context, t *testing.T, r resource.Resource) (resourceschema.Schema, tftypes.Value) {
	t.Helper()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}

	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type = %T, want an object", schemaResp.Schema.Type().TerraformType(ctx))
	}
	values := map[string]tftypes.Value{}
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	return schemaResp.Schema, tftypes.NewValue(objectType, values)
}

// shimObject fills the named attributes into a copy of the all-null object.
func shimObject(ctx context.Context, t *testing.T, s resourceschema.Schema, raw tftypes.Value, set map[string]attr.Value) tftypes.Value {
	t.Helper()
	data := tfsdk.State{Schema: s, Raw: raw}
	for name, value := range set {
		if diags := data.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("setting %s: %v", name, diags)
		}
	}
	return data.Raw
}

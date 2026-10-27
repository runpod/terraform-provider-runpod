package provider

import (
	"context"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// Schema coverage: every resource and data source the provider registers is
// constructed here and put through the framework's own implementation
// validation, which is otherwise only reached when Terraform starts the
// provider. The description assertions keep the generated registry pages from
// listing an undocumented attribute.

// describedAttribute is the part of the framework's attribute interface these
// assertions need; resource and data-source attributes both satisfy it.
type describedAttribute interface {
	GetDescription() string
}

func TestProviderMetadataAndSchema(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	var metaResp provider.MetadataResponse
	p.Metadata(ctx, provider.MetadataRequest{}, &metaResp)
	if metaResp.TypeName != "runpod" {
		t.Fatalf("type name = %q, want runpod", metaResp.TypeName)
	}

	var schemaResp provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	apiKey, ok := schemaResp.Schema.Attributes["api_key"]
	if !ok {
		t.Fatal("provider schema has no api_key attribute")
	}
	if !apiKey.IsSensitive() {
		t.Fatal("api_key must be sensitive")
	}

	described := map[string]describedAttribute{}
	for name, attribute := range schemaResp.Schema.Attributes {
		described[name] = attribute
	}
	assertAttributesDescribed(t, "provider", described)
}

func TestResourceSchemas(t *testing.T) {
	ctx := context.Background()
	want := []string{
		"runpod_account_ssh_keys",
		"runpod_cluster",
		"runpod_global_volume",
		"runpod_machine",
		"runpod_network_volume",
		"runpod_pod",
		"runpod_pod_action",
		"runpod_registry",
		"runpod_registry_delegation",
		"runpod_serverless_endpoint",
		"runpod_template",
	}

	var got []string
	for _, newResource := range New("test")().Resources(ctx) {
		r := newResource()

		var metaResp resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "runpod"}, &metaResp)
		got = append(got, metaResp.TypeName)

		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
		if schemaResp.Diagnostics.HasError() {
			t.Fatalf("%s: schema: %v", metaResp.TypeName, schemaResp.Diagnostics)
		}
		if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Fatalf("%s: invalid schema implementation: %v", metaResp.TypeName, diags)
		}
		if schemaResp.Schema.GetDescription() == "" {
			t.Errorf("%s: schema has no Description", metaResp.TypeName)
		}

		described := map[string]describedAttribute{}
		for name, attribute := range schemaResp.Schema.Attributes {
			described[name] = attribute
			switch nested := attribute.(type) {
			case resourceschema.ListNestedAttribute:
				for nestedName, nestedAttribute := range nested.NestedObject.Attributes {
					described[name+"."+nestedName] = nestedAttribute
				}
			case resourceschema.SingleNestedAttribute:
				for nestedName, nestedAttribute := range nested.Attributes {
					described[name+"."+nestedName] = nestedAttribute
				}
			}
		}
		assertAttributesDescribed(t, metaResp.TypeName, described)
	}

	assertTypeNames(t, "resources", got, want)
}

func TestDataSourceSchemas(t *testing.T) {
	ctx := context.Background()
	want := []string{
		"runpod_billing",
		"runpod_billing_endpoint",
		"runpod_billing_network_volume",
		"runpod_billing_pod",
		"runpod_catalog_templates",
		"runpod_cluster",
		"runpod_cluster_billing",
		"runpod_cluster_pods",
		"runpod_clusters",
		"runpod_cpu_type",
		"runpod_cpu_types",
		"runpod_data_center",
		"runpod_data_centers",
		"runpod_endpoint_billing",
		"runpod_endpoint_build",
		"runpod_endpoint_builds",
		"runpod_endpoint_releases",
		"runpod_endpoint_worker_logs",
		"runpod_endpoint_workers",
		"runpod_global_volume",
		"runpod_gpu_type",
		"runpod_gpu_types",
		"runpod_machine",
		"runpod_machines",
		"runpod_network_volume_billing",
		"runpod_network_volumes",
		"runpod_pod",
		"runpod_pod_billing",
		"runpod_pod_logs",
		"runpod_pods",
		"runpod_registries",
		"runpod_registry_delegations",
		"runpod_serverless_billing",
		"runpod_serverless_endpoints",
		"runpod_template",
		"runpod_templates",
		"runpod_user",
	}

	var got []string
	for _, newDataSource := range New("test")().DataSources(ctx) {
		d := newDataSource()

		var metaResp datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "runpod"}, &metaResp)
		got = append(got, metaResp.TypeName)

		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
		if schemaResp.Diagnostics.HasError() {
			t.Fatalf("%s: schema: %v", metaResp.TypeName, schemaResp.Diagnostics)
		}
		if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Fatalf("%s: invalid schema implementation: %v", metaResp.TypeName, diags)
		}
		if schemaResp.Schema.GetDescription() == "" {
			t.Errorf("%s: schema has no Description", metaResp.TypeName)
		}

		described := map[string]describedAttribute{}
		for name, attribute := range schemaResp.Schema.Attributes {
			described[name] = attribute
			switch nested := attribute.(type) {
			case datasourceschema.ListNestedAttribute:
				for nestedName, nestedAttribute := range nested.NestedObject.Attributes {
					described[name+"."+nestedName] = nestedAttribute
				}
			case datasourceschema.SingleNestedAttribute:
				for nestedName, nestedAttribute := range nested.Attributes {
					described[name+"."+nestedName] = nestedAttribute
				}
			}
		}
		assertAttributesDescribed(t, metaResp.TypeName, described)
	}

	assertTypeNames(t, "data sources", got, want)
}

func assertAttributesDescribed(t *testing.T, owner string, attributes map[string]describedAttribute) {
	t.Helper()
	for name, attribute := range attributes {
		if attribute.GetDescription() == "" {
			t.Errorf("%s: attribute %q has no Description, so its registry docs entry would be bare", owner, name)
		}
	}
}

func assertTypeNames(t *testing.T, kind string, got, want []string) {
	t.Helper()
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("%s: registered %d (%v), want %d (%v)", kind, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: registered %v, want %v", kind, got, want)
		}
	}
}

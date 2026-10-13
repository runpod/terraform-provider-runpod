package provider

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

func testEcrDelegation() sdktypes.EcrDelegation {
	name := "my-delegation"
	uri := "123456789012.dkr.ecr.us-east-2.amazonaws.com"
	return sdktypes.EcrDelegation{
		Id:                "deleg_abc123",
		Name:              &name,
		AwsRegion:         "us-east-2",
		AwsUser:           "123456789012",
		Repository:        "runpod/deployment",
		Tag:               "latest",
		DockerRegistryUri: &uri,
		DelegatorUserId:   "user_xyz",
		CreatedAt:         time.Date(2026, 3, 13, 20, 0, 0, 0, time.UTC),
	}
}

func TestFlattenDelegationListItem(t *testing.T) {
	item := flattenDelegationListItem(testEcrDelegation())

	if item.Id.ValueString() != "deleg_abc123" || item.Name.ValueString() != "my-delegation" {
		t.Fatalf("identity: %+v", item)
	}
	if item.AwsRegion.ValueString() != "us-east-2" || item.AwsUser.ValueString() != "123456789012" {
		t.Fatalf("aws: %+v", item)
	}
	if item.Repository.ValueString() != "runpod/deployment" || item.Tag.ValueString() != "latest" {
		t.Fatalf("image: %+v", item)
	}
	if item.DockerRegistryUri.ValueString() != "123456789012.dkr.ecr.us-east-2.amazonaws.com" {
		t.Fatalf("docker_registry_uri = %q", item.DockerRegistryUri.ValueString())
	}
	if item.DelegatorUserId.ValueString() != "user_xyz" {
		t.Fatalf("delegator_user_id = %q", item.DelegatorUserId.ValueString())
	}
	if item.CreatedAt.ValueString() != "2026-03-13T20:00:00Z" {
		t.Fatalf("created_at = %q", item.CreatedAt.ValueString())
	}
}

func TestFlattenDelegationListItemAbsentOptionals(t *testing.T) {
	delegation := testEcrDelegation()
	// Both the name and the formatted registry URI are optional in the
	// response; a delegation created without a name must flatten to null
	// rather than an empty string.
	delegation.Name = nil
	delegation.DockerRegistryUri = nil

	item := flattenDelegationListItem(delegation)
	if !item.Name.IsNull() || !item.DockerRegistryUri.IsNull() {
		t.Fatalf("absent optionals must flatten to null: %+v", item)
	}
	if item.Id.ValueString() != "deleg_abc123" {
		t.Fatalf("id was lost: %+v", item)
	}
}

// The nested attribute set has to cover every field of the list response, or a
// regenerated SDK silently drops data from the data source.
func TestRegistryDelegationsSchemaCoversResponse(t *testing.T) {
	ctx := context.Background()
	var schemaResp datasource.SchemaResponse
	(&registryDelegationsDataSource{}).Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatal(diags)
	}

	delegations, ok := schemaResp.Schema.Attributes["delegations"].(datasourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("delegations is %T, want a ListNestedAttribute", schemaResp.Schema.Attributes["delegations"])
	}

	// One entry per field of sdktypes.EcrDelegation.
	want := []string{
		"aws_region",
		"aws_user",
		"created_at",
		"delegator_user_id",
		"docker_registry_uri",
		"id",
		"name",
		"repository",
		"tag",
	}
	got := make([]string, 0, len(delegations.NestedObject.Attributes))
	for name := range delegations.NestedObject.Attributes {
		got = append(got, name)
	}
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("delegation entry has %d attributes (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("delegation entry attributes = %v, want %v", got, want)
		}
	}
}

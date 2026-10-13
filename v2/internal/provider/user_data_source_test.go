package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

func TestFlattenUserMapsIdAndEmailAndNullsTheRest(t *testing.T) {
	// The GraphQL User type carries id and email of the 1.x fields; the quota,
	// key, name, verified and cloud-tier attributes have no source there and
	// read null however much the response carries.
	model := flattenUser(map[string]any{
		"id":       "user-1",
		"email":    "ada@example.com",
		"name":     "Ada",
		"gpuLimit": float64(16),
	})

	if model.Id.ValueString() != "user-1" || model.Email.ValueString() != "ada@example.com" {
		t.Errorf("id/email: %+v", model)
	}
	if !model.Name.IsNull() || !model.PubKey.IsNull() || !model.Verified.IsNull() || !model.CloudType.IsNull() ||
		!model.GpuLimit.IsNull() || !model.GpuUsage.IsNull() || !model.StorageLimit.IsNull() || !model.StorageUsage.IsNull() {
		t.Errorf("attributes without a GraphQL source should read null: %+v", model)
	}
}

func TestFlattenUserNullsAbsentFields(t *testing.T) {
	model := flattenUser(map[string]any{"id": "user-1"})
	if model.Id.ValueString() != "user-1" || !model.Email.IsNull() {
		t.Errorf("absent email should read null: %+v", model)
	}
}

func TestReadUserThroughTheMyselfQuery(t *testing.T) {
	var sent string
	client := graphQLStub(t, func(query string, _ map[string]any) string {
		sent = query
		return `{"data":{"myself":{"id":"user-1","email":"ada@example.com"}}}`
	})

	state, err := readUser(context.Background(), client)
	if err != nil {
		t.Fatalf("readUser: %v", err)
	}
	if !strings.Contains(sent, "myself") || !strings.Contains(sent, "email") {
		t.Errorf("the request did not carry the myself query: %q", sent)
	}
	if state.Id.ValueString() != "user-1" || state.Email.ValueString() != "ada@example.com" {
		t.Errorf("state: %+v", state)
	}
	if !state.GpuLimit.IsNull() {
		t.Errorf("gpu_limit should read null: %+v", state)
	}
}

func TestReadUserFailsOnAnEmptyAccount(t *testing.T) {
	for name, body := range map[string]string{
		"null account": `{"data":{"myself":null}}`,
		"no id":        `{"data":{"myself":{"email":"ada@example.com"}}}`,
	} {
		client := graphQLStub(t, func(string, map[string]any) string { return body })
		if _, err := readUser(context.Background(), client); err == nil {
			t.Errorf("%s: expected an error", name)
		} else if !strings.Contains(err.Error(), "no account") {
			t.Errorf("%s: error = %q", name, err)
		}
	}
}

func TestReadUserSurfacesGraphQLErrors(t *testing.T) {
	client := graphQLStub(t, func(string, map[string]any) string {
		return `{"errors":[{"message":"Unauthorized"}],"data":{"myself":null}}`
	})
	if _, err := readUser(context.Background(), client); err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Errorf("error = %v, want the GraphQL error", err)
	}
}

func TestUserDataSourceIsDeprecated(t *testing.T) {
	schema := dataSourceSchemaOf(t, NewUserDataSource())

	if schema.DeprecationMessage != userDeprecation {
		t.Errorf("runpod_user deprecation = %q", schema.DeprecationMessage)
	}
	if !strings.Contains(schema.Description, userDeprecation) {
		t.Errorf("runpod_user description does not repeat the deprecation: %q", schema.Description)
	}
	if !strings.Contains(userDeprecation, "GraphQL") || !strings.Contains(userDeprecation, "REST v1 /user is gone") {
		t.Errorf("the deprecation must name GraphQL as the source and the gone v1 route: %q", userDeprecation)
	}
}

func TestUserAttributesWithoutAGraphQLSourceSayTheyReadNull(t *testing.T) {
	var resp datasource.SchemaResponse
	NewUserDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	for _, name := range []string{"name", "pub_key", "verified", "cloud_type", "gpu_limit", "gpu_usage", "storage_limit", "storage_usage"} {
		if !strings.Contains(resp.Schema.Attributes[name].GetDescription(), "Always null") {
			t.Errorf("%s: description must say it reads null: %q", name, resp.Schema.Attributes[name].GetDescription())
		}
	}
	for _, name := range []string{"id", "email"} {
		if !strings.Contains(resp.Schema.Attributes[name].GetDescription(), "myself query") {
			t.Errorf("%s: description must name its GraphQL source: %q", name, resp.Schema.Attributes[name].GetDescription())
		}
	}
}

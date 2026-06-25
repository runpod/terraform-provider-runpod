package datasource_user

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// TestAccUserDataSource_riab asserts the DESIRED behavior: the user data source
// returns a non-empty user id from the live GraphQL endpoint. It FAILS today —
// the provider queries `user { id pubKey }` but the schema exposes `myself`
// ("Cannot query field user"), and CE-1652 (R1) would break the unwrap anyway.
// Green here == both the query/schema and CE-1652 are fixed.
//
// Gated on RIAB_ACC=1 with RUNPOD_API_KEY=$TEST_USER_JWT and
// RUNPOD_GRAPHQL_URL=http://localhost:4000/graphql.
func TestAccUserDataSource_riab(t *testing.T) {
	if os.Getenv("RIAB_ACC") != "1" {
		t.Skip("set RIAB_ACC=1 + RUNPOD_API_KEY + RUNPOD_GRAPHQL_URL to run live riab tests")
	}
	if os.Getenv("RUNPOD_API_KEY") == "" || os.Getenv("RUNPOD_GRAPHQL_URL") == "" {
		t.Fatal("RUNPOD_API_KEY and RUNPOD_GRAPHQL_URL must be set")
	}

	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: UserDataSourceSchema(ctx)}}
	(&UserDataSource{}).Read(ctx, datasource.ReadRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("user data source Read failed (RED until the query/schema mismatch + CE-1652 are fixed): %v", resp.Diagnostics.Errors())
	}
	var u UserModel
	resp.State.Get(ctx, &u)
	if u.Id.ValueString() == "" {
		t.Fatal("user data source returned an empty id")
	}
	t.Logf("user id=%s", u.Id.ValueString())
}

package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestGlobalVolumeDataSourceReadsByID(t *testing.T) {
	ctx := context.Background()
	srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeGlobalVolume(w, http.StatusOK, globalVolumeJSON)
	})

	var state globalVolumeModel
	readDataSource(ctx, t, &globalVolumeDataSource{client: shimTestClient(t, srv.URL)},
		map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.String, "gsb_abc")}, &state)

	assertGlobalVolumeRoutes(t, *seen, "GET /v2/global-volumes/gsb_abc")
	if (*seen)[0].Body != nil {
		t.Errorf("a read sends no body, got %v", (*seen)[0].Body)
	}
	assertTrainingDataVolume(t, state, "training-data")
}

func TestGlobalVolumeDataSourceFailsOnAMissingVolume(t *testing.T) {
	ctx := context.Background()
	srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeGlobalVolumeProblem(w, http.StatusNotFound, "global volume not found")
	})

	d := &globalVolumeDataSource{client: shimTestClient(t, srv.URL)}
	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	objectType := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	config := tftypes.NewValue(objectType, map[string]tftypes.Value{
		"id":                 tftypes.NewValue(tftypes.String, "gsb_missing"),
		"name":               tftypes.NewValue(tftypes.String, nil),
		"created_at":         tftypes.NewValue(tftypes.String, nil),
		"created_by_user_id": tftypes.NewValue(tftypes.String, nil),
	})

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config}}, resp)

	assertGlobalVolumeRoutes(t, *seen, "GET /v2/global-volumes/gsb_missing")
	if !resp.Diagnostics.HasError() {
		t.Fatal("looking up a volume that does not exist must fail the read")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "404") ||
		!strings.Contains(detail, "global volume not found") {
		t.Errorf("error detail = %q, want the status and the API detail", detail)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a failed lookup must not write state")
	}
}

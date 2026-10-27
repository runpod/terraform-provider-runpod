package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// These drive the resource's real CRUD and import against a stub of the v2
// global volume routes, so what they pin is the request each operation sends
// (method, path and JSON body) and how the answer lands in state.

const globalVolumeJSON = `{"id":"gsb_abc","name":"training-data","createdAt":"2026-10-01T12:00:00Z","createdByUserId":"user_123"}`

// globalVolumeRequest is one request the stub saw. Body is the decoded JSON
// body, nil when the request carried none.
type globalVolumeRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

func (r globalVolumeRequest) route() string { return r.Method + " " + r.Path }

// globalVolumeServer serves the v2 global volume routes through answer and
// records every request. Anything outside /v2/global-volumes, a /v1/ path
// above all, fails the test: these operations have no other host to reach.
func globalVolumeServer(t *testing.T, answer http.HandlerFunc) (*httptest.Server, *[]globalVolumeRequest) {
	t.Helper()
	var seen []globalVolumeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := globalVolumeRequest{Method: r.Method, Path: r.URL.Path}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the request body: %v", err)
		}
		if len(raw) > 0 {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("%s: Content-Type = %q, want application/json", request.route(), r.Header.Get("Content-Type"))
			}
			if err := json.Unmarshal(raw, &request.Body); err != nil {
				t.Errorf("%s: body %q is not a JSON object: %v", request.route(), raw, err)
			}
		}
		seen = append(seen, request)

		if strings.HasPrefix(r.URL.Path, "/v1/") || !strings.HasPrefix(r.URL.Path, "/v2/global-volumes") {
			t.Errorf("unexpected %s: only /v2/global-volumes may be called", request.route())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		answer(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

// assertGlobalVolumeRoutes checks the exact sequence of method and path the
// stub saw.
func assertGlobalVolumeRoutes(t *testing.T, seen []globalVolumeRequest, want ...string) {
	t.Helper()
	var got []string
	for _, request := range seen {
		got = append(got, request.route())
	}
	if !slices.Equal(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

func writeGlobalVolume(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// writeGlobalVolumeProblem answers with an RFC 9457 problem document, the
// error shape the v2 routes use.
func writeGlobalVolumeProblem(w http.ResponseWriter, status int, detail string) {
	body, err := json.Marshal(map[string]any{"title": http.StatusText(status), "status": status, "detail": detail})
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// globalVolumeState is a stored global volume as the resource last wrote it.
func globalVolumeState(ctx context.Context, t *testing.T, s resourceschema.Schema, raw tftypes.Value, name string) tftypes.Value {
	t.Helper()
	return shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":                 types.StringValue("gsb_abc"),
		"name":               types.StringValue(name),
		"created_at":         types.StringValue("2026-10-01T12:00:00Z"),
		"created_by_user_id": types.StringValue("user_123"),
	})
}

func readGlobalVolumeModel(ctx context.Context, t *testing.T, state tfsdk.State) globalVolumeModel {
	t.Helper()
	var model globalVolumeModel
	if diags := state.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	return model
}

func assertTrainingDataVolume(t *testing.T, model globalVolumeModel, wantName string) {
	t.Helper()
	if model.Id.ValueString() != "gsb_abc" || model.Name.ValueString() != wantName {
		t.Fatalf("id/name = %v / %v, want gsb_abc / %s", model.Id, model.Name, wantName)
	}
	if model.CreatedAt.ValueString() != "2026-10-01T12:00:00Z" {
		t.Errorf("created_at = %v, want 2026-10-01T12:00:00Z", model.CreatedAt)
	}
	if model.CreatedByUserId.ValueString() != "user_123" {
		t.Errorf("created_by_user_id = %v, want user_123", model.CreatedByUserId)
	}
}

func TestGlobalVolumeCreatePostsOnlyTheName(t *testing.T) {
	ctx := context.Background()
	srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeGlobalVolume(w, http.StatusCreated, globalVolumeJSON)
	})

	r := &globalVolumeResource{client: shimTestClient(t, srv.URL)}
	s, raw := shimResourceSchema(ctx, t, r)
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{"name": types.StringValue("training-data")})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}

	assertGlobalVolumeRoutes(t, *seen, "POST /v2/global-volumes")
	if want := map[string]any{"name": "training-data"}; !reflect.DeepEqual((*seen)[0].Body, want) {
		t.Fatalf("create body = %v, want %v", (*seen)[0].Body, want)
	}
	assertTrainingDataVolume(t, readGlobalVolumeModel(ctx, t, resp.State), "training-data")
}

func TestGlobalVolumeCreateKeepsAnUnknownCreatorNull(t *testing.T) {
	ctx := context.Background()
	srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeGlobalVolume(w, http.StatusCreated,
			`{"id":"gsb_abc","name":"training-data","createdAt":"2026-10-01T12:00:00Z","createdByUserId":null}`)
	})

	r := &globalVolumeResource{client: shimTestClient(t, srv.URL)}
	s, raw := shimResourceSchema(ctx, t, r)
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{"name": types.StringValue("training-data")})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}

	assertGlobalVolumeRoutes(t, *seen, "POST /v2/global-volumes")
	if saved := readGlobalVolumeModel(ctx, t, resp.State); !saved.CreatedByUserId.IsNull() {
		t.Fatalf("created_by_user_id = %v, want null when the API records no creator", saved.CreatedByUserId)
	}
}

func TestGlobalVolumeCreateReportsTheAccountCap(t *testing.T) {
	ctx := context.Background()
	const detail = "global volume limit reached for this account"
	srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeGlobalVolumeProblem(w, http.StatusBadRequest, detail)
	})

	r := &globalVolumeResource{client: shimTestClient(t, srv.URL)}
	s, raw := shimResourceSchema(ctx, t, r)
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{"name": types.StringValue("training-data")})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)

	assertGlobalVolumeRoutes(t, *seen, "POST /v2/global-volumes")
	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused create must be an error")
	}
	if got := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(got, "400") || !strings.Contains(got, detail) {
		t.Errorf("error detail = %q, want the status and the API detail", got)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("nothing was created, so no state may be written")
	}
}

func TestGlobalVolumeReadRefreshesFromTheAPI(t *testing.T) {
	ctx := context.Background()
	srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeGlobalVolume(w, http.StatusOK, globalVolumeJSON)
	})

	r := &globalVolumeResource{client: shimTestClient(t, srv.URL)}
	s, raw := shimResourceSchema(ctx, t, r)
	stored := globalVolumeState(ctx, t, s, raw, "renamed-elsewhere")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: stored}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: stored}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}

	assertGlobalVolumeRoutes(t, *seen, "GET /v2/global-volumes/gsb_abc")
	if (*seen)[0].Body != nil {
		t.Errorf("a read sends no body, got %v", (*seen)[0].Body)
	}
	assertTrainingDataVolume(t, readGlobalVolumeModel(ctx, t, resp.State), "training-data")
}

func TestGlobalVolumeReadDropsAVolumeTheAPINoLongerHas(t *testing.T) {
	ctx := context.Background()
	srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeGlobalVolumeProblem(w, http.StatusNotFound, "global volume not found")
	})

	r := &globalVolumeResource{client: shimTestClient(t, srv.URL)}
	s, raw := shimResourceSchema(ctx, t, r)
	stored := globalVolumeState(ctx, t, s, raw, "training-data")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: stored}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: stored}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}

	assertGlobalVolumeRoutes(t, *seen, "GET /v2/global-volumes/gsb_abc")
	if !resp.State.Raw.IsNull() {
		t.Fatal("a 404 means the volume is gone: the resource must leave state so the next plan recreates it")
	}
}

func TestGlobalVolumeUpdatePatchesTheName(t *testing.T) {
	ctx := context.Background()
	srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeGlobalVolume(w, http.StatusOK,
			`{"id":"gsb_abc","name":"renamed-volume","createdAt":"2026-10-01T12:00:00Z","createdByUserId":"user_123"}`)
	})

	r := &globalVolumeResource{client: shimTestClient(t, srv.URL)}
	s, raw := shimResourceSchema(ctx, t, r)
	stored := globalVolumeState(ctx, t, s, raw, "training-data")
	planned := globalVolumeState(ctx, t, s, raw, "renamed-volume")
	config := shimObject(ctx, t, s, raw, map[string]attr.Value{"name": types.StringValue("renamed-volume")})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: stored}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planned},
		State:  tfsdk.State{Schema: s, Raw: stored},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}

	assertGlobalVolumeRoutes(t, *seen, "PATCH /v2/global-volumes/gsb_abc")
	if want := map[string]any{"name": "renamed-volume"}; !reflect.DeepEqual((*seen)[0].Body, want) {
		t.Fatalf("update body = %v, want %v", (*seen)[0].Body, want)
	}
	assertTrainingDataVolume(t, readGlobalVolumeModel(ctx, t, resp.State), "renamed-volume")
}

func TestGlobalVolumeUpdateKeepsTheStoredVolumeOnARefusedRename(t *testing.T) {
	ctx := context.Background()
	srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeGlobalVolumeProblem(w, http.StatusNotFound, "global volume not found")
	})

	r := &globalVolumeResource{client: shimTestClient(t, srv.URL)}
	s, raw := shimResourceSchema(ctx, t, r)
	stored := globalVolumeState(ctx, t, s, raw, "training-data")
	planned := globalVolumeState(ctx, t, s, raw, "renamed-volume")
	config := shimObject(ctx, t, s, raw, map[string]attr.Value{"name": types.StringValue("renamed-volume")})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: stored}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planned},
		State:  tfsdk.State{Schema: s, Raw: stored},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)

	assertGlobalVolumeRoutes(t, *seen, "PATCH /v2/global-volumes/gsb_abc")
	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused rename must fail the apply")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "404") ||
		!strings.Contains(detail, "global volume not found") {
		t.Errorf("error detail = %q, want the status and the API detail", detail)
	}
	// The framework keeps the prior state on an error; the resource must not
	// have written the planned name over it.
	assertTrainingDataVolume(t, readGlobalVolumeModel(ctx, t, resp.State), "training-data")
}

func TestGlobalVolumeDelete(t *testing.T) {
	ctx := context.Background()
	const mounted = "Cannot delete a global volume while pods still mount it"
	for _, tc := range []struct {
		name      string
		status    int
		wantError []string
	}{
		{name: "deleted", status: http.StatusNoContent},
		{name: "already gone", status: http.StatusNotFound},
		{name: "still mounted", status: http.StatusConflict, wantError: []string{"409", mounted, "Terminate every pod"}},
		// A status the SDK does not retry, so the stub sees exactly one DELETE:
		// the transport retries an idempotent request on a 5xx.
		{name: "refused", status: http.StatusForbidden, wantError: []string{"403"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch tc.status {
				case http.StatusNoContent:
					w.WriteHeader(http.StatusNoContent)
				case http.StatusConflict:
					writeGlobalVolumeProblem(w, tc.status, mounted)
				default:
					writeGlobalVolumeProblem(w, tc.status, http.StatusText(tc.status))
				}
			})

			r := &globalVolumeResource{client: shimTestClient(t, srv.URL)}
			s, raw := shimResourceSchema(ctx, t, r)
			stored := globalVolumeState(ctx, t, s, raw, "training-data")

			resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: stored}}
			r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: stored}}, resp)

			assertGlobalVolumeRoutes(t, *seen, "DELETE /v2/global-volumes/gsb_abc")
			if len(tc.wantError) == 0 {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
				}
				return
			}
			if !resp.Diagnostics.HasError() {
				t.Fatalf("status %d must fail the destroy", tc.status)
			}
			detail := resp.Diagnostics.Errors()[0].Detail()
			for _, want := range tc.wantError {
				if !strings.Contains(detail, want) {
					t.Errorf("error detail %q does not contain %q", detail, want)
				}
			}
		})
	}
}

func TestGlobalVolumeImportReadsTheVolume(t *testing.T) {
	ctx := context.Background()
	srv, seen := globalVolumeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeGlobalVolume(w, http.StatusOK, globalVolumeJSON)
	})

	// An import is ImportState followed by Read.
	r := &globalVolumeResource{client: shimTestClient(t, srv.URL)}
	s, raw := shimResourceSchema(ctx, t, r)

	importResp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "gsb_abc"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatal(importResp.Diagnostics)
	}

	readResp := &resource.ReadResponse{State: importResp.State}
	r.Read(ctx, resource.ReadRequest{State: importResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatal(readResp.Diagnostics)
	}

	assertGlobalVolumeRoutes(t, *seen, "GET /v2/global-volumes/gsb_abc")
	assertTrainingDataVolume(t, readGlobalVolumeModel(ctx, t, readResp.State), "training-data")
}

func TestGlobalVolumeNameValidation(t *testing.T) {
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	(&globalVolumeResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	attribute, ok := schemaResp.Schema.Attributes["name"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("name attribute is %T, want a StringAttribute", schemaResp.Schema.Attributes["name"])
	}

	for value, wantValid := range map[string]bool{
		"training-data": true,
		"a":             true,
		"two words":     true,
		"line\nbreak":   true,
		strings.Repeat("a", globalVolumeNameMaxLength): true,
		// The limit counts characters, as the API does, not bytes.
		strings.Repeat("é", globalVolumeNameMaxLength):   true,
		strings.Repeat("a", globalVolumeNameMaxLength+1): false,
		"":                   false,
		"   ":                false,
		" leading":           false,
		"trailing ":          false,
		"trailing-newline\n": false,
		"\tleading-tab":      false,
		"\vleading-vt":       false,
		"trailing-nel\u0085": false,
		" non-breaking":      false,
		"ideographic　":       false,
	} {
		var valResp validator.StringResponse
		for _, v := range attribute.Validators {
			v.ValidateString(ctx, validator.StringRequest{
				Path:        path.Root("name"),
				ConfigValue: types.StringValue(value),
			}, &valResp)
		}
		if valResp.Diagnostics.HasError() == wantValid {
			t.Errorf("name %q: valid = %v, want %v", value, !valResp.Diagnostics.HasError(), wantValid)
		}
	}
}

func TestGlobalVolumeComputedAttributesSurviveARename(t *testing.T) {
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	(&globalVolumeResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}

	// A rename changes nothing but the name, so the plan shows the stored
	// identifier, timestamp and creator instead of "known after apply".
	for _, name := range []string{"id", "created_at", "created_by_user_id"} {
		attribute, ok := schemaResp.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok {
			t.Fatalf("%s attribute is %T, want a StringAttribute", name, schemaResp.Schema.Attributes[name])
		}
		if !attribute.Computed || attribute.Optional || attribute.Required {
			t.Errorf("%s must be computed only", name)
		}
		if len(attribute.PlanModifiers) != 1 {
			t.Errorf("%s: %d plan modifiers, want UseStateForUnknown", name, len(attribute.PlanModifiers))
		}
	}
}

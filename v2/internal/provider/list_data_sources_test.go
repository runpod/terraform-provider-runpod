package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// pagingServer stands in for a cursor-paginated v2 list route. A request
// without a cursor is answered with first; the request carrying nextCursor is
// answered with second. Each body states its own pagination block, so a test
// spells out the envelope the provider has to walk rather than relying on the
// stub to synthesise one. Every query string the route saw is appended to
// queries, in order, because re-asking for page one forever and stopping after
// page one are both failures this has to tell apart.
func pagingServer(t *testing.T, queries *[]string, nextCursor, first, second string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		*queries = append(*queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		switch cursor {
		case "":
			_, _ = w.Write([]byte(first))
		case nextCursor:
			_, _ = w.Write([]byte(second))
		default:
			t.Errorf("route asked for cursor %q, which this stub never issued", cursor)
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// readDataSource drives a data source's Read the way Terraform does: build the
// schema, hand it a config whose named attributes are set and the rest null,
// and decode the state it wrote into model. The data-source mirror of
// shimResourceSchema.
func readDataSource(ctx context.Context, t *testing.T, d datasource.DataSource, config map[string]tftypes.Value, model any) {
	t.Helper()

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
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
	for name, value := range config {
		values[name] = value
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, nil)}}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, values)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if diags := resp.State.Get(ctx, model); diags.HasError() {
		t.Fatal(diags)
	}
}

// wantPagedQueries is the query sequence a two-page walk has to produce: the
// first page asks for nothing, the second passes the cursor through verbatim.
func wantPagedQueries(cursor string) []string {
	return []string{"", "cursor=" + cursor}
}

// TestPodsDataSourceIncludeClusterPods drives the account listing's Read
// against a stub API: include_cluster_pods has to reach the wire as the query
// parameter the route reads, and unset has to send nothing rather than
// false: the API's own default is what an unset attribute means. The value
// also has to come back in state, since the response does not echo it.
func TestPodsDataSourceIncludeClusterPods(t *testing.T) {
	ctx := context.Background()

	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pods":[{"id":"pod_abc123","name":"trainer","image":"runpod/pytorch:1.0.2",` +
			`"status":"RUNNING","cloud":"SECURE","disk":50,"dataCenterId":"US-TX-3",` +
			`"createdAt":"2026-06-01T10:00:00Z","gpu":{"id":"NVIDIA A100 80GB PCIe","count":2}}]}`))
	}))
	defer srv.Close()

	d := &podsDataSource{client: shimTestClient(t, srv.URL)}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type = %T, want an object", schemaResp.Schema.Type().TerraformType(ctx))
	}

	for _, testCase := range []struct {
		name       string
		configured tftypes.Value
		wantQuery  string
		wantState  types.Bool
	}{
		{
			name:       "unset",
			configured: tftypes.NewValue(tftypes.Bool, nil),
			wantQuery:  "",
			wantState:  types.BoolNull(),
		},
		{
			name:       "true",
			configured: tftypes.NewValue(tftypes.Bool, true),
			wantQuery:  "includeClusterPods=true",
			wantState:  types.BoolValue(true),
		},
		{
			name:       "false",
			configured: tftypes.NewValue(tftypes.Bool, false),
			wantQuery:  "includeClusterPods=false",
			wantState:  types.BoolValue(false),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			values := map[string]tftypes.Value{}
			for name, attributeType := range objectType.AttributeTypes {
				values[name] = tftypes.NewValue(attributeType, nil)
			}
			values["include_cluster_pods"] = testCase.configured

			resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, nil)}}
			d.Read(ctx, datasource.ReadRequest{
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, values)},
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}

			if gotPath != "/v2/pods" {
				t.Errorf("path = %q, want /v2/pods", gotPath)
			}
			if gotQuery != testCase.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, testCase.wantQuery)
			}

			var state podsDataSourceModel
			if diags := resp.State.Get(ctx, &state); diags.HasError() {
				t.Fatal(diags)
			}
			if !state.IncludeClusterPods.Equal(testCase.wantState) {
				t.Errorf("include_cluster_pods in state = %v, want %v", state.IncludeClusterPods, testCase.wantState)
			}
			if len(state.Pods) != 1 || state.Pods[0].Id.ValueString() != "pod_abc123" ||
				state.Pods[0].GpuTypeId.ValueString() != "NVIDIA A100 80GB PCIe" ||
				state.Pods[0].GpuCount.ValueInt64() != 2 {
				t.Errorf("pods: %+v", state.Pods)
			}
		})
	}
}

// TestTemplatesDataSourceFollowsPaginationCursor and its serverless-endpoints
// twin hold the account-wide listings to what their descriptions promise. Both
// routes are cursor-paginated and cap a page at the server default, so a Read
// that fires one request writes a truncated list to state and calls it all of
// them.
func TestTemplatesDataSourceFollowsPaginationCursor(t *testing.T) {
	ctx := context.Background()

	var queries []string
	srv := pagingServer(t, &queries, "cur_templates_2",
		`{"templates":[{"id":"tpl_first","name":"first","image":"runpod/pytorch:1.0.2","category":"NVIDIA",`+
			`"disk":20,"public":false,"serverless":false}],`+
			`"pagination":{"hasNextPage":true,"nextCursor":"cur_templates_2"}}`,
		`{"templates":[{"id":"tpl_second","name":"second","image":"runpod/pytorch:1.0.2","category":"CPU",`+
			`"disk":10,"public":true,"serverless":true}],`+
			`"pagination":{"hasNextPage":false,"nextCursor":null}}`)

	var state templatesDataSourceModel
	readDataSource(ctx, t, &templatesDataSource{client: shimTestClient(t, srv.URL)}, nil, &state)

	if want := wantPagedQueries("cur_templates_2"); !slices.Equal(queries, want) {
		t.Fatalf("queries = %q, want %q", queries, want)
	}
	if len(state.Templates) != 2 ||
		state.Templates[0].Id.ValueString() != "tpl_first" ||
		state.Templates[1].Id.ValueString() != "tpl_second" {
		t.Fatalf("templates = %+v, want both pages in order", state.Templates)
	}
	if !state.Templates[1].Serverless.ValueBool() || state.Templates[1].Category.ValueString() != "CPU" {
		t.Errorf("second page entry: %+v", state.Templates[1])
	}
}

func TestServerlessEndpointsDataSourceFollowsPaginationCursor(t *testing.T) {
	ctx := context.Background()

	var queries []string
	srv := pagingServer(t, &queries, "cur_endpoints_2",
		`{"endpoints":[{"id":"ep_first","name":"first","createdAt":"2026-06-01T12:00:00Z",`+
			`"workers":{"min":0,"max":3}}],`+
			`"pagination":{"hasNextPage":true,"nextCursor":"cur_endpoints_2"}}`,
		`{"endpoints":[{"id":"ep_second","name":"second","createdAt":"2026-06-02T12:00:00Z",`+
			`"workers":{"min":1,"max":5}}],`+
			`"pagination":{"hasNextPage":false,"nextCursor":null}}`)

	var state serverlessEndpointsDataSourceModel
	readDataSource(ctx, t, &serverlessEndpointsDataSource{client: shimTestClient(t, srv.URL)}, nil, &state)

	if want := wantPagedQueries("cur_endpoints_2"); !slices.Equal(queries, want) {
		t.Fatalf("queries = %q, want %q", queries, want)
	}
	if len(state.Endpoints) != 2 ||
		state.Endpoints[0].Id.ValueString() != "ep_first" ||
		state.Endpoints[1].Id.ValueString() != "ep_second" {
		t.Fatalf("endpoints = %+v, want both pages in order", state.Endpoints)
	}
	if state.Endpoints[1].WorkersMax.ValueInt64() != 5 {
		t.Errorf("second page entry: %+v", state.Endpoints[1])
	}
}

func TestPodsDataSourceFollowsPaginationCursorPastAnEmptyPage(t *testing.T) {
	ctx := context.Background()

	// The cluster filter can leave the first page empty while hasNextPage is
	// still true; stopping there would report an account with no pods.
	var queries []string
	srv := pagingServer(t, &queries, "cur_pods_2",
		`{"pods":[],"pagination":{"hasNextPage":true,"nextCursor":"cur_pods_2"}}`,
		`{"pods":[{"id":"pod_second","name":"trainer","image":"runpod/pytorch:1.0.2","status":"RUNNING",`+
			`"cloud":"SECURE","disk":50,"createdAt":"2026-06-01T10:00:00Z"}],`+
			`"pagination":{"hasNextPage":false,"nextCursor":null}}`)

	var state podsDataSourceModel
	readDataSource(ctx, t, &podsDataSource{client: shimTestClient(t, srv.URL)}, nil, &state)

	if want := wantPagedQueries("cur_pods_2"); !slices.Equal(queries, want) {
		t.Fatalf("queries = %q, want %q", queries, want)
	}
	if len(state.Pods) != 1 || state.Pods[0].Id.ValueString() != "pod_second" {
		t.Fatalf("pods = %+v, want the pod on the second page", state.Pods)
	}
}

func TestPodsDataSourceKeepsIncludeClusterPodsOnEveryPage(t *testing.T) {
	ctx := context.Background()

	// A cursor is only valid with the parameters that issued it, so the
	// filter has to ride along on the follow-up request too.
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"pods":[],"pagination":{"hasNextPage":true,"nextCursor":"cur_pods_2"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"pods":[],"pagination":{"hasNextPage":false,"nextCursor":null}}`))
	}))
	defer srv.Close()

	var state podsDataSourceModel
	readDataSource(ctx, t, &podsDataSource{client: shimTestClient(t, srv.URL)},
		map[string]tftypes.Value{"include_cluster_pods": tftypes.NewValue(tftypes.Bool, true)}, &state)

	want := []string{"includeClusterPods=true", "includeClusterPods=true&cursor=cur_pods_2"}
	if !slices.Equal(queries, want) {
		t.Fatalf("queries = %q, want %q", queries, want)
	}
}

func queueRequestUrls(t *testing.T) *sdktypes.EndpointRequestUrls {
	t.Helper()
	var urls sdktypes.EndpointRequestUrls
	if err := urls.FromQueueBasedRequestUrls(sdktypes.QueueBasedRequestUrls{
		Run:        "https://api.runpod.ai/v2/ep_abc123/run",
		RunSync:    "https://api.runpod.ai/v2/ep_abc123/runsync",
		Status:     "https://api.runpod.ai/v2/ep_abc123/status/{job_id}",
		Stream:     "https://api.runpod.ai/v2/ep_abc123/stream/{job_id}",
		Cancel:     "https://api.runpod.ai/v2/ep_abc123/cancel/{job_id}",
		Retry:      "https://api.runpod.ai/v2/ep_abc123/retry/{job_id}",
		PurgeQueue: "https://api.runpod.ai/v2/ep_abc123/purge-queue",
		Health:     "https://api.runpod.ai/v2/ep_abc123/health",
	}); err != nil {
		t.Fatal(err)
	}
	return &urls
}

func loadBalancerRequestUrls(t *testing.T) *sdktypes.EndpointRequestUrls {
	t.Helper()
	var urls sdktypes.EndpointRequestUrls
	if err := urls.FromLoadBalancingRequestUrls(sdktypes.LoadBalancingRequestUrls{
		Base:   "https://ep_lb123.api.runpod.ai",
		Health: "https://ep_lb123.api.runpod.ai/health",
	}); err != nil {
		t.Fatal(err)
	}
	return &urls
}

func endpointListItemAttribute(t *testing.T, item endpointListItemModel, name string) string {
	t.Helper()
	if item.RequestUrls.IsNull() {
		t.Fatal("request_urls is null")
	}
	value, ok := item.RequestUrls.Attributes()[name]
	if !ok {
		t.Fatalf("request_urls has no %q attribute", name)
	}
	return value.String()
}

func TestFlattenEndpointListItemQueueUrls(t *testing.T) {
	ctx := context.Background()
	image := "runpod/worker-vllm:1.0.0"
	endpointType := sdktypes.EndpointTypeQueue
	endpoint := sdktypes.Endpoint{
		Id:          "ep_abc123",
		Name:        "vllm",
		Image:       &image,
		Type:        &endpointType,
		CreatedAt:   time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		RequestUrls: queueRequestUrls(t),
	}
	// Endpoint.Workers is an inline struct in the generated client, so its
	// fields are set after construction rather than in the literal.
	endpoint.Workers.Min = 0
	endpoint.Workers.Max = 3

	item, diags := flattenEndpointListItem(ctx, endpoint)
	if diags.HasError() {
		t.Fatal(diags)
	}

	if item.Id.ValueString() != "ep_abc123" || item.Name.ValueString() != "vllm" ||
		item.Image.ValueString() != image {
		t.Fatalf("identity: %+v", item)
	}
	if item.EndpointType.ValueString() != "QUEUE" ||
		item.WorkersMin.ValueInt64() != 0 || item.WorkersMax.ValueInt64() != 3 ||
		item.CreatedAt.ValueString() != "2026-06-01T12:00:00Z" {
		t.Fatalf("fields: %+v", item)
	}

	if got := endpointListItemAttribute(t, item, "run"); got != `"https://api.runpod.ai/v2/ep_abc123/run"` {
		t.Fatalf("run = %s", got)
	}
	if got := endpointListItemAttribute(t, item, "purge_queue"); got != `"https://api.runpod.ai/v2/ep_abc123/purge-queue"` {
		t.Fatalf("purge_queue = %s", got)
	}
	if got := endpointListItemAttribute(t, item, "health"); got != `"https://api.runpod.ai/v2/ep_abc123/health"` {
		t.Fatalf("health = %s", got)
	}
	// base belongs to the load-balancer shape only.
	if got := endpointListItemAttribute(t, item, "base"); got != "<null>" {
		t.Fatalf("base = %s, want null on a QUEUE endpoint", got)
	}

	assertDataSourceModelFits(t, NewServerlessEndpointsDataSource,
		serverlessEndpointsDataSourceModel{Endpoints: []endpointListItemModel{item}})
}

func TestFlattenEndpointListItemLoadBalancerUrls(t *testing.T) {
	ctx := context.Background()
	endpointType := sdktypes.EndpointTypeLoadBalancer
	endpoint := sdktypes.Endpoint{
		Id:          "ep_lb123",
		Name:        "lb",
		Type:        &endpointType,
		CreatedAt:   time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		RequestUrls: loadBalancerRequestUrls(t),
	}
	endpoint.Workers.Min = 1
	endpoint.Workers.Max = 2

	item, diags := flattenEndpointListItem(ctx, endpoint)
	if diags.HasError() {
		t.Fatal(diags)
	}

	if got := endpointListItemAttribute(t, item, "base"); got != `"https://ep_lb123.api.runpod.ai"` {
		t.Fatalf("base = %s", got)
	}
	if got := endpointListItemAttribute(t, item, "health"); got != `"https://ep_lb123.api.runpod.ai/health"` {
		t.Fatalf("health = %s", got)
	}
	// The queue operations belong to the queue shape only.
	for _, name := range []string{"run", "run_sync", "status", "stream", "cancel", "retry", "purge_queue"} {
		if got := endpointListItemAttribute(t, item, name); got != "<null>" {
			t.Fatalf("%s = %s, want null on a LOAD_BALANCER endpoint", name, got)
		}
	}
	if !item.Image.IsNull() {
		t.Fatalf("image = %v, want null when the API omits it", item.Image)
	}

	assertDataSourceModelFits(t, NewServerlessEndpointsDataSource,
		serverlessEndpointsDataSourceModel{Endpoints: []endpointListItemModel{item}})
}

func TestFlattenEndpointListItemWithoutRequestUrls(t *testing.T) {
	ctx := context.Background()
	endpoint := sdktypes.Endpoint{
		Id:        "ep_bare",
		Name:      "bare",
		CreatedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
	}
	endpoint.Workers.Min = 0
	endpoint.Workers.Max = 1

	item, diags := flattenEndpointListItem(ctx, endpoint)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !item.RequestUrls.IsNull() {
		t.Fatalf("request_urls = %v, want null when the API omits them", item.RequestUrls)
	}
	if !item.EndpointType.IsNull() {
		t.Fatalf("endpoint_type = %v, want null when the API omits it", item.EndpointType)
	}

	assertDataSourceModelFits(t, NewServerlessEndpointsDataSource,
		serverlessEndpointsDataSourceModel{Endpoints: []endpointListItemModel{item}})
}

func TestRegistriesDataSourceReadsUsernamesAsNullWithoutGraphQL(t *testing.T) {
	ctx := context.Background()

	// The GraphQL schema has no containerRegistryAuths root field and its
	// ContainerRegistryAuth type has no username, so any request for one fails.
	// The listing has to come from REST v2 alone.
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet || r.URL.Path != "/v2/registries" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"registries":[{"id":"reg_abc","name":"ghcr"}]}`))
	}))
	defer srv.Close()

	var state registriesDataSourceModel
	readDataSource(ctx, t, &registriesDataSource{client: shimTestClient(t, srv.URL)}, nil, &state)

	if !slices.Equal(paths, []string{"GET /v2/registries"}) {
		t.Errorf("requests = %q, want the REST v2 listing only", paths)
	}
	if len(state.Registries) != 1 || state.Registries[0].Id.ValueString() != "reg_abc" ||
		state.Registries[0].Name.ValueString() != "ghcr" {
		t.Fatalf("registries = %+v", state.Registries)
	}
	if !state.Registries[0].Username.IsNull() {
		t.Errorf("username = %v, want null", state.Registries[0].Username)
	}
}

func TestRegistriesDataSourceDeprecatesUsername(t *testing.T) {
	ctx := context.Background()
	var schemaResp datasource.SchemaResponse
	NewRegistriesDataSource().Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}

	registries, ok := schemaResp.Schema.Attributes["registries"].(datasourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("registries attribute is %T", schemaResp.Schema.Attributes["registries"])
	}
	username, ok := registries.NestedObject.Attributes["username"]
	if !ok {
		t.Fatal("the registry entries carry no username attribute")
	}
	if username.GetDeprecationMessage() != registryUsernameDeprecation {
		t.Errorf("deprecation message = %q", username.GetDeprecationMessage())
	}
}

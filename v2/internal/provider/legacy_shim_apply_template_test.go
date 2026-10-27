package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// templateShimCall is one request the template stub received, on either
// host. Body is the decoded JSON body: the v2 request, or the GraphQL
// request's query and variables.
type templateShimCall struct {
	Method string
	Path   string
	Body   map[string]any
}

// Query is the GraphQL document of a call to /graphql.
func (c templateShimCall) Query() string {
	query, _ := c.Body["query"].(string)
	return query
}

// Variables is the GraphQL variables of a call to /graphql.
func (c templateShimCall) Variables() map[string]any {
	variables, _ := c.Body["variables"].(map[string]any)
	return variables
}

type templateShimRecorder struct {
	mu    sync.Mutex
	calls []templateShimCall
}

func (r *templateShimRecorder) record(call templateShimCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *templateShimRecorder) Calls() []templateShimCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]templateShimCall(nil), r.calls...)
}

// GraphQLQueries lists the documents posted to /graphql, in order.
func (r *templateShimRecorder) GraphQLQueries() []string {
	var queries []string
	for _, call := range r.Calls() {
		if call.Path == "/graphql" {
			queries = append(queries, call.Query())
		}
	}
	return queries
}

// V2Call returns the one REST v2 call made with method, failing the test
// unless there is exactly one.
func (r *templateShimRecorder) V2Call(t *testing.T, method string) templateShimCall {
	t.Helper()
	var found []templateShimCall
	for _, call := range r.Calls() {
		if call.Method == method && strings.HasPrefix(call.Path, "/v2/") {
			found = append(found, call)
		}
	}
	if len(found) != 1 {
		t.Fatalf("REST v2 %s calls = %+v, want exactly one", method, found)
	}
	return found[0]
}

// newTemplateShimStub serves REST v2 and the GraphQL API from one httptest
// server, the way the template resource talks to both in one apply: v2
// answers what v2 returns for the method and path, GraphQL what graphQL
// returns for the document. A request to any other path, a REST v1 route
// above all, fails the test.
func newTemplateShimStub(
	t *testing.T,
	v2 func(method, path string) (int, string),
	graphQL func(query string, variables map[string]any) string,
) (*templateResource, *templateShimRecorder) {
	t.Helper()
	recorder := &templateShimRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := templateShimCall{Method: r.Method, Path: r.URL.Path}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the %s %s body: %v", r.Method, r.URL.Path, err)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &call.Body); err != nil {
				t.Errorf("decoding the %s %s body: %v", r.Method, r.URL.Path, err)
			}
		}
		recorder.record(call)

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/v1/"):
			t.Errorf("request reached a REST v1 path: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/graphql" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(graphQL(call.Query(), call.Variables())))
		case strings.HasPrefix(r.URL.Path, "/v2/"):
			status, body := v2(r.Method, r.URL.Path)
			if status == 0 {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				status = http.StatusInternalServerError
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	return &templateResource{
		client: shimTestClient(t, srv.URL),
		// A plain client: the retrying default would back off for seconds on
		// the failure cases.
		legacy: newTestLegacyClient(t, legacy.Config{
			APIKey:     "test-key",
			GraphQLURL: srv.URL + "/graphql",
			HTTPClient: &http.Client{Timeout: 5 * time.Second},
		}),
	}, recorder
}

const v2TemplateTrainer = `{"id":"tpl_abc","name":"trainer","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,` +
	`"args":"","env":{},"ports":[],"allowedCudaVersions":[]}`

const v2TemplateRenamed = `{"id":"tpl_abc","name":"renamed","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,` +
	`"args":"","env":{},"ports":[],"allowedCudaVersions":[]}`

func stringList(ctx context.Context, t *testing.T, values ...string) types.List {
	t.Helper()
	list, diags := types.ListValueFrom(ctx, types.StringType, append([]string{}, values...))
	if diags.HasError() {
		t.Fatal(diags)
	}
	return list
}

func listStrings(ctx context.Context, t *testing.T, list types.List) []string {
	t.Helper()
	var values []string
	if diags := list.ElementsAs(ctx, &values, false); diags.HasError() {
		t.Fatal(diags)
	}
	return values
}

func bodyStrings(t *testing.T, body map[string]any, key string) []string {
	t.Helper()
	raw, ok := body[key].([]any)
	if !ok {
		t.Fatalf("%s = %#v, want a list in the body", key, body[key])
	}
	values := make([]string, 0, len(raw))
	for _, value := range raw {
		values = append(values, value.(string))
	}
	return values
}

func TestTemplateCreateSendsTheEntrypointToV2AndTheReadmeToGraphQL(t *testing.T) {
	ctx := context.Background()

	r, recorder := newTemplateShimStub(t,
		func(method, path string) (int, string) {
			if method == http.MethodPost && path == "/v2/templates" {
				return http.StatusCreated, `{"id":"tpl_abc","name":"trainer","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,` +
					`"args":"{\"entrypoint\":[\"/bin/bash\",\"-c\"]}","entrypoint":["/bin/bash","-c"],` +
					`"env":{},"ports":[],"allowedCudaVersions":[]}`
			}
			return 0, ""
		},
		func(query string, _ map[string]any) string {
			if query == legacyTemplateForSaveQuery {
				return storedTemplate
			}
			return `{"data":{"saveTemplate":{"readme":"# trainer","earned":0,"isRunpod":false,"runtimeInMin":0}}}`
		},
	)
	s, raw := shimResourceSchema(ctx, t, r)
	// A heredoc readme: saveTemplate stores it trimmed.
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":              types.StringValue("trainer"),
		"image":             types.StringValue("runpod/pytorch:1.0.2"),
		"docker_entrypoint": stringList(ctx, t, "/bin/bash", "-c"),
		"readme":            types.StringValue("# trainer\n"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	create := recorder.V2Call(t, http.MethodPost)
	if got := bodyStrings(t, create.Body, "entrypoint"); !slices.Equal(got, []string{"/bin/bash", "-c"}) {
		t.Errorf("v2 entrypoint = %v, want [/bin/bash -c]", got)
	}
	for _, key := range []string{"args", "readme", "dockerEntrypoint"} {
		if _, ok := create.Body[key]; ok {
			t.Errorf("the v2 create carries %q: %v", key, create.Body)
		}
	}

	calls := recorder.Calls()
	if len(calls) != 3 || calls[0].Path != "/v2/templates" ||
		calls[1].Query() != legacyTemplateForSaveQuery || calls[2].Query() != legacySaveTemplateReadmeMutation {
		t.Fatalf("calls = %+v, want the v2 create, then the GraphQL read and saveTemplate", calls)
	}
	if calls[1].Variables()["id"] != "tpl_abc" {
		t.Errorf("read-for-save variables = %v, want the created id", calls[1].Variables())
	}
	input, ok := calls[2].Variables()["input"].(map[string]any)
	if !ok || input["id"] != "tpl_abc" || input["readme"] != "# trainer\n" {
		t.Errorf("saveTemplate input = %v, want id tpl_abc and the configured readme", calls[2].Variables()["input"])
	}

	var saved templateResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if got := listStrings(ctx, t, saved.DockerEntrypoint); !slices.Equal(got, []string{"/bin/bash", "-c"}) {
		t.Errorf("docker_entrypoint = %v, want the v2 read-back", got)
	}
	if saved.Args.ValueString() != `{"entrypoint":["/bin/bash","-c"]}` {
		t.Errorf("args = %q, want the stored command the API encoded the entrypoint into", saved.Args.ValueString())
	}
	if saved.Readme.ValueString() != "# trainer\n" {
		t.Errorf("readme = %q, want the configured text, which differs from the stored one only by the trim", saved.Readme.ValueString())
	}
	if saved.Earned.IsNull() || saved.Earned.ValueFloat64() != 0 || saved.IsRunpod.IsNull() || saved.IsRunpod.ValueBool() ||
		saved.RuntimeInMin.IsNull() || saved.RuntimeInMin.ValueInt64() != 0 {
		t.Errorf("read-backs = %+v, want the saveTemplate answer", saved)
	}
}

func TestTemplateReadFillsTheShimFieldsFromGraphQL(t *testing.T) {
	ctx := context.Background()
	v2 := func(method, path string) (int, string) {
		if method == http.MethodGet && path == "/v2/templates/tpl_abc" {
			return http.StatusOK, `{"id":"tpl_abc","name":"trainer","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,` +
				`"args":"{\"entrypoint\":[\"/bin/sh\"]}","entrypoint":["/bin/sh"],"env":{},"ports":[],"allowedCudaVersions":[]}`
		}
		return 0, ""
	}

	for _, tc := range []struct {
		name     string
		answer   string
		warnings int
		check    func(t *testing.T, state templateResourceModel)
	}{
		{
			name:   "answered",
			answer: `{"data":{"podTemplate":{"readme":"# console","earned":1.5,"isRunpod":true,"runtimeInMin":120}}}`,
			check: func(t *testing.T, state templateResourceModel) {
				if state.Readme.ValueString() != "# console" || state.Earned.ValueFloat64() != 1.5 ||
					!state.IsRunpod.ValueBool() || state.RuntimeInMin.ValueInt64() != 120 {
					t.Errorf("shim attributes = %+v, want the GraphQL answer", state)
				}
			},
		},
		{
			// Not a failure: a template the GraphQL API does not report to the
			// caller nulls the read-backs silently.
			name:   "not visible",
			answer: `{"data":{"podTemplate":null}}`,
			check: func(t *testing.T, state templateResourceModel) {
				if !state.Earned.IsNull() || !state.IsRunpod.IsNull() || !state.RuntimeInMin.IsNull() {
					t.Errorf("read-backs = %+v, want null", state)
				}
			},
		},
		{
			name:     "failed",
			answer:   `{"errors":[{"message":"internal error"}]}`,
			warnings: 1,
			check: func(t *testing.T, state templateResourceModel) {
				if !state.Earned.IsNull() || !state.IsRunpod.IsNull() || !state.RuntimeInMin.IsNull() {
					t.Errorf("read-backs = %+v, want null after a failed read", state)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, recorder := newTemplateShimStub(t, v2, func(string, map[string]any) string { return tc.answer })
			s, raw := shimResourceSchema(ctx, t, r)
			prior := shimObject(ctx, t, s, raw, map[string]attr.Value{
				"id":             types.StringValue("tpl_abc"),
				"name":           types.StringValue("trainer"),
				"image":          types.StringValue("runpod/pytorch:1.0.2"),
				"earned":         types.Float64Value(9),
				"is_runpod":      types.BoolValue(false),
				"runtime_in_min": types.Int64Value(9),
			})

			resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: prior}}
			r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: prior}}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("a shim read must not fail the refresh: %v", resp.Diagnostics)
			}
			if got := resp.Diagnostics.WarningsCount(); got != tc.warnings {
				t.Fatalf("warnings = %d, want %d: %v", got, tc.warnings, resp.Diagnostics)
			}

			calls := recorder.Calls()
			if len(calls) != 2 || calls[0].Method != http.MethodGet || calls[0].Path != "/v2/templates/tpl_abc" ||
				calls[1].Query() != legacyTemplateFieldsQuery || calls[1].Variables()["id"] != "tpl_abc" {
				t.Fatalf("calls = %+v, want the v2 GET, then the GraphQL template query", calls)
			}

			var state templateResourceModel
			if diags := resp.State.Get(ctx, &state); diags.HasError() {
				t.Fatal(diags)
			}
			if got := listStrings(ctx, t, state.DockerEntrypoint); !slices.Equal(got, []string{"/bin/sh"}) {
				t.Errorf("docker_entrypoint = %v, want the v2 template's", got)
			}
			tc.check(t, state)
		})
	}
}

func TestTemplateUpdateSkipsTheReadmeWriteWhenUnconfigured(t *testing.T) {
	ctx := context.Background()

	// The shape terraform core sends on an update of a template that never set
	// readme or docker_entrypoint: the plan carries the prior state value for
	// both, the configuration carries neither. Neither may be written from
	// that; the shim only reads.
	r, recorder := newTemplateShimStub(t,
		func(method, path string) (int, string) {
			if method == http.MethodPatch && path == "/v2/templates/tpl_abc" {
				return http.StatusOK, v2TemplateRenamed
			}
			return 0, ""
		},
		func(query string, _ map[string]any) string {
			if query != legacyTemplateFieldsQuery {
				t.Errorf("unexpected GraphQL document: %s", query)
			}
			return `{"data":{"podTemplate":{"readme":"# stale","earned":0,"isRunpod":false,"runtimeInMin":0}}}`
		},
	)
	s, raw := shimResourceSchema(ctx, t, r)
	prior := map[string]attr.Value{
		"id":                types.StringValue("tpl_abc"),
		"name":              types.StringValue("trainer"),
		"image":             types.StringValue("runpod/pytorch:1.0.2"),
		"docker_entrypoint": stringList(ctx, t, "/bin/bash"),
		"readme":            types.StringValue("# stale"),
	}
	state := shimObject(ctx, t, s, raw, prior)
	prior["name"] = types.StringValue("renamed")
	plan := shimObject(ctx, t, s, raw, prior)
	config := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":  types.StringValue("renamed"),
		"image": types.StringValue("runpod/pytorch:1.0.2"),
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	patch := recorder.V2Call(t, http.MethodPatch)
	if patch.Body["name"] != "renamed" {
		t.Errorf("v2 PATCH = %v, want the rename", patch.Body)
	}
	if _, ok := patch.Body["entrypoint"]; ok {
		t.Errorf("an unconfigured docker_entrypoint must not be sent: %v", patch.Body)
	}
	if queries := recorder.GraphQLQueries(); len(queries) != 1 || queries[0] != legacyTemplateFieldsQuery {
		t.Fatalf("GraphQL documents = %v, want the read only, no saveTemplate", queries)
	}
}

func TestTemplateUpdateSendsTheEntrypointWithoutArgs(t *testing.T) {
	ctx := context.Background()

	// docker_entrypoint and args encode into the same stored command, which
	// the schema keeps from both being configured. An entrypoint configured
	// alone goes out without args, so the API keeps the stored command and
	// re-encodes it with the new entrypoint; args then reads back in the JSON
	// form it was re-encoded into.
	for _, tc := range []struct {
		name       string
		configured []string
		answer     string
		wantArgs   string
	}{
		{
			name:       "set",
			configured: []string{"/bin/bash", "-c"},
			answer: `{"id":"tpl_abc","name":"trainer","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,` +
				`"args":"{\"entrypoint\":[\"/bin/bash\",\"-c\"],\"cmd\":[\"python\",\"main.py\"]}",` +
				`"entrypoint":["/bin/bash","-c"],"cmd":["python","main.py"],"env":{},"ports":[],"allowedCudaVersions":[]}`,
			wantArgs: `{"entrypoint":["/bin/bash","-c"],"cmd":["python","main.py"]}`,
		},
		{
			// [] clears the entrypoint, and the API leaves a cleared one out
			// of the answer.
			name:       "cleared",
			configured: []string{},
			answer: `{"id":"tpl_abc","name":"trainer","image":"runpod/pytorch:1.0.2","category":"NVIDIA","disk":50,` +
				`"args":"python main.py","cmd":["python","main.py"],"env":{},"ports":[],"allowedCudaVersions":[]}`,
			wantArgs: "python main.py",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, recorder := newTemplateShimStub(t,
				func(method, path string) (int, string) {
					if method == http.MethodPatch && path == "/v2/templates/tpl_abc" {
						return http.StatusOK, tc.answer
					}
					return 0, ""
				},
				func(string, map[string]any) string {
					return `{"data":{"podTemplate":{"readme":null,"earned":0,"isRunpod":false,"runtimeInMin":0}}}`
				},
			)
			s, raw := shimResourceSchema(ctx, t, r)
			state := shimObject(ctx, t, s, raw, map[string]attr.Value{
				"id":                types.StringValue("tpl_abc"),
				"name":              types.StringValue("trainer"),
				"image":             types.StringValue("runpod/pytorch:1.0.2"),
				"args":              types.StringValue(`{"entrypoint":["/bin/sh"],"cmd":["python","main.py"]}`),
				"docker_entrypoint": stringList(ctx, t, "/bin/sh"),
			})
			// args is Computed and not configured, so the plan leaves it
			// unknown until the API answers.
			plan := shimObject(ctx, t, s, raw, map[string]attr.Value{
				"id":                types.StringValue("tpl_abc"),
				"name":              types.StringValue("trainer"),
				"image":             types.StringValue("runpod/pytorch:1.0.2"),
				"args":              types.StringUnknown(),
				"docker_entrypoint": stringList(ctx, t, tc.configured...),
			})
			config := shimObject(ctx, t, s, raw, map[string]attr.Value{
				"name":              types.StringValue("trainer"),
				"image":             types.StringValue("runpod/pytorch:1.0.2"),
				"docker_entrypoint": stringList(ctx, t, tc.configured...),
			})

			resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
			r.Update(ctx, resource.UpdateRequest{
				Plan:   tfsdk.Plan{Schema: s, Raw: plan},
				State:  tfsdk.State{Schema: s, Raw: state},
				Config: tfsdk.Config{Schema: s, Raw: config},
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}

			patch := recorder.V2Call(t, http.MethodPatch)
			if got := bodyStrings(t, patch.Body, "entrypoint"); !slices.Equal(got, tc.configured) {
				t.Errorf("v2 entrypoint = %v, want %v", got, tc.configured)
			}
			if _, ok := patch.Body["args"]; ok {
				t.Errorf("args must not go out beside the entrypoint: %v", patch.Body)
			}
			if queries := recorder.GraphQLQueries(); len(queries) != 1 || queries[0] != legacyTemplateFieldsQuery {
				t.Errorf("GraphQL documents = %v, want the read only", queries)
			}

			var saved templateResourceModel
			if diags := resp.State.Get(ctx, &saved); diags.HasError() {
				t.Fatal(diags)
			}
			if saved.DockerEntrypoint.IsNull() || !slices.Equal(listStrings(ctx, t, saved.DockerEntrypoint), tc.configured) {
				t.Errorf("docker_entrypoint = %v, want %v", saved.DockerEntrypoint, tc.configured)
			}
			if saved.Args.ValueString() != tc.wantArgs {
				t.Errorf("args = %q, want %q", saved.Args.ValueString(), tc.wantArgs)
			}
		})
	}
}

func TestTemplateDockerEntrypointConflictsWithArgs(t *testing.T) {
	ctx := context.Background()

	// REST v2 accepts args together with an entrypoint only when both describe
	// the same command. A bare args string is a CMD with no entrypoint, so
	// the 1.x shape of a start command in args plus docker_entrypoint would
	// be refused or rewritten by the API; the schema refuses it at plan time
	// instead.
	r := &templateResource{}
	s, raw := shimResourceSchema(ctx, t, r)
	attribute, ok := s.Attributes["docker_entrypoint"].(resourceschema.ListAttribute)
	if !ok {
		t.Fatalf("docker_entrypoint is %T, want a list attribute", s.Attributes["docker_entrypoint"])
	}

	for _, tc := range []struct {
		name      string
		set       map[string]attr.Value
		wantError bool
	}{
		{"both", map[string]attr.Value{
			"args": types.StringValue("python main.py"), "docker_entrypoint": stringList(ctx, t, "/bin/bash", "-c"),
		}, true},
		{"entrypoint alone", map[string]attr.Value{"docker_entrypoint": stringList(ctx, t, "/bin/bash", "-c")}, false},
		{"args alone", map[string]attr.Value{"args": types.StringValue("python main.py")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := tfsdk.Config{Schema: s, Raw: shimObject(ctx, t, s, raw, tc.set)}
			var entrypoint types.List
			if diags := config.GetAttribute(ctx, path.Root("docker_entrypoint"), &entrypoint); diags.HasError() {
				t.Fatal(diags)
			}

			var diags diag.Diagnostics
			for _, v := range attribute.Validators {
				resp := &validator.ListResponse{}
				v.ValidateList(ctx, validator.ListRequest{
					Path:           path.Root("docker_entrypoint"),
					PathExpression: path.MatchRoot("docker_entrypoint"),
					ConfigValue:    entrypoint,
					Config:         config,
				}, resp)
				diags.Append(resp.Diagnostics...)
			}
			if diags.HasError() != tc.wantError {
				t.Fatalf("error = %v, want %v: %v", diags.HasError(), tc.wantError, diags)
			}
		})
	}
}

func TestTemplateValidateConfigRejectsAnEmptyReadme(t *testing.T) {
	ctx := context.Background()
	r := &templateResource{}
	s, raw := shimResourceSchema(ctx, t, r)

	for _, tc := range []struct {
		name      string
		set       map[string]attr.Value
		wantError bool
	}{
		{"unset", map[string]attr.Value{}, false},
		{"text", map[string]attr.Value{"readme": types.StringValue("# trainer")}, false},
		{"empty", map[string]attr.Value{"readme": types.StringValue("")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &resource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: s, Raw: shimObject(ctx, t, s, raw, tc.set)},
			}, resp)
			if resp.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("error = %v, want %v: %v", resp.Diagnostics.HasError(), tc.wantError, resp.Diagnostics)
			}
			if !tc.wantError {
				return
			}
			withPath, ok := resp.Diagnostics.Errors()[0].(diag.DiagnosticWithPath)
			if !ok || !withPath.Path().Equal(path.Root("readme")) {
				t.Fatalf("error = %v, want it on readme", resp.Diagnostics.Errors()[0])
			}
		})
	}
}

func TestTemplateUpdateSavesStateOnLegacyWriteFailure(t *testing.T) {
	ctx := context.Background()

	// The v2 rename has already landed when saveTemplate fails. Returning
	// before State.Set would leave state saying name = "trainer" while the
	// platform says "renamed", and the next plan would see no rename to make.
	r, recorder := newTemplateShimStub(t,
		func(method, path string) (int, string) {
			if method == http.MethodPatch && path == "/v2/templates/tpl_abc" {
				return http.StatusOK, v2TemplateRenamed
			}
			return 0, ""
		},
		func(query string, _ map[string]any) string {
			if query == legacyTemplateForSaveQuery {
				return storedTemplate
			}
			return `{"errors":[{"message":"Template not found"}],"data":{"saveTemplate":null}}`
		},
	)
	s, raw := shimResourceSchema(ctx, t, r)
	state := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":     types.StringValue("tpl_abc"),
		"name":   types.StringValue("trainer"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("hi"),
	})
	plan := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":     types.StringValue("tpl_abc"),
		"name":   types.StringValue("renamed"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("hi"),
	})
	config := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":   types.StringValue("renamed"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("hi"),
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("a configured readme saveTemplate refused must be an error: %v", resp.Diagnostics)
	}
	if queries := recorder.GraphQLQueries(); len(queries) != 2 || queries[1] != legacySaveTemplateReadmeMutation {
		t.Fatalf("GraphQL documents = %v, want the read-for-save and saveTemplate", queries)
	}
	var saved templateResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Name.ValueString() != "renamed" {
		t.Fatalf("state name = %q, want the rename the v2 update already applied", saved.Name.ValueString())
	}
	if saved.Readme.ValueString() != "hi" {
		t.Fatalf("state readme = %v, want the configured value so the next plan converges", saved.Readme)
	}
}

func TestTemplateCreateKeepsReadmeOnLegacyWriteFailure(t *testing.T) {
	ctx := context.Background()

	// A template created through v2 that the GraphQL API does not report to
	// the caller: the read-for-save answers null, so no mutation goes out.
	// A null readme in state would put it at odds with the configuration on
	// every later plan, and every later apply would re-run this failing write.
	r, recorder := newTemplateShimStub(t,
		func(method, path string) (int, string) {
			if method == http.MethodPost && path == "/v2/templates" {
				return http.StatusCreated, v2TemplateTrainer
			}
			return 0, ""
		},
		func(string, map[string]any) string { return `{"data":{"podTemplate":null}}` },
	)
	s, raw := shimResourceSchema(ctx, t, r)
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":   types.StringValue("trainer"),
		"image":  types.StringValue("runpod/pytorch:1.0.2"),
		"readme": types.StringValue("# trainer"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)

	if queries := recorder.GraphQLQueries(); len(queries) != 1 || queries[0] != legacyTemplateForSaveQuery {
		t.Fatalf("GraphQL documents = %v, want the read-for-save only", queries)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the template exists in the account: a failed shim write must still write state")
	}
	var saved templateResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Id.ValueString() != "tpl_abc" {
		t.Fatalf("state id = %q, want tpl_abc", saved.Id.ValueString())
	}
	if saved.Readme.ValueString() != "# trainer" {
		t.Fatalf("state readme = %v, want the configured value so the next plan converges", saved.Readme)
	}
	if !saved.Earned.IsNull() || !saved.IsRunpod.IsNull() || !saved.RuntimeInMin.IsNull() {
		t.Fatalf("the read-backs the failed call never answered must be null: %+v", saved)
	}
	if !resp.Diagnostics.HasError() {
		t.Fatalf("a configured readme the GraphQL API cannot apply is an error: %v", resp.Diagnostics)
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "cannot be applied to this template") {
		t.Fatalf("the error must name the cause rather than relay a bare null: %q", detail)
	}
}

func TestTemplateCreateWithoutReadmeNeverWritesThroughGraphQL(t *testing.T) {
	ctx := context.Background()

	r, recorder := newTemplateShimStub(t,
		func(method, path string) (int, string) {
			if method == http.MethodPost && path == "/v2/templates" {
				return http.StatusCreated, v2TemplateTrainer
			}
			return 0, ""
		},
		func(query string, _ map[string]any) string {
			if query != legacyTemplateFieldsQuery {
				t.Errorf("unexpected GraphQL document: %s", query)
			}
			return `{"data":{"podTemplate":{"readme":null,"earned":0,"isRunpod":false,"runtimeInMin":0}}}`
		},
	)
	s, raw := shimResourceSchema(ctx, t, r)
	configured := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"name":  types.StringValue("trainer"),
		"image": types.StringValue("runpod/pytorch:1.0.2"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(raw.Type(), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: configured},
		Config: tfsdk.Config{Schema: s, Raw: configured},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := recorder.V2Call(t, http.MethodPost).Body["entrypoint"]; ok {
		t.Error("an unconfigured docker_entrypoint must not be sent")
	}
	if queries := recorder.GraphQLQueries(); len(queries) != 1 {
		t.Fatalf("GraphQL documents = %v, want the read only", queries)
	}
	var saved templateResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if !saved.DockerEntrypoint.IsNull() || !saved.Readme.IsNull() {
		t.Fatalf("state = %+v, want docker_entrypoint and readme null for a template with neither", saved)
	}
}

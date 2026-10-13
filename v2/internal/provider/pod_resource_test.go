package provider

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// docker_entrypoint and docker_start_cmd are the v2 pod's entrypoint and cmd,
// which the API decodes from args. These drive the resource's real Read and
// ImportState against a stub API that serves both hosts, the way
// legacy_shim_apply_test.go does, so what they pin is where the two lists
// come from.

// podCommandJSON is a v2 pod whose args carries an explicit entrypoint and
// cmd, reported back in both representations the way the API documents.
const podCommandJSON = `{"id":"pod_abc","name":"trainer","image":"runpod/pytorch:1.0.2","status":"RUNNING",` +
	`"createdAt":"2026-09-17T09:00:00Z","disk":50,"env":{},"ports":[],"dataCenterIds":[],"cost":0.35,` +
	`"args":"{\"entrypoint\":[\"/bin/bash\",\"-c\"],\"cmd\":[\"python -u main.py\"]}",` +
	`"entrypoint":["/bin/bash","-c"],"cmd":["python -u main.py"]}`

func assertPodCommandLists(ctx context.Context, t *testing.T, model podResourceModel) {
	t.Helper()
	var entrypoint, startCmd []string
	if diags := model.DockerEntrypoint.ElementsAs(ctx, &entrypoint, false); diags.HasError() {
		t.Fatal(diags)
	}
	if diags := model.DockerStartCmd.ElementsAs(ctx, &startCmd, false); diags.HasError() {
		t.Fatal(diags)
	}
	if !slices.Equal(entrypoint, []string{"/bin/bash", "-c"}) {
		t.Errorf("docker_entrypoint = %v, want the v2 pod's [/bin/bash -c]", entrypoint)
	}
	if !slices.Equal(startCmd, []string{"python -u main.py"}) {
		t.Errorf("docker_start_cmd = %v, want the v2 pod's [python -u main.py]", startCmd)
	}
}

func TestPodReadFillsTheCommandListsFromV2(t *testing.T) {
	ctx := context.Background()

	// The REST v1 pod answers different command lists on purpose: state has
	// to carry the v2 pod's, while machine_id still comes from v1.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/pods/pod_abc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(podCommandJSON))
		case r.Method == http.MethodGet && r.URL.Path == "/pods/pod_abc":
			_, _ = w.Write([]byte(`{"machineId":"machine-1","dockerEntrypoint":["/bin/sh"],"dockerStartCmd":["sleep","infinity"]}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &podResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)
	state := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":          types.StringValue("pod_abc"),
		"name":        types.StringValue("trainer"),
		"image":       types.StringValue("runpod/pytorch:1.0.2"),
		"gpu_type_id": types.StringValue("NVIDIA A100 80GB PCIe"),
	})

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: state}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}

	var saved podResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	assertPodCommandLists(ctx, t, saved)
	if saved.MachineId.ValueString() != "machine-1" {
		t.Errorf("machine_id = %v, want the REST v1 value", saved.MachineId)
	}
}

func TestPodImportFillsTheCommandLists(t *testing.T) {
	ctx := context.Background()

	// An import is ImportState followed by Read. The REST v1 route answers
	// 404, as it does for a pod it does not know: the command lists come from
	// the v2 read regardless, and only the shim attributes read null.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/pods/pod_abc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(podCommandJSON))
		case r.Method == http.MethodGet && r.URL.Path == "/pods/pod_abc":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := &podResource{
		client: shimTestClient(t, srv.URL),
		legacy: newTestLegacyClient(t, legacy.Config{APIKey: "test-key", RestV1URL: srv.URL}),
	}
	s, raw := shimResourceSchema(ctx, t, r)

	importResp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "pod_abc"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatal(importResp.Diagnostics)
	}

	readResp := &resource.ReadResponse{State: importResp.State}
	r.Read(ctx, resource.ReadRequest{State: importResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatal(readResp.Diagnostics)
	}
	if readResp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("a 404 from the REST v1 route is silent: %v", readResp.Diagnostics)
	}

	var saved podResourceModel
	if diags := readResp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Id.ValueString() != "pod_abc" || saved.Name.ValueString() != "trainer" {
		t.Fatalf("imported state = %+v, want the v2 pod", saved)
	}
	assertPodCommandLists(ctx, t, saved)
	if !saved.MachineId.IsNull() || !saved.VolumeEncrypted.IsNull() {
		t.Errorf("shim attributes = %v / %v, want null after the REST v1 404", saved.MachineId, saved.VolumeEncrypted)
	}
}

func TestPodCommandPlanModifierKeepsStateUnlessArgsChanges(t *testing.T) {
	ctx := context.Background()
	s, raw := shimResourceSchema(ctx, t, &podResource{})

	prior, diags := types.ListValueFrom(ctx, types.StringType, []string{"/bin/bash", "-c"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	stateArgs := types.StringValue(`{"entrypoint":["/bin/bash","-c"]}`)
	stateRaw := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":                types.StringValue("pod_abc"),
		"args":              stateArgs,
		"docker_entrypoint": prior,
	})
	// configWithArgs builds a configuration that sets args to value, which
	// may be unknown; a plan is the one data type that can hold that.
	configWithArgs := func(value types.String) tftypes.Value {
		config := tfsdk.Plan{Schema: s, Raw: raw}
		if diags := config.SetAttribute(ctx, path.Root("args"), value); diags.HasError() {
			t.Fatal(diags)
		}
		return config.Raw
	}

	for _, tc := range []struct {
		name      string
		state     tftypes.Value
		config    tftypes.Value
		planValue types.List
		wantState bool
	}{
		{
			name:  "create has no prior value to keep",
			state: tftypes.NewValue(raw.Type(), nil), config: raw,
			planValue: types.ListUnknown(types.StringType),
		},
		{
			name:  "an update that sends no args keeps the prior lists",
			state: stateRaw, config: raw,
			planValue: types.ListUnknown(types.StringType), wantState: true,
		},
		{
			name:  "an update that sends the same args keeps the prior lists",
			state: stateRaw, config: configWithArgs(stateArgs),
			planValue: types.ListUnknown(types.StringType), wantState: true,
		},
		{
			name:  "an update that sends a different args re-derives them",
			state: stateRaw, config: configWithArgs(types.StringValue("sleep infinity")),
			planValue: types.ListUnknown(types.StringType),
		},
		{
			name:  "an args not known until apply re-derives them",
			state: stateRaw, config: configWithArgs(types.StringUnknown()),
			planValue: types.ListUnknown(types.StringType),
		},
		{
			name:  "a known plan value is left alone",
			state: stateRaw, config: configWithArgs(types.StringValue("sleep infinity")),
			planValue: types.ListNull(types.StringType),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stateValue := types.ListNull(types.StringType)
			if !tc.state.IsNull() {
				stateValue = prior
			}
			resp := &planmodifier.ListResponse{PlanValue: tc.planValue}
			usePodCommandStateUnlessArgsChanges().PlanModifyList(ctx, planmodifier.ListRequest{
				Path:       path.Root("docker_entrypoint"),
				State:      tfsdk.State{Schema: s, Raw: tc.state},
				StateValue: stateValue,
				Config:     tfsdk.Config{Schema: s, Raw: tc.config},
				Plan:       tfsdk.Plan{Schema: s, Raw: stateRaw},
				PlanValue:  tc.planValue,
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}

			if tc.wantState {
				if !resp.PlanValue.Equal(prior) {
					t.Fatalf("planned %v, want the prior state value %v", resp.PlanValue, prior)
				}
				return
			}
			if !resp.PlanValue.Equal(tc.planValue) {
				t.Fatalf("planned %v, want the incoming %v left alone", resp.PlanValue, tc.planValue)
			}
		})
	}
}

func TestPodCommandListsAreReadOnlyAndNotDeprecated(t *testing.T) {
	ctx := context.Background()
	s, _ := shimResourceSchema(ctx, t, &podResource{})

	for _, name := range []string{"docker_entrypoint", "docker_start_cmd"} {
		attribute, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("the pod schema has no %q attribute", name)
		}
		if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("%s must be read-only: the API decodes it from args", name)
		}
		if attribute.GetDeprecationMessage() != "" {
			t.Errorf("%s is read from REST v2 and must not be deprecated: %q", name, attribute.GetDeprecationMessage())
		}
		if shimPlanModifierCount(attribute) != 1 {
			t.Errorf("%s must carry the command plan modifier", name)
		}
	}
}

// podEnvServer stands in for the v2 pod routes with the backend's env
// semantics: the PATCH replaces the pod's whole env with the one it is sent
// (podEditJob stores input.env as given), and an omitted env leaves it as it
// is. live is the env the pod carries before the update; patched receives the
// decoded PATCH body.
func podEnvServer(t *testing.T, live map[string]string, patched *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		podJSON := func(name string, env map[string]string) []byte {
			encoded, _ := json.Marshal(env)
			return []byte(`{"id":"pod_abc","name":"` + name + `","image":"runpod/pytorch:1.0.2","status":"RUNNING",` +
				`"createdAt":"2026-09-17T09:00:00Z","disk":50,"env":` + string(encoded) +
				`,"ports":["8888/http","22/tcp"],"dataCenterIds":[],"cost":0.35}`)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/pods/pod_abc":
			_, _ = w.Write(podJSON("trainer", live))
		case r.Method == http.MethodPatch && r.URL.Path == "/v2/pods/pod_abc":
			body := map[string]any{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decoding the PATCH body: %v", err)
			}
			*patched = body
			env := live
			if sent, ok := body["env"].(map[string]any); ok {
				env = map[string]string{}
				for key, value := range sent {
					env[key], _ = value.(string)
				}
			}
			name, _ := body["name"].(string)
			_, _ = w.Write(podJSON(name, env))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// provisionedPodEnv is a pod created from a template with start_ssh and
// start_jupyter: one key from the configuration, one from the template, and
// the two the provisioner injected.
func provisionedPodEnv() map[string]string {
	return map[string]string{
		"MODEL":            "mistral",
		"HF_HOME":          "/workspace/hf",
		"PUBLIC_KEY":       "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample user@host",
		"JUPYTER_PASSWORD": "generated-secret",
	}
}

// initPrivateState gives a response the empty private state the framework
// hands every resource call; its type sits in an internal package, so it is
// built by reflection. field is a pointer to the response's Private field.
func initPrivateState(t *testing.T, field any) {
	t.Helper()
	value := reflect.ValueOf(field).Elem()
	value.Set(reflect.New(value.Type().Elem()))
}

// updatePodEnv runs Update from a prior env to a planned one. priorConfigured
// is the private state flag a create or update with a configured env leaves;
// the flag after the update is returned with the saved state.
func updatePodEnv(ctx context.Context, t *testing.T, r *podResource, priorEnv, plannedEnv map[string]string, priorConfigured bool, plannedName string) (podResourceModel, bool) {
	t.Helper()
	s, raw := shimResourceSchema(ctx, t, r)
	envValue := func(env map[string]string) types.Map {
		value, diags := types.MapValueFrom(ctx, types.StringType, env)
		if diags.HasError() {
			t.Fatal(diags)
		}
		return value
	}
	state := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":          types.StringValue("pod_abc"),
		"name":        types.StringValue("trainer"),
		"image":       types.StringValue("runpod/pytorch:1.0.2"),
		"gpu_type_id": types.StringValue("NVIDIA A100 80GB PCIe"),
		"env":         envValue(priorEnv),
	})
	plan := shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":          types.StringValue("pod_abc"),
		"name":        types.StringValue(plannedName),
		"image":       types.StringValue("runpod/pytorch:1.0.2"),
		"gpu_type_id": types.StringValue("NVIDIA A100 80GB PCIe"),
		"env":         envValue(plannedEnv),
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	initPrivateState(t, &resp.Private)
	if priorConfigured {
		if diags := setEnvConfigured(ctx, resp.Private, true); diags.HasError() {
			t.Fatal(diags)
		}
	}
	r.Update(ctx, resource.UpdateRequest{
		Plan:    tfsdk.Plan{Schema: s, Raw: plan},
		State:   tfsdk.State{Schema: s, Raw: state},
		Config:  tfsdk.Config{Schema: s, Raw: plan},
		Private: resp.Private,
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var saved podResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	configured, diags := envConfigured(ctx, resp.Private)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return saved, configured
}

func TestPodRenameLeavesInheritedEnvAlone(t *testing.T) {
	ctx := context.Background()

	// A rename with an unchanged env used to send the configured keys as the
	// whole env, which removed the template's HF_HOME and the provisioned SSH
	// and Jupyter values from the pod.
	var patched map[string]any
	srv := podEnvServer(t, provisionedPodEnv(), &patched)
	r := &podResource{client: shimTestClient(t, srv.URL)}

	configured := map[string]string{"MODEL": "mistral"}
	saved, _ := updatePodEnv(ctx, t, r, configured, configured, true, "renamed")

	if _, sent := patched["env"]; sent {
		t.Errorf("PATCH body = %v, want no env when the configured env did not change", patched)
	}
	if patched["name"] != "renamed" {
		t.Errorf("PATCH body = %v, want the rename", patched)
	}
	var env map[string]string
	if diags := saved.Env.ElementsAs(ctx, &env, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(env) != 1 || env["MODEL"] != "mistral" {
		t.Errorf("state env = %v, want the configured keys only", env)
	}
}

func TestPodEnvChangeMergesOntoTheLiveEnv(t *testing.T) {
	ctx := context.Background()

	var patched map[string]any
	live := provisionedPodEnv()
	live["DEBUG"] = "1"
	srv := podEnvServer(t, live, &patched)
	r := &podResource{client: shimTestClient(t, srv.URL)}

	// MODEL changes, DEBUG is dropped from the configuration, and the
	// template and provisioner keys were never configured.
	updatePodEnv(ctx, t, r,
		map[string]string{"MODEL": "mistral", "DEBUG": "1"},
		map[string]string{"MODEL": "llama"},
		true, "trainer")

	sent, ok := patched["env"].(map[string]any)
	if !ok {
		t.Fatalf("PATCH body = %v, want an env", patched)
	}
	want := map[string]any{
		"MODEL":            "llama",
		"HF_HOME":          "/workspace/hf",
		"PUBLIC_KEY":       live["PUBLIC_KEY"],
		"JUPYTER_PASSWORD": "generated-secret",
	}
	if len(sent) != len(want) {
		t.Errorf("env sent = %v, want %v", sent, want)
	}
	for key, value := range want {
		if sent[key] != value {
			t.Errorf("env sent[%s] = %v, want %v", key, sent[key], value)
		}
	}
}

func TestMergeConfiguredEnvLetsTheConfigurationOwnProvisionedKeys(t *testing.T) {
	// A configured PUBLIC_KEY or JUPYTER_PASSWORD wins at create, where the
	// provisioner injects one only when the env has none, so it wins on an
	// update too: a new planned value is sent, and one dropped from the
	// configuration is removed.
	live := provisionedPodEnv()
	live["JUPYTER_PASSWORD"] = "mine"
	merged := mergeConfiguredEnv(live,
		map[string]string{"MODEL": "mistral", "JUPYTER_PASSWORD": "mine"},
		map[string]string{"MODEL": "llama", "PUBLIC_KEY": "ssh-ed25519 other"},
		true)

	want := map[string]string{
		"MODEL":      "llama",
		"HF_HOME":    "/workspace/hf",
		"PUBLIC_KEY": "ssh-ed25519 other",
	}
	if !maps.Equal(merged, want) {
		t.Errorf("merged = %v, want %v", merged, want)
	}
}

func TestMergeConfiguredEnvKeepsProvisionedKeysTheConfigurationNeverHeld(t *testing.T) {
	// A prior env read whole from the API, after an import or while env was
	// unset, holds the provisioner's two keys without the configuration ever
	// having set them, so leaving them out of the configuration keeps them.
	// Every other key the configuration leaves out is removed.
	live := provisionedPodEnv()
	merged := mergeConfiguredEnv(live, live, map[string]string{"MODEL": "llama"}, false)

	want := map[string]string{
		"MODEL":            "llama",
		"PUBLIC_KEY":       live["PUBLIC_KEY"],
		"JUPYTER_PASSWORD": "generated-secret",
	}
	if !maps.Equal(merged, want) {
		t.Errorf("merged = %v, want %v", merged, want)
	}
}

func TestPodUpdateRotatesAConfiguredPublicKey(t *testing.T) {
	ctx := context.Background()

	// The pod was created with a configured PUBLIC_KEY, which the provisioner
	// kept. Rotating it in the configuration has to reach the pod.
	var patched map[string]any
	live := provisionedPodEnv()
	live["PUBLIC_KEY"] = "ssh-ed25519 old-key"
	srv := podEnvServer(t, live, &patched)
	r := &podResource{client: shimTestClient(t, srv.URL)}

	saved, configured := updatePodEnv(ctx, t, r,
		map[string]string{"MODEL": "mistral", "PUBLIC_KEY": "ssh-ed25519 old-key"},
		map[string]string{"MODEL": "mistral", "PUBLIC_KEY": "ssh-ed25519 new-key"},
		true, "trainer")

	sent, ok := patched["env"].(map[string]any)
	if !ok {
		t.Fatalf("PATCH body = %v, want an env", patched)
	}
	want := map[string]any{
		"MODEL":            "mistral",
		"HF_HOME":          "/workspace/hf",
		"PUBLIC_KEY":       "ssh-ed25519 new-key",
		"JUPYTER_PASSWORD": "generated-secret",
	}
	if !maps.Equal(sent, want) {
		t.Errorf("env sent = %v, want %v", sent, want)
	}
	var env map[string]string
	if diags := saved.Env.ElementsAs(ctx, &env, false); diags.HasError() {
		t.Fatal(diags)
	}
	if env["PUBLIC_KEY"] != "ssh-ed25519 new-key" || len(env) != 2 {
		t.Errorf("state env = %v, want the configured keys with the new PUBLIC_KEY", env)
	}
	if !configured {
		t.Error("env_configured private state = false, want true after an update with a configured env")
	}
}

func TestPodImportThenAConfiguredSubsetSendsOnlyThoseKeysPlusTheProvisionedOnes(t *testing.T) {
	ctx := context.Background()

	var patched map[string]any
	srv := podEnvServer(t, provisionedPodEnv(), &patched)
	r := &podResource{client: shimTestClient(t, srv.URL)}
	s, raw := shimResourceSchema(ctx, t, r)

	importResp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "pod_abc"}, importResp)
	readResp := &resource.ReadResponse{State: importResp.State}
	initPrivateState(t, &readResp.Private)
	r.Read(ctx, resource.ReadRequest{State: importResp.State, Private: readResp.Private}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatal(readResp.Diagnostics)
	}
	var imported podResourceModel
	if diags := readResp.State.Get(ctx, &imported); diags.HasError() {
		t.Fatal(diags)
	}
	var importedEnv map[string]string
	if diags := imported.Env.ElementsAs(ctx, &importedEnv, false); diags.HasError() {
		t.Fatal(diags)
	}
	priorConfigured, diags := envConfigured(ctx, readResp.Private)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if priorConfigured {
		t.Fatal("env_configured private state = true after an import, want false")
	}

	// The first apply configures one key. The template's HF_HOME is in the
	// imported state but not the configuration, so it goes; the
	// provisioner's two keys stay.
	saved, configured := updatePodEnv(ctx, t, r, importedEnv, map[string]string{"MODEL": "llama"}, priorConfigured, "trainer")

	sent, ok := patched["env"].(map[string]any)
	if !ok {
		t.Fatalf("PATCH body = %v, want an env", patched)
	}
	want := map[string]any{
		"MODEL":            "llama",
		"PUBLIC_KEY":       provisionedPodEnv()["PUBLIC_KEY"],
		"JUPYTER_PASSWORD": "generated-secret",
	}
	if !maps.Equal(sent, want) {
		t.Errorf("env sent = %v, want %v", sent, want)
	}
	var env map[string]string
	if diags := saved.Env.ElementsAs(ctx, &env, false); diags.HasError() {
		t.Fatal(diags)
	}
	if !maps.Equal(env, map[string]string{"MODEL": "llama"}) {
		t.Errorf("state env = %v, want the configured key only, as planned", env)
	}
	if !configured {
		t.Error("env_configured private state = false, want true once env is configured")
	}
}

func TestPodImportFillsTheEnv(t *testing.T) {
	ctx := context.Background()

	var patched map[string]any
	srv := podEnvServer(t, provisionedPodEnv(), &patched)
	r := &podResource{client: shimTestClient(t, srv.URL)}
	s, raw := shimResourceSchema(ctx, t, r)

	importResp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "pod_abc"}, importResp)
	readResp := &resource.ReadResponse{State: importResp.State}
	r.Read(ctx, resource.ReadRequest{State: importResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatal(readResp.Diagnostics)
	}

	var saved podResourceModel
	if diags := readResp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatal(diags)
	}
	var env map[string]string
	if diags := saved.Env.ElementsAs(ctx, &env, false); diags.HasError() {
		t.Fatal(diags)
	}
	if saved.Env.IsNull() || len(env) != len(provisionedPodEnv()) || env["HF_HOME"] != "/workspace/hf" {
		t.Errorf("imported env = %v, want the pod's env", saved.Env)
	}
}

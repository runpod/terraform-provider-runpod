package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// graphQLCall is one request a recordingGraphQLStub received.
type graphQLCall struct {
	Method    string
	Path      string
	Query     string
	Variables map[string]any
}

// graphQLRecorder holds the requests a recordingGraphQLStub received. The
// handler goroutine and the test goroutine both touch it, so every access
// goes through the mutex.
type graphQLRecorder struct {
	mu    sync.Mutex
	calls []graphQLCall
}

func (r *graphQLRecorder) record(call graphQLCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

// Calls returns a copy of every request received so far.
func (r *graphQLRecorder) Calls() []graphQLCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]graphQLCall(nil), r.calls...)
}

// recordingGraphQLStub serves the GraphQL endpoint at /graphql, answers each
// request with what answer returns, and records the method, path, document
// and variables, so a test can assert the operation sent and that nothing
// reached any other path. A request to any other path fails the test.
func recordingGraphQLStub(t *testing.T, answer func(query string, variables map[string]any) string) (*legacy.Client, *graphQLRecorder) {
	t.Helper()
	recorder := &graphQLRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decoding the GraphQL request: %v", err)
		}
		recorder.record(graphQLCall{Method: r.Method, Path: r.URL.Path, Query: request.Query, Variables: request.Variables})
		if r.URL.Path != "/graphql" {
			t.Errorf("request to %s %s, want only POST /graphql", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer(request.Query, request.Variables)))
	}))
	t.Cleanup(srv.Close)
	// A plain client: the retrying default would back off for seconds on the
	// failure cases.
	client := newTestLegacyClient(t, legacy.Config{
		APIKey:     "test-key",
		GraphQLURL: srv.URL + "/graphql",
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	})
	return client, recorder
}

// assertOnlyGraphQL fails the test unless every recorded request was a POST
// to /graphql.
func assertOnlyGraphQL(t *testing.T, recorder *graphQLRecorder) {
	t.Helper()
	for _, call := range recorder.Calls() {
		if call.Method != http.MethodPost || call.Path != "/graphql" {
			t.Errorf("request %s %s, want POST /graphql", call.Method, call.Path)
		}
		if strings.Contains(call.Path, "/v1/") {
			t.Errorf("request reached a REST v1 path: %s", call.Path)
		}
	}
}

func TestFetchLegacyPodFieldsSendsThePodQuery(t *testing.T) {
	client, recorder := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"pod":{"machineId":"m-1","volumeEncrypted":true}}}`
	})

	pod, err := fetchLegacyPodFields(context.Background(), client, "pod-1")
	if err != nil {
		t.Fatal(err)
	}
	if pod == nil || pod.MachineID == nil || *pod.MachineID != "m-1" || pod.VolumeEncrypted == nil || !*pod.VolumeEncrypted {
		t.Fatalf("pod = %+v, want machineId m-1 and volumeEncrypted true", pod)
	}

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if calls[0].Query != legacyPodFieldsQuery || calls[0].Variables["podId"] != "pod-1" {
		t.Errorf("sent %q with %v", calls[0].Query, calls[0].Variables)
	}
	assertOnlyGraphQL(t, recorder)
}

func TestFetchLegacyPodFieldsReportsAnUnknownPodAsNil(t *testing.T) {
	client, _ := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"pod":null}}`
	})

	pod, err := fetchLegacyPodFields(context.Background(), client, "pod-1")
	if err != nil || pod != nil {
		t.Fatalf("pod = %+v, err = %v; want nil, nil", pod, err)
	}
}

func TestFetchLegacyPodFieldsSurfacesGraphQLErrors(t *testing.T) {
	client, _ := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"errors":[{"message":"pod not found"}],"data":null}`
	})

	if _, err := fetchLegacyPodFields(context.Background(), client, "pod-1"); err == nil || !strings.Contains(err.Error(), "pod not found") {
		t.Fatalf("err = %v, want the GraphQL error", err)
	}
}

func TestFetchLegacyTemplateFieldsSendsTheTemplateQuery(t *testing.T) {
	client, recorder := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"podTemplate":{"readme":"# hi","earned":1.5,"isRunpod":false,"runtimeInMin":90}}}`
	})

	template, err := fetchLegacyTemplateFields(context.Background(), client, "tpl-1")
	if err != nil {
		t.Fatal(err)
	}
	if template == nil || *template.Readme != "# hi" || *template.Earned != 1.5 || *template.IsRunpod || *template.RuntimeInMin != 90 {
		t.Fatalf("template = %+v", template)
	}
	calls := recorder.Calls()
	if len(calls) != 1 || calls[0].Query != legacyTemplateFieldsQuery || calls[0].Variables["id"] != "tpl-1" {
		t.Fatalf("calls = %+v", calls)
	}
	assertOnlyGraphQL(t, recorder)
}

func TestFetchLegacyTemplateFieldsReportsAnInvisibleTemplateAsNil(t *testing.T) {
	client, _ := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"podTemplate":null}}`
	})

	template, err := fetchLegacyTemplateFields(context.Background(), client, "tpl-1")
	if err != nil || template != nil {
		t.Fatalf("template = %+v, err = %v; want nil, nil", template, err)
	}
}

func TestFetchLegacyEndpointOwner(t *testing.T) {
	owner := "user-1"
	for name, tc := range map[string]struct {
		answer string
		want   *string
	}{
		"owner":       {answer: `{"data":{"myself":{"endpoint":{"userId":"user-1"}}}}`, want: &owner},
		"no endpoint": {answer: `{"data":{"myself":{"endpoint":null}}}`},
		"no owner":    {answer: `{"data":{"myself":{"endpoint":{"userId":null}}}}`},
	} {
		t.Run(name, func(t *testing.T) {
			client, recorder := recordingGraphQLStub(t, func(string, map[string]any) string { return tc.answer })

			got, err := fetchLegacyEndpointOwner(context.Background(), client, "ep-1")
			if err != nil {
				t.Fatal(err)
			}
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("owner = %v, want %v", got, tc.want)
			}
			calls := recorder.Calls()
			if len(calls) != 1 || calls[0].Query != legacyEndpointOwnerQuery || calls[0].Variables["id"] != "ep-1" {
				t.Fatalf("calls = %+v", calls)
			}
			assertOnlyGraphQL(t, recorder)
		})
	}
}

// storedTemplate is a podTemplate answer to legacyTemplateForSaveQuery.
const storedTemplate = `{"data":{"podTemplate":{
  "id":"tpl-1","name":"trainer","containerDiskInGb":20,"volumeInGb":0,"volumeMountPath":"/data",
  "dockerArgs":"python serve.py","env":[{"key":"MODEL","value":"llama"},{"key":"EMPTY","value":null}],
  "ports":"8888/http","portsConfig":[{"port":"8888","name":"jupyter"}],
  "startJupyter":true,"startSsh":false,"startScript":null,"advancedStart":false,
  "isPublic":false,"isServerless":true,"category":"CPU","config":{"templateId":"tpl-1"},
  "containerRegistryAuthId":"auth-1","recommendedGPUIds":null,"incompatibleGPUIds":null,
  "compatibleGPUIds":null,"minVram":null,"minRam":null,"allowedCudaVersions":["12.4"],
  "networkVolumeIds":null}}}`

func TestSaveLegacyTemplateReadmeSendsTheTemplateBackWithTheNewReadme(t *testing.T) {
	client, recorder := recordingGraphQLStub(t, func(query string, _ map[string]any) string {
		if query == legacyTemplateForSaveQuery {
			return storedTemplate
		}
		return `{"data":{"saveTemplate":{"readme":"# new","earned":0,"isRunpod":false,"runtimeInMin":0}}}`
	})

	saved, err := saveLegacyTemplateReadme(context.Background(), client, "tpl-1", "# new")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Readme == nil || *saved.Readme != "# new" {
		t.Fatalf("saved = %+v, want the new readme read back", saved)
	}

	calls := recorder.Calls()
	if len(calls) != 2 || calls[0].Query != legacyTemplateForSaveQuery || calls[1].Query != legacySaveTemplateReadmeMutation {
		t.Fatalf("want the read then the mutation, got %+v", calls)
	}
	assertOnlyGraphQL(t, recorder)

	input, ok := calls[1].Variables["input"].(map[string]any)
	if !ok {
		t.Fatalf("input = %T", calls[1].Variables["input"])
	}
	// Every field the resolver would default when absent is echoed as read.
	want := map[string]any{
		"id": "tpl-1", "readme": "# new", "name": "trainer", "containerDiskInGb": float64(20),
		"volumeInGb": float64(0), "volumeMountPath": "/data", "dockerArgs": "python serve.py",
		"category": "CPU", "isServerless": true, "ports": "8888/http", "containerRegistryAuthId": "auth-1",
	}
	for key, value := range want {
		if input[key] != value {
			t.Errorf("input[%q] = %v, want %v", key, input[key], value)
		}
	}
	if config, ok := input["config"].(map[string]any); !ok || config["templateId"] != "tpl-1" {
		t.Errorf("config = %v, want the stored config echoed", input["config"])
	}
	if env, ok := input["env"].([]any); !ok || len(env) != 2 {
		t.Errorf("env = %v, want both variables, a null value sent as empty", input["env"])
	}
	if _, ok := input["imageName"]; ok {
		t.Error("imageName must stay out of the input, or the resolver re-verifies the image")
	}
	for _, key := range []string{"startScript", "recommendedGPUIds", "minVram"} {
		if _, ok := input[key]; ok {
			t.Errorf("%s is null on the template and must not be sent", key)
		}
	}
}

func TestSaveLegacyTemplateReadmeRefusesWhatWouldChangeTheTemplate(t *testing.T) {
	for name, stored := range map[string]string{
		"no category":      strings.Replace(storedTemplate, `"category":"CPU"`, `"category":null`, 1),
		"unknown category": strings.Replace(storedTemplate, `"category":"CPU"`, `"category":"TPU"`, 1),
		"no disk size":     strings.Replace(storedTemplate, `"containerDiskInGb":20`, `"containerDiskInGb":null`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			client, recorder := recordingGraphQLStub(t, func(string, map[string]any) string { return stored })

			if _, err := saveLegacyTemplateReadme(context.Background(), client, "tpl-1", "# new"); err == nil {
				t.Fatal("want an error")
			}
			if calls := recorder.Calls(); len(calls) != 1 {
				t.Fatalf("calls = %d, want the read only, no mutation", len(calls))
			}
		})
	}
}

func TestSaveLegacyTemplateReadmeReportsAnInvisibleTemplate(t *testing.T) {
	client, _ := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"podTemplate":null}}`
	})

	_, err := saveLegacyTemplateReadme(context.Background(), client, "tpl-1", "# new")
	if !errors.Is(err, errLegacyTemplateNotVisible) {
		t.Fatalf("err = %v, want errLegacyTemplateNotVisible", err)
	}
}

func TestFetchLegacyServerlessBilledTimeSendsTheBillingQuery(t *testing.T) {
	client, recorder := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"myself":{"billing":{"serverless":[{"endpointId":"ep_a","time":"2026-06-01T00:00:00.000Z","timeBilledSeconds":90}]}}}}`
	})

	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	rows, err := fetchLegacyServerlessBilledTime(context.Background(), client, "DAILY", start, start.AddDate(0, 0, 7))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].EndpointID == nil || *rows[0].EndpointID != "ep_a" || rows[0].Time == nil ||
		!rows[0].Time.Equal(start) || rows[0].TimeBilledSeconds == nil || *rows[0].TimeBilledSeconds != 90 {
		t.Fatalf("rows = %+v, want ep_a, 2026-06-01, 90 s", rows)
	}

	assertOnlyGraphQL(t, recorder)
	calls := recorder.Calls()
	if len(calls) != 1 || calls[0].Query != legacyServerlessBilledTimeQuery {
		t.Fatalf("calls = %+v, want one legacyServerlessBilledTimeQuery", calls)
	}
	input, ok := calls[0].Variables["input"].(map[string]any)
	if !ok || input["granularity"] != "DAILY" || input["startDate"] != "2026-06-01T00:00:00Z" ||
		input["endDate"] != "2026-06-08T00:00:00Z" {
		t.Errorf("variables = %v, want input {DAILY, 2026-06-01T00:00:00Z, 2026-06-08T00:00:00Z}", calls[0].Variables)
	}
}

func TestFetchLegacyServerlessBilledTimeTreatsWithheldBillingAsNoRows(t *testing.T) {
	// User.billing answers null for a caller that may not read the account's
	// billing: no rows, not an error.
	client, _ := recordingGraphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"myself":{"billing":null}}}`
	})

	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	rows, err := fetchLegacyServerlessBilledTime(context.Background(), client, "HOURLY", start, start.Add(time.Hour))
	if err != nil || rows != nil {
		t.Fatalf("rows = %+v, err = %v, want no rows and no error", rows, err)
	}
}

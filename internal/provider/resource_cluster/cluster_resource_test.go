package resource_cluster

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func clusterConfig(t *testing.T, m ClusterModel) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	sch := ClusterResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if diags := st.Set(ctx, &m); diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}
	return tfsdk.Config{Schema: sch, Raw: st.Raw}
}

func clusterState(t *testing.T, m ClusterModel) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	sch := ClusterResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if diags := st.Set(ctx, &m); diags.HasError() {
		t.Fatalf("building state: %v", diags)
	}
	return st
}

func configureClusterResource(t *testing.T, r *ClusterResource, srv *httptest.Server) {
	t.Helper()
	req := resource.ConfigureRequest{
		ProviderData: client.NewRunPodClient("test-key", srv.URL, ""),
	}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to configure resource: %v", resp.Diagnostics)
	}
}

func baseClusterModel() ClusterModel {
	return ClusterModel{
		Pods:          []ClusterPodModel{},
		DataCenterIds: types.ListNull(types.StringType),
		Env:           types.MapNull(types.StringType),
	}
}

func TestClusterCreate_SendsMutationAndSetsID(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"createCluster":{"id":"cl-1","name":"baseline","type":"SLURM","podCount":2}}}`))
	}))
	defer srv.Close()

	r := &ClusterResource{}
	configureClusterResource(t, r, srv)

	m := baseClusterModel()
	m.GpuTypeId = types.StringValue("NVIDIA H100 80GB HBM3")
	m.PodCount = types.Int64Value(2)
	m.GpuCountPerPod = types.Int64Value(8)
	m.Type = types.StringValue("SLURM")
	m.DeployCost = types.Float64Value(80.0)
	m.StartSsh = types.BoolValue(true)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: ClusterResourceSchema(context.Background())}}
	r.Create(context.Background(), resource.CreateRequest{Config: clusterConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diags: %v", resp.Diagnostics)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth header = %q, want Bearer test-key", gotAuth)
	}
	query, _ := gotBody["query"].(string)
	if !strings.Contains(query, "createCluster") {
		t.Fatalf("query missing createCluster: %q", query)
	}
	variables, _ := gotBody["variables"].(map[string]interface{})
	input, _ := variables["input"].(map[string]interface{})
	if input["gpuTypeId"] != "NVIDIA H100 80GB HBM3" {
		t.Fatalf("gpuTypeId = %v", input["gpuTypeId"])
	}
	if input["podCount"] != float64(2) || input["gpuCountPerPod"] != float64(8) {
		t.Fatalf("counts = %v/%v", input["podCount"], input["gpuCountPerPod"])
	}
	// clusterName should be absent when unset, not an empty string.
	if _, present := input["clusterName"]; present {
		t.Fatalf("clusterName present when null: %v", input["clusterName"])
	}
	if input["startSsh"] != true {
		t.Fatalf("startSsh = %v, want true", input["startSsh"])
	}

	var got ClusterModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if got.Id.ValueString() != "cl-1" {
		t.Fatalf("id = %q", got.Id.ValueString())
	}
	if got.Name.ValueString() != "baseline" {
		t.Fatalf("name = %q", got.Name.ValueString())
	}
	if len(got.Pods) != 0 {
		t.Fatalf("pods at create should be empty, got %d", len(got.Pods))
	}
}

func TestClusterCreate_EnvAndDataCenters_Converted(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"createCluster":{"id":"cl-2","name":"x","type":"SLURM","podCount":1}}}`))
	}))
	defer srv.Close()

	r := &ClusterResource{}
	configureClusterResource(t, r, srv)

	m := baseClusterModel()
	m.GpuTypeId = types.StringValue("NVIDIA A40")
	m.PodCount = types.Int64Value(1)
	m.GpuCountPerPod = types.Int64Value(1)
	m.Type = types.StringValue("SLURM")
	m.DataCenterIds, _ = types.ListValue(types.StringType, []attr.Value{
		types.StringValue("AP-IN-2"), types.StringValue("EUR-IS-3"),
	})
	m.Env, _ = types.MapValue(types.StringType, map[string]attr.Value{
		"FOO": types.StringValue("bar"),
		"BAZ": types.StringValue("qux"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: ClusterResourceSchema(context.Background())}}
	r.Create(context.Background(), resource.CreateRequest{Config: clusterConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diags: %v", resp.Diagnostics)
	}
	input, _ := gotBody["variables"].(map[string]interface{})["input"].(map[string]interface{})

	dcs, _ := input["dataCenterIds"].([]interface{})
	if len(dcs) != 2 || dcs[0] != "AP-IN-2" || dcs[1] != "EUR-IS-3" {
		t.Fatalf("dataCenterIds = %v", dcs)
	}

	envList, _ := input["env"].([]interface{})
	if len(envList) != 2 {
		t.Fatalf("env length = %d, want 2", len(envList))
	}
	gotEnv := map[string]string{}
	for _, e := range envList {
		em, _ := e.(map[string]interface{})
		gotEnv[em["key"].(string)] = em["value"].(string)
	}
	if gotEnv["FOO"] != "bar" || gotEnv["BAZ"] != "qux" {
		t.Fatalf("env conversion = %v", gotEnv)
	}
}

func TestClusterRead_NotFound_RemovesResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"myself":{"cluster":null}}}`))
	}))
	defer srv.Close()

	r := &ClusterResource{}
	configureClusterResource(t, r, srv)

	m := baseClusterModel()
	m.Id = types.StringValue("cl-gone")
	m.Name = types.StringValue("old")
	m.GpuTypeId = types.StringValue("NVIDIA A40")
	m.PodCount = types.Int64Value(1)
	m.GpuCountPerPod = types.Int64Value(1)
	m.Type = types.StringValue("SLURM")

	resp := &resource.ReadResponse{State: clusterState(t, m)}
	r.Read(context.Background(), resource.ReadRequest{State: clusterState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diags: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected resource to be removed from state on null cluster")
	}
}

func TestClusterRead_PopulatesPods(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"myself":{"cluster":{
			"id":"cl-1","name":"baseline","type":"SLURM","podCount":2,"gpuCountPerPod":8,
			"pods":[
				{"id":"pod-0","name":"baseline-pod-0","clusterIdx":0,"clusterRole":"SLURM_CONTROLLER","clusterIp":"10.65.0.2","desiredStatus":"RUNNING"},
				{"id":"pod-1","name":"baseline-pod-1","clusterIdx":1,"clusterRole":"SLURM_COMPUTE","clusterIp":"10.65.0.3","desiredStatus":"RUNNING"}
			]
		}}}}`))
	}))
	defer srv.Close()

	r := &ClusterResource{}
	configureClusterResource(t, r, srv)

	m := baseClusterModel()
	m.Id = types.StringValue("cl-1")
	m.Name = types.StringValue("baseline")
	m.GpuTypeId = types.StringValue("NVIDIA H100 80GB HBM3")
	m.PodCount = types.Int64Value(2)
	m.GpuCountPerPod = types.Int64Value(8)
	m.Type = types.StringValue("SLURM")

	resp := &resource.ReadResponse{State: clusterState(t, m)}
	r.Read(context.Background(), resource.ReadRequest{State: clusterState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diags: %v", resp.Diagnostics)
	}
	var got ClusterModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if len(got.Pods) != 2 {
		t.Fatalf("pods = %d, want 2", len(got.Pods))
	}
	if got.Pods[0].ClusterRole.ValueString() != "SLURM_CONTROLLER" || got.Pods[0].ClusterIp.ValueString() != "10.65.0.2" {
		t.Fatalf("pod0 = %+v", got.Pods[0])
	}
	if got.Pods[1].ClusterIdx.ValueInt64() != 1 {
		t.Fatalf("pod1 idx = %v", got.Pods[1].ClusterIdx)
	}
}

func TestClusterDelete_SendsMutation(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), "deleteCluster") {
			t.Errorf("delete body missing deleteCluster")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"deleteCluster":true}}`))
	}))
	defer srv.Close()

	r := &ClusterResource{}
	configureClusterResource(t, r, srv)

	m := baseClusterModel()
	m.Id = types.StringValue("cl-1")

	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: clusterState(t, m)}, resp)

	if !called {
		t.Fatal("delete mutation was not called")
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diags: %v", resp.Diagnostics)
	}
}

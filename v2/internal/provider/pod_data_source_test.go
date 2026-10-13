package provider

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// assertDataSourceModelFits runs a flattened model through the same conversion
// the framework performs in State.Set: every schema attribute must find a
// tfsdk-tagged field of a convertible type. It catches a model that has drifted
// from its schema without needing a live Terraform run.
func assertDataSourceModelFits(t *testing.T, newDataSource func() datasource.DataSource, model any) {
	t.Helper()
	ctx := context.Background()

	var schemaResp datasource.SchemaResponse
	newDataSource().Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResp.Diagnostics)
	}
	objectType, ok := schemaResp.Schema.Type().(types.ObjectType)
	if !ok {
		t.Fatalf("schema type = %T, want types.ObjectType", schemaResp.Schema.Type())
	}
	if _, diags := types.ObjectValueFrom(ctx, objectType.AttributeTypes(), model); diags.HasError() {
		t.Fatalf("model does not fit the schema: %v", diags)
	}
}

func TestFlattenPodDetailGpuPod(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Date(2026, 3, 13, 20, 0, 0, 0, time.UTC)
	startedAt := createdAt.Add(time.Minute)
	gpuCount := 2
	uptime := 3600
	util := 42
	memoryUtil := 17
	privatePort := 8888
	publicPort := 40000
	portIp := "213.0.0.1"
	portType := "http"
	rank := 0
	role := sdktypes.PodClusterRole("controller")
	networkIp := "10.65.1.42"
	internalDns := "pod_abc123.runpod.internal"
	cuda := "12.8"
	template := "tpl_abc"
	registry := "reg_abc"
	dataCenter := "US-TX-3"

	pod := sdktypes.Pod{
		Id:          "pod_abc123",
		Name:        "my-training-pod",
		Image:       "runpod/pytorch:2.8.0",
		Status:      sdktypes.PodStatus("RUNNING"),
		Cloud:       sdktypes.Cloud("SECURE"),
		Disk:        50,
		Args:        "--verbose",
		Cost:        0.5,
		Locked:      true,
		CudaVersion: &cuda,
		Template:    &template,
		Registry:    &registry,
		Actions:     []sdktypes.PodAction{sdktypes.PodAction("STOP"), sdktypes.PodAction("RESET")},
		Env:         map[string]string{"JUPYTER_PASSWORD": "changeme"},
		Ports:       []string{"8888/http", "22/tcp"},
		CreatedAt:   createdAt,
		StartedAt:   &startedAt,
		GlobalNetworking: sdktypes.PodGlobalNetworking{
			Enabled:     true,
			Ip:          &networkIp,
			InternalDns: &internalDns,
		},
		DataCenterId: &dataCenter,
		Gpu: &sdktypes.GpuConfig{
			Id:        "NVIDIA GeForce RTX 4090",
			Count:     &gpuCount,
			Memory:    48,
			VcpuCount: 16,
		},
		Cluster: &sdktypes.PodCluster{
			Id:   "cluster_abc123",
			Ip:   &networkIp,
			Rank: &rank,
			Role: &role,
		},
		Mounts: sdktypes.Mounts{
			Network: &[]sdktypes.NetworkMount{{Path: "/runpod-volume", VolumeId: "vol_xyz"}},
		},
		Runtime: &sdktypes.PodRuntime{
			Uptime: &uptime,
			Cpu:    &sdktypes.Utilization{Util: &util},
			Memory: &sdktypes.Utilization{Util: &memoryUtil},
			Gpus:   &[]sdktypes.PodGpuUtilization{{Util: &util, MemoryUtil: &memoryUtil}},
			Ports: &[]sdktypes.PodRuntimePort{{
				Ip:      &portIp,
				Private: &privatePort,
				Public:  &publicPort,
				Type:    &portType,
			}},
		},
		Ssh: sdktypes.PodSsh{
			Proxy: &sdktypes.PodSshEndpoint{
				Command:  "ssh pod_abc123@ssh.runpod.io",
				Host:     "ssh.runpod.io",
				Port:     22,
				Username: "pod_abc123",
			},
		},
	}

	model, diags := flattenPodDetail(ctx, pod)
	if diags.HasError() {
		t.Fatal(diags)
	}

	if model.Id.ValueString() != "pod_abc123" || model.Name.ValueString() != "my-training-pod" {
		t.Fatalf("id/name: %+v", model)
	}
	if model.Status.ValueString() != "RUNNING" || model.Cloud.ValueString() != "SECURE" {
		t.Fatalf("status/cloud: %+v", model)
	}
	if model.Cost.ValueFloat64() < 0.49 || model.Cost.ValueFloat64() > 0.51 {
		t.Fatalf("cost = %v, want ~0.5", model.Cost.ValueFloat64())
	}
	if !model.Locked.ValueBool() || model.CudaVersion.ValueString() != "12.8" {
		t.Fatalf("locked/cuda_version: %+v", model)
	}
	if model.CreatedAt.ValueString() != "2026-03-13T20:00:00Z" {
		t.Fatalf("created_at = %q", model.CreatedAt.ValueString())
	}
	if model.StartedAt.ValueString() != "2026-03-13T20:01:00Z" {
		t.Fatalf("started_at = %q", model.StartedAt.ValueString())
	}
	if model.GpuTypeId.ValueString() != "NVIDIA GeForce RTX 4090" || model.GpuCount.ValueInt64() != 2 {
		t.Fatalf("gpu: %+v", model)
	}
	if model.GpuMemory.ValueFloat64() != 48 || model.GpuVcpuCount.ValueFloat64() != 16 {
		t.Fatalf("gpu sizing: %+v", model)
	}
	if !model.CpuTypeId.IsNull() || !model.CpuMemory.IsNull() || !model.CpuVcpuCount.IsNull() {
		t.Fatalf("cpu fields must be null on a GPU pod: %+v", model)
	}
	if !model.GlobalNetworking.ValueBool() ||
		model.GlobalNetworkingIp.ValueString() != "10.65.1.42" ||
		model.GlobalNetworkingInternalDns.ValueString() != "pod_abc123.runpod.internal" {
		t.Fatalf("global networking: %+v", model)
	}
	if got := model.Actions.Elements(); len(got) != 2 {
		t.Fatalf("actions = %v, want 2 entries", got)
	}
	if model.Cluster == nil || model.Cluster.Id.ValueString() != "cluster_abc123" ||
		model.Cluster.Rank.ValueInt64() != 0 || model.Cluster.Role.ValueString() != "controller" {
		t.Fatalf("cluster: %+v", model.Cluster)
	}
	if model.Mounts == nil || model.Mounts.Persistent != nil || len(model.Mounts.Network) != 1 ||
		model.Mounts.Network[0].VolumeId.ValueString() != "vol_xyz" ||
		model.Mounts.Network[0].Path.ValueString() != "/runpod-volume" {
		t.Fatalf("mounts: %+v", model.Mounts)
	}
	// The runtime block is the object-valued shape the runpod_pod resource
	// reports too: one shared model and one shared flatten.
	var runtime podRuntimeModel
	if d := model.Runtime.As(ctx, &runtime, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatal(d)
	}
	if runtime.UptimeSeconds.ValueInt64() != 3600 || runtime.CpuUtil.ValueInt64() != 42 ||
		runtime.MemoryUtil.ValueInt64() != 17 {
		t.Fatalf("runtime: %+v", runtime)
	}
	var runtimeGpus []podGpuUtilizationModel
	if d := runtime.Gpus.ElementsAs(ctx, &runtimeGpus, false); d.HasError() {
		t.Fatal(d)
	}
	if len(runtimeGpus) != 1 || runtimeGpus[0].Util.ValueInt64() != 42 {
		t.Fatalf("runtime gpus: %+v", runtimeGpus)
	}
	var runtimePorts []podRuntimePortModel
	if d := runtime.Ports.ElementsAs(ctx, &runtimePorts, false); d.HasError() {
		t.Fatal(d)
	}
	if len(runtimePorts) != 1 || runtimePorts[0].PublicPort.ValueInt64() != 40000 ||
		runtimePorts[0].PrivatePort.ValueInt64() != 8888 ||
		runtimePorts[0].Type.ValueString() != "http" {
		t.Fatalf("runtime ports: %+v", runtimePorts)
	}
	if model.Ssh == nil || model.Ssh.Proxy == nil || model.Ssh.Proxy.Port.ValueInt64() != 22 {
		t.Fatalf("ssh proxy: %+v", model.Ssh)
	}
	if model.Ssh.Direct != nil {
		t.Fatalf("ssh direct must be null when the API omits it: %+v", model.Ssh.Direct)
	}

	assertDataSourceModelFits(t, NewPodDataSource, model)
}

func TestFlattenPodDetailCpuPod(t *testing.T) {
	ctx := context.Background()

	pod := sdktypes.Pod{
		Id:        "pod_cpu",
		Name:      "cpu-pod",
		Image:     "runpod/base:1.0.0",
		Status:    sdktypes.PodStatus("EXITED"),
		Cloud:     sdktypes.Cloud("SECURE"),
		Disk:      20,
		Env:       map[string]string{},
		Ports:     []string{},
		Actions:   []sdktypes.PodAction{},
		CreatedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Cpu:       &sdktypes.CpuConfig{Id: "cpu5c", Memory: 16, VcpuCount: 4},
		Mounts: sdktypes.Mounts{
			Persistent: &sdktypes.PersistentMount{Path: "/workspace", Size: 20},
		},
	}

	model, diags := flattenPodDetail(ctx, pod)
	if diags.HasError() {
		t.Fatal(diags)
	}

	if model.CpuTypeId.ValueString() != "cpu5c" || model.CpuMemory.ValueInt64() != 16 ||
		model.CpuVcpuCount.ValueInt64() != 4 {
		t.Fatalf("cpu: %+v", model)
	}
	if !model.GpuTypeId.IsNull() || !model.GpuCount.IsNull() ||
		!model.GpuMemory.IsNull() || !model.GpuVcpuCount.IsNull() {
		t.Fatalf("gpu fields must be null on a CPU pod: %+v", model)
	}
	if !model.StartedAt.IsNull() {
		t.Fatalf("started_at = %v, want null", model.StartedAt)
	}
	if !model.Runtime.IsNull() {
		t.Fatalf("runtime must be null when the pod is not running: %+v", model.Runtime)
	}
	if model.Cluster != nil {
		t.Fatalf("cluster must be null for a standalone pod: %+v", model.Cluster)
	}
	if model.Mounts == nil || model.Mounts.Persistent == nil ||
		model.Mounts.Persistent.Path.ValueString() != "/workspace" ||
		model.Mounts.Persistent.Size.ValueInt64() != 20 ||
		len(model.Mounts.Network) != 0 {
		t.Fatalf("mounts: %+v", model.Mounts)
	}
	if model.Ssh == nil || model.Ssh.Proxy != nil || model.Ssh.Direct != nil {
		t.Fatalf("ssh: %+v", model.Ssh)
	}

	assertDataSourceModelFits(t, NewPodDataSource, model)
}

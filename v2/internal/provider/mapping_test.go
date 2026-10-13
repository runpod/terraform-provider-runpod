package provider

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

func TestFlattenGpuTypes(t *testing.T) {
	gpu := sdktypes.GpuType{
		Id:        "NVIDIA GeForce RTX 4090",
		Name:      "RTX 4090",
		Memory:    24,
		Secure:    true,
		Community: false,
	}
	gpu.Price.Secure = 0.44
	gpu.Price.Community = 0.31

	models := flattenGpuTypes([]sdktypes.GpuType{gpu})
	if len(models) != 1 {
		t.Fatalf("len = %d, want 1", len(models))
	}
	m := models[0]
	if m.Id.ValueString() != "NVIDIA GeForce RTX 4090" ||
		m.Name.ValueString() != "RTX 4090" ||
		m.Memory.ValueInt64() != 24 ||
		!m.Secure.ValueBool() || m.Community.ValueBool() {
		t.Fatalf("unexpected model: %+v", m)
	}
	if m.PriceSecure.ValueFloat64() < 0.43 || m.PriceSecure.ValueFloat64() > 0.45 {
		t.Fatalf("price_secure = %v, want ~0.44", m.PriceSecure.ValueFloat64())
	}
}

func TestExpandCreatePod(t *testing.T) {
	ctx := context.Background()
	env, diags := types.MapValueFrom(ctx, types.StringType, map[string]string{"FOO": "bar"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	ports, diags := types.ListValueFrom(ctx, types.StringType, []string{"8888/http"})
	if diags.HasError() {
		t.Fatal(diags)
	}

	plan := podResourceModel{
		Name:      types.StringValue("my-pod"),
		Image:     types.StringValue("runpod/pytorch:2.8.0"),
		GpuTypeId: types.StringValue("NVIDIA GeForce RTX 4090"),
		GpuCount:  types.Int64Value(2),
		Cloud:     types.StringValue("SECURE"),
		Disk:      types.Int64Value(20),
		Env:       env,
		Ports:     ports,
	}

	body, diags := expandCreatePod(ctx, plan)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Name != "my-pod" || body.Image == nil || *body.Image != "runpod/pytorch:2.8.0" {
		t.Fatalf("name/image: %+v", body)
	}
	if body.Gpu == nil || body.Gpu.Id != "NVIDIA GeForce RTX 4090" || body.Gpu.Count == nil || *body.Gpu.Count != 2 {
		t.Fatalf("gpu: %+v", body.Gpu)
	}
	if body.Cloud == nil || *body.Cloud != sdktypes.Cloud("SECURE") {
		t.Fatalf("cloud: %v", body.Cloud)
	}
	if body.Disk == nil || *body.Disk != 20 {
		t.Fatalf("disk: %v", body.Disk)
	}
	if body.Env == nil || (*body.Env)["FOO"] != "bar" {
		t.Fatalf("env: %v", body.Env)
	}
	if body.Ports == nil || len(*body.Ports) != 1 || (*body.Ports)[0] != "8888/http" {
		t.Fatalf("ports: %v", body.Ports)
	}
}

func TestExpandCreatePodOmitsUnsetOptionals(t *testing.T) {
	plan := podResourceModel{
		Name:      types.StringValue("minimal-pod"),
		Image:     types.StringValue("runpod/pytorch:2.8.0"),
		GpuTypeId: types.StringValue("NVIDIA GeForce RTX 4090"),
		GpuCount:  types.Int64Null(),
		Cloud:     types.StringNull(),
		Disk:      types.Int64Null(),
		Env:       types.MapNull(types.StringType),
		Ports:     types.ListNull(types.StringType),
	}

	body, diags := expandCreatePod(context.Background(), plan)
	if diags.HasError() {
		t.Fatal(diags)
	}
	// gpu is required by the API, so it is always sent; an omitted gpu_count
	// leaves the server default in place rather than sending a zero.
	if body.Gpu == nil || body.Gpu.Id != "NVIDIA GeForce RTX 4090" || body.Gpu.Count != nil {
		t.Fatalf("gpu must carry the type with no count: %+v", body.Gpu)
	}
	if body.Cloud != nil || body.Disk != nil || body.Env != nil || body.Ports != nil {
		t.Fatalf("unset optionals must stay nil: %+v", body)
	}
}

func TestExpandCreatePodCpu(t *testing.T) {
	body, diags := expandCreatePod(context.Background(), podResourceModel{
		Name:      types.StringValue("cpu-pod"),
		Image:     types.StringValue("runpod/pytorch:2.8.0"),
		CpuTypeId: types.StringValue("cpu5c"),
		VcpuCount: types.Int64Value(4),
		GpuTypeId: types.StringNull(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Gpu != nil {
		t.Fatalf("a CPU pod must carry no gpu config: %+v", body.Gpu)
	}
	if body.Cpu == nil || body.Cpu.Id != "cpu5c" || body.Cpu.VcpuCount != 4 {
		t.Fatalf("cpu: %+v", body.Cpu)
	}
}

func TestExpandCreatePodGpuConstraintsAndFlags(t *testing.T) {
	ctx := context.Background()
	versions, diags := types.ListValueFrom(ctx, types.StringType, []string{"12.8", "12.6"})
	if diags.HasError() {
		t.Fatal(diags)
	}

	body, diags := expandCreatePod(ctx, podResourceModel{
		Name:                types.StringValue("constrained-pod"),
		TemplateId:          types.StringValue("30zmvf89kd"),
		Image:               types.StringNull(),
		GpuTypeId:           types.StringValue("NVIDIA GeForce RTX 4090"),
		AllowedCudaVersions: versions,
		MinRamPerGpu:        types.Int64Value(8),
		MinVcpuCountPerGpu:  types.Int64Value(2),
		StartSsh:            types.BoolValue(true),
		StartJupyter:        types.BoolValue(true),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Image != nil {
		t.Fatalf("an image-less pod must omit image: %v", *body.Image)
	}
	if body.TemplateId == nil || *body.TemplateId != "30zmvf89kd" {
		t.Fatalf("template_id: %v", body.TemplateId)
	}
	if body.Gpu.AllowedCudaVersions == nil || len(*body.Gpu.AllowedCudaVersions) != 2 ||
		(*body.Gpu.AllowedCudaVersions)[0] != "12.8" {
		t.Fatalf("allowed_cuda_versions: %v", body.Gpu.AllowedCudaVersions)
	}
	if body.Gpu.MinCudaVersion != nil {
		t.Fatalf("an unset min_cuda_version must stay nil, got %q", *body.Gpu.MinCudaVersion)
	}
	if body.Gpu.MinRamPerGpu == nil || *body.Gpu.MinRamPerGpu != 8 ||
		body.Gpu.MinVcpuCountPerGpu == nil || *body.Gpu.MinVcpuCountPerGpu != 2 {
		t.Fatalf("placement filters: %+v", body.Gpu)
	}
	if body.StartSsh == nil || !*body.StartSsh || body.StartJupyter == nil || !*body.StartJupyter {
		t.Fatalf("start flags: ssh=%v jupyter=%v", body.StartSsh, body.StartJupyter)
	}

	// A CUDA floor is the alternative to the exact set, and travels alone.
	body, diags = expandCreatePod(ctx, podResourceModel{
		Name:           types.StringValue("floor-pod"),
		Image:          types.StringValue("runpod/pytorch:2.8.0"),
		GpuTypeId:      types.StringValue("NVIDIA GeForce RTX 4090"),
		MinCudaVersion: types.StringValue("12.1"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Gpu.MinCudaVersion == nil || *body.Gpu.MinCudaVersion != "12.1" ||
		body.Gpu.AllowedCudaVersions != nil {
		t.Fatalf("min_cuda_version alone: %+v", body.Gpu)
	}
}

func TestExpandPodMounts(t *testing.T) {
	ctx := context.Background()
	persistent, diags := types.ObjectValueFrom(ctx, persistentMountAttrTypes, persistentMountModel{
		Size: types.Int64Value(20),
		Path: types.StringValue("/workspace"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	network, diags := types.ListValueFrom(ctx, networkMountObjectType, []networkMountModel{{
		VolumeId: types.StringValue("vol_xyz"),
		Path:     types.StringValue("/runpod-volume"),
	}})
	if diags.HasError() {
		t.Fatal(diags)
	}

	mounts, diags := expandPodMounts(ctx, podResourceModel{PersistentVolume: persistent})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if mounts == nil || mounts.Persistent == nil || mounts.Persistent.Size != 20 ||
		mounts.Persistent.Path != "/workspace" || mounts.Network != nil {
		t.Fatalf("persistent mount: %+v", mounts)
	}

	mounts, diags = expandPodMounts(ctx, podResourceModel{NetworkVolumes: network})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if mounts == nil || mounts.Network == nil || len(*mounts.Network) != 1 ||
		(*mounts.Network)[0].VolumeId != "vol_xyz" || (*mounts.Network)[0].Path != "/runpod-volume" ||
		mounts.Persistent != nil {
		t.Fatalf("network mount: %+v", mounts)
	}

	// No mount block: the field is left off the body, which a PATCH reads as
	// "leave the existing mount alone".
	mounts, diags = expandPodMounts(ctx, podResourceModel{
		PersistentVolume: types.ObjectNull(persistentMountAttrTypes),
		NetworkVolumes:   types.ListNull(networkMountObjectType),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if mounts != nil {
		t.Fatalf("unset mounts must stay nil: %+v", mounts)
	}
}

func TestFlattenPodCpuAndMounts(t *testing.T) {
	ctx := context.Background()
	template := "30zmvf89kd"
	pod := &sdktypes.Pod{
		Id:        "pod_cpu",
		Name:      "cpu-pod",
		Image:     "runpod/pytorch:2.8.0",
		Status:    sdktypes.PodStatusRunning,
		Cloud:     sdktypes.CloudSecure,
		Cpu:       &sdktypes.CpuConfig{Id: "cpu5c", VcpuCount: 4, Memory: 16},
		Template:  &template,
		CreatedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		Mounts: sdktypes.Mounts{Network: &[]sdktypes.NetworkMount{{
			VolumeId: "vol_xyz",
			Path:     "/runpod-volume",
		}}},
	}

	var model podResourceModel
	if diags := flattenPod(ctx, pod, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.CpuTypeId.ValueString() != "cpu5c" || model.VcpuCount.ValueInt64() != 4 {
		t.Fatalf("cpu: %+v", model)
	}
	if !model.GpuTypeId.IsNull() || !model.GpuCount.IsNull() {
		t.Fatalf("a CPU pod must report no gpu: %+v", model)
	}
	if model.TemplateId.ValueString() != template {
		t.Fatalf("template_id = %q, want %q", model.TemplateId.ValueString(), template)
	}
	if !model.PersistentVolume.IsNull() {
		t.Fatalf("no persistent mount, got %+v", model.PersistentVolume)
	}
	var volumes []networkMountModel
	if d := model.NetworkVolumes.ElementsAs(ctx, &volumes, false); d.HasError() {
		t.Fatal(d)
	}
	if len(volumes) != 1 || volumes[0].VolumeId.ValueString() != "vol_xyz" ||
		volumes[0].Path.ValueString() != "/runpod-volume" {
		t.Fatalf("network volumes: %+v", volumes)
	}

	// A persistent mount, and a pod with no template: an unknown template_id
	// resolves to null rather than staying unknown in state.
	pod.Cpu = nil
	pod.Gpu = &sdktypes.GpuConfig{Id: "NVIDIA GeForce RTX 4090"}
	pod.Template = nil
	pod.Mounts = sdktypes.Mounts{Persistent: &sdktypes.PersistentMount{Size: 20, Path: "/workspace"}}

	model = podResourceModel{TemplateId: types.StringUnknown()}
	if diags := flattenPod(ctx, pod, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.TemplateId.IsNull() {
		t.Fatalf("template_id must resolve to null, got %+v", model.TemplateId)
	}
	var persistent persistentMountModel
	if d := model.PersistentVolume.As(ctx, &persistent, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatal(d)
	}
	if persistent.Size.ValueInt64() != 20 || persistent.Path.ValueString() != "/workspace" {
		t.Fatalf("persistent volume: %+v", persistent)
	}
	if !model.NetworkVolumes.IsNull() {
		t.Fatalf("no network mount, got %+v", model.NetworkVolumes)
	}
}

func TestFlattenPod(t *testing.T) {
	count := 2
	dc := "US-TX-3"
	pod := &sdktypes.Pod{
		Id:           "pod_abc123",
		Name:         "my-pod",
		Image:        "runpod/pytorch:2.8.0",
		Status:       sdktypes.PodStatus("RUNNING"),
		Cloud:        sdktypes.Cloud("SECURE"),
		Disk:         20,
		Env:          map[string]string{"FOO": "bar"},
		Ports:        []string{"8888/http"},
		CreatedAt:    time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		DataCenterId: &dc,
		Gpu:          &sdktypes.GpuConfig{Id: "NVIDIA GeForce RTX 4090", Count: &count},
	}

	// env arrives unknown from a create plan that does not set it, which is
	// the case where the response is adopted.
	model := podResourceModel{Env: types.MapUnknown(types.StringType)}
	diags := flattenPod(context.Background(), pod, &model)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if model.Id.ValueString() != "pod_abc123" ||
		model.Status.ValueString() != "RUNNING" ||
		model.Cloud.ValueString() != "SECURE" ||
		model.Disk.ValueInt64() != 20 ||
		model.DataCenterId.ValueString() != "US-TX-3" ||
		model.GpuTypeId.ValueString() != "NVIDIA GeForce RTX 4090" ||
		model.GpuCount.ValueInt64() != 2 ||
		model.CreatedAt.ValueString() != "2026-06-01T12:00:00Z" {
		t.Fatalf("unexpected model: %+v", model)
	}

	var envOut map[string]string
	if d := model.Env.ElementsAs(context.Background(), &envOut, false); d.HasError() {
		t.Fatal(d)
	}
	if envOut["FOO"] != "bar" {
		t.Fatalf("env: %v", envOut)
	}
}

func TestFlattenPodCommandLists(t *testing.T) {
	ctx := context.Background()

	// The 1.x docker_entrypoint and docker_start_cmd are the v2 pod's
	// entrypoint and cmd, the exec-form lists the API decodes args into.
	entrypoint := []string{"/bin/bash", "-c"}
	cmd := []string{"python -u main.py"}
	pod := &sdktypes.Pod{
		Id:         "pod_abc123",
		Args:       `{"entrypoint":["/bin/bash","-c"],"cmd":["python -u main.py"]}`,
		Entrypoint: &entrypoint,
		Cmd:        &cmd,
	}

	var model podResourceModel
	if diags := flattenPod(ctx, pod, &model); diags.HasError() {
		t.Fatal(diags)
	}
	var gotEntrypoint, gotCmd []string
	if d := model.DockerEntrypoint.ElementsAs(ctx, &gotEntrypoint, false); d.HasError() {
		t.Fatal(d)
	}
	if d := model.DockerStartCmd.ElementsAs(ctx, &gotCmd, false); d.HasError() {
		t.Fatal(d)
	}
	if !slices.Equal(gotEntrypoint, entrypoint) {
		t.Fatalf("docker_entrypoint = %v, want %v", gotEntrypoint, entrypoint)
	}
	if !slices.Equal(gotCmd, cmd) {
		t.Fatalf("docker_start_cmd = %v, want %v", gotCmd, cmd)
	}

	// A pod the response reports without either list: null, not an empty
	// command, and still a list of strings so the state can hold it.
	pod.Entrypoint, pod.Cmd = nil, nil
	if diags := flattenPod(ctx, pod, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.DockerEntrypoint.IsNull() || model.DockerEntrypoint.ElementType(ctx) != types.StringType {
		t.Fatalf("docker_entrypoint = %v, want a null list of strings", model.DockerEntrypoint)
	}
	if !model.DockerStartCmd.IsNull() || model.DockerStartCmd.ElementType(ctx) != types.StringType {
		t.Fatalf("docker_start_cmd = %v, want a null list of strings", model.DockerStartCmd)
	}
}

func TestNetworkVolumeRoundTrip(t *testing.T) {
	plan := networkVolumeResourceModel{
		Name:       types.StringValue("my-dataset"),
		Size:       types.Int64Value(50),
		DataCenter: types.StringValue("EU-RO-1"),
		Type:       types.StringValue("HIGH_PERFORMANCE"),
	}
	body := expandCreateNetworkVolume(plan)
	if body.Name != "my-dataset" || body.Size != 50 || body.DataCenter != "EU-RO-1" {
		t.Fatalf("expand: %+v", body)
	}
	if body.Type == nil || *body.Type != sdktypes.VolumeType("HIGH_PERFORMANCE") {
		t.Fatalf("type: %v", body.Type)
	}

	var model networkVolumeResourceModel
	flattenNetworkVolume(&sdktypes.NetworkVolume{
		Id: "agv6w2qcg7", Name: "my-dataset", Size: 50, DataCenter: "EU-RO-1",
		Type: sdktypes.VolumeType("HIGH_PERFORMANCE"),
	}, &model)
	if model.Id.ValueString() != "agv6w2qcg7" || model.Size.ValueInt64() != 50 ||
		model.Type.ValueString() != "HIGH_PERFORMANCE" {
		t.Fatalf("flatten: %+v", model)
	}
}

func TestExpandCreateNetworkVolumeOmitsUnsetType(t *testing.T) {
	body := expandCreateNetworkVolume(networkVolumeResourceModel{
		Name:       types.StringValue("v"),
		Size:       types.Int64Value(10),
		DataCenter: types.StringValue("EU-RO-1"),
		Type:       types.StringNull(),
	})
	if body.Type != nil {
		t.Fatalf("unset type must stay nil, got %v", *body.Type)
	}
}

func TestFlattenTemplate(t *testing.T) {
	tpl := &sdktypes.Template{
		Id:       "tpl_abc",
		Name:     "training",
		Image:    "runpod/pytorch:2.8.0",
		Category: sdktypes.TemplateCategory("NVIDIA"),
		Disk:     30,
		Env:      map[string]string{"A": "b"},
		Ports:    []string{"22/tcp"},
		Public:   false,
	}
	var model templateResourceModel
	if diags := flattenTemplate(context.Background(), tpl, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Id.ValueString() != "tpl_abc" || model.Category.ValueString() != "NVIDIA" ||
		model.Disk.ValueInt64() != 30 || model.Public.ValueBool() {
		t.Fatalf("flatten: %+v", model)
	}
}

func TestExpandCreateTemplateContainerAndMountFields(t *testing.T) {
	ctx := context.Background()
	persistent, diags := types.ObjectValueFrom(ctx, persistentMountAttrTypes, persistentMountModel{
		Size: types.Int64Value(20),
		Path: types.StringValue("/workspace"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	versions, diags := types.ListValueFrom(ctx, types.StringType, []string{"12.8"})
	if diags.HasError() {
		t.Fatal(diags)
	}

	body, diags := expandCreateTemplate(ctx, templateResourceModel{
		Name:                types.StringValue("training"),
		Image:               types.StringValue("runpod/pytorch:2.8.0"),
		Registry:            types.StringValue("reg123"),
		StartSsh:            types.BoolValue(true),
		StartJupyter:        types.BoolValue(false),
		AllowedCudaVersions: versions,
		PersistentVolume:    persistent,
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Registry == nil || *body.Registry != "reg123" {
		t.Fatalf("registry: %v", body.Registry)
	}
	if body.StartSsh == nil || !*body.StartSsh {
		t.Fatalf("start_ssh: %v", body.StartSsh)
	}
	if body.StartJupyter == nil || *body.StartJupyter {
		t.Fatalf("start_jupyter must be sent as false, got %v", body.StartJupyter)
	}
	if body.AllowedCudaVersions == nil || len(*body.AllowedCudaVersions) != 1 ||
		(*body.AllowedCudaVersions)[0] != "12.8" {
		t.Fatalf("allowed_cuda_versions: %v", body.AllowedCudaVersions)
	}
	if body.Mounts == nil || body.Mounts.Persistent == nil ||
		body.Mounts.Persistent.Size != 20 || body.Mounts.Persistent.Path != "/workspace" {
		t.Fatalf("mounts: %+v", body.Mounts)
	}

	// Nothing configured: the mount object is left off entirely, which a
	// PATCH reads as "leave the existing mount alone".
	body, diags = expandCreateTemplate(ctx, templateResourceModel{
		Name:             types.StringValue("bare"),
		Image:            types.StringValue("runpod/pytorch:2.8.0"),
		PersistentVolume: types.ObjectNull(persistentMountAttrTypes),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Mounts != nil || body.Registry != nil || body.StartSsh != nil ||
		body.StartJupyter != nil || body.AllowedCudaVersions != nil {
		t.Fatalf("unset optionals must stay nil: %+v", body)
	}
}

func TestFlattenTemplateContainerAndMountFields(t *testing.T) {
	ctx := context.Background()
	registry := "reg123"
	tpl := &sdktypes.Template{
		Id:                  "tpl_abc",
		Name:                "training",
		Image:               "runpod/pytorch:2.8.0",
		Category:            sdktypes.TemplateCategory("NVIDIA"),
		Registry:            &registry,
		StartSsh:            true,
		StartJupyter:        false,
		AllowedCudaVersions: []string{"12.8", "12.6"},
		Mounts: sdktypes.TemplateMounts{Persistent: &sdktypes.PersistentMount{
			Size: 20,
			Path: "/workspace",
		}},
	}

	var model templateResourceModel
	if diags := flattenTemplate(ctx, tpl, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Registry.ValueString() != "reg123" || !model.StartSsh.ValueBool() ||
		model.StartJupyter.ValueBool() {
		t.Fatalf("registry/start flags: %+v", model)
	}
	var versions []string
	if d := model.AllowedCudaVersions.ElementsAs(ctx, &versions, false); d.HasError() {
		t.Fatal(d)
	}
	if len(versions) != 2 || versions[0] != "12.8" {
		t.Fatalf("allowed_cuda_versions: %v", versions)
	}
	var persistent persistentMountModel
	if d := model.PersistentVolume.As(ctx, &persistent, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatal(d)
	}
	if persistent.Size.ValueInt64() != 20 || persistent.Path.ValueString() != "/workspace" {
		t.Fatalf("persistent_volume: %+v", persistent)
	}

	// A template with no mount and no registry: both flatten to null rather
	// than to zero values.
	tpl.Mounts = sdktypes.TemplateMounts{}
	tpl.Registry = nil
	var bare templateResourceModel
	if diags := flattenTemplate(ctx, tpl, &bare); diags.HasError() {
		t.Fatal(diags)
	}
	if !bare.PersistentVolume.IsNull() || !bare.Registry.IsNull() {
		t.Fatalf("absent mount/registry must flatten to null: %+v", bare)
	}
}

func TestFlattenPodReportedRuntimeFields(t *testing.T) {
	ctx := context.Background()
	cuda := "12.8"
	clusterIp := "10.65.0.2"
	started := time.Date(2026, 6, 1, 12, 5, 0, 0, time.UTC)
	pod := &sdktypes.Pod{
		Id:          "pod_abc123",
		Name:        "my-pod",
		Image:       "runpod/pytorch:2.8.0",
		Status:      sdktypes.PodStatusRunning,
		Cloud:       sdktypes.CloudSecure,
		Cost:        0.35,
		Locked:      true,
		CudaVersion: &cuda,
		StartedAt:   &started,
		CreatedAt:   time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		Gpu:         &sdktypes.GpuConfig{Id: "NVIDIA GeForce RTX 4090", Memory: 62, VcpuCount: 16},
		Cluster:     &sdktypes.PodCluster{Id: "cluster_abc123", Ip: &clusterIp},
	}

	model := podResourceModel{Env: types.MapUnknown(types.StringType)}
	if diags := flattenPod(ctx, pod, &model); diags.HasError() {
		t.Fatal(diags)
	}
	// cost is a float32 on the wire: widening it naively reports
	// 0.34999999403953552 for the 0.35 the API sent.
	if model.Cost.ValueFloat64() != 0.35 {
		t.Fatalf("cost = %v, want 0.35", model.Cost.ValueFloat64())
	}
	if !model.Locked.ValueBool() || model.CudaVersion.ValueString() != "12.8" {
		t.Fatalf("locked/cuda_version: %+v", model)
	}
	if model.StartedAt.ValueString() != "2026-06-01T12:05:00Z" {
		t.Fatalf("started_at = %q", model.StartedAt.ValueString())
	}
	if model.ClusterId.ValueString() != "cluster_abc123" || model.ClusterIp.ValueString() != "10.65.0.2" {
		t.Fatalf("cluster: %+v", model)
	}
	if model.MemoryInGb.ValueFloat64() != 62 {
		t.Fatalf("memory_in_gb = %v, want the GPU pod's total 62", model.MemoryInGb.ValueFloat64())
	}

	// A standalone CPU pod: memory comes from the CPU flavor, and the
	// cluster and CUDA fields are null rather than zero values.
	pod.Gpu = nil
	pod.Cpu = &sdktypes.CpuConfig{Id: "cpu5c", VcpuCount: 4, Memory: 16}
	pod.Cluster = nil
	pod.CudaVersion = nil
	pod.StartedAt = nil
	pod.Locked = false
	pod.Cost = 0

	var cpuModel podResourceModel
	cpuModel.Env = types.MapUnknown(types.StringType)
	if diags := flattenPod(ctx, pod, &cpuModel); diags.HasError() {
		t.Fatal(diags)
	}
	if cpuModel.MemoryInGb.ValueFloat64() != 16 {
		t.Fatalf("memory_in_gb = %v, want the CPU flavor's 16", cpuModel.MemoryInGb.ValueFloat64())
	}
	if !cpuModel.ClusterId.IsNull() || !cpuModel.ClusterIp.IsNull() ||
		!cpuModel.CudaVersion.IsNull() || !cpuModel.StartedAt.IsNull() {
		t.Fatalf("absent fields must flatten to null: %+v", cpuModel)
	}
	if cpuModel.Cost.ValueFloat64() != 0 || cpuModel.Locked.ValueBool() {
		t.Fatalf("a stopped pod costs nothing and is unlocked: %+v", cpuModel)
	}
}

func TestFlattenPodRuntime(t *testing.T) {
	ctx := context.Background()
	uptime := 3600
	cpuUtil := 45
	memoryUtil := 62
	gpuUtil := 94
	gpuMemoryUtil := 78
	ip := "45.23.12.1"
	private := 8888
	public := 43210
	portType := "http"

	runtime := &sdktypes.PodRuntime{
		Uptime: &uptime,
		Cpu:    &sdktypes.Utilization{Util: &cpuUtil},
		Memory: &sdktypes.Utilization{Util: &memoryUtil},
		Gpus:   &[]sdktypes.PodGpuUtilization{{Util: &gpuUtil, MemoryUtil: &gpuMemoryUtil}},
		Ports: &[]sdktypes.PodRuntimePort{{
			Ip:      &ip,
			Private: &private,
			Public:  &public,
			Type:    &portType,
		}},
	}

	object, diags := flattenPodRuntime(ctx, runtime)
	if diags.HasError() {
		t.Fatal(diags)
	}
	var metrics podRuntimeModel
	if d := object.As(ctx, &metrics, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatal(d)
	}
	if metrics.UptimeSeconds.ValueInt64() != 3600 || metrics.CpuUtil.ValueInt64() != 45 ||
		metrics.MemoryUtil.ValueInt64() != 62 {
		t.Fatalf("utilization: %+v", metrics)
	}

	var gpus []podGpuUtilizationModel
	if d := metrics.Gpus.ElementsAs(ctx, &gpus, false); d.HasError() {
		t.Fatal(d)
	}
	if len(gpus) != 1 || gpus[0].Util.ValueInt64() != 94 || gpus[0].MemoryUtil.ValueInt64() != 78 {
		t.Fatalf("gpus: %+v", gpus)
	}

	// The port mapping keeps the API's numbers under names that say which
	// side of the container they are on.
	var ports []podRuntimePortModel
	if d := metrics.Ports.ElementsAs(ctx, &ports, false); d.HasError() {
		t.Fatal(d)
	}
	if len(ports) != 1 || ports[0].Ip.ValueString() != "45.23.12.1" ||
		ports[0].PrivatePort.ValueInt64() != 8888 || ports[0].PublicPort.ValueInt64() != 43210 ||
		ports[0].Type.ValueString() != "http" {
		t.Fatalf("ports: %+v", ports)
	}

	// A pod that is not RUNNING reports no runtime at all.
	object, diags = flattenPodRuntime(ctx, nil)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !object.IsNull() {
		t.Fatalf("absent runtime must flatten to null, got %+v", object)
	}

	// A partial report: missing readings stay null instead of reading as a
	// zero percent utilization.
	object, diags = flattenPodRuntime(ctx, &sdktypes.PodRuntime{Uptime: &uptime})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if d := object.As(ctx, &metrics, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatal(d)
	}
	if !metrics.CpuUtil.IsNull() || !metrics.MemoryUtil.IsNull() ||
		!metrics.Gpus.IsNull() || !metrics.Ports.IsNull() {
		t.Fatalf("missing readings must stay null: %+v", metrics)
	}
	if metrics.UptimeSeconds.ValueInt64() != 3600 {
		t.Fatalf("uptime = %v", metrics.UptimeSeconds.ValueInt64())
	}

	// And the whole block reaches the pod model through flattenPod.
	pod := &sdktypes.Pod{
		Id:        "pod_runtime",
		Name:      "runtime-pod",
		Image:     "runpod/pytorch:2.8.0",
		Status:    sdktypes.PodStatusRunning,
		Cloud:     sdktypes.CloudSecure,
		Gpu:       &sdktypes.GpuConfig{Id: "NVIDIA GeForce RTX 4090"},
		CreatedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		Runtime:   runtime,
	}
	model := podResourceModel{Env: types.MapUnknown(types.StringType)}
	if diags := flattenPod(ctx, pod, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Runtime.IsNull() {
		t.Fatal("a running pod's runtime must reach the model")
	}
	pod.Runtime = nil
	if diags := flattenPod(ctx, pod, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.Runtime.IsNull() {
		t.Fatalf("a pod with no runtime must clear the block, got %+v", model.Runtime)
	}
}

func TestFlattenEndpointRequestUrls(t *testing.T) {
	ctx := context.Background()

	var queueUrls sdktypes.EndpointRequestUrls
	if err := queueUrls.FromQueueBasedRequestUrls(sdktypes.QueueBasedRequestUrls{
		Run:        "https://api.runpod.ai/v2/4m7x2k9q/run",
		RunSync:    "https://api.runpod.ai/v2/4m7x2k9q/runsync",
		Status:     "https://api.runpod.ai/v2/4m7x2k9q/status/{job_id}",
		Stream:     "https://api.runpod.ai/v2/4m7x2k9q/stream/{job_id}",
		Cancel:     "https://api.runpod.ai/v2/4m7x2k9q/cancel/{job_id}",
		Retry:      "https://api.runpod.ai/v2/4m7x2k9q/retry/{job_id}",
		PurgeQueue: "https://api.runpod.ai/v2/4m7x2k9q/purge-queue",
		Health:     "https://api.runpod.ai/v2/4m7x2k9q/health",
	}); err != nil {
		t.Fatal(err)
	}

	object, diags := flattenEndpointRequestUrls(ctx, &queueUrls, "QUEUE")
	if diags.HasError() {
		t.Fatal(diags)
	}
	var urls endpointRequestUrlsModel
	if d := object.As(ctx, &urls, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatal(d)
	}
	if urls.Run.ValueString() != "https://api.runpod.ai/v2/4m7x2k9q/run" ||
		urls.RunSync.ValueString() != "https://api.runpod.ai/v2/4m7x2k9q/runsync" ||
		urls.PurgeQueue.ValueString() != "https://api.runpod.ai/v2/4m7x2k9q/purge-queue" ||
		urls.Health.ValueString() != "https://api.runpod.ai/v2/4m7x2k9q/health" {
		t.Fatalf("queue urls: %+v", urls)
	}
	if !urls.Base.IsNull() {
		t.Fatalf("a queue endpoint has no base url, got %+v", urls.Base)
	}

	// The union carries no discriminator, so the routing type picks the
	// shape: the load-balancer form fills base and health only.
	var balancerUrls sdktypes.EndpointRequestUrls
	if err := balancerUrls.FromLoadBalancingRequestUrls(sdktypes.LoadBalancingRequestUrls{
		Base:   "https://4m7x2k9q.api.runpod.ai",
		Health: "https://4m7x2k9q.api.runpod.ai/ping",
	}); err != nil {
		t.Fatal(err)
	}

	object, diags = flattenEndpointRequestUrls(ctx, &balancerUrls, "LOAD_BALANCER")
	if diags.HasError() {
		t.Fatal(diags)
	}
	if d := object.As(ctx, &urls, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatal(d)
	}
	if urls.Base.ValueString() != "https://4m7x2k9q.api.runpod.ai" ||
		urls.Health.ValueString() != "https://4m7x2k9q.api.runpod.ai/ping" {
		t.Fatalf("load balancer urls: %+v", urls)
	}
	if !urls.Run.IsNull() || !urls.Status.IsNull() {
		t.Fatalf("a load-balancer endpoint has no job urls, got %+v", urls)
	}

	// No URLs reported at all: a fresh create can answer without them.
	object, diags = flattenEndpointRequestUrls(ctx, nil, "QUEUE")
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !object.IsNull() {
		t.Fatalf("absent request urls must flatten to null, got %+v", object)
	}
}

func TestEndpointScalingRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		scalingType string
		value       float64
	}{
		{"QUEUE_DELAY", 4},
		// The API holds queue delay as a float32. These are the values that
		// do not survive a naive widen (0.7 comes back 0.699999988079071),
		// and scaling_value is Required, so a drifted one fails the apply.
		{"QUEUE_DELAY", 0.7},
		{"QUEUE_DELAY", 0.5},
		{"QUEUE_DELAY", 1.2},
		{"QUEUE_DELAY", 2.3},
		{"REQUEST_COUNT", 7},
	} {
		scaling, diags := expandEndpointScaling(tc.scalingType, tc.value)
		if diags.HasError() {
			t.Fatalf("%s: %v", tc.scalingType, diags)
		}
		gotType, gotValue, err := flattenEndpointScaling(scaling)
		if err != nil {
			t.Fatalf("%s: %v", tc.scalingType, err)
		}
		if gotType != tc.scalingType || gotValue != tc.value {
			t.Fatalf("round trip: got (%s, %v), want (%s, %v)", gotType, gotValue, tc.scalingType, tc.value)
		}
	}

	if _, diags := expandEndpointScaling("BOGUS", 1); !diags.HasError() {
		t.Fatal("invalid scaling_type must error")
	}
}

func TestExpandEndpointScalingRequestCountTakesWholeNumbers(t *testing.T) {
	// The API takes an integer count of at least 1. A fractional value would
	// otherwise be truncated to a different number than the plan holds, and
	// 0.5, which the attribute's own AtLeast(0.5) validator allows because
	// queue delay needs it, would be truncated to a count of 0 the API
	// rejects.
	for _, value := range []float64{2.7, 0.5, 0.9, 0} {
		if _, diags := expandEndpointScaling("REQUEST_COUNT", value); !diags.HasError() {
			t.Fatalf("REQUEST_COUNT scaling_value %v must be rejected", value)
		}
	}
	for _, value := range []float64{1, 4, 100} {
		scaling, diags := expandEndpointScaling("REQUEST_COUNT", value)
		if diags.HasError() {
			t.Fatalf("REQUEST_COUNT scaling_value %v must be accepted: %v", value, diags)
		}
		gotType, got, err := flattenEndpointScaling(scaling)
		if err != nil || gotType != "REQUEST_COUNT" || got != value {
			t.Fatalf("round trip of %v: got (%s, %v, %v)", value, gotType, got, err)
		}
	}
}

func TestExpandCreateEndpoint(t *testing.T) {
	ctx := context.Background()
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}

	plan := serverlessEndpointResourceModel{
		Name:         types.StringValue("my-endpoint"),
		Image:        types.StringValue("runpod/worker:1"),
		EndpointType: types.StringValue("QUEUE"),
		GpuPools:     pools,
		GpuCount:     types.Int64Value(1),
		ScalingType:  types.StringValue("QUEUE_DELAY"),
		ScalingValue: types.Float64Value(4),
		WorkersMin:   types.Int64Value(0),
		WorkersMax:   types.Int64Value(2),
	}
	body, diags := expandCreateEndpoint(ctx, plan)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Name != "my-endpoint" || string(body.Type) != "QUEUE" {
		t.Fatalf("name/type: %+v", body)
	}
	if len(body.Gpu.Pools) != 1 || body.Gpu.Pools[0] != "ADA_24" || body.Gpu.Count == nil || *body.Gpu.Count != 1 {
		t.Fatalf("gpu: %+v", body.Gpu)
	}
	if body.Workers == nil || body.Workers.Min == nil || *body.Workers.Min != 0 ||
		body.Workers.Max == nil || *body.Workers.Max != 2 {
		t.Fatalf("workers: %+v", body.Workers)
	}
	scalingType, value, err := flattenEndpointScaling(body.Scaling)
	if err != nil || scalingType != "QUEUE_DELAY" || value != 4 {
		t.Fatalf("scaling: (%s, %v, %v)", scalingType, value, err)
	}
}

func TestFlattenEndpoint(t *testing.T) {
	ctx := context.Background()
	idle := 5
	disk := 30
	count := 1
	image := "runpod/mock-worker:dev"
	registry := "reg123"
	endpointType := sdktypes.EndpointType("QUEUE")
	env := map[string]string{"A": "b"}

	var scaling sdktypes.EndpointScaling
	if err := scaling.FromQueueDelayScaling(sdktypes.QueueDelayScaling{
		Type: sdktypes.QueueDelayScalingTypeQueueDelay, QueueDelay: 4,
	}); err != nil {
		t.Fatal(err)
	}

	endpoint := &sdktypes.Endpoint{
		Id:             "ep_abc123",
		Name:           "my-endpoint",
		Image:          &image,
		Type:           &endpointType,
		Timeout:        300000,
		Disk:           &disk,
		Env:            &env,
		Registry:       &registry,
		Scaling:        scaling,
		Gpu:            &sdktypes.EndpointGpuConfig{Pools: []string{"ADA_24"}, Count: &count},
		DataCenterIds:  []string{"EU-RO-1"},
		NetworkVolumes: []string{},
		CreatedAt:      time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
	}
	endpoint.Workers.Min = 0
	endpoint.Workers.Max = 2
	endpoint.Workers.IdleTimeout = &idle

	var model serverlessEndpointResourceModel
	if diags := flattenEndpoint(ctx, endpoint, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Id.ValueString() != "ep_abc123" ||
		model.Timeout.ValueInt64() != 300000 ||
		model.Registry.ValueString() != "reg123" ||
		model.IdleTimeout.ValueInt64() != 5 ||
		model.WorkersMax.ValueInt64() != 2 ||
		model.ScalingType.ValueString() != "QUEUE_DELAY" ||
		model.ScalingValue.ValueFloat64() != 4 {
		t.Fatalf("unexpected model: %+v", model)
	}

	// A nil registry/idle_timeout must flatten to null, not zero values.
	endpoint.Registry = nil
	endpoint.Workers.IdleTimeout = nil
	var sparse serverlessEndpointResourceModel
	if diags := flattenEndpoint(ctx, endpoint, &sparse); diags.HasError() {
		t.Fatal(diags)
	}
	if !sparse.Registry.IsNull() || !sparse.IdleTimeout.IsNull() {
		t.Fatalf("nil pointers must flatten to null: %+v", sparse)
	}
}

func TestExpandCreateEndpointSendsTimeoutAndRegistry(t *testing.T) {
	ctx := context.Background()
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	plan := serverlessEndpointResourceModel{
		Name:         types.StringValue("ep"),
		Image:        types.StringValue("img"),
		EndpointType: types.StringValue("QUEUE"),
		GpuPools:     pools,
		ScalingType:  types.StringValue("REQUEST_COUNT"),
		ScalingValue: types.Float64Value(7),
		Timeout:      types.Int64Value(300000),
		Registry:     types.StringValue("reg123"),
	}
	body, diags := expandCreateEndpoint(ctx, plan)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Timeout == nil || *body.Timeout != 300000 {
		t.Fatalf("timeout not sent on create: %v", body.Timeout)
	}
	if body.Registry == nil || *body.Registry != "reg123" {
		t.Fatalf("registry not sent on create: %v", body.Registry)
	}
}

func TestExpandCreateEndpointCpu(t *testing.T) {
	ctx := context.Background()
	cpu, diags := types.ListValueFrom(ctx, endpointCpuObjectType, []endpointCpuModel{
		{CpuTypeId: types.StringValue("cpu5c"), VcpuCount: types.Int64Value(4)},
		{CpuTypeId: types.StringValue("cpu5c"), VcpuCount: types.Int64Value(8)},
	})
	if diags.HasError() {
		t.Fatal(diags)
	}

	body, diags := expandCreateEndpoint(ctx, serverlessEndpointResourceModel{
		Name:         types.StringValue("cpu-endpoint"),
		Image:        types.StringValue("runpod/mock-worker:dev"),
		EndpointType: types.StringValue("QUEUE"),
		GpuPools:     types.ListNull(types.StringType),
		Cpu:          cpu,
		ScalingType:  types.StringValue("QUEUE_DELAY"),
		ScalingValue: types.Float64Value(4),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Gpu != nil {
		t.Fatalf("a CPU endpoint must carry no gpu config: %+v", body.Gpu)
	}
	if body.Cpu == nil || len(*body.Cpu) != 2 {
		t.Fatalf("cpu: %+v", body.Cpu)
	}
	if (*body.Cpu)[0].Id != "cpu5c" || (*body.Cpu)[0].VcpuCount != 4 || (*body.Cpu)[1].VcpuCount != 8 {
		t.Fatalf("cpu configs did not keep their submitted order/values: %+v", *body.Cpu)
	}
}

func TestExpandCreateEndpointGpuConstraints(t *testing.T) {
	ctx := context.Background()
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	excluded, diags := types.ListValueFrom(ctx, types.StringType, []string{"NVIDIA L40"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	versions, diags := types.ListValueFrom(ctx, types.StringType, []string{"12.8"})
	if diags.HasError() {
		t.Fatal(diags)
	}

	body, diags := expandCreateEndpoint(ctx, serverlessEndpointResourceModel{
		Name:                types.StringValue("ep"),
		TemplateId:          types.StringValue("30zmvf89kd"),
		Image:               types.StringNull(),
		EndpointType:        types.StringValue("QUEUE"),
		GpuPools:            pools,
		ExcludedTypes:       excluded,
		AllowedCudaVersions: versions,
		Flashboot:           types.StringValue(string(sdktypes.FlashBootPriority)),
		ScalingType:         types.StringValue("QUEUE_DELAY"),
		ScalingValue:        types.Float64Value(4),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Image != nil {
		t.Fatalf("an image-less endpoint must omit image: %v", *body.Image)
	}
	if body.TemplateId == nil || *body.TemplateId != "30zmvf89kd" {
		t.Fatalf("template_id: %v", body.TemplateId)
	}
	if body.Gpu.ExcludedTypes == nil || len(*body.Gpu.ExcludedTypes) != 1 ||
		(*body.Gpu.ExcludedTypes)[0] != "NVIDIA L40" {
		t.Fatalf("excluded_types: %v", body.Gpu.ExcludedTypes)
	}
	if body.Gpu.AllowedCudaVersions == nil || (*body.Gpu.AllowedCudaVersions)[0] != "12.8" {
		t.Fatalf("allowed_cuda_versions: %v", body.Gpu.AllowedCudaVersions)
	}
	if body.Gpu.MinCudaVersion != nil {
		t.Fatalf("an unset min_cuda_version must stay nil, got %q", *body.Gpu.MinCudaVersion)
	}
	if body.Flashboot == nil || *body.Flashboot != sdktypes.FlashBootPriority {
		t.Fatalf("flashboot: %v", body.Flashboot)
	}
}

func TestFlattenEndpointCpuAndGpuConstraints(t *testing.T) {
	ctx := context.Background()
	var scaling sdktypes.EndpointScaling
	if err := scaling.FromRequestCountScaling(sdktypes.RequestCountScaling{
		Type: sdktypes.RequestCountScalingTypeRequestCount, RequestCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	minCuda := "12.1"
	excluded := []string{"NVIDIA L40"}

	endpoint := &sdktypes.Endpoint{
		Id:        "ep_gpu",
		Name:      "gpu-endpoint",
		Scaling:   scaling,
		Flashboot: sdktypes.FlashBootEnabled,
		Gpu: &sdktypes.EndpointGpuConfig{
			Pools:               []string{"ADA_24"},
			AllowedCudaVersions: []string{"12.8"},
			MinCudaVersion:      &minCuda,
			ExcludedTypes:       &excluded,
		},
	}

	var model serverlessEndpointResourceModel
	if diags := flattenEndpoint(ctx, endpoint, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Flashboot.ValueString() != "FLASHBOOT" {
		t.Fatalf("flashboot = %q", model.Flashboot.ValueString())
	}
	if model.MinCudaVersion.ValueString() != "12.1" {
		t.Fatalf("min_cuda_version = %q", model.MinCudaVersion.ValueString())
	}
	var allowed, excludedOut []string
	if d := model.AllowedCudaVersions.ElementsAs(ctx, &allowed, false); d.HasError() {
		t.Fatal(d)
	}
	if d := model.ExcludedTypes.ElementsAs(ctx, &excludedOut, false); d.HasError() {
		t.Fatal(d)
	}
	if len(allowed) != 1 || allowed[0] != "12.8" || len(excludedOut) != 1 || excludedOut[0] != "NVIDIA L40" {
		t.Fatalf("gpu constraints: allowed=%v excluded=%v", allowed, excludedOut)
	}
	if !model.Cpu.IsNull() {
		t.Fatalf("a GPU endpoint must report no cpu: %+v", model.Cpu)
	}

	// A CPU endpoint: the gpu block is absent, so its attributes flatten to
	// null rather than to zero values.
	endpoint.Gpu = nil
	endpoint.Cpu = &[]sdktypes.CpuConfig{{Id: "cpu5c", VcpuCount: 4, Memory: 16}}

	var cpuModel serverlessEndpointResourceModel
	if diags := flattenEndpoint(ctx, endpoint, &cpuModel); diags.HasError() {
		t.Fatal(diags)
	}
	if !cpuModel.GpuPools.IsNull() || !cpuModel.GpuCount.IsNull() ||
		!cpuModel.AllowedCudaVersions.IsNull() || !cpuModel.MinCudaVersion.IsNull() ||
		!cpuModel.ExcludedTypes.IsNull() {
		t.Fatalf("a CPU endpoint must report no gpu config: %+v", cpuModel)
	}
	var configs []endpointCpuModel
	if d := cpuModel.Cpu.ElementsAs(ctx, &configs, false); d.HasError() {
		t.Fatal(d)
	}
	if len(configs) != 1 || configs[0].CpuTypeId.ValueString() != "cpu5c" ||
		configs[0].VcpuCount.ValueInt64() != 4 {
		t.Fatalf("cpu: %+v", configs)
	}
}

func TestFlattenKeepsConfiguredEnvAgainstInjectedKeys(t *testing.T) {
	ctx := context.Background()
	configured, diags := types.MapValueFrom(ctx, types.StringType, map[string]string{"FOO": "bar"})
	if diags.HasError() {
		t.Fatal(diags)
	}

	// start_ssh injects PUBLIC_KEY, start_jupyter injects JUPYTER_PASSWORD,
	// and a template merges its own keys in. env is Optional+Computed, so a
	// configured map has to survive the apply exactly as planned.
	pod := &sdktypes.Pod{
		Id:        "pod_env",
		Name:      "env-pod",
		Image:     "runpod/pytorch:2.8.0",
		Status:    sdktypes.PodStatusRunning,
		Cloud:     sdktypes.CloudSecure,
		Gpu:       &sdktypes.GpuConfig{Id: "NVIDIA GeForce RTX 4090"},
		CreatedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		Env: map[string]string{
			"FOO":              "bar",
			"PUBLIC_KEY":       "ssh-ed25519 AAAA",
			"JUPYTER_PASSWORD": "generated",
		},
	}

	model := podResourceModel{Env: configured}
	if diags := flattenPod(ctx, pod, &model); diags.HasError() {
		t.Fatal(diags)
	}
	var env map[string]string
	if d := model.Env.ElementsAs(ctx, &env, false); d.HasError() {
		t.Fatal(d)
	}
	if len(env) != 1 || env["FOO"] != "bar" {
		t.Fatalf("a configured env must survive the injected keys, got %v", env)
	}

	// An unconfigured env is Unknown in the plan and has to be filled in, or
	// it would stay unknown in state.
	model = podResourceModel{Env: types.MapUnknown(types.StringType)}
	if diags := flattenPod(ctx, pod, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if d := model.Env.ElementsAs(ctx, &env, false); d.HasError() {
		t.Fatal(d)
	}
	if len(env) != 3 || env["PUBLIC_KEY"] == "" {
		t.Fatalf("an unset env must adopt the response, got %v", env)
	}

	// Same rule on the endpoint, where a template's env is merged per key.
	var scaling sdktypes.EndpointScaling
	if err := scaling.FromRequestCountScaling(sdktypes.RequestCountScaling{
		Type: sdktypes.RequestCountScalingTypeRequestCount, RequestCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	responseEnv := map[string]string{"FOO": "bar", "FROM_TEMPLATE": "1"}
	endpoint := &sdktypes.Endpoint{
		Id:      "ep_env",
		Name:    "env-endpoint",
		Scaling: scaling,
		Env:     &responseEnv,
		Gpu:     &sdktypes.EndpointGpuConfig{Pools: []string{"ADA_24"}},
	}

	endpointModel := serverlessEndpointResourceModel{Env: configured}
	if diags := flattenEndpoint(ctx, endpoint, &endpointModel); diags.HasError() {
		t.Fatal(diags)
	}
	if d := endpointModel.Env.ElementsAs(ctx, &env, false); d.HasError() {
		t.Fatal(d)
	}
	if len(env) != 1 || env["FOO"] != "bar" {
		t.Fatalf("a configured endpoint env must survive the merged keys, got %v", env)
	}
}

func TestFlattenEndpointKeepsPlannedTypeImageAndClearedCudaFloor(t *testing.T) {
	ctx := context.Background()
	var scaling sdktypes.EndpointScaling
	if err := scaling.FromRequestCountScaling(sdktypes.RequestCountScaling{
		Type: sdktypes.RequestCountScalingTypeRequestCount, RequestCount: 1,
	}); err != nil {
		t.Fatal(err)
	}

	// A response that omits type, image and the CUDA floor: endpoint_type is
	// Required and image was configured, so neither may be nulled, and a
	// configured "" floor, the API's way to clear one, has to stay "" to
	// match the plan.
	endpoint := &sdktypes.Endpoint{
		Id:      "ep_sparse",
		Name:    "sparse",
		Scaling: scaling,
		Gpu:     &sdktypes.EndpointGpuConfig{Pools: []string{"ADA_24"}},
	}

	model := serverlessEndpointResourceModel{
		EndpointType:   types.StringValue("QUEUE"),
		Image:          types.StringValue("runpod/mock-worker:dev"),
		MinCudaVersion: types.StringValue(""),
	}
	if diags := flattenEndpoint(ctx, endpoint, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.EndpointType.ValueString() != "QUEUE" {
		t.Fatalf("endpoint_type must keep the planned value, got %+v", model.EndpointType)
	}
	if model.Image.ValueString() != "runpod/mock-worker:dev" {
		t.Fatalf("image must keep the planned value, got %+v", model.Image)
	}
	if model.MinCudaVersion.IsNull() || model.MinCudaVersion.ValueString() != "" {
		t.Fatalf("a cleared CUDA floor must stay \"\", got %+v", model.MinCudaVersion)
	}

	// With nothing planned, the same absent fields resolve to null.
	unset := serverlessEndpointResourceModel{
		Image:          types.StringUnknown(),
		MinCudaVersion: types.StringUnknown(),
	}
	if diags := flattenEndpoint(ctx, endpoint, &unset); diags.HasError() {
		t.Fatal(diags)
	}
	if !unset.Image.IsNull() || !unset.MinCudaVersion.IsNull() {
		t.Fatalf("unset image/min_cuda_version must resolve to null: %+v", unset)
	}
}

func TestPodActionForDesiredState(t *testing.T) {
	for _, tc := range []struct {
		desired string
		status  string
		action  sdktypes.PodAction
		needed  bool
	}{
		{"RUNNING", "EXITED", sdktypes.PodActionStart, true},
		{"EXITED", "RUNNING", sdktypes.PodActionStop, true},
		// A pod fresh out of create is PROVISIONING or STARTING, and the API
		// accepts stop from both; without them a desired_state of EXITED
		// would bill until a second apply caught the pod in RUNNING.
		{"EXITED", "PROVISIONING", sdktypes.PodActionStop, true},
		{"EXITED", "STARTING", sdktypes.PodActionStop, true},
		// ERROR accepts start, the other half of the same spec paragraph.
		{"RUNNING", "ERROR", sdktypes.PodActionStart, true},
		{"RUNNING", "RUNNING", "", false},
		{"EXITED", "EXITED", "", false},
		{"RUNNING", "PROVISIONING", "", false},
		{"RUNNING", "PENDING", "", false},
		{"EXITED", "TERMINATED", "", false},
		{"EXITED", "ERROR", "", false},
	} {
		action, needed := podActionForDesiredState(tc.desired, sdktypes.PodStatus(tc.status))
		if action != tc.action || needed != tc.needed {
			t.Fatalf("(%s,%s): got (%v,%v), want (%v,%v)", tc.desired, tc.status, action, needed, tc.action, tc.needed)
		}
	}
}

func TestRefreshDesiredStateTracksActualRunState(t *testing.T) {
	// A pod stopped outside Terraform: the refresh has to report EXITED so the
	// plan diffs against the configured RUNNING and the apply restarts it.
	model := podResourceModel{
		DesiredState: types.StringValue("RUNNING"),
		Status:       types.StringValue("EXITED"),
	}
	refreshDesiredState(&model)
	if model.DesiredState.ValueString() != "EXITED" {
		t.Fatalf("desired_state = %q, want EXITED", model.DesiredState.ValueString())
	}

	// A transitional status is not a steady state; leave the previous value.
	model = podResourceModel{
		DesiredState: types.StringValue("RUNNING"),
		Status:       types.StringValue("PROVISIONING"),
	}
	refreshDesiredState(&model)
	if model.DesiredState.ValueString() != "RUNNING" {
		t.Fatalf("desired_state = %q, want RUNNING", model.DesiredState.ValueString())
	}

	// Unmanaged run state stays unmanaged.
	model = podResourceModel{
		DesiredState: types.StringNull(),
		Status:       types.StringValue("EXITED"),
	}
	refreshDesiredState(&model)
	if !model.DesiredState.IsNull() {
		t.Fatalf("unmanaged desired_state must stay null, got %q", model.DesiredState.ValueString())
	}
}

func TestExpandCreateTemplateSendsServerless(t *testing.T) {
	ctx := context.Background()
	body, diags := expandCreateTemplate(ctx, templateResourceModel{
		Name:       types.StringValue("worker"),
		Image:      types.StringValue("runpod/pytorch:2.8.0"),
		Serverless: types.BoolValue(true),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Serverless == nil || !*body.Serverless {
		t.Fatalf("serverless not sent on create: %v", body.Serverless)
	}

	body, diags = expandCreateTemplate(ctx, templateResourceModel{
		Name:       types.StringValue("pod-template"),
		Image:      types.StringValue("runpod/pytorch:2.8.0"),
		Serverless: types.BoolUnknown(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Serverless != nil {
		t.Fatalf("unset serverless must stay nil, got %v", *body.Serverless)
	}
}

func TestEndpointPortsRoundTrip(t *testing.T) {
	ctx := context.Background()
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	ports, diags := types.ListValueFrom(ctx, types.StringType, []string{"8000/http"})
	if diags.HasError() {
		t.Fatal(diags)
	}

	body, diags := expandCreateEndpoint(ctx, serverlessEndpointResourceModel{
		Name:         types.StringValue("lb"),
		Image:        types.StringValue("runpod/mock-worker:dev"),
		EndpointType: types.StringValue("LOAD_BALANCER"),
		GpuPools:     pools,
		ScalingType:  types.StringValue("REQUEST_COUNT"),
		ScalingValue: types.Float64Value(1),
		Ports:        ports,
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Ports == nil || len(*body.Ports) != 1 || (*body.Ports)[0] != "8000/http" {
		t.Fatalf("ports not sent on create: %v", body.Ports)
	}

	endpointPorts := []string{"8000/http"}
	endpointType := sdktypes.EndpointType("LOAD_BALANCER")
	var scaling sdktypes.EndpointScaling
	if err := scaling.FromRequestCountScaling(sdktypes.RequestCountScaling{
		Type: sdktypes.RequestCountScalingTypeRequestCount, RequestCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	endpoint := &sdktypes.Endpoint{
		Id:      "ep_abc123",
		Name:    "lb",
		Type:    &endpointType,
		Scaling: scaling,
		Ports:   &endpointPorts,
	}
	var model serverlessEndpointResourceModel
	if diags := flattenEndpoint(ctx, endpoint, &model); diags.HasError() {
		t.Fatal(diags)
	}
	var flattened []string
	if d := model.Ports.ElementsAs(ctx, &flattened, false); d.HasError() {
		t.Fatal(d)
	}
	if len(flattened) != 1 || flattened[0] != "8000/http" {
		t.Fatalf("ports did not round-trip: %v", flattened)
	}

	// A queue endpoint carries no ports; the attribute must flatten to null.
	endpoint.Ports = nil
	var queueModel serverlessEndpointResourceModel
	if diags := flattenEndpoint(ctx, endpoint, &queueModel); diags.HasError() {
		t.Fatal(diags)
	}
	if !queueModel.Ports.IsNull() {
		t.Fatalf("absent ports must flatten to null: %+v", queueModel.Ports)
	}
}

func TestExpandCreateEndpointOmitsUnknownWorkers(t *testing.T) {
	ctx := context.Background()
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}

	// workers_min, workers_max, and idle_timeout are Optional + Computed, so an
	// omitted one arrives Unknown: the body must carry no workers object at all.
	body, diags := expandCreateEndpoint(ctx, serverlessEndpointResourceModel{
		Name:         types.StringValue("ep"),
		Image:        types.StringValue("runpod/mock-worker:dev"),
		EndpointType: types.StringValue("QUEUE"),
		GpuPools:     pools,
		ScalingType:  types.StringValue("QUEUE_DELAY"),
		ScalingValue: types.Float64Value(4),
		WorkersMin:   types.Int64Unknown(),
		WorkersMax:   types.Int64Unknown(),
		IdleTimeout:  types.Int64Unknown(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Workers != nil {
		t.Fatalf("unknown worker bounds must omit workers: %+v", *body.Workers)
	}
}

// The deprecated REST v1 shims. Each attribute here worked in the 1.0.8
// provider and has no REST v2 equivalent, so these tests pin the v1 wire
// shape onto the state shape and the failure semantics onto the diagnostics.

func TestFlattenLegacyPod(t *testing.T) {
	machineId := "machine-1"
	encrypted := true
	pod := &legacy.PodV1{
		MachineID:        &machineId,
		DockerEntrypoint: []string{"/bin/bash"},
		DockerStartCmd:   []string{"-c", "sleep infinity"},
		VolumeEncrypted:  &encrypted,
	}

	// The v1 command lists are not read: docker_entrypoint and
	// docker_start_cmd come from the v2 pod, so whatever flattenPod set stays.
	model := podResourceModel{
		DockerEntrypoint: types.ListNull(types.StringType),
		DockerStartCmd:   types.ListNull(types.StringType),
	}
	flattenLegacyPod(pod, &model)
	if model.MachineId.ValueString() != "machine-1" || !model.VolumeEncrypted.ValueBool() {
		t.Fatalf("machine_id/volume_encrypted: %+v", model)
	}
	if !model.DockerEntrypoint.IsNull() || !model.DockerStartCmd.IsNull() {
		t.Fatalf("the REST v1 command lists must not reach the model: %+v", model)
	}
}

func TestFlattenLegacyPodEmptyResponse(t *testing.T) {
	// A pod the v1 host reports with none of these fields set.
	var model podResourceModel
	flattenLegacyPod(&legacy.PodV1{}, &model)
	if !model.MachineId.IsNull() || !model.VolumeEncrypted.IsNull() {
		t.Fatalf("empty pod: %+v", model)
	}
}

func TestClearLegacyPodFieldsNullsEveryShimAttribute(t *testing.T) {
	ctx := context.Background()
	machineId := "machine-1"
	encrypted := true
	entrypoint, diags := types.ListValueFrom(ctx, types.StringType, []string{"/bin/bash"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	model := podResourceModel{DockerEntrypoint: entrypoint}
	flattenLegacyPod(&legacy.PodV1{MachineID: &machineId, VolumeEncrypted: &encrypted}, &model)

	// A failed REST v1 read nulls the shim attributes and leaves the command
	// lists the v2 read already filled.
	clearLegacyPodFields(&model)
	if !model.MachineId.IsNull() || !model.VolumeEncrypted.IsNull() {
		t.Fatalf("every shim attribute must be null after a failed read: %+v", model)
	}
	if len(model.DockerEntrypoint.Elements()) != 1 {
		t.Fatalf("docker_entrypoint = %v, want the v2 value kept", model.DockerEntrypoint)
	}
}

func TestAppendLegacyReadFailureWarnsAndWriteFailureErrors(t *testing.T) {
	var readDiags diag.Diagnostics
	appendLegacyReadFailure(&readDiags, "pod", errors.New("rest v1 is gone"))
	if readDiags.HasError() {
		t.Fatalf("a shim read failure must not break the apply: %v", readDiags)
	}
	if readDiags.WarningsCount() != 1 {
		t.Fatalf("want one warning, got %v", readDiags)
	}
	if !strings.Contains(readDiags.Warnings()[0].Detail(), "rest v1 is gone") {
		t.Fatalf("the warning must carry the cause: %q", readDiags.Warnings()[0].Detail())
	}

	var writeDiags diag.Diagnostics
	appendLegacyWriteFailure(&writeDiags, "template", errors.New("rest v1 rejected the patch"))
	if !writeDiags.HasError() {
		t.Fatalf("a configured shim input that cannot be applied must be an error: %v", writeDiags)
	}
}

func TestExpandLegacyEndpointPatchCarriesOnlySetFields(t *testing.T) {
	// The patch is built from the configuration, where an unset attribute is
	// null on every operation.
	empty := expandLegacyEndpointPatch(serverlessEndpointResourceModel{
		ComputeType:       types.StringNull(),
		GpuTypePriority:   types.StringNull(),
		CpuFlavorPriority: types.StringNull(),
	})
	if !empty.IsEmpty() {
		t.Fatalf("an endpoint configuring no shim input must send nothing: %+v", empty)
	}

	// A configuration value that is not resolved yet is not a value to send.
	unresolved := expandLegacyEndpointPatch(serverlessEndpointResourceModel{
		GpuTypePriority:   types.StringUnknown(),
		CpuFlavorPriority: types.StringNull(),
	})
	if !unresolved.IsEmpty() {
		t.Fatalf("an unresolved shim input must send nothing: %+v", unresolved)
	}

	gpu := expandLegacyEndpointPatch(serverlessEndpointResourceModel{
		ComputeType:       types.StringValue("GPU"),
		GpuTypePriority:   types.StringValue("availability"),
		CpuFlavorPriority: types.StringNull(),
	})
	if gpu.CpuFlavorPriority != nil {
		t.Fatalf("cpu_flavor_priority must stay out of a GPU patch: %v", *gpu.CpuFlavorPriority)
	}
	if gpu.GpuTypePriority == nil || *gpu.GpuTypePriority != "availability" {
		t.Fatalf("gpu_type_priority: %+v", gpu)
	}

	cpu := expandLegacyEndpointPatch(serverlessEndpointResourceModel{
		ComputeType:       types.StringNull(),
		GpuTypePriority:   types.StringNull(),
		CpuFlavorPriority: types.StringValue("availability"),
	})
	if cpu.GpuTypePriority != nil {
		t.Fatalf("only cpu_flavor_priority was configured: %+v", cpu)
	}
	if cpu.CpuFlavorPriority == nil || *cpu.CpuFlavorPriority != "availability" {
		t.Fatalf("cpu_flavor_priority: %+v", cpu)
	}
}

func TestExpandLegacyEndpointPatchNeverCarriesComputeType(t *testing.T) {
	// compute_type is derived from the GPU or CPU block REST v2 reports and
	// policed against it, so a v1 PATCH carrying it could change nothing,
	// while its failure is an error. The canonical 1.x migration shape is
	// gpu_pools plus compute_type and no priority, and that must reach the
	// deprecated host with nothing to send.
	for _, family := range []string{"GPU", "CPU"} {
		patch := expandLegacyEndpointPatch(serverlessEndpointResourceModel{
			ComputeType:       types.StringValue(family),
			GpuTypePriority:   types.StringNull(),
			CpuFlavorPriority: types.StringNull(),
		})
		if !patch.IsEmpty() {
			t.Fatalf("compute_type = %q built a patch %+v, want nothing to send", family, patch)
		}
	}
}

func TestFlattenLegacyEndpoint(t *testing.T) {
	ctx := context.Background()
	templateVersion := 7
	version := 3
	uptime := 1200.0
	lastBusy := 45.0
	workerId := "worker-1"
	podId := "pod-1"
	status := "RUNNING"
	startTime := "2026-09-17T09:00:00Z"

	var model serverlessEndpointResourceModel
	diags := flattenLegacyEndpoint(ctx, &legacy.EndpointV1{
		TemplateVersion: &templateVersion,
		UserID:          "user-1",
		Version:         &version,
		Workers: []legacy.EndpointWorkerV1{{
			ID:         &workerId,
			PodID:      &podId,
			Status:     &status,
			UptimeMs:   &uptime,
			StartTime:  &startTime,
			LastBusyMs: &lastBusy,
		}},
	}, &model)
	if diags.HasError() {
		t.Fatal(diags)
	}

	if model.TemplateVersion.ValueInt64() != 7 || model.Version.ValueInt64() != 3 {
		t.Fatalf("template_version/version: %+v", model)
	}
	var users []string
	if d := model.Users.ElementsAs(ctx, &users, false); d.HasError() {
		t.Fatal(d)
	}
	if len(users) != 1 || users[0] != "user-1" {
		t.Fatalf("users must be the owner wrapped in a one-element list: %v", users)
	}
	var workers []endpointLegacyWorkerModel
	if d := model.Workers.ElementsAs(ctx, &workers, false); d.HasError() {
		t.Fatal(d)
	}
	if len(workers) != 1 {
		t.Fatalf("len(workers) = %d, want 1", len(workers))
	}
	worker := workers[0]
	if worker.Id.ValueString() != "worker-1" || worker.PodId.ValueString() != "pod-1" ||
		worker.Status.ValueString() != "RUNNING" || worker.UptimeMs.ValueInt64() != 1200 ||
		worker.StartTime.ValueString() != "2026-09-17T09:00:00Z" || worker.LastBusyMs.ValueInt64() != 45 {
		t.Fatalf("worker: %+v", worker)
	}
}

func TestFlattenLegacyEndpointEmptyAndCleared(t *testing.T) {
	ctx := context.Background()

	// An endpoint built from an image has no template version, a fresh one has
	// no workers, and neither is an error: every absent read-back goes null,
	// the same null a failed read produces, so "nothing here" has one shape.
	var model serverlessEndpointResourceModel
	if diags := flattenLegacyEndpoint(ctx, &legacy.EndpointV1{}, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.TemplateVersion.IsNull() || !model.Version.IsNull() || !model.Users.IsNull() {
		t.Fatalf("absent v1 fields must flatten to null: %+v", model)
	}
	if !model.Workers.IsNull() {
		t.Fatalf("an endpoint with no workers must report null, not an empty list: %+v", model.Workers)
	}

	clearLegacyEndpointFields(&model)
	if !model.TemplateVersion.IsNull() || !model.Users.IsNull() ||
		!model.Version.IsNull() || !model.Workers.IsNull() {
		t.Fatalf("every shim read-back must be null after a failed read: %+v", model)
	}
}

func TestFlattenEndpointDerivesComputeType(t *testing.T) {
	ctx := context.Background()
	var scaling sdktypes.EndpointScaling
	if err := scaling.FromQueueDelayScaling(sdktypes.QueueDelayScaling{
		Type: sdktypes.QueueDelayScalingTypeQueueDelay, QueueDelay: 4,
	}); err != nil {
		t.Fatal(err)
	}
	endpoint := &sdktypes.Endpoint{
		Id:      "ep_abc",
		Name:    "inference",
		Scaling: scaling,
		Gpu:     &sdktypes.EndpointGpuConfig{Pools: []string{"ADA_24"}},
	}

	var gpuModel serverlessEndpointResourceModel
	gpuModel.ComputeType = types.StringUnknown()
	if diags := flattenEndpoint(ctx, endpoint, &gpuModel); diags.HasError() {
		t.Fatal(diags)
	}
	if gpuModel.ComputeType.ValueString() != "GPU" {
		t.Fatalf("compute_type = %q, want GPU", gpuModel.ComputeType.ValueString())
	}

	endpoint.Gpu = nil
	endpoint.Cpu = &[]sdktypes.CpuConfig{{Id: "cpu5c", VcpuCount: 4}}
	var cpuModel serverlessEndpointResourceModel
	cpuModel.ComputeType = types.StringUnknown()
	if diags := flattenEndpoint(ctx, endpoint, &cpuModel); diags.HasError() {
		t.Fatal(diags)
	}
	if cpuModel.ComputeType.ValueString() != "CPU" {
		t.Fatalf("compute_type = %q, want CPU", cpuModel.ComputeType.ValueString())
	}
}

func TestExpandLegacyTemplatePatchCarriesOnlySetFields(t *testing.T) {
	ctx := context.Background()

	// The patch is built from the configuration, where an unset attribute is
	// null on every operation. On an update the plan carries the prior state
	// value for both, because they are Optional + Computed, and a patch built
	// from that would write to the deprecated host on every apply.
	empty, diags := expandLegacyTemplatePatch(ctx, templateResourceModel{
		DockerEntrypoint: types.ListNull(types.StringType),
		Readme:           types.StringNull(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !empty.IsEmpty() {
		t.Fatalf("a template configuring no shim input must send nothing: %+v", empty)
	}

	// A configuration value that is not resolved yet is not a value to send.
	unresolved, diags := expandLegacyTemplatePatch(ctx, templateResourceModel{
		DockerEntrypoint: types.ListUnknown(types.StringType),
		Readme:           types.StringUnknown(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !unresolved.IsEmpty() {
		t.Fatalf("an unresolved shim input must send nothing: %+v", unresolved)
	}

	entrypoint, listDiags := types.ListValueFrom(ctx, types.StringType, []string{"/bin/bash"})
	if listDiags.HasError() {
		t.Fatal(listDiags)
	}
	patch, diags := expandLegacyTemplatePatch(ctx, templateResourceModel{
		DockerEntrypoint: entrypoint,
		Readme:           types.StringNull(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if patch.Readme != nil {
		t.Fatalf("an unset readme must stay out of the patch: %q", *patch.Readme)
	}
	if patch.DockerEntrypoint == nil || len(*patch.DockerEntrypoint) != 1 ||
		(*patch.DockerEntrypoint)[0] != "/bin/bash" {
		t.Fatalf("docker_entrypoint: %+v", patch)
	}

	// An explicitly empty readme clears the text, so it is sent rather than
	// treated as unset.
	cleared, diags := expandLegacyTemplatePatch(ctx, templateResourceModel{
		DockerEntrypoint: types.ListNull(types.StringType),
		Readme:           types.StringValue(""),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if cleared.Readme == nil || *cleared.Readme != "" {
		t.Fatalf("an empty readme must be sent: %+v", cleared)
	}
}

func TestFlattenLegacyTemplate(t *testing.T) {
	ctx := context.Background()
	readme := "hello"
	earned := 1.25
	isRunpod := true
	runtime := 90.0

	var model templateResourceModel
	diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{
		DockerEntrypoint: []string{"/bin/bash"},
		Readme:           &readme,
		Earned:           &earned,
		IsRunpod:         &isRunpod,
		RuntimeInMin:     &runtime,
	}, false, &model)
	if diags.HasError() {
		t.Fatal(diags)
	}

	var entrypoint []string
	if d := model.DockerEntrypoint.ElementsAs(ctx, &entrypoint, false); d.HasError() {
		t.Fatal(d)
	}
	if len(entrypoint) != 1 || entrypoint[0] != "/bin/bash" {
		t.Fatalf("docker_entrypoint: %v", entrypoint)
	}
	if model.Readme.ValueString() != "hello" || model.Earned.ValueFloat64() != 1.25 ||
		!model.IsRunpod.ValueBool() || model.RuntimeInMin.ValueInt64() != 90 {
		t.Fatalf("template shim fields: %+v", model)
	}

	// A template the v1 host reports bare, onto a model holding nothing: every
	// absent field is null, the way 1.0.8 left an attribute unset when the
	// response did not carry it. A zero is a reported earning, not a silence.
	var bare templateResourceModel
	if diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{}, false, &bare); diags.HasError() {
		t.Fatal(diags)
	}
	if !bare.DockerEntrypoint.IsNull() || !bare.Readme.IsNull() {
		t.Fatalf("absent configurable fields must stay null: %+v", bare)
	}
	if !bare.Earned.IsNull() || !bare.IsRunpod.IsNull() || !bare.RuntimeInMin.IsNull() {
		t.Fatalf("absent counters must be null, not zero: %+v", bare)
	}

	// The read-backs go null on a failed call; the two configured inputs are
	// kept, which TestClearLegacyTemplateFieldsKeepsTheConfiguredInputs covers.
	clearLegacyTemplateFields(&model)
	if !model.Earned.IsNull() || !model.IsRunpod.IsNull() || !model.RuntimeInMin.IsNull() {
		t.Fatalf("every shim read-back must be null after a failed read: %+v", model)
	}
}

func TestFlattenLegacyTemplateKeepsThePlannedValueTheResponseOmits(t *testing.T) {
	ctx := context.Background()

	// A v1 PATCH that answers with a partial body must not report a configured
	// readme as cleared: that would be an inconsistent result after apply.
	planned, listDiags := types.ListValueFrom(ctx, types.StringType, []string{"/bin/bash"})
	if listDiags.HasError() {
		t.Fatal(listDiags)
	}
	model := templateResourceModel{
		DockerEntrypoint: planned,
		Readme:           types.StringValue("hello"),
	}
	if diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{}, true, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Readme.ValueString() != "hello" {
		t.Fatalf("readme = %v, want the planned value kept", model.Readme)
	}
	if len(model.DockerEntrypoint.Elements()) != 1 {
		t.Fatalf("docker_entrypoint = %v, want the planned value kept", model.DockerEntrypoint)
	}

	// An empty string and an empty list are answers, not omissions: a
	// configuration clearing either one round trips.
	cleared := ""
	model = templateResourceModel{
		DockerEntrypoint: planned,
		Readme:           types.StringValue("hello"),
	}
	if diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{
		DockerEntrypoint: []string{},
		Readme:           &cleared,
	}, true, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Readme.ValueString() != "" || model.Readme.IsNull() {
		t.Fatalf("readme = %v, want the cleared empty string", model.Readme)
	}
	if model.DockerEntrypoint.IsNull() || len(model.DockerEntrypoint.Elements()) != 0 {
		t.Fatalf("docker_entrypoint = %v, want an empty list", model.DockerEntrypoint)
	}

	// State cannot hold unknown, so an unresolved planned value the response
	// does not carry resolves to null.
	model = templateResourceModel{
		DockerEntrypoint: types.ListUnknown(types.StringType),
		Readme:           types.StringUnknown(),
	}
	if diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{}, true, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.Readme.IsNull() || !model.DockerEntrypoint.IsNull() {
		t.Fatalf("an unknown planned value must resolve to null: %+v", model)
	}
}

func TestFlattenLegacyTemplateFollowsAGetThatNoLongerReportsTheField(t *testing.T) {
	ctx := context.Background()

	// The keep-the-planned-value rule belongs to the PATCH response alone. A
	// GET reports the template as it is, and `"readme": null` decodes to the
	// same nil an omitted field does, so keeping "hi" here would leave a
	// readme cleared in the console invisible to every later plan.
	planned, listDiags := types.ListValueFrom(ctx, types.StringType, []string{"/bin/bash"})
	if listDiags.HasError() {
		t.Fatal(listDiags)
	}
	model := templateResourceModel{
		DockerEntrypoint: planned,
		Readme:           types.StringValue("hi"),
	}
	if diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{}, false, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.Readme.IsNull() {
		t.Fatalf("readme = %v, want null so the cleared readme shows in the next plan", model.Readme)
	}
	if !model.DockerEntrypoint.IsNull() {
		t.Fatalf("docker_entrypoint = %v, want null", model.DockerEntrypoint)
	}

	// What the GET does report still lands.
	readme := "# trainer"
	model = templateResourceModel{Readme: types.StringValue("hi")}
	if diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{
		DockerEntrypoint: []string{"/bin/bash"},
		Readme:           &readme,
	}, false, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Readme.ValueString() != "# trainer" || len(model.DockerEntrypoint.Elements()) != 1 {
		t.Fatalf("a reported field must land: %+v", model)
	}
}

func TestAppendLegacyReadFailureStaysSilentOnANotFound(t *testing.T) {
	// The v1 route does not know a CPU pod, a cluster pod, or a template
	// created through v2. That is missing legacy data, not a failed read, and
	// warning about it on every refresh forever is noise nobody can act on.
	var diags diag.Diagnostics
	appendLegacyReadFailure(&diags, "template", &legacy.NotFoundError{Path: "/templates/tpl_abc"})
	if diags.HasError() || diags.WarningsCount() != 0 {
		t.Fatalf("a 404 must produce no diagnostic: %v", diags)
	}

	appendLegacyReadFailure(&diags, "template", errors.New("rest v1 is gone"))
	if diags.WarningsCount() != 1 {
		t.Fatalf("every other failure still warns: %v", diags)
	}
}

func TestResyncConfiguredEnvTracksOnlyTheConfiguredKeys(t *testing.T) {
	ctx := context.Background()

	configured, diags := types.MapValueFrom(ctx, types.StringType, map[string]string{
		"MODEL": "llama",
		"SEED":  "1",
		"GONE":  "yes",
	})
	if diags.HasError() {
		t.Fatal(diags)
	}

	// The API merges the template's env in and injects its own keys; only the
	// configured ones come back, and one the API no longer carries drops out
	// so the drift shows up in the next plan.
	env := configured
	if diags := resyncConfiguredEnv(ctx, map[string]string{
		"MODEL":            "mistral",
		"SEED":             "1",
		"PUBLIC_KEY":       "ssh-ed25519 …",
		"JUPYTER_PASSWORD": "changeme",
	}, &env); diags.HasError() {
		t.Fatal(diags)
	}

	var refreshed map[string]string
	if diags := env.ElementsAs(ctx, &refreshed, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(refreshed) != 2 {
		t.Fatalf("env = %v, want only the configured keys the API still carries", refreshed)
	}
	if refreshed["MODEL"] != "mistral" || refreshed["SEED"] != "1" {
		t.Fatalf("env = %v, want the API's values for the configured keys", refreshed)
	}

	// A configuration that sets one of the two keys the API owns keeps its own
	// value: the API rewrites PUBLIC_KEY with the account's registered keys and
	// generates JUPYTER_PASSWORD, so resyncing them would put the same diff on
	// every plan forever.
	apiOwned, diags := types.MapValueFrom(ctx, types.StringType, map[string]string{
		"PUBLIC_KEY":       "ssh-ed25519 AAAA…",
		"JUPYTER_PASSWORD": "hunter2",
		"MODEL":            "llama",
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	env = apiOwned
	if diags := resyncConfiguredEnv(ctx, map[string]string{
		"PUBLIC_KEY":       "ssh-ed25519 BBBB… account-key",
		"JUPYTER_PASSWORD": "generated-by-the-api",
		"MODEL":            "mistral",
	}, &env); diags.HasError() {
		t.Fatal(diags)
	}
	if diags := env.ElementsAs(ctx, &refreshed, false); diags.HasError() {
		t.Fatal(diags)
	}
	if refreshed["PUBLIC_KEY"] != "ssh-ed25519 AAAA…" || refreshed["JUPYTER_PASSWORD"] != "hunter2" {
		t.Fatalf("env = %v, want the configured values for the keys the API owns", refreshed)
	}
	if refreshed["MODEL"] != "mistral" {
		t.Fatalf("env = %v, want every other key still resynced", refreshed)
	}

	// An unknown env is the flatten path's to fill, so a refresh leaves it
	// alone.
	unknown := types.MapUnknown(types.StringType)
	if diags := resyncConfiguredEnv(ctx, map[string]string{"MODEL": "mistral"}, &unknown); diags.HasError() {
		t.Fatal(diags)
	}
	if !unknown.IsUnknown() {
		t.Fatalf("env = %v, want unknown untouched", unknown)
	}

	// A null env is what an import leaves, and nothing was ever planned for
	// it: the refresh takes the whole response rather than leaving it null.
	imported := types.MapNull(types.StringType)
	if diags := resyncConfiguredEnv(ctx, map[string]string{"MODEL": "mistral", "PUBLIC_KEY": "ssh-ed25519 AAAA…"}, &imported); diags.HasError() {
		t.Fatal(diags)
	}
	if diags := imported.ElementsAs(ctx, &refreshed, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(refreshed) != 2 || refreshed["MODEL"] != "mistral" || refreshed["PUBLIC_KEY"] != "ssh-ed25519 AAAA…" {
		t.Fatalf("env = %v, want the whole response after an import", refreshed)
	}
}

func TestClearLegacyTemplateFieldsKeepsTheConfiguredInputs(t *testing.T) {
	ctx := context.Background()

	// The v1 host refusing a template REST v2 created, a 404 the read path
	// passes over in silence, must not null what the configuration set. State
	// would then disagree with the configuration on every later plan, and every
	// later apply would re-run the same failing write, forever. Only the three
	// read-backs go null.
	entrypoint, diags := types.ListValueFrom(ctx, types.StringType, []string{"/bin/bash"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	model := templateResourceModel{
		DockerEntrypoint: entrypoint,
		Readme:           types.StringValue("# trainer"),
		Earned:           types.Float64Value(12.5),
		IsRunpod:         types.BoolValue(true),
		RuntimeInMin:     types.Int64Value(900),
	}

	clearLegacyTemplateFields(&model)

	if model.Readme.ValueString() != "# trainer" {
		t.Fatalf("readme = %v, want the configured value so the next plan converges", model.Readme)
	}
	if len(model.DockerEntrypoint.Elements()) != 1 {
		t.Fatalf("docker_entrypoint = %v, want the configured value", model.DockerEntrypoint)
	}
	if !model.Earned.IsNull() || !model.IsRunpod.IsNull() || !model.RuntimeInMin.IsNull() {
		t.Fatalf("the read-backs must be null after a failed call: %+v", model)
	}

	// State cannot hold unknown: an input planned unknown, which is every
	// create the configuration does not set, still resolves to null.
	unset := templateResourceModel{
		DockerEntrypoint: types.ListUnknown(types.StringType),
		Readme:           types.StringUnknown(),
	}
	clearLegacyTemplateFields(&unset)
	if !unset.DockerEntrypoint.IsNull() || !unset.Readme.IsNull() {
		t.Fatalf("an unresolved input must resolve to null: %+v", unset)
	}

	// The resource keeps a configured input because the configuration owns it.
	// The data source has no configuration and rebuilds its state from the v2
	// read every call, so its four shim attributes stay read-backs there.
	detail := templateDataSourceModel{
		Readme:       types.StringValue("# stale"),
		Earned:       types.Float64Value(1.5),
		IsRunpod:     types.BoolValue(true),
		RuntimeInMin: types.Int64Value(120),
	}
	clearLegacyTemplateDetailFields(&detail)
	if !detail.Readme.IsNull() || !detail.Earned.IsNull() || !detail.IsRunpod.IsNull() || !detail.RuntimeInMin.IsNull() {
		t.Fatalf("a data source read-back is null after a failed read: %+v", detail)
	}
}

func TestFlattenLegacyTemplateKeepsThePlannedCountersAPatchOmits(t *testing.T) {
	ctx := context.Background()

	// The partial PATCH echo the fromPatch rule exists for, now over the three
	// counters: 204, or a body carrying only the fields it was sent, must not
	// report a template's earnings as zero. They are Computed and planned
	// unknown, so no plan diff would ever flag the fabricated value.
	model := templateResourceModel{
		Readme:       types.StringValue("# trainer"),
		Earned:       types.Float64Value(12.5),
		IsRunpod:     types.BoolValue(true),
		RuntimeInMin: types.Int64Value(900),
	}
	if diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{}, true, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Earned.ValueFloat64() != 12.5 || !model.IsRunpod.ValueBool() || model.RuntimeInMin.ValueInt64() != 900 {
		t.Fatalf("a partial patch response must keep the planned counters: %+v", model)
	}

	// A reported zero is an answer, not an omission.
	zero := 0.0
	no := false
	none := 0.0
	model = templateResourceModel{
		Earned:       types.Float64Value(12.5),
		IsRunpod:     types.BoolValue(true),
		RuntimeInMin: types.Int64Value(900),
	}
	if diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{
		Earned: &zero, IsRunpod: &no, RuntimeInMin: &none,
	}, true, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Earned.ValueFloat64() != 0 || model.IsRunpod.ValueBool() || model.RuntimeInMin.ValueInt64() != 0 {
		t.Fatalf("a reported zero must land: %+v", model)
	}

	// A GET still reports the template as it is, so an absent counter is null.
	model = templateResourceModel{
		Earned:       types.Float64Value(12.5),
		IsRunpod:     types.BoolValue(true),
		RuntimeInMin: types.Int64Value(900),
	}
	if diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{}, false, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.Earned.IsNull() || !model.IsRunpod.IsNull() || !model.RuntimeInMin.IsNull() {
		t.Fatalf("a get that omits the counters nulls them: %+v", model)
	}

	// The minutes are truncated the way 1.0.8 truncated the JSON number.
	fractional := 90.7
	model = templateResourceModel{}
	if diags := flattenLegacyTemplate(ctx, &legacy.TemplateV1{RuntimeInMin: &fractional}, false, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.RuntimeInMin.ValueInt64() != 90 {
		t.Fatalf("runtime_in_min = %v, want 90", model.RuntimeInMin)
	}
}

func TestFlattenLegacyAbsentScalarsAreNullNotEmpty(t *testing.T) {
	ctx := context.Background()

	// v1.0.8 wrote machine_id only for a present, non-empty machineId and
	// volume_encrypted only for a present volumeEncrypted. A CPU or cluster pod
	// carries neither, and a 1.x configuration's `machine_id != null` check has
	// to keep answering what it did.
	empty := ""
	var model podResourceModel
	flattenLegacyPod(&legacy.PodV1{MachineID: &empty}, &model)
	if !model.MachineId.IsNull() {
		t.Fatalf("machine_id = %v, want null rather than an empty string", model.MachineId)
	}
	if !model.VolumeEncrypted.IsNull() {
		t.Fatalf("volume_encrypted = %v, want null rather than false", model.VolumeEncrypted)
	}

	// A reported false is an answer.
	no := false
	flattenLegacyPod(&legacy.PodV1{VolumeEncrypted: &no}, &model)
	if model.VolumeEncrypted.IsNull() || model.VolumeEncrypted.ValueBool() {
		t.Fatalf("volume_encrypted = %v, want a reported false", model.VolumeEncrypted)
	}

	// The same rule over the worker strings, plus the truncation the two
	// millisecond counters go through.
	uptime := 1234.5
	var endpointModel serverlessEndpointResourceModel
	if diags := flattenLegacyEndpoint(ctx, &legacy.EndpointV1{
		Workers: []legacy.EndpointWorkerV1{{UptimeMs: &uptime}},
	}, &endpointModel); diags.HasError() {
		t.Fatal(diags)
	}
	var workers []endpointLegacyWorkerModel
	if d := endpointModel.Workers.ElementsAs(ctx, &workers, false); d.HasError() {
		t.Fatal(d)
	}
	if len(workers) != 1 {
		t.Fatalf("len(workers) = %d, want 1", len(workers))
	}
	if !workers[0].Id.IsNull() || !workers[0].PodId.IsNull() || !workers[0].Status.IsNull() {
		t.Fatalf("absent worker fields must be null: %+v", workers[0])
	}
	if workers[0].UptimeMs.ValueInt64() != 1234 {
		t.Fatalf("uptime_ms = %v, want the truncated 1234", workers[0].UptimeMs)
	}
	if !workers[0].LastBusyMs.IsNull() {
		t.Fatalf("last_busy_ms = %v, want null", workers[0].LastBusyMs)
	}
}

func TestAppendLegacyWriteFailureSeparatesALandedWriteFromAMissingObject(t *testing.T) {
	// A 2xx body that would not decode came back from a write the host already
	// took: failing the apply there reports a change that landed as an error.
	var decodeDiags diag.Diagnostics
	appendLegacyWriteFailure(&decodeDiags, "endpoint", &legacy.DecodeError{
		Err: errors.New("cannot unmarshal number 1234.5 into Go struct field"),
	})
	if decodeDiags.HasError() {
		t.Fatalf("a write that landed must not fail the apply: %v", decodeDiags)
	}
	if decodeDiags.WarningsCount() != 1 {
		t.Fatalf("want one warning, got %v", decodeDiags)
	}

	// A 404 is still an error, and says what the user can do about it rather
	// than relaying a bare "templates/tpl_abc was not found".
	var notFoundDiags diag.Diagnostics
	appendLegacyWriteFailure(&notFoundDiags, "template", &legacy.NotFoundError{Path: "/templates/tpl_abc"})
	if !notFoundDiags.HasError() {
		t.Fatalf("a configured input the legacy host cannot apply is an error: %v", notFoundDiags)
	}
	detail := notFoundDiags.Errors()[0].Detail()
	if !strings.Contains(detail, "does not know this template") || !strings.Contains(detail, "Drop the deprecated attribute") {
		t.Fatalf("the 404 must name its cause and the way out: %q", detail)
	}
}

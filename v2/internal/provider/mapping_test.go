package provider

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
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

func TestExpandPodMountsGlobal(t *testing.T) {
	ctx := context.Background()
	network, diags := types.ListValueFrom(ctx, networkMountObjectType, []networkMountModel{{
		VolumeId: types.StringValue("vol_xyz"),
		Path:     types.StringValue("/runpod-volume"),
	}})
	if diags.HasError() {
		t.Fatal(diags)
	}
	global, diags := types.ListValueFrom(ctx, networkMountObjectType, []networkMountModel{{
		VolumeId: types.StringValue("gsb_abc"),
		Path:     types.StringValue("/data"),
	}})
	if diags.HasError() {
		t.Fatal(diags)
	}
	globalNoPath, diags := types.ListValueFrom(ctx, networkMountObjectType, []networkMountModel{{
		VolumeId: types.StringValue("gsb_abc"),
		Path:     types.StringNull(),
	}})
	if diags.HasError() {
		t.Fatal(diags)
	}

	mounts, diags := expandPodMounts(ctx, podResourceModel{GlobalVolumes: global})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if mounts == nil || mounts.Global == nil || len(*mounts.Global) != 1 || (*mounts.Global)[0].VolumeId != "gsb_abc" ||
		(*mounts.Global)[0].Path == nil || *(*mounts.Global)[0].Path != "/data" || mounts.Network != nil || mounts.Persistent != nil {
		t.Fatalf("global mount: %+v", mounts)
	}

	// A global volume combines with a network volume in one mounts object.
	mounts, diags = expandPodMounts(ctx, podResourceModel{NetworkVolumes: network, GlobalVolumes: global})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if mounts == nil || mounts.Global == nil || mounts.Network == nil || len(*mounts.Network) != 1 || len(*mounts.Global) != 1 {
		t.Fatalf("network and global mounts: %+v", mounts)
	}

	// An unset path is left off, so the API picks one on create and keeps
	// the current one on a PATCH.
	mounts, diags = expandPodMounts(ctx, podResourceModel{GlobalVolumes: globalNoPath})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if mounts == nil || mounts.Global == nil || (*mounts.Global)[0].Path != nil {
		t.Fatalf("global mount without a path: %+v", mounts)
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
	if !model.GlobalVolumes.IsNull() {
		t.Fatalf("no global mount, got %+v", model.GlobalVolumes)
	}

	// A global mount next to the network one, read back with the path the
	// API chose.
	globalPath := "/global"
	pod.Mounts.Global = &[]sdktypes.GlobalMount{{VolumeId: "gsb_abc", Path: &globalPath}}
	model = podResourceModel{}
	if diags := flattenPod(ctx, pod, &model); diags.HasError() {
		t.Fatal(diags)
	}
	var globals []networkMountModel
	if d := model.GlobalVolumes.ElementsAs(ctx, &globals, false); d.HasError() {
		t.Fatal(d)
	}
	if len(globals) != 1 || globals[0].VolumeId.ValueString() != "gsb_abc" || globals[0].Path.ValueString() != "/global" {
		t.Fatalf("global volumes: %+v", globals)
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

func TestConfiguredTemplateReadmeReadsOnlyTheConfiguration(t *testing.T) {
	// readme is Optional+Computed, so on an update the plan carries the prior
	// state value even when the configuration never set it. The write is
	// decided from the configuration, where an unset readme is null on every
	// operation.
	if got := configuredTemplateReadme(templateResourceModel{Readme: types.StringNull()}); got != nil {
		t.Fatalf("unset readme = %q, want nothing to write", *got)
	}
	if got := configuredTemplateReadme(templateResourceModel{Readme: types.StringUnknown()}); got != nil {
		t.Fatalf("unknown readme = %q, want nothing to write", *got)
	}
	got := configuredTemplateReadme(templateResourceModel{Readme: types.StringValue("# trainer")})
	if got == nil || *got != "# trainer" {
		t.Fatalf("configured readme = %v, want # trainer", got)
	}
}

func TestValidateTemplateReadmeRejectsAReadmeThatCannotApply(t *testing.T) {
	for _, tc := range []struct {
		name      string
		readme    types.String
		wantError bool
	}{
		{"unset", types.StringNull(), false},
		{"not known until apply", types.StringUnknown(), false},
		{"text", types.StringValue("# trainer\n"), false},
		{"empty", types.StringValue(""), true},
		{"whitespace only", types.StringValue(" \n\t"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateTemplateReadme(&diags, tc.readme)
			if diags.HasError() != tc.wantError {
				t.Fatalf("error = %v, want %v: %v", diags.HasError(), tc.wantError, diags)
			}
			if !tc.wantError {
				return
			}
			withPath, ok := diags.Errors()[0].(diag.DiagnosticWithPath)
			if !ok || !withPath.Path().Equal(path.Root("readme")) {
				t.Fatalf("error = %v, want it on readme", diags.Errors()[0])
			}
		})
	}
}

func TestExpandTemplateEntrypointSendsOnlyAConfiguredList(t *testing.T) {
	ctx := context.Background()

	// The plan carries the prior entrypoint when the configuration leaves it
	// out, so only the configuration decides what is sent.
	for _, unset := range []types.List{types.ListNull(types.StringType), types.ListUnknown(types.StringType)} {
		got, diags := expandTemplateEntrypoint(ctx, templateResourceModel{DockerEntrypoint: unset})
		if diags.HasError() {
			t.Fatal(diags)
		}
		if got != nil {
			t.Fatalf("entrypoint for %v = %v, want it left out", unset, *got)
		}
	}

	// [] is how the API clears the entrypoint, so it is sent, not omitted.
	cleared, diags := types.ListValueFrom(ctx, types.StringType, []string{})
	if diags.HasError() {
		t.Fatal(diags)
	}
	got, diags := expandTemplateEntrypoint(ctx, templateResourceModel{DockerEntrypoint: cleared})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if got == nil || len(*got) != 0 {
		t.Fatalf("entrypoint = %v, want an empty list to send", got)
	}

	configured, diags := types.ListValueFrom(ctx, types.StringType, []string{"/bin/bash", "-c"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	got, diags = expandTemplateEntrypoint(ctx, templateResourceModel{DockerEntrypoint: configured})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if got == nil || !slices.Equal(*got, []string{"/bin/bash", "-c"}) {
		t.Fatalf("entrypoint = %v, want [/bin/bash -c]", got)
	}
}

func TestFlattenTemplateEntrypointKeepsAPlannedClear(t *testing.T) {
	ctx := context.Background()

	reported := []string{"/bin/bash", "-c"}
	got, diags := flattenTemplateEntrypoint(ctx, &reported, types.ListUnknown(types.StringType))
	if diags.HasError() {
		t.Fatal(diags)
	}
	var entrypoint []string
	if diags := got.ElementsAs(ctx, &entrypoint, false); diags.HasError() {
		t.Fatal(diags)
	}
	if !slices.Equal(entrypoint, reported) {
		t.Fatalf("docker_entrypoint = %v, want %v", entrypoint, reported)
	}

	// A cleared entrypoint can come back absent or empty; a planned [] must
	// stay [] either way, or the apply fails against a null.
	planned, diags := types.ListValueFrom(ctx, types.StringType, []string{})
	if diags.HasError() {
		t.Fatal(diags)
	}
	empty := []string{}
	for _, answer := range []*[]string{nil, &empty} {
		got, diags := flattenTemplateEntrypoint(ctx, answer, planned)
		if diags.HasError() {
			t.Fatal(diags)
		}
		if got.IsNull() || len(got.Elements()) != 0 {
			t.Fatalf("docker_entrypoint = %v, want the planned []", got)
		}
	}

	// Nothing planned: an absent entrypoint is null, typed as a list of
	// strings so the state can hold it.
	for _, unset := range []types.List{types.ListNull(types.StringType), types.ListUnknown(types.StringType)} {
		got, diags := flattenTemplateEntrypoint(ctx, nil, unset)
		if diags.HasError() {
			t.Fatal(diags)
		}
		if !got.IsNull() || got.ElementType(ctx) != types.StringType {
			t.Fatalf("docker_entrypoint = %v, want a null list of strings", got)
		}
	}
}

func TestFlattenLegacyTemplateInto(t *testing.T) {
	readme := "# trainer"
	earned := 12.5
	yes := true
	minutes := int64(900)
	answer := &legacyTemplateFields{Readme: &readme, Earned: &earned, IsRunpod: &yes, RuntimeInMin: &minutes}

	model := templateResourceModel{Readme: types.StringUnknown()}
	flattenLegacyTemplateInto(answer, &model.Readme, &model.Earned, &model.IsRunpod, &model.RuntimeInMin)
	if model.Readme.ValueString() != "# trainer" || model.Earned.ValueFloat64() != 12.5 ||
		!model.IsRunpod.ValueBool() || model.RuntimeInMin.ValueInt64() != 900 {
		t.Fatalf("model = %+v, want the four answered values", model)
	}

	// saveTemplate stores the trimmed text: a heredoc readme's trailing
	// newline is not a change, so the planned value stays.
	model.Readme = types.StringValue("# trainer\n")
	flattenLegacyTemplateInto(answer, &model.Readme, &model.Earned, &model.IsRunpod, &model.RuntimeInMin)
	if model.Readme.ValueString() != "# trainer\n" {
		t.Fatalf("readme = %q, want the planned text kept", model.Readme.ValueString())
	}

	// Different text is a change made elsewhere, and the answer wins.
	model.Readme = types.StringValue("# old")
	flattenLegacyTemplateInto(answer, &model.Readme, &model.Earned, &model.IsRunpod, &model.RuntimeInMin)
	if model.Readme.ValueString() != "# trainer" {
		t.Fatalf("readme = %q, want the answered text", model.Readme.ValueString())
	}

	// A reported zero is an answer; an absent field is null.
	zero := 0.0
	no := false
	none := int64(0)
	model = templateResourceModel{Readme: types.StringValue("# trainer")}
	flattenLegacyTemplateInto(&legacyTemplateFields{Earned: &zero, IsRunpod: &no, RuntimeInMin: &none},
		&model.Readme, &model.Earned, &model.IsRunpod, &model.RuntimeInMin)
	if !model.Readme.IsNull() {
		t.Fatalf("readme = %v, want null for a template with none", model.Readme)
	}
	if model.Earned.IsNull() || model.Earned.ValueFloat64() != 0 || model.IsRunpod.IsNull() || model.IsRunpod.ValueBool() ||
		model.RuntimeInMin.IsNull() || model.RuntimeInMin.ValueInt64() != 0 {
		t.Fatalf("model = %+v, want the reported zeros", model)
	}

	flattenLegacyTemplateInto(&legacyTemplateFields{}, &model.Readme, &model.Earned, &model.IsRunpod, &model.RuntimeInMin)
	if !model.Earned.IsNull() || !model.IsRunpod.IsNull() || !model.RuntimeInMin.IsNull() {
		t.Fatalf("model = %+v, want null for absent fields", model)
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

func TestClearLegacyTemplateFieldsKeepsTheConfiguredReadme(t *testing.T) {
	// A shim call that did not answer must not null what the configuration
	// set. State would then disagree with the configuration on every later
	// plan, and every later apply would re-run the same failing write. Only
	// the three read-backs go null.
	model := templateResourceModel{
		Readme:       types.StringValue("# trainer"),
		Earned:       types.Float64Value(12.5),
		IsRunpod:     types.BoolValue(true),
		RuntimeInMin: types.Int64Value(900),
	}

	clearLegacyTemplateFields(&model)

	if model.Readme.ValueString() != "# trainer" {
		t.Fatalf("readme = %v, want the configured value so the next plan converges", model.Readme)
	}
	if !model.Earned.IsNull() || !model.IsRunpod.IsNull() || !model.RuntimeInMin.IsNull() {
		t.Fatalf("the read-backs must be null after a failed call: %+v", model)
	}

	// State cannot hold unknown: a readme planned unknown, which is every
	// create the configuration does not set it on, resolves to null.
	unset := templateResourceModel{Readme: types.StringUnknown()}
	clearLegacyTemplateFields(&unset)
	if !unset.Readme.IsNull() {
		t.Fatalf("an unresolved readme must resolve to null: %+v", unset)
	}

	// The data source has no configuration and rebuilds its state from the v2
	// read every call, so its four shim attributes are all read-backs.
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

package provider

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// clusterMountsType mirrors the mounts attribute's object type, so a test can
// build the value the framework would hand expandCreateCluster.
func clusterMountsType() types.ObjectType {
	persistent := types.ObjectType{AttrTypes: map[string]attr.Type{
		"path": types.StringType,
		"size": types.Int64Type,
	}}
	network := types.ObjectType{AttrTypes: map[string]attr.Type{
		"path":      types.StringType,
		"volume_id": types.StringType,
	}}
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"persistent": persistent,
		"network":    types.ListType{ElemType: network},
	}}
}

func networkMountsObject(t *testing.T, mountPath, volumeId string) types.Object {
	t.Helper()
	mountsType := clusterMountsType()
	networkType := mountsType.AttrTypes["network"].(types.ListType)

	entry, diags := types.ObjectValue(networkType.ElemType.(types.ObjectType).AttrTypes, map[string]attr.Value{
		"path":      types.StringValue(mountPath),
		"volume_id": types.StringValue(volumeId),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	list, diags := types.ListValue(networkType.ElemType, []attr.Value{entry})
	if diags.HasError() {
		t.Fatal(diags)
	}
	object, diags := types.ObjectValue(mountsType.AttrTypes, map[string]attr.Value{
		"persistent": types.ObjectNull(mountsType.AttrTypes["persistent"].(types.ObjectType).AttrTypes),
		"network":    list,
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	return object
}

func persistentMountsObject(t *testing.T, mountPath string, size int64) types.Object {
	t.Helper()
	mountsType := clusterMountsType()
	persistentType := mountsType.AttrTypes["persistent"].(types.ObjectType)

	persistent, diags := types.ObjectValue(persistentType.AttrTypes, map[string]attr.Value{
		"path": types.StringValue(mountPath),
		"size": types.Int64Value(size),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	object, diags := types.ObjectValue(mountsType.AttrTypes, map[string]attr.Value{
		"persistent": persistent,
		"network":    types.ListNull(mountsType.AttrTypes["network"].(types.ListType).ElemType),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	return object
}

func TestExpandCreateCluster(t *testing.T) {
	ctx := context.Background()
	env, diags := types.MapValueFrom(ctx, types.StringType, map[string]string{"NCCL_DEBUG": "INFO"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	ports, diags := types.ListValueFrom(ctx, types.StringType, []string{"22/tcp", "8888/http"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	dataCenters, diags := types.ListValueFrom(ctx, types.StringType, []string{"US-TX-3"})
	if diags.HasError() {
		t.Fatal(diags)
	}

	plan := clusterResourceModel{
		Name:           types.StringValue("my-training-cluster"),
		Type:           types.StringValue("TRAINING"),
		GpuTypeId:      types.StringValue("NVIDIA H100 80GB HBM3"),
		GpuCountPerPod: types.Int64Value(8),
		PodCount:       types.Int64Value(4),
		Image:          types.StringValue("runpod/pytorch:2.8.0"),
		Args:           types.StringValue("--epochs 3"),
		Disk:           types.Int64Value(50),
		Env:            env,
		Ports:          ports,
		DataCenterIds:  dataCenters,
		Mounts:         networkMountsObject(t, "/runpod-volume", "vol_xyz"),
		StartSsh:       types.BoolValue(true),
		StartJupyter:   types.BoolValue(false),
	}

	body, diags := expandCreateCluster(ctx, plan)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Name != "my-training-cluster" || body.Type != sdktypes.ClusterTypeTraining {
		t.Fatalf("name/type: %+v", body)
	}
	if body.Compute.GpuTypeId != "NVIDIA H100 80GB HBM3" ||
		body.Compute.GpuCountPerPod != 8 || body.Compute.PodCount != 4 {
		t.Fatalf("compute: %+v", body.Compute)
	}
	if body.Image == nil || *body.Image != "runpod/pytorch:2.8.0" {
		t.Fatalf("image: %v", body.Image)
	}
	if body.Args == nil || *body.Args != "--epochs 3" {
		t.Fatalf("args: %v", body.Args)
	}
	if body.Disk == nil || *body.Disk != 50 {
		t.Fatalf("disk: %v", body.Disk)
	}
	if body.Env == nil || (*body.Env)["NCCL_DEBUG"] != "INFO" {
		t.Fatalf("env: %v", body.Env)
	}
	if body.Ports == nil || len(*body.Ports) != 2 || (*body.Ports)[0] != "22/tcp" {
		t.Fatalf("ports: %v", body.Ports)
	}
	if body.DataCenterIds == nil || len(*body.DataCenterIds) != 1 || (*body.DataCenterIds)[0] != "US-TX-3" {
		t.Fatalf("data center ids: %v", body.DataCenterIds)
	}
	if body.StartSsh == nil || !*body.StartSsh {
		t.Fatalf("start_ssh: %v", body.StartSsh)
	}
	if body.StartJupyter == nil || *body.StartJupyter {
		t.Fatalf("start_jupyter: %v", body.StartJupyter)
	}
	if body.Mounts == nil || body.Mounts.Network == nil || len(*body.Mounts.Network) != 1 {
		t.Fatalf("mounts: %+v", body.Mounts)
	}
	mount := (*body.Mounts.Network)[0]
	if mount.Path != "/runpod-volume" || mount.VolumeId != "vol_xyz" {
		t.Fatalf("network mount: %+v", mount)
	}
	if body.Mounts.Persistent != nil {
		t.Fatalf("a network mount must not also send a persistent one: %+v", body.Mounts.Persistent)
	}
}

func TestExpandCreateClusterPersistentMount(t *testing.T) {
	ctx := context.Background()
	body, diags := expandCreateCluster(ctx, clusterResourceModel{
		Name:           types.StringValue("slurm"),
		Type:           types.StringValue("SLURM"),
		GpuTypeId:      types.StringValue("NVIDIA H100 80GB HBM3"),
		GpuCountPerPod: types.Int64Value(1),
		PodCount:       types.Int64Value(2),
		Mounts:         persistentMountsObject(t, "/workspace", 20),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Mounts == nil || body.Mounts.Persistent == nil {
		t.Fatalf("mounts: %+v", body.Mounts)
	}
	if body.Mounts.Persistent.Path != "/workspace" || body.Mounts.Persistent.Size != 20 {
		t.Fatalf("persistent mount: %+v", body.Mounts.Persistent)
	}
	if body.Mounts.Network != nil {
		t.Fatalf("a persistent mount must not also send a network one: %+v", body.Mounts.Network)
	}
}

func TestExpandCreateClusterOmitsUnsetOptionals(t *testing.T) {
	ctx := context.Background()
	// The container configuration is Optional without Computed, so an omitted
	// attribute arrives null: the body must carry no field for it at all, and
	// no empty mounts object either.
	body, diags := expandCreateCluster(ctx, clusterResourceModel{
		Name:           types.StringValue("bare"),
		Type:           types.StringValue("APPLICATION"),
		GpuTypeId:      types.StringValue("NVIDIA GeForce RTX 4090"),
		GpuCountPerPod: types.Int64Value(1),
		PodCount:       types.Int64Value(2),
		Image:          types.StringNull(),
		Args:           types.StringNull(),
		Disk:           types.Int64Null(),
		Env:            types.MapNull(types.StringType),
		Ports:          types.ListNull(types.StringType),
		DataCenterIds:  types.ListNull(types.StringType),
		Mounts:         types.ObjectNull(clusterMountsType().AttrTypes),
		StartSsh:       types.BoolNull(),
		StartJupyter:   types.BoolNull(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Image != nil || body.Args != nil || body.Disk != nil || body.Env != nil ||
		body.Ports != nil || body.DataCenterIds != nil || body.StartSsh != nil || body.StartJupyter != nil {
		t.Fatalf("unset optionals must be omitted: %+v", body)
	}
	if body.Mounts != nil {
		t.Fatalf("absent mounts must be omitted, not sent empty: %+v", body.Mounts)
	}
}

func testCluster() *sdktypes.Cluster {
	dataCenter := "US-TX-3"
	template := "tpl_abc"
	sshEndpoint := "1.2.3.4:22001"
	vxlanId := 42
	vxlanPort := 4789
	return &sdktypes.Cluster{
		Id:           "cluster_abc123",
		Name:         "my-training-cluster",
		Type:         sdktypes.ClusterTypeTraining,
		DataCenterId: &dataCenter,
		Template:     &template,
		CreatedAt:    time.Date(2026, 6, 29, 20, 0, 0, 0, time.UTC),
		Compute: sdktypes.ClusterCompute{
			GpuTypeId:      "NVIDIA H100 80GB HBM3",
			GpuCountPerPod: 8,
			PodCount:       4,
		},
		Pods: sdktypes.ClusterPodsSummary{
			Total:    4,
			ByStatus: map[string]int{"RUNNING": 3, "PROVISIONING": 1},
		},
		Primary: &sdktypes.ClusterPrimary{
			PodId:       "pod_node0",
			Status:      sdktypes.PodStatusRunning,
			SshEndpoint: &sshEndpoint,
		},
		Network: &sdktypes.ClusterNetwork{
			Cidr:      "10.65.0.0/16",
			VxlanId:   &vxlanId,
			VxlanPort: &vxlanPort,
		},
	}
}

func TestFlattenCluster(t *testing.T) {
	ctx := context.Background()
	// The create-only container configuration has to survive a read: the
	// cluster object does not echo it, and clearing it would diff every plan.
	model := clusterResourceModel{
		Image:    types.StringValue("runpod/pytorch:2.8.0"),
		StartSsh: types.BoolValue(true),
	}

	if diags := flattenCluster(ctx, testCluster(), &model); diags.HasError() {
		t.Fatal(diags)
	}

	if model.Id.ValueString() != "cluster_abc123" ||
		model.Name.ValueString() != "my-training-cluster" ||
		model.Type.ValueString() != "TRAINING" {
		t.Fatalf("identity: %+v", model)
	}
	if model.GpuTypeId.ValueString() != "NVIDIA H100 80GB HBM3" ||
		model.GpuCountPerPod.ValueInt64() != 8 || model.PodCount.ValueInt64() != 4 {
		t.Fatalf("compute: %+v", model)
	}
	if model.DataCenterId.ValueString() != "US-TX-3" || model.Template.ValueString() != "tpl_abc" {
		t.Fatalf("placement: %+v", model)
	}
	if model.CreatedAt.ValueString() != "2026-06-29T20:00:00Z" {
		t.Fatalf("created_at = %q", model.CreatedAt.ValueString())
	}
	if model.PodsTotal.ValueInt64() != 4 {
		t.Fatalf("pods_total = %d", model.PodsTotal.ValueInt64())
	}
	var byStatus map[string]int64
	if diags := model.PodsByStatus.ElementsAs(ctx, &byStatus, false); diags.HasError() {
		t.Fatal(diags)
	}
	if byStatus["RUNNING"] != 3 || byStatus["PROVISIONING"] != 1 {
		t.Fatalf("pods_by_status = %v", byStatus)
	}
	if model.PrimaryPodId.ValueString() != "pod_node0" ||
		model.PrimaryStatus.ValueString() != "RUNNING" ||
		model.PrimarySshEndpoint.ValueString() != "1.2.3.4:22001" {
		t.Fatalf("primary: %+v", model)
	}
	if model.NetworkCidr.ValueString() != "10.65.0.0/16" ||
		model.NetworkVxlanId.ValueInt64() != 42 || model.NetworkVxlanPort.ValueInt64() != 4789 {
		t.Fatalf("network: %+v", model)
	}
	if model.Image.ValueString() != "runpod/pytorch:2.8.0" || !model.StartSsh.ValueBool() {
		t.Fatalf("create-only configuration must survive a read: %+v", model)
	}
}

func TestFlattenClusterAbsentOptionals(t *testing.T) {
	ctx := context.Background()
	cluster := testCluster()
	// A freshly created cluster has no placement, no primary, and no network
	// yet; every one of those must flatten to null rather than an empty value.
	cluster.DataCenterId = nil
	cluster.Template = nil
	cluster.Primary = nil
	cluster.Network = nil
	cluster.Pods = sdktypes.ClusterPodsSummary{Total: 0, ByStatus: map[string]int{}}

	var model clusterResourceModel
	if diags := flattenCluster(ctx, cluster, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.DataCenterId.IsNull() || !model.Template.IsNull() {
		t.Fatalf("placement: %+v", model)
	}
	if !model.PrimaryPodId.IsNull() || !model.PrimaryStatus.IsNull() || !model.PrimarySshEndpoint.IsNull() {
		t.Fatalf("primary: %+v", model)
	}
	if !model.NetworkCidr.IsNull() || !model.NetworkVxlanId.IsNull() || !model.NetworkVxlanPort.IsNull() {
		t.Fatalf("network: %+v", model)
	}
	if model.PodsByStatus.IsNull() || len(model.PodsByStatus.Elements()) != 0 {
		t.Fatalf("pods_by_status must be an empty map, not null: %+v", model.PodsByStatus)
	}
}

func TestFlattenClusterPartialNetwork(t *testing.T) {
	ctx := context.Background()
	cluster := testCluster()
	// The overlay CIDR is assigned before the VXLAN identifiers.
	cluster.Network = &sdktypes.ClusterNetwork{Cidr: "10.65.0.0/16"}

	var model clusterResourceModel
	if diags := flattenCluster(ctx, cluster, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.NetworkCidr.ValueString() != "10.65.0.0/16" {
		t.Fatalf("network_cidr = %q", model.NetworkCidr.ValueString())
	}
	if !model.NetworkVxlanId.IsNull() || !model.NetworkVxlanPort.IsNull() {
		t.Fatalf("unassigned vxlan fields must be null: %+v", model)
	}
}

func TestClusterSchemaValidatesType(t *testing.T) {
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	(&clusterResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}
	attribute, ok := schemaResp.Schema.Attributes["type"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("type attribute is %T, want a StringAttribute", schemaResp.Schema.Attributes["type"])
	}

	for value, wantValid := range map[string]bool{
		"TRAINING":    true,
		"SLURM":       true,
		"RAY":         true,
		"APPLICATION": true,
		"training":    false,
		"KUBERNETES":  false,
	} {
		var valResp validator.StringResponse
		for _, v := range attribute.Validators {
			v.ValidateString(ctx, validator.StringRequest{
				Path:        path.Root("type"),
				ConfigValue: types.StringValue(value),
			}, &valResp)
		}
		if valResp.Diagnostics.HasError() == wantValid {
			t.Errorf("type %q: valid = %v, want %v", value, !valResp.Diagnostics.HasError(), wantValid)
		}
	}
}

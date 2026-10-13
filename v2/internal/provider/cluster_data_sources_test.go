package provider

import (
	"context"
	"testing"
	"time"

	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

func TestFlattenClusterSummary(t *testing.T) {
	ctx := context.Background()
	model, diags := flattenClusterSummary(ctx, *testCluster())
	if diags.HasError() {
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
	if model.DataCenterId.ValueString() != "US-TX-3" ||
		model.CreatedAt.ValueString() != "2026-06-29T20:00:00Z" ||
		model.PodsTotal.ValueInt64() != 4 {
		t.Fatalf("placement: %+v", model)
	}
	var byStatus map[string]int64
	if diags := model.PodsByStatus.ElementsAs(ctx, &byStatus, false); diags.HasError() {
		t.Fatal(diags)
	}
	if byStatus["RUNNING"] != 3 {
		t.Fatalf("pods_by_status = %v", byStatus)
	}
	if model.PrimaryPodId.ValueString() != "pod_node0" ||
		model.PrimarySshEndpoint.ValueString() != "1.2.3.4:22001" {
		t.Fatalf("primary: %+v", model)
	}
	if model.NetworkVxlanPort.ValueInt64() != 4789 {
		t.Fatalf("network: %+v", model)
	}
}

func TestFlattenClusterSummaryAbsentOptionals(t *testing.T) {
	ctx := context.Background()
	cluster := testCluster()
	cluster.DataCenterId = nil
	cluster.Template = nil
	cluster.Primary = nil
	cluster.Network = nil

	model, diags := flattenClusterSummary(ctx, *cluster)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !model.DataCenterId.IsNull() || !model.Template.IsNull() ||
		!model.PrimaryPodId.IsNull() || !model.PrimaryStatus.IsNull() ||
		!model.PrimarySshEndpoint.IsNull() || !model.NetworkCidr.IsNull() ||
		!model.NetworkVxlanId.IsNull() || !model.NetworkVxlanPort.IsNull() {
		t.Fatalf("absent cluster fields must flatten to null: %+v", model)
	}
}

func testClusterPod() sdktypes.Pod {
	dataCenter := "US-TX-3"
	gpuCount := 8
	rank := 0
	ip := "10.65.0.2"
	role := sdktypes.PodClusterRoleSlurmController
	return sdktypes.Pod{
		Id:           "pod_node0",
		Name:         "my-cluster-node-0",
		Status:       sdktypes.PodStatusRunning,
		Image:        "runpod/pytorch:2.8.0",
		Cloud:        sdktypes.CloudSecure,
		Disk:         50,
		Cost:         2.8,
		DataCenterId: &dataCenter,
		CreatedAt:    time.Date(2026, 6, 29, 20, 0, 0, 0, time.UTC),
		Gpu: &sdktypes.GpuConfig{
			Id:    "NVIDIA H100 80GB HBM3",
			Count: &gpuCount,
		},
		Cluster: &sdktypes.PodCluster{
			Id:   "cluster_abc123",
			Rank: &rank,
			Ip:   &ip,
			Role: &role,
		},
	}
}

func TestFlattenClusterPod(t *testing.T) {
	item := flattenClusterPod(testClusterPod())

	if item.Id.ValueString() != "pod_node0" ||
		item.Name.ValueString() != "my-cluster-node-0" ||
		item.Status.ValueString() != "RUNNING" ||
		item.Image.ValueString() != "runpod/pytorch:2.8.0" ||
		item.Cloud.ValueString() != "SECURE" {
		t.Fatalf("identity: %+v", item)
	}
	if item.Disk.ValueInt64() != 50 || item.DataCenterId.ValueString() != "US-TX-3" ||
		item.CreatedAt.ValueString() != "2026-06-29T20:00:00Z" {
		t.Fatalf("placement: %+v", item)
	}
	if item.Cost.ValueFloat64() < 2.79 || item.Cost.ValueFloat64() > 2.81 {
		t.Fatalf("cost = %v, want ~2.8", item.Cost.ValueFloat64())
	}
	if item.GpuTypeId.ValueString() != "NVIDIA H100 80GB HBM3" || item.GpuCount.ValueInt64() != 8 {
		t.Fatalf("gpu: %+v", item)
	}
	if item.ClusterRank.ValueInt64() != 0 || item.ClusterRank.IsNull() {
		t.Fatalf("cluster_rank must be the assigned rank 0, not null: %+v", item.ClusterRank)
	}
	if item.ClusterRole.ValueString() != "SLURM_CONTROLLER" || item.ClusterIp.ValueString() != "10.65.0.2" {
		t.Fatalf("cluster membership: %+v", item)
	}
}

func TestFlattenClusterPodUnassignedMembership(t *testing.T) {
	pod := testClusterPod()
	// A provisioning member has no rank, address, or role yet; a TRAINING or
	// APPLICATION member never gets a role at all.
	pod.Cluster = &sdktypes.PodCluster{Id: "cluster_abc123"}
	pod.Gpu = nil

	item := flattenClusterPod(pod)
	if !item.ClusterRank.IsNull() || !item.ClusterIp.IsNull() || !item.ClusterRole.IsNull() {
		t.Fatalf("unassigned membership must flatten to null: %+v", item)
	}
	if !item.GpuTypeId.IsNull() || !item.GpuCount.IsNull() {
		t.Fatalf("a pod with no GPU config must flatten to null: %+v", item)
	}
}

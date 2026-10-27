package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFlattenLegacyPod(t *testing.T) {
	machineId := "machine-1"
	encrypted := true
	pod := &legacyPodFields{
		MachineID:       &machineId,
		VolumeEncrypted: &encrypted,
	}

	// docker_entrypoint and docker_start_cmd come from the v2 pod, so
	// whatever flattenPod set stays.
	model := podResourceModel{
		DockerEntrypoint: types.ListNull(types.StringType),
		DockerStartCmd:   types.ListNull(types.StringType),
	}
	flattenLegacyPod(pod, &model)
	if model.MachineId.ValueString() != "machine-1" || !model.VolumeEncrypted.ValueBool() {
		t.Fatalf("machine_id/volume_encrypted: %+v", model)
	}
	if !model.DockerEntrypoint.IsNull() || !model.DockerStartCmd.IsNull() {
		t.Fatalf("the shim must leave the v2 command lists alone: %+v", model)
	}
}

func TestFlattenLegacyPodEmptyResponse(t *testing.T) {
	// A partial GraphQL answer that carries neither field.
	var model podResourceModel
	flattenLegacyPod(&legacyPodFields{}, &model)
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
	flattenLegacyPod(&legacyPodFields{MachineID: &machineId, VolumeEncrypted: &encrypted}, &model)

	// A failed GraphQL read nulls the shim attributes and leaves the command
	// lists the v2 read already filled.
	clearLegacyPodFields(&model)
	if !model.MachineId.IsNull() || !model.VolumeEncrypted.IsNull() {
		t.Fatalf("every shim attribute must be null after a failed read: %+v", model)
	}
	if len(model.DockerEntrypoint.Elements()) != 1 {
		t.Fatalf("docker_entrypoint = %v, want the v2 value kept", model.DockerEntrypoint)
	}
}

func TestFlattenLegacyPodAbsentScalarsAreNullNotEmpty(t *testing.T) {
	// v1.0.8 wrote machine_id only for a present, non-empty machineId and
	// volume_encrypted only for a present volumeEncrypted, and a 1.x
	// configuration's `machine_id != null` check has to keep answering what it
	// did when the GraphQL answer lacks either.
	empty := ""
	var model podResourceModel
	flattenLegacyPod(&legacyPodFields{MachineID: &empty}, &model)
	if !model.MachineId.IsNull() {
		t.Fatalf("machine_id = %v, want null rather than an empty string", model.MachineId)
	}
	if !model.VolumeEncrypted.IsNull() {
		t.Fatalf("volume_encrypted = %v, want null rather than false", model.VolumeEncrypted)
	}

	// A reported false is an answer.
	no := false
	flattenLegacyPod(&legacyPodFields{VolumeEncrypted: &no}, &model)
	if model.VolumeEncrypted.IsNull() || model.VolumeEncrypted.ValueBool() {
		t.Fatalf("volume_encrypted = %v, want a reported false", model.VolumeEncrypted)
	}
}

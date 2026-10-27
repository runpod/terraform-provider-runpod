package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

// flattenLegacyPod maps the GraphQL pod onto the two deprecated read-backs.
// The other REST v1 fields the 1.x provider read are served from REST v2
// instead: costPerHr and memoryInGb as cost and memory_in_gb, dockerEntrypoint
// and dockerStartCmd as docker_entrypoint and docker_start_cmd.
func flattenLegacyPod(pod *legacyPodFields, model *podResourceModel) {
	// The GraphQL schema declares both non-null, so a reported false is an
	// answer and stays false. An absent field still maps to null rather than
	// "" or false, and so does an empty machine id, so a 1.x configuration's
	// `machine_id != null` check keeps meaning the pod sits on a machine.
	model.MachineId = legacyNonEmptyString(pod.MachineID)
	model.VolumeEncrypted = types.BoolPointerValue(pod.VolumeEncrypted)
}

// clearLegacyPodFields nulls the shim read-backs, for the path where the
// GraphQL API did not answer or does not know the pod. Both are Computed, so
// a null is a legal value and the apply survives the shim failing.
func clearLegacyPodFields(model *podResourceModel) {
	model.MachineId = types.StringNull()
	model.VolumeEncrypted = types.BoolNull()
}

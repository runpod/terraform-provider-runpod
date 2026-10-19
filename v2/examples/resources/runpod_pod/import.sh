# Pods are imported by pod id. The API does not return template_id, start_ssh,
# start_jupyter, allowed_cuda_versions, min_cuda_version, min_ram_per_gpu,
# min_vcpu_count_per_gpu or data_center_ids, so an import leaves them empty
# and each one the configuration sets forces replacement on the first apply.
# Name them in lifecycle { ignore_changes = [...] } before that apply. The
# import also reads the pod's whole env into state, so the first apply with a
# configured env removes every key the configuration leaves out, template keys
# included; PUBLIC_KEY and JUPYTER_PASSWORD stay on the pod. Copy the keys to
# keep into the configuration before that apply.
terraform import runpod_pod.training pod_abc123

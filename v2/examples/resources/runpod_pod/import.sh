# Pods are imported by pod id. The import reads the pod's whole env into
# state, so the first apply with a configured env removes every key the
# configuration leaves out, template keys included; PUBLIC_KEY and
# JUPYTER_PASSWORD stay on the pod. Copy the keys to keep into the
# configuration before that apply.
terraform import runpod_pod.training pod_abc123

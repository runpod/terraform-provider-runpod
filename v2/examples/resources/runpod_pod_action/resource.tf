# Assumes runpod_pod.jupyter, runpod_pod.batch and the variables image and
# shift are declared elsewhere in the configuration.

# Restart the pod whenever the image it should run changes. The action is
# imperative: it runs on create and on every triggers change, and destroying
# this resource leaves the pod as it is.
resource "runpod_pod_action" "restart_on_image_change" {
  pod_id = runpod_pod.jupyter.id
  action = "restart"

  triggers = {
    image = var.image
  }
}

# Stop a pod on demand. Do not combine this with runpod_pod.desired_state on
# the same pod: desired_state manages the run state declaratively, and the two
# would undo each other on every apply.
resource "runpod_pod_action" "stop_overnight" {
  pod_id = runpod_pod.batch.id
  action = "stop"

  triggers = {
    shift = var.shift
  }
}

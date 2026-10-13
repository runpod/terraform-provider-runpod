data "runpod_endpoint_build" "latest" {
  endpoint_id = "ep_abc123"
  id          = "build_abc"
}

output "built_image" {
  description = "Image the build produced; null until it completes."
  value       = data.runpod_endpoint_build.latest.image_name
}

output "build_failure" {
  description = "Failure detail; null unless the build failed."
  value       = data.runpod_endpoint_build.latest.error
}

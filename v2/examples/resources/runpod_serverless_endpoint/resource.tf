resource "runpod_serverless_endpoint" "inference" {
  name          = "inference-endpoint"
  image         = "runpod/mock-worker:dev"
  endpoint_type = "QUEUE"
  gpu_pools     = ["ADA_24"]
  gpu_count     = 1
  flashboot     = "FLASHBOOT"
  scaling_type  = "QUEUE_DELAY"
  scaling_value = 4
  workers_min   = 0
  workers_max   = 3

  # Narrow the pool down: workers run on every type in the pool except these,
  # and only on hosts at or above the CUDA floor.
  excluded_types   = ["NVIDIA L40"]
  min_cuda_version = "12.1"
}

# A load-balancer endpoint routes requests straight to the worker's own HTTP
# paths, so it declares the ports the worker listens on. The API accepts only
# REQUEST_COUNT scaling for this routing mode.
resource "runpod_serverless_endpoint" "http" {
  name          = "inference-http"
  image         = "runpod/mock-worker:dev"
  endpoint_type = "LOAD_BALANCER"
  gpu_pools     = ["ADA_24"]
  scaling_type  = "REQUEST_COUNT"
  scaling_value = 4
  workers_min   = 0
  workers_max   = 3
  ports         = ["8000/http"]
}

# A CPU endpoint: cpu replaces gpu_pools, and each entry is a flavor the
# workers may be placed on. The container settings come from a serverless
# template here instead of an image.
resource "runpod_serverless_endpoint" "cpu" {
  name          = "inference-cpu"
  template_id   = runpod_template.cpu_worker.id
  endpoint_type = "QUEUE"
  scaling_type  = "REQUEST_COUNT"
  scaling_value = 1
  workers_min   = 0
  workers_max   = 2

  cpu = [
    {
      cpu_type_id = "cpu5c"
      vcpu_count  = 4
    },
    {
      cpu_type_id = "cpu5c"
      vcpu_count  = 8
    },
  ]
}

resource "runpod_template" "cpu_worker" {
  name       = "inference-cpu-worker"
  image      = "runpod/mock-worker:dev"
  serverless = true
  disk       = 10
}

# Queue endpoints report their job operation URLs; a load-balancer endpoint
# reports base and health instead, and the other attributes come back null.
output "inference_run_url" {
  value = runpod_serverless_endpoint.inference.request_urls.run
}

output "http_base_url" {
  value = runpod_serverless_endpoint.http.request_urls.base
}

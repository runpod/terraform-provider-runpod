resource "runpod_template" "this" {
  name       = var.name
  image      = var.image
  args       = var.args
  disk       = var.disk
  env        = var.env
  ports      = var.ports
  serverless = var.serverless
  public     = var.public
  category   = var.category

  registry              = var.registry
  start_ssh             = var.start_ssh
  start_jupyter         = var.start_jupyter
  allowed_cuda_versions = var.allowed_cuda_versions

  # Host-local persistent storage for containers created from the template,
  # replacing the 1.x volume_in_gb and volume_mount_path pair.
  persistent_volume = var.persistent_volume

  # Deprecated, served by the REST v1 shim: the 1.x docker_entrypoint and
  # readme attributes. Both are applied on create and update, and both are read
  # back, so a template that already has them fills them in.
  docker_entrypoint = var.docker_entrypoint
  readme            = var.readme
}

# The template read back by id — what a pod or endpoint pointing at it sees.
data "runpod_template" "this" {
  id = runpod_template.this.id
}

# Every template visible to the account, including the one above.
data "runpod_templates" "all" {
  depends_on = [runpod_template.this]
}

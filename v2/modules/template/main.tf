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

  # The 1.x docker_entrypoint, now the v2 template's entrypoint. The API stores
  # it inside args, so it cannot be set together with args.
  docker_entrypoint = var.docker_entrypoint

  # Deprecated, served by the GraphQL shim: the 1.x readme, written through
  # saveTemplate on create and update and read back, so a template that already
  # has one fills it in.
  readme = var.readme
}

# The template read back by id — what a pod or endpoint pointing at it sees.
data "runpod_template" "this" {
  id = runpod_template.this.id
}

# Every template visible to the account, including the one above.
data "runpod_templates" "all" {
  depends_on = [runpod_template.this]
}

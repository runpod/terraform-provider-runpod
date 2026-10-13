data "runpod_templates" "all" {}

output "serverless_template_ids" {
  description = "Templates usable by Serverless endpoints."
  value       = [for t in data.runpod_templates.all.templates : t.id if t.serverless]
}

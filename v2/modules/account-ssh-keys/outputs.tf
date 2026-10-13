output "id" {
  description = "Always \"account\": the keys are one account-wide document."
  value       = runpod_account_ssh_keys.this.id
}

output "keys" {
  description = "The keys now registered on the account, as the API reports them."
  value       = runpod_account_ssh_keys.this.keys
}

output "key_count" {
  description = "How many keys are registered."
  value       = length(runpod_account_ssh_keys.this.keys)
}

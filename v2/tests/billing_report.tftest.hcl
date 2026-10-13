provider "runpod" {}

# Read-only spend reporting. Amounts depend on what the account has run, so the
# assertions check the shape of the response rather than a value: an account
# that has spent nothing still returns a resolved window and its buckets.

run "last_week_by_day" {
  module {
    source = "../modules/billing-report"
  }

  variables {
    bucket_size = "day"
    last_n      = 7
  }

  assert {
    condition     = output.window.bucket_size == "day"
    error_message = "the API resolved a bucket size other than the requested day"
  }
  assert {
    condition     = output.total_spend >= 0
    error_message = "total spend came back negative"
  }
  assert {
    condition     = length(output.spend_by_bucket) > 0
    error_message = "a seven-day window returned no buckets"
  }
  assert {
    condition     = output.pod_spend >= 0 && output.serverless_spend >= 0
    error_message = "a per-resource total came back negative"
  }
  assert {
    condition     = output.endpoint_spend >= 0 && output.network_volume_spend >= 0 && output.cluster_spend >= 0
    error_message = "a per-resource total came back negative"
  }
}

run "last_three_months" {
  module {
    source = "../modules/billing-report"
  }

  variables {
    bucket_size = "month"
    last_n      = 3
  }

  assert {
    condition     = output.window.bucket_size == "month"
    error_message = "the API resolved a bucket size other than the requested month"
  }
  assert {
    condition     = output.pod_count >= 0
    error_message = "the distinct pod count came back negative"
  }
}

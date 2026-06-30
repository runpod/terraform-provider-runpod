package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Release-validation suite. These encode the pod-MVP release criteria as
// executable acceptance tests and run against the REAL API (staging/prod), NOT
// runpod-in-a-box — riab is excluded from the release gate (see
// release-readiness-2026-06-30.md).
//
// Gating: resource.Test auto-skips without TF_ACC=1; releasePreCheck additionally
// requires RUNPOD_ACC=1 + RUNPOD_API_KEY + RUNPOD_BASE_URL pointed at the target
// environment. They assert release-READY behavior, so they FAIL on the current
// open blockers (CE-1658/1660/1663/1662) and turn green only when the provider
// is actually release-ready — i.e. this suite IS the sign-off gate.
//
// Image / GPU type can be overridden per environment via RUNPOD_TEST_IMAGE and
// RUNPOD_TEST_GPU_TYPE_ID.

func releasePreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("RUNPOD_ACC") != "1" {
		t.Skip("release validation: set RUNPOD_ACC=1 + RUNPOD_API_KEY + RUNPOD_BASE_URL (real API / staging-prod) to run")
	}
	for _, k := range []string{"RUNPOD_API_KEY", "RUNPOD_BASE_URL"} {
		if os.Getenv(k) == "" {
			t.Fatalf("release validation requires %s", k)
		}
	}
}

func releaseEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func releasePodConfig(name string, extra string) string {
	return fmt.Sprintf(`
provider "runpod" {}

resource "runpod_pod" "v" {
  name        = %q
  image_name  = %q
  gpu_type_id = %q
  gpu_count   = 1
  cloud_type  = "SECURE"
%s
}
`, name, releaseEnv("RUNPOD_TEST_IMAGE", "runpod/base:0.0.0"),
		releaseEnv("RUNPOD_TEST_GPU_TYPE_ID", "NVIDIA GeForce RTX 4090"), extra)
}

// TestRelease_PodLifecycle_NoDrift creates a pod with start_ssh/start_jupyter
// OMITTED. terraform-plugin-testing automatically refreshes and re-plans after
// apply and fails on a non-empty plan — so this catches CE-1660 (inconsistent
// result on apply) and CE-1658 (status drift from desiredStatus mapping).
func TestRelease_PodLifecycle_NoDrift(t *testing.T) {
	releasePreCheck(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: releasePodConfig("tf-release-nodrift", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("runpod_pod.v", "id"),
					resource.TestCheckResourceAttr("runpod_pod.v", "name", "tf-release-nodrift"),
				),
			},
		},
	})
}

// TestRelease_PodConfigRoundTrip sets the attributes CE-1663 currently drops and
// asserts they round-trip into state. If Create silently drops them, the
// post-apply state/plan is inconsistent and this fails.
func TestRelease_PodConfigRoundTrip(t *testing.T) {
	releasePreCheck(t)
	extra := `  docker_args          = "--foo"
  container_disk_in_gb = 20
  start_ssh            = true
  start_jupyter        = true
  stop_after           = "1h"`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: releasePodConfig("tf-release-roundtrip", extra),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("runpod_pod.v", "docker_args", "--foo"),
					resource.TestCheckResourceAttr("runpod_pod.v", "container_disk_in_gb", "20"),
					resource.TestCheckResourceAttr("runpod_pod.v", "start_ssh", "true"),
					resource.TestCheckResourceAttr("runpod_pod.v", "start_jupyter", "true"),
					resource.TestCheckResourceAttr("runpod_pod.v", "stop_after", "1h"),
				),
			},
		},
	})
}

// TestRelease_PodImport_RoundTrip verifies terraform import works (CE-1662).
func TestRelease_PodImport_RoundTrip(t *testing.T) {
	releasePreCheck(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: releasePodConfig("tf-release-import", ""),
				Check:  resource.TestCheckResourceAttrSet("runpod_pod.v", "id"),
			},
			{
				ResourceName:      "runpod_pod.v",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

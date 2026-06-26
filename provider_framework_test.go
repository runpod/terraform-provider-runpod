package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccProtoV6ProviderFactories serves the real provider over the plugin
// protocol so resource.Test exercises actual HCL the way a user would.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"runpod": providerserver.NewProtocol6WithError(newProvider()),
}

func testAccPreCheck(t *testing.T) {
	for _, k := range []string{"RUNPOD_API_KEY", "RUNPOD_BASE_URL"} {
		if os.Getenv(k) == "" {
			t.Fatalf("%s must be set for acceptance tests", k)
		}
	}
}

func testAccPodConfig(name, image string) string {
	return fmt.Sprintf(`
provider "runpod" {}

resource "runpod_pod" "test" {
  name          = %q
  image_name    = %q
  gpu_count     = 1
  cloud_type    = "SECURE"
  start_ssh     = true
  start_jupyter = true
}
`, name, image)
}

// testAccPodDemoConfig mirrors the team demo's main.tf (start_ssh/start_jupyter set,
// pod_id output) but deploys from image_name rather than a template_id, so it does
// not depend on a template being seeded in the local stack (riab seeds none).
func testAccPodDemoConfig(name, image string) string {
	return fmt.Sprintf(`
provider "runpod" {}

resource "runpod_pod" "demo" {
  name          = %q
  image_name    = %q
  gpu_count     = 1
  cloud_type    = "SECURE"
  start_ssh     = true
  start_jupyter = true
}

output "pod_id" {
  value = runpod_pod.demo.id
}
`, name, image)
}

// testAccCheckPodDestroy verifies — via the real API — that pods in state are
// gone after destroy.
func testAccCheckPodDestroy(s *terraform.State) error {
	base := os.Getenv("RUNPOD_BASE_URL")
	key := os.Getenv("RUNPOD_API_KEY")
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "runpod_pod" {
			continue
		}
		req, _ := http.NewRequest("GET", base+"/pods/"+rs.Primary.ID, nil)
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("checking pod %s: %w", rs.Primary.ID, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("pod %s still exists after destroy (status %d)", rs.Primary.ID, resp.StatusCode)
		}
	}
	return nil
}

// TestAccPodDemo_framework mimics the working team demo: create a pod (start_ssh/
// start_jupyter set) and destroy it — the path the demo exercised. Deploys from
// image_name (not template_id) so it doesn't depend on a seeded template (riab
// seeds none). ExpectNonEmptyPlan tolerates the known post-apply drift from
// CE-1658, which the demo never surfaced because it did apply→destroy without a
// second plan. Remove ExpectNonEmptyPlan once CE-1658 is fixed. Green == the MVP
// pod create/destroy works end-to-end (capacity permitting).
//
//	TF_ACC=1 RUNPOD_API_KEY=$TEST_USER_JWT RUNPOD_BASE_URL=http://localhost:8081/v1 \
//	  go test . -run TestAccPodDemo_framework -v
func TestAccPodDemo_framework(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPodDestroy,
		Steps: []resource.TestStep{
			{
				Config:             testAccPodDemoConfig("tf-demo", "runpod/test:latest"),
				ExpectNonEmptyPlan: true, // tolerate CE-1658 post-apply drift (demo did apply→destroy only)
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("runpod_pod.demo", "id"),
					resource.TestCheckResourceAttr("runpod_pod.demo", "name", "tf-demo"),
					resource.TestCheckResourceAttr("runpod_pod.demo", "image_name", "runpod/test:latest"),
				),
			},
		},
	})
}

// TestAccDataSources_framework reads each no-input data source through real HCL
// via the plugin protocol and asserts it returns data (an "id"). All are RED
// today — CE-1652 (double-unwrap) plus query/schema mismatches against the
// live schema (e.g. provider sends `user`/`gpus`; schema exposes `myself` and
// has no `gpus`) — see CE-1661. Green per case == that data source works.
//
// Needs TF_ACC=1, RUNPOD_API_KEY=$TEST_USER_JWT, and
// RUNPOD_GRAPHQL_URL=http://localhost:4000/graphql (+ terraform binary).
// TestAccProviderConfig_apiKeyFromBlock asserts the DESIRED behavior: an api_key
// set in the provider block authenticates. RED today (CE-1649) — Configure can't
// decode its config (map reflect error, swallowed), so the block is ignored and,
// with the env var cleared, configuration fails with "Missing API Key". Green ==
// CE-1649 fixed. Fails at Configure (no pod created), so no CheckDestroy needed.
func TestAccProviderConfig_apiKeyFromBlock(t *testing.T) {
	if os.Getenv("RIAB_ACC") != "1" {
		t.Skip("set RIAB_ACC=1 + RUNPOD_API_KEY + RUNPOD_BASE_URL to run live riab tests")
	}
	key := os.Getenv("RUNPOD_API_KEY")
	if key == "" || os.Getenv("RUNPOD_BASE_URL") == "" {
		t.Fatal("RUNPOD_API_KEY and RUNPOD_BASE_URL must be set")
	}
	t.Setenv("RUNPOD_API_KEY", "") // force the provider to rely on the HCL block

	cfg := fmt.Sprintf(`
provider "runpod" {
  api_key = %q
}

resource "runpod_pod" "demo" {
  name          = "tf-cfgkey"
  image_name    = "runpod/test:latest"
  gpu_count     = 1
  cloud_type    = "SECURE"
  start_ssh     = true
  start_jupyter = true
}
`, key)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             cfg,
				ExpectNonEmptyPlan: true, // CE-1658 drift, if it ever gets past configure
				Check:              resource.TestCheckResourceAttrSet("runpod_pod.demo", "id"),
			},
		},
	})
}

// TestAccPodCreate_InvalidTemplate_framework is negative API-level coverage: a bad
// template_id must surface the API error through terraform (not panic, not
// silently succeed). Green — verifies error propagation. Deterministic and
// capacity-independent (fails at template lookup, before provisioning).
func TestAccPodCreate_InvalidTemplate_framework(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "runpod" {}

resource "runpod_pod" "bad" {
  name        = "tf-bad-template"
  template_id = "does-not-exist-xyz"
  gpu_count   = 1
}
`,
				ExpectError: regexp.MustCompile("template not found|API Error"),
			},
		},
	})
}

// TestAccPodValidation_framework checks the provider's config validation surfaces
// through real HCL: specifying both template_id and image_name, or neither, must
// fail the apply with a clear error. Capacity-independent (validation fires in
// Create before any API call), so these are reliable greens.
func TestAccPodValidation_framework(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		errRe string
	}{
		{
			"both_template_and_image",
			`
  name        = "tf-neg"
  gpu_count   = 1
  template_id = "test-template"
  image_name  = "runpod/test:latest"`,
			"Cannot specify both",
		},
		{
			"neither_template_nor_image",
			`
  name      = "tf-neg"
  gpu_count = 1`,
			"Must specify either",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      fmt.Sprintf("provider \"runpod\" {}\nresource \"runpod_pod\" \"x\" {%s\n}\n", tc.body),
						ExpectError: regexp.MustCompile(tc.errRe),
					},
				},
			})
		})
	}
}

// TestAccPodImport_framework asserts `terraform import` works: create a pod,
// then import it and verify state. RED today — no resource implements
// ImportState, so the import step fails ("resource does not support import").
// Green == import implemented for runpod_pod.
func TestAccPodImport_framework(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPodDestroy,
		Steps: []resource.TestStep{
			{
				Config:             testAccPodDemoConfig("tf-import", "runpod/test:latest"),
				ExpectNonEmptyPlan: true, // CE-1658 drift
			},
			{
				ResourceName:      "runpod_pod.demo",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccPodUpdate_framework is the canonical create→update sequence: change a
// mutable attribute (name; Optional+Computed, no RequiresReplace, so it's an
// in-place Update) and expect the new value. RED today — pod Update is an empty
// no-op (CE-1655), so the in-place update returns null state and the apply
// fails. Green == Update applies the change.
func TestAccPodUpdate_framework(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPodDestroy,
		Steps: []resource.TestStep{
			{
				Config:             testAccPodDemoConfig("tf-upd-a", "runpod/test:latest"),
				ExpectNonEmptyPlan: true, // CE-1658 drift
				Check:              resource.TestCheckResourceAttr("runpod_pod.demo", "name", "tf-upd-a"),
			},
			{
				Config:             testAccPodDemoConfig("tf-upd-b", "runpod/test:latest"),
				ExpectNonEmptyPlan: true,
				Check:              resource.TestCheckResourceAttr("runpod_pod.demo", "name", "tf-upd-b"),
			},
		},
	})
}

func TestAccDataSources_framework(t *testing.T) {
	cases := []struct {
		name string
		hcl  string
		addr string
	}{
		{"gpu_types", `data "runpod_gpu_types" "test" {}`, "data.runpod_gpu_types.test"},
		{"user", `data "runpod_user" "test" {}`, "data.runpod_user.test"},
		{"data_centers", `data "runpod_data_centers" "test" {}`, "data.runpod_data_centers.test"},
		{"machines", `data "runpod_machines" "test" {}`, "data.runpod_machines.test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: "provider \"runpod\" {}\n" + tc.hcl,
						Check:  resource.TestCheckResourceAttrSet(tc.addr, "id"),
					},
				},
			})
		})
	}
}

// TestAccPodResource_framework drives the pod resource through real HCL via the
// terraform-plugin-testing framework: apply → assert state → (framework refresh
// + post-apply plan idempotency check) → destroy → CheckDestroy. Config mirrors
// the demo (sets start_ssh/start_jupyter).
//
// Verified behavior (2026-06-25, riab):
//   - apply SUCCEEDS (create works — the demo path).
//   - RED on the framework's post-apply idempotency check: "refresh plan was not
//     empty" — CE-1658: Read maps status/created_at/gpuTypeId/cloudType to
//     fields the v1 API doesn't return, so applied state doesn't round-trip and a
//     follow-up plan shows perpetual drift.
//   - Separately, OMITTING start_ssh/start_jupyter triggers CE-1660,
//     "inconsistent result after apply" (those bools come back null vs planned).
//
// Green here == CE-1658 fixed (clean apply + empty follow-up plan).
//
// Auto-skips unless TF_ACC set. Needs the terraform binary + a live endpoint:
//
//	TF_ACC=1 RUNPOD_API_KEY=$TEST_USER_JWT RUNPOD_BASE_URL=http://localhost:8081/v1 \
//	  go test . -run TestAccPodResource_framework -v
func TestAccPodResource_framework(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPodDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPodConfig("tf-fw-acc", "runpod/test:latest"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("runpod_pod.test", "id"),
					resource.TestCheckResourceAttr("runpod_pod.test", "name", "tf-fw-acc"),
					resource.TestCheckResourceAttr("runpod_pod.test", "image_name", "runpod/test:latest"),
				),
			},
		},
	})
}

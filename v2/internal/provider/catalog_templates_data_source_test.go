package provider

import (
	"context"
	"testing"

	"github.com/runpod/go-sdk/v2/lib/catalog"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

func TestFlattenCatalogTemplates(t *testing.T) {
	ctx := context.Background()

	state, diags := flattenCatalogTemplates(ctx, string(catalog.CatalogTemplateSourceVerified), []sdktypes.Template{
		{
			Id:                  "tpl_public",
			Name:                "RunPod PyTorch",
			Image:               "runpod/pytorch:2.8.0",
			Category:            sdktypes.TemplateCategoryNVIDIA,
			Disk:                50,
			Env:                 map[string]string{"JUPYTER_PASSWORD": "changeme"},
			Ports:               []string{"8888/http"},
			Public:              true,
			AllowedCudaVersions: []string{"12.8"},
			StartJupyter:        true,
			Mounts: sdktypes.TemplateMounts{
				Persistent: &sdktypes.PersistentMount{Path: "/workspace", Size: 20},
			},
		},
		{
			Id:                  "tpl_serverless",
			Name:                "vLLM worker",
			Image:               "runpod/worker-vllm:1.0.0",
			Category:            sdktypes.TemplateCategoryNVIDIA,
			Disk:                20,
			Env:                 map[string]string{},
			Ports:               []string{},
			Public:              true,
			Serverless:          true,
			AllowedCudaVersions: []string{},
		},
	})
	if diags.HasError() {
		t.Fatal(diags)
	}

	if state.Source.ValueString() != "verified" {
		t.Fatalf("source = %q, want verified", state.Source.ValueString())
	}
	if len(state.Templates) != 2 {
		t.Fatalf("templates = %d, want 2", len(state.Templates))
	}
	first := state.Templates[0]
	if first.Id.ValueString() != "tpl_public" || first.Name.ValueString() != "RunPod PyTorch" {
		t.Fatalf("first template: %+v", first)
	}
	if !first.StartJupyter.ValueBool() || first.StartSsh.ValueBool() {
		t.Fatalf("first template start flags: %+v", first)
	}
	if first.Mounts == nil || first.Mounts.Persistent == nil ||
		first.Mounts.Persistent.Size.ValueInt64() != 20 {
		t.Fatalf("first template mounts: %+v", first.Mounts)
	}
	second := state.Templates[1]
	if !second.Serverless.ValueBool() {
		t.Fatalf("second template must be serverless: %+v", second)
	}
	if second.Mounts == nil || second.Mounts.Persistent != nil {
		t.Fatalf("second template mounts: %+v", second.Mounts)
	}

	assertDataSourceModelFits(t, NewCatalogTemplatesDataSource, state)
}

func TestFlattenCatalogTemplatesEmpty(t *testing.T) {
	ctx := context.Background()

	state, diags := flattenCatalogTemplates(ctx, string(catalog.CatalogTemplateSourceOfficial), nil)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if state.Templates == nil || len(state.Templates) != 0 {
		t.Fatalf("templates = %v, want an empty list", state.Templates)
	}

	assertDataSourceModelFits(t, NewCatalogTemplatesDataSource, state)
}

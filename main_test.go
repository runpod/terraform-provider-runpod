package main

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func strPtr(s string) *string { return &s }

// makeProviderConfig builds a tfsdk.Config matching the provider schema. Pass nil
// for an attribute to make it null (exercising the env-fallback path).
func makeProviderConfig(t *testing.T, p *runpodProvider, apiKey, baseURL *string) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	sr := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, sr)

	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"api_key":  tftypes.String,
		"base_url": tftypes.String,
	}}
	val := func(s *string) tftypes.Value {
		if s == nil {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, *s)
	}
	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"api_key":  val(apiKey),
		"base_url": val(baseURL),
	})
	return tfsdk.Config{Schema: sr.Schema, Raw: raw}
}

func configureProvider(t *testing.T, apiKey, baseURL *string) (*runpodProvider, *provider.ConfigureResponse) {
	t.Helper()
	p := &runpodProvider{}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: makeProviderConfig(t, p, apiKey, baseURL)}, resp)
	return p, resp
}

func TestConfigure_ConfigApiKeyWins(t *testing.T) {
	t.Setenv("RUNPOD_API_KEY", "envkey123")
	p, resp := configureProvider(t, strPtr("cfgkey123"), nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if p.apiKey != "cfgkey123" {
		t.Errorf("apiKey = %q, want config value to override env", p.apiKey)
	}
}

func TestConfigure_EnvApiKeyFallback(t *testing.T) {
	t.Setenv("RUNPOD_API_KEY", "envkey123")
	p, resp := configureProvider(t, nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if p.apiKey != "envkey123" {
		t.Errorf("apiKey = %q, want env fallback", p.apiKey)
	}
}

func TestConfigure_ConfigBaseURLWins(t *testing.T) {
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", "https://env.runpod.io/v1")
	p, resp := configureProvider(t, nil, strPtr("https://cfg.runpod.io/v1"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if p.baseUrl != "https://cfg.runpod.io/v1" {
		t.Errorf("baseUrl = %q, want config value to override env", p.baseUrl)
	}
}

func TestConfigure_BaseURLDefault(t *testing.T) {
	t.Setenv("RUNPOD_API_KEY", "envkey123")
	t.Setenv("RUNPOD_BASE_URL", "")
	p, resp := configureProvider(t, nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if p.baseUrl != "https://rest.runpod.io/v1" {
		t.Errorf("baseUrl = %q, want default", p.baseUrl)
	}
}

func TestConfigure_MissingApiKeyErrors(t *testing.T) {
	t.Setenv("RUNPOD_API_KEY", "")
	_, resp := configureProvider(t, nil, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a 'Missing API Key' error diagnostic, got none")
	}
}

// TestConfigure_ShortApiKey_Panics characterizes a latent panic: on the success
// path Configure logs `p.apiKey[:8]`, which slices out of range when the key is
// shorter than 8 characters. With a 5-char key, Configure panics instead of
// returning. When fixed (guard the slice), this test should be updated.
func TestConfigure_ShortApiKey_Panics(t *testing.T) {
	t.Setenv("RUNPOD_API_KEY", "short") // 5 chars < 8
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected a slice-out-of-range panic from p.apiKey[:8] with a <8-char key; none occurred (slice may now be guarded)")
		}
	}()
	p := &runpodProvider{}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: makeProviderConfig(t, p, nil, nil)}, resp)
}

// TestResourceSchemas is a smoke check: every registered resource builds its
// schema without diagnostics and exposes at least one attribute. It does not
// run full framework schema validation — it catches gross breakage / gen drift.
func TestResourceSchemas(t *testing.T) {
	ctx := context.Background()
	p := &runpodProvider{}
	for i, ctor := range p.Resources(ctx) {
		r := ctor()
		resp := &resource.SchemaResponse{}
		r.Schema(ctx, resource.SchemaRequest{}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("resource[%d] schema diagnostics: %v", i, resp.Diagnostics)
		}
		if len(resp.Schema.Attributes) == 0 {
			t.Errorf("resource[%d] schema has no attributes", i)
		}
	}
}

func TestDataSourceSchemas(t *testing.T) {
	ctx := context.Background()
	p := &runpodProvider{}
	for i, ctor := range p.DataSources(ctx) {
		d := ctor()
		resp := &datasource.SchemaResponse{}
		d.Schema(ctx, datasource.SchemaRequest{}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("datasource[%d] schema diagnostics: %v", i, resp.Diagnostics)
		}
		if len(resp.Schema.Attributes) == 0 {
			t.Errorf("datasource[%d] schema has no attributes", i)
		}
	}
}

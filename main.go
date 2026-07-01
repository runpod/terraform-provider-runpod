package main

import (
	"context"
	"log"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	resource_pod "github.com/runpod/terraform-provider-runpod/internal/provider/resource_pod"
	resource_pod_action "github.com/runpod/terraform-provider-runpod/internal/provider/resource_pod_action"
	resource_machine "github.com/runpod/terraform-provider-runpod/internal/provider/resource_machine"
	resource_network_volume "github.com/runpod/terraform-provider-runpod/internal/provider/resource_network_volume"
	resource_endpoint "github.com/runpod/terraform-provider-runpod/internal/provider/resource_endpoint"
	resource_template "github.com/runpod/terraform-provider-runpod/internal/provider/resource_template"
	resource_container_registry_auth "github.com/runpod/terraform-provider-runpod/internal/provider/resource_container_registry_auth"

	datasource_pod "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_pod"
	datasource_machine "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_machine"
	datasource_machines "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_machines"
	datasource_gpu_types "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_gpu_types"
	datasource_data_centers "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_data_centers"
	datasource_user "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_user"
	datasource_template "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_template"
	datasource_container_registry_auth "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_container_registry_auth"

	datasource_billing_pod "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_billing_pod"
	datasource_billing_network_volume "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_billing_network_volume"
	datasource_billing_endpoint "github.com/runpod/terraform-provider-runpod/internal/provider/datasource_billing_endpoint"
)

func main() {
	log.Println("Starting RunPod provider...")

	err := providerserver.Serve(context.Background(), func() provider.Provider {
		log.Println("Creating provider instance...")
		return newProvider()
	}, providerserver.ServeOpts{
		Address: "registry.terraform.io/runpod/runpod",
	})

	if err != nil {
		log.Printf("Provider server error: %v\n", err)
	}
}

func newProvider() provider.Provider {
	log.Println("newProvider() called")
	return &runpodProvider{}
}

type runpodProvider struct {
	apiKey     string
	baseUrl    string
	graphqlUrl string
}

type providerConfig struct {
	ApiKey  types.String `tfsdk:"api_key"`
	BaseUrl types.String `tfsdk:"base_url"`
}

func (p *runpodProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	log.Println("Metadata() called")
	resp.TypeName = "runpod"
}

func (p *runpodProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	log.Println("Schema() called")
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				Description:         "RunPod API key",
				MarkdownDescription: "RunPod API key. Can also be set via the `RUNPOD_API_KEY` environment variable.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				Description:         "RunPod API base URL",
				MarkdownDescription: "RunPod API base URL. Can also be set via the `RUNPOD_BASE_URL` environment variable. Defaults to `https://rest.runpod.io/v1`.",
			},
		},
	}
}

func (p *runpodProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	log.Println("Configure() called")

	var cfg providerConfig
	diags := req.Config.Get(ctx, &cfg)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !cfg.ApiKey.IsNull() && !cfg.ApiKey.IsUnknown() {
		p.apiKey = cfg.ApiKey.ValueString()
	}
	if !cfg.BaseUrl.IsNull() && !cfg.BaseUrl.IsUnknown() {
		p.baseUrl = cfg.BaseUrl.ValueString()
	}

	if p.apiKey == "" {
		p.apiKey = os.Getenv("RUNPOD_API_KEY")
	}
	if p.baseUrl == "" {
		p.baseUrl = os.Getenv("RUNPOD_BASE_URL")
	}

	if p.baseUrl == "" {
		p.baseUrl = "https://rest.runpod.io/v1"
	}
	p.graphqlUrl = "https://api.runpod.io/graphql"

	if p.apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing API Key",
			"RunPod API key is required. Set it via provider config or RUNPOD_API_KEY environment variable.",
		)
		return
	}

	log.Printf("Using API key: %s\n", p.apiKey[:8]+"...")
	log.Printf("API base URL: %s\n", p.baseUrl)
	log.Printf("GraphQL URL: %s\n", p.graphqlUrl)
}

func (p *runpodProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	log.Println("DataSources() called")
	return []func() datasource.DataSource{
		datasource_pod.NewPodDataSource,
		datasource_machine.NewMachineDataSource,
		datasource_machines.NewMachinesDataSource,
		datasource_gpu_types.NewGpuTypesDataSource,
		datasource_data_centers.NewDataCentersDataSource,
		datasource_user.NewUserDataSource,
		datasource_template.NewTemplateDataSource,
		datasource_container_registry_auth.NewContainerRegistryAuthDataSource,
		datasource_billing_pod.NewBillingPodDataSource,
		datasource_billing_network_volume.NewBillingNetworkVolumeDataSource,
		datasource_billing_endpoint.NewBillingEndpointDataSource,
	}
}

func (p *runpodProvider) Resources(ctx context.Context) []func() resource.Resource {
	log.Println("Resources() called")
	return []func() resource.Resource{
		resource_pod.NewPodResource,
		resource_pod_action.NewPodActionResource,
		resource_machine.NewMachineResource,
		resource_network_volume.NewNetworkVolumeResource,
		resource_endpoint.NewEndpointResource,
		resource_template.NewTemplateResource,
		resource_container_registry_auth.NewContainerRegistryAuthResource,
	}
}

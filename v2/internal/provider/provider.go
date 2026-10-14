package provider

import (
	"context"
	"os"
	"runtime"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
)

var _ provider.Provider = (*runpodProvider)(nil)

type runpodProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &runpodProvider{version: version}
	}
}

type runpodProviderModel struct {
	APIKey     types.String `tfsdk:"api_key"`
	BaseURL    types.String `tfsdk:"base_url"`
	GraphQLURL types.String `tfsdk:"graphql_url"`
	RestV1URL  types.String `tfsdk:"rest_v1_url"`
}

func (p *runpodProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "runpod"
	resp.Version = p.version
}

func (p *runpodProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Runpod GPU and CPU pods, Serverless endpoints, templates, network volumes, Instant Clusters, account SSH keys and container registry credentials through the Runpod REST API v2.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "Runpod API key. Falls back to the RUNPOD_API_KEY environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"base_url": schema.StringAttribute{
				Description: "API base URL. Leave unset for the production API; set it only to target a " +
					"non-production Runpod environment. Falls back to the RUNPOD_API_BASE_URL " +
					"environment variable, then the production API.",
				Optional: true,
			},
			"graphql_url": schema.StringAttribute{
				Description: "GraphQL endpoint for the deprecated GraphQL shims: the runpod_machine " +
					"resource, the runpod_machine / runpod_machines data sources and the runpod_user " +
					"data source. Falls back to the " +
					"RUNPOD_GRAPHQL_URL environment variable, then https://api.runpod.io/graphql. " +
					"Ignored by everything on the v2 REST API.",
				Optional: true,
			},
			"rest_v1_url": schema.StringAttribute{
				Description: "REST v1 base URL for the deprecated REST v1 shims: the runpod_billing_pod / " +
					"runpod_billing_network_volume / runpod_billing_endpoint data sources, and the " +
					"deprecated attributes on runpod_pod, " +
					"runpod_template (readme and docker_entrypoint among them), the runpod_template " +
					"data source and runpod_serverless_endpoint. Falls back to the RUNPOD_REST_V1_URL " +
					"environment variable, then RUNPOD_BASE_URL (the 1.x name for this host), then " +
					"https://rest.runpod.io/v1. Ignored by everything on the v2 REST API.",
				Optional: true,
			},
		},
	}
}

func (p *runpodProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config runpodProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	agent := userAgent(p.version, req.TerraformVersion)
	opts := []runpod.Option{runpod.WithUserAgent(agent)}
	// An unknown value, such as api_key = var.key before the variable is
	// resolved, stringifies to "", which would override the environment fallback and
	// fail as a missing key. Leave it to the SDK's own env lookup instead.
	if !config.APIKey.IsNull() && !config.APIKey.IsUnknown() {
		opts = append(opts, runpod.WithAPIKey(config.APIKey.ValueString()))
	}
	baseURL := os.Getenv("RUNPOD_API_BASE_URL")
	if !config.BaseURL.IsNull() && !config.BaseURL.IsUnknown() {
		baseURL = config.BaseURL.ValueString()
	}
	if baseURL != "" {
		opts = append(opts, runpod.WithBaseURL(baseURL))
	}

	client, err := runpod.New(opts...)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to configure the Runpod client",
			"Creating the API client failed: "+err.Error()+
				". Set the api_key provider attribute or the RUNPOD_API_KEY environment variable.",
		)
		return
	}

	// Both clients read the same key, so a failure here is the same missing-key
	// case as above rather than a second thing for the operator to fix.
	legacyClient, err := newLegacyClient(config, agent)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to configure the Runpod client",
			"Creating the client for the deprecated REST v1 and GraphQL shims failed: "+err.Error()+
				". Set the api_key provider attribute or the RUNPOD_API_KEY environment variable.",
		)
		return
	}

	configured := &clients{API: client, Legacy: legacyClient}
	resp.DataSourceData = configured
	resp.ResourceData = configured
}

// userAgent is sent by the v2 client, and by the deprecated GraphQL and REST
// v1 shims after the 1.x value (see newLegacyClient), so 2.0.0 traffic can be
// told apart from 1.x on every host.
// WithUserAgent replaces the SDK's own string, so the SDK part is added back.
func userAgent(providerVersion, terraformVersion string) string {
	return "terraform-provider-runpod/" + providerVersion +
		" (terraform " + terraformVersion + "; " + runtime.Version() + ")" +
		" runpod-go-sdk/" + runpod.Version
}

func (p *runpodProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewGpuTypesDataSource,
		NewGpuTypeDataSource,
		NewCpuTypesDataSource,
		NewCpuTypeDataSource,
		NewDataCentersDataSource,
		NewDataCenterDataSource,
		NewPodsDataSource,
		NewTemplatesDataSource,
		NewNetworkVolumesDataSource,
		NewServerlessEndpointsDataSource,
		NewRegistriesDataSource,
		NewPodDataSource,
		NewTemplateDataSource,
		NewPodLogsDataSource,
		NewEndpointWorkersDataSource,
		NewEndpointWorkerLogsDataSource,
		NewEndpointBuildsDataSource,
		NewEndpointBuildDataSource,
		NewEndpointReleasesDataSource,
		NewCatalogTemplatesDataSource,
		NewBillingDataSource,
		NewPodBillingDataSource,
		NewServerlessBillingDataSource,
		NewEndpointBillingDataSource,
		NewNetworkVolumeBillingDataSource,
		NewClusterBillingDataSource,
		NewClustersDataSource,
		NewClusterDataSource,
		NewClusterPodsDataSource,
		NewRegistryDelegationsDataSource,
		NewMachineDataSource,
		NewMachinesDataSource,
		NewUserDataSource,
		NewLegacyPodBillingDataSource,
		NewLegacyNetworkVolumeBillingDataSource,
		NewLegacyEndpointBillingDataSource,
	}
}

func (p *runpodProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPodResource,
		NewNetworkVolumeResource,
		NewTemplateResource,
		NewRegistryResource,
		NewRegistryDelegationResource,
		NewServerlessEndpointResource,
		NewClusterResource,
		NewAccountSshKeysResource,
		NewPodActionResource,
		NewMachineResource,
	}
}

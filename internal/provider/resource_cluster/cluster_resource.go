package resource_cluster

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewClusterResource() resource.Resource {
	return &ClusterResource{}
}

type ClusterResource struct {
	rlClient *client.RunPodClient
}

const defaultGraphQLEndpoint = "https://api.runpod.io/graphql"

func (r *ClusterResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		if clientWrapper, ok := req.ProviderData.(*client.RunPodClientWrapper); ok {
			// Clusters are GraphQL-only; the wrapper strips the endpoint, so
			// rebuild it (env override first, like getClient, then default).
			graphqlEndpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
			if graphqlEndpoint == "" {
				graphqlEndpoint = defaultGraphQLEndpoint
			}
			r.rlClient = &client.RunPodClient{
				APIKey:          clientWrapper.APIKey,
				GraphQLEndpoint: graphqlEndpoint,
				RestBaseURL:     clientWrapper.RestBaseURL,
				Client:          &http.Client{Timeout: 60 * time.Second},
			}
		} else if client, ok := req.ProviderData.(*client.RunPodClient); ok {
			r.rlClient = client
		}
	}
}

func (r *ClusterResource) getClient() *client.RunPodClient {
	if r.rlClient != nil {
		return r.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	graphqlEndpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if graphqlEndpoint == "" {
		graphqlEndpoint = defaultGraphQLEndpoint
	}
	// Clusters are GraphQL-only; the REST base URL is unused here.
	r.rlClient = client.NewRunPodClient(apiKey, graphqlEndpoint, "")
	return r.rlClient
}

func (r *ClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_cluster"
}

func (r *ClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = ClusterResourceSchema(ctx)
}

func (r *ClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config ClusterModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		mutation CreateCluster($input: CreateClusterInput!) {
			createCluster(input: $input) {
				id
				name
				type
				podCount
			}
		}
	`

	input := map[string]interface{}{
		"gpuTypeId":      config.GpuTypeId.ValueString(),
		"podCount":       config.PodCount.ValueInt64(),
		"gpuCountPerPod": config.GpuCountPerPod.ValueInt64(),
		"type":           config.Type.ValueString(),
	}
	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		input["clusterName"] = config.Name.ValueString()
	}
	if !config.DeployCost.IsNull() && !config.DeployCost.IsUnknown() {
		input["deployCost"] = config.DeployCost.ValueFloat64()
	}
	if !config.TemplateId.IsNull() && !config.TemplateId.IsUnknown() {
		input["templateId"] = config.TemplateId.ValueString()
	}
	if !config.ImageName.IsNull() && !config.ImageName.IsUnknown() {
		input["imageName"] = config.ImageName.ValueString()
	}
	if !config.ContainerDiskInGb.IsNull() && !config.ContainerDiskInGb.IsUnknown() {
		input["containerDiskInGb"] = config.ContainerDiskInGb.ValueInt64()
	}
	if !config.Ports.IsNull() && !config.Ports.IsUnknown() {
		input["ports"] = config.Ports.ValueString()
	}
	if !config.NetworkVolumeId.IsNull() && !config.NetworkVolumeId.IsUnknown() {
		input["networkVolumeId"] = config.NetworkVolumeId.ValueString()
	}
	if !config.StartSsh.IsNull() && !config.StartSsh.IsUnknown() {
		input["startSsh"] = config.StartSsh.ValueBool()
	}
	if !config.DataCenterIds.IsNull() && !config.DataCenterIds.IsUnknown() {
		var dcs []string
		if d := config.DataCenterIds.ElementsAs(ctx, &dcs, false); d.HasError() {
			resp.Diagnostics.Append(d...)
			return
		}
		input["dataCenterIds"] = dcs
	}
	if !config.Env.IsNull() && !config.Env.IsUnknown() {
		envVars := make([]map[string]interface{}, 0, len(config.Env.Elements()))
		for k, v := range config.Env.Elements() {
			if s, ok := v.(types.String); ok {
				envVars = append(envVars, map[string]interface{}{"key": k, "value": s.ValueString()})
			}
		}
		input["env"] = envVars
	}

	variables := map[string]interface{}{"input": input}

	result, err := r.getClient().Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	cluster, ok := result["createCluster"].(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError("API Error", "createCluster not in response")
		return
	}

	id, ok := cluster["id"].(string)
	if !ok || id == "" {
		resp.Diagnostics.AddError("API Error", "Failed to get cluster ID from response")
		return
	}
	config.Id = types.StringValue(id)

	if v, ok := cluster["name"].(string); ok {
		config.Name = types.StringValue(v)
	}
	if v, ok := cluster["type"].(string); ok {
		config.Type = types.StringValue(v)
	}

	// Pods are still provisioning at this point; refresh() populates them.
	config.Pods = []ClusterPodModel{}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *ClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var config ClusterModel
	diags := req.State.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		query GetCluster($id: String!) {
			myself {
				cluster(input: { id: $id }) {
					id
					name
					type
					podCount
					gpuCountPerPod
					pods {
						id
						name
						clusterIdx
						clusterRole
						clusterIp
						desiredStatus
					}
				}
			}
		}
	`

	variables := map[string]interface{}{
		"id": config.Id.ValueString(),
	}

	result, err := r.getClient().Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	myself, _ := result["myself"].(map[string]interface{})
	cluster, _ := myself["cluster"].(map[string]interface{})
	if cluster == nil {
		// Deleted out-of-band: drop from state instead of erroring.
		resp.State.RemoveResource(ctx)
		return
	}

	if v, ok := cluster["name"].(string); ok {
		config.Name = types.StringValue(v)
	}
	if v, ok := cluster["type"].(string); ok {
		config.Type = types.StringValue(v)
	}
	if v, ok := cluster["podCount"].(float64); ok {
		config.PodCount = types.Int64Value(int64(v))
	}
	if v, ok := cluster["gpuCountPerPod"].(float64); ok {
		config.GpuCountPerPod = types.Int64Value(int64(v))
	}

	config.Pods = []ClusterPodModel{}
	if pods, ok := cluster["pods"].([]interface{}); ok {
		for _, p := range pods {
			pm, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			pod := ClusterPodModel{}
			if v, ok := pm["id"].(string); ok {
				pod.Id = types.StringValue(v)
			}
			if v, ok := pm["name"].(string); ok {
				pod.Name = types.StringValue(v)
			}
			if v, ok := pm["clusterIdx"].(float64); ok {
				pod.ClusterIdx = types.Int64Value(int64(v))
			}
			if v, ok := pm["clusterRole"].(string); ok {
				pod.ClusterRole = types.StringValue(v)
			}
			if v, ok := pm["clusterIp"].(string); ok {
				pod.ClusterIp = types.StringValue(v)
			}
			if v, ok := pm["desiredStatus"].(string); ok {
				pod.DesiredStatus = types.StringValue(v)
			}
			config.Pods = append(config.Pods, pod)
		}
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *ClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Unreachable in practice: every non-computed attribute RequiresReplace.
	resp.Diagnostics.AddError(
		"Cluster does not support in-place updates",
		"The RunPod cluster API has no update operation; all attribute changes must force replacement.",
	)
}

func (r *ClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var config ClusterModel
	diags := req.State.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		mutation DeleteCluster($id: String!) {
			deleteCluster(input: { id: $id })
		}
	`

	variables := map[string]interface{}{
		"id": config.Id.ValueString(),
	}

	_, err := r.getClient().Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}
}

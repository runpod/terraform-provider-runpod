package datasource_endpoint_jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/attr"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewEndpointJobsDataSource() datasource.DataSource {
	return &EndpointJobsDataSource{}
}

type EndpointJobsDataSource struct {
	rlClient *client.RunPodClient
}

func (r *EndpointJobsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *EndpointJobsDataSource) getClient() *client.RunPodClient {
	if r.rlClient != nil {
		return r.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	baseURL := os.Getenv("RUNPOD_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.runpod.io/v2"
	}
	r.rlClient = client.NewRunPodClient(apiKey, "https://api.runpod.io/graphql", baseURL)
	return r.rlClient
}

func (r *EndpointJobsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_endpoint_jobs"
}

func (r *EndpointJobsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = EndpointJobsDataSourceSchema(ctx)
}

func (r *EndpointJobsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config EndpointJobsDataSourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	rlClient := r.getClient()

	url := rlClient.RestBaseURL + "/serverless/" + config.EndpointId.ValueString() + "/jobs"

	queryParams := []string{}
	if !config.StatusFilter.IsNull() && config.StatusFilter.ValueString() != "" {
		queryParams = append(queryParams, "status="+config.StatusFilter.ValueString())
	}
	if !config.Limit.IsNull() && config.Limit.ValueInt64() > 0 {
		queryParams = append(queryParams, "limit="+strconv.FormatInt(config.Limit.ValueInt64(), 10))
	}
	if !config.Cursor.IsNull() && config.Cursor.ValueString() != "" {
		queryParams = append(queryParams, "cursor="+config.Cursor.ValueString())
	}

	if len(queryParams) > 0 {
		url += "?" + strings.Join(queryParams, "&")
	}

	reqHTTP, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rlClient.APIKey))

	httpClient := &http.Client{}
	respHTTP, err := httpClient.Do(reqHTTP)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to make API call: %v", err))
		return
	}
	defer respHTTP.Body.Close()

	respBody, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
		return
	}

	if respHTTP.StatusCode != 200 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to list jobs (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response: %v", err))
		return
	}

	jobsData, ok := result["jobs"].([]interface{})
	if !ok {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get jobs from response: %v", result))
		return
	}

	jobs := make([]attr.Value, 0)
	for _, job := range jobsData {
		if jobMap, ok := job.(map[string]interface{}); ok {
			jobObj := map[string]attr.Value{
				"id":           types.StringValue(jobMap["id"].(string)),
				"status":       types.StringValue(jobMap["status"].(string)),
				"created_at":   types.StringValue(jobMap["createdAt"].(string)),
				"duration_ms":  types.Int64Value(int64(jobMap["durationMs"].(float64))),
				"completed_at": types.StringValue(jobMap["completedAt"].(string)),
				"input":        types.StringValue(jobMap["input"].(string)),
				"output":       types.StringValue(jobMap["output"].(string)),
			}
			jobVal, diags := types.ObjectValue(map[string]attr.Type{
				"id":           types.StringType,
				"status":       types.StringType,
				"created_at":   types.StringType,
				"duration_ms":  types.Int64Type,
				"completed_at": types.StringType,
				"input":        types.StringType,
				"output":       types.StringType,
			}, jobObj)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			jobs = append(jobs, jobVal)
		}
	}

	jobsList, diags := types.ListValue(types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":           types.StringType,
			"status":       types.StringType,
			"created_at":   types.StringType,
			"duration_ms":  types.Int64Type,
			"completed_at": types.StringType,
			"input":        types.StringType,
			"output":       types.StringType,
		},
	}, jobs)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	config.Jobs = jobsList

	if nextCursor, ok := result["cursor"].(string); ok {
		config.NextCursor = types.StringValue(nextCursor)
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

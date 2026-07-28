package datasource_endpoint_job_logs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewEndpointJobLogsDataSource() datasource.DataSource {
	return &EndpointJobLogsDataSource{}
}

type EndpointJobLogsDataSource struct {
	rlClient *client.RunPodClient
}

func (r *EndpointJobLogsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *EndpointJobLogsDataSource) getClient() *client.RunPodClient {
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

func (r *EndpointJobLogsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_endpoint_job_logs"
}

func (r *EndpointJobLogsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = EndpointJobLogsDataSourceSchema(ctx)
}

func (r *EndpointJobLogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config EndpointJobLogsDataSourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	rlClient := r.getClient()

	endpointId := config.EndpointId.ValueString()
	jobId := config.JobId.ValueString()

	wsURL := fmt.Sprintf("%s/serverless/%s/jobs/%s/logs", rlClient.BaseURL(), endpointId, jobId)
	
	u, err := url.Parse(wsURL)
	if err != nil {
		resp.Diagnostics.AddError("URL Error", fmt.Sprintf("Failed to parse URL: %v", err))
		return
	}

	scheme := "https"
	if u.Scheme == "wss" || (u.Scheme == "" && strings.HasPrefix(wsURL, "wss://")) {
		scheme = "wss"
	}
	u.Scheme = scheme
	u.Path = strings.Replace(u.Path, "/logs", "", 1) + "/logs"

	reqHTTP, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rlClient.APIKey))

	httpClient := &http.Client{}
	respHTTP, err := httpClient.Do(reqHTTP)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to make API call: %v", err))
		return
	}
	defer respHTTP.Body.Close()

	if respHTTP.StatusCode != 200 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get logs (status: %d)", respHTTP.StatusCode))
		return
	}

	logs, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read logs: %v", err))
		return
	}

	config.Logs = types.StringValue(string(logs))

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

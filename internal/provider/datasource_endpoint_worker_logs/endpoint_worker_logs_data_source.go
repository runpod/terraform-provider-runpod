package datasource_endpoint_worker_logs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewEndpointWorkerLogsDataSource() datasource.DataSource {
	return &EndpointWorkerLogsDataSource{}
}

type EndpointWorkerLogsDataSource struct {
	rlClient *client.RunPodClient
}

func (r *EndpointWorkerLogsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *EndpointWorkerLogsDataSource) getClient() *client.RunPodClient {
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

func (r *EndpointWorkerLogsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_endpoint_worker_logs"
}

func (r *EndpointWorkerLogsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = EndpointWorkerLogsDataSourceSchema(ctx)
}

func (r *EndpointWorkerLogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config EndpointWorkerLogsDataSourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	rlClient := r.getClient()

	endpointId := config.EndpointId.ValueString()
	workerId := config.WorkerId.ValueString()

	wsURL := fmt.Sprintf("%s/serverless/%s/workers/%s/logs", rlClient.BaseURL(), endpointId, workerId)
	
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

	// Worker logs are server-sent events on a stream that the API holds open
	// for live workers. Read with a deadline and return whatever arrived; this
	// data source is a snapshot, not a tail.
	type readResult struct {
		data []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		data, err := io.ReadAll(respHTTP.Body)
		done <- readResult{data, err}
	}()

	var logs []byte
	select {
	case r := <-done:
		if r.err != nil {
			resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read logs: %v", r.err))
			return
		}
		logs = r.data
	case <-time.After(15 * time.Second):
		respHTTP.Body.Close() // unblocks the goroutine read
		r := <-done
		logs = r.data
	}

	config.Logs = types.StringValue(string(logs))

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

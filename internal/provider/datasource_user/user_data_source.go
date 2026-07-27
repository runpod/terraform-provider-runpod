package datasource_user

import (
	"os"
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

type UserDataSource struct {
	rlClient *client.RunPodClient
}

func (d *UserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *UserDataSource) getClient() *client.RunPodClient {
	if d.rlClient != nil {
		return d.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	endpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if endpoint == "" {
		endpoint = "https://api.runpod.io/graphql"
	}
	baseURL := os.Getenv("RUNPOD_BASE_URL")
	if baseURL == "" {
		baseURL = "https://rest.runpod.io/v1"
	}
	d.rlClient = client.NewRunPodClient(apiKey, endpoint, baseURL)
	return d.rlClient
}

func (d *UserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_user"
}

func (d *UserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = UserDataSourceSchema(ctx)
}

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	rlClient := d.getClient()
	result, err := rlClient.RestQuery(ctx, "GET", "user", nil)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	var idVal, nameVal, emailVal, pubKeyVal, cloudTypeStr string
	var verifiedVal bool
	var gpuLimitVal, gpuUsageVal, storageLimitVal, storageUsageVal float64

	// Parse v2 REST response (RestQuery already unwraps the {data: {...}} envelope)
	// Fields are now directly accessible from result
	// Parse id
	if id, ok := result["id"].(string); ok {
		idVal = id
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'id' is missing or not a string in user response")
		return
	}

	// Parse name (optional)
	if name, ok := result["name"].(string); ok {
		nameVal = name
	}

	// Parse email (optional)
	if email, ok := result["email"].(string); ok {
		emailVal = email
	}

	// Parse pubKey (optional)
	if pubKey, ok := result["pubKey"].(string); ok {
		pubKeyVal = pubKey
	}

	// Parse verified (optional)
	if verified, ok := result["verified"].(bool); ok {
		verifiedVal = verified
	}

	// Parse cloudType (optional)
	if cloudType, ok := result["cloudType"].(string); ok {
		cloudTypeStr = cloudType
	}

	// Parse gpuLimit (optional)
	if gpuLimit, ok := result["gpuLimit"].(float64); ok {
		gpuLimitVal = gpuLimit
	}

	// Parse gpuUsage (optional)
	if gpuUsage, ok := result["gpuUsage"].(float64); ok {
		gpuUsageVal = gpuUsage
	}

	// Parse storageLimit (optional)
	if storageLimit, ok := result["storageLimit"].(float64); ok {
		storageLimitVal = storageLimit
	}

	// Parse storageUsage (optional)
	if storageUsage, ok := result["storageUsage"].(float64); ok {
		storageUsageVal = storageUsage
	}

		// Map fields to model
		// id → id
		// pubKey → pub_key (for backward compatibility)
		// name, email, verified, cloudType, gpuLimit, gpuUsage, storageLimit, storageUsage

		model := UserModel{
			Id:           types.StringValue(idVal),
			Name:         types.StringValue(nameVal),
			Email:        types.StringValue(emailVal),
			PubKey:       types.StringValue(pubKeyVal),
			Verified:     types.BoolValue(verifiedVal),
			CloudType:    types.StringValue(cloudTypeStr),
			GpuLimit:     types.Float64Value(gpuLimitVal),
			GpuUsage:     types.Float64Value(gpuUsageVal),
			StorageLimit: types.Float64Value(storageLimitVal),
			StorageUsage: types.Float64Value(storageUsageVal),
		}
	diags := resp.State.Set(ctx, &model)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

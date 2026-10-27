package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	runpod "github.com/runpod/go-sdk/v2"
)

var _ datasource.DataSourceWithConfigure = (*globalVolumeDataSource)(nil)

func NewGlobalVolumeDataSource() datasource.DataSource { return &globalVolumeDataSource{} }

type globalVolumeDataSource struct {
	client *runpod.Client
}

func (d *globalVolumeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_global_volume"
}

func (d *globalVolumeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A single global volume looked up by identifier. Global volumes are usage-billed storage " +
			"that is not tied to a data center.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "Global volume identifier.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Human-readable name; not unique.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "When the volume was created, in RFC 3339 format.",
			},
			"created_by_user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifier of the user who created the volume; null when the API has no creator recorded.",
			},
		},
	}
}

func (d *globalVolumeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *globalVolumeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config globalVolumeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.GlobalVolumes.GetGlobalVolumeWithResponse(ctx, config.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading global volume failed", err.Error())
		return
	}
	// A data source has no state to drop, so a missing volume is an error like
	// any other status without a body.
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading global volume failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	var state globalVolumeModel
	flattenGlobalVolume(apiResp.JSON200, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

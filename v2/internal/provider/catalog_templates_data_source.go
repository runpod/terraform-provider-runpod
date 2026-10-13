package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/catalog"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// runpod_catalog_templates: the publicly shared template catalog, one slice at
// a time. Entries carry the same attributes as the runpod_template data
// source.

var (
	_ datasource.DataSource              = (*catalogTemplatesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*catalogTemplatesDataSource)(nil)
)

func NewCatalogTemplatesDataSource() datasource.DataSource { return &catalogTemplatesDataSource{} }

type catalogTemplatesDataSource struct {
	client *runpod.Client
}

type catalogTemplatesDataSourceModel struct {
	Source    types.String          `tfsdk:"source"`
	Templates []templateDetailModel `tfsdk:"templates"`
}

func (d *catalogTemplatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_templates"
}

func (d *catalogTemplatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Publicly shared templates from the Runpod catalog. One slice per read, selected by source; the account's own templates come from runpod_templates instead.",
		Attributes: map[string]schema.Attribute{
			"source": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Which slice of the catalog to read: \"official\" for Runpod-curated templates (the default), " +
					"\"verified\" for Runpod-verified community templates, or \"community\" for all other publicly shared templates.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(catalog.CatalogTemplateSourceOfficial),
						string(catalog.CatalogTemplateSourceVerified),
						string(catalog.CatalogTemplateSourceCommunity),
					),
				},
			},
			"templates": schema.ListNestedAttribute{
				Computed:     true,
				Description:  "One entry per public template in the selected slice.",
				NestedObject: schema.NestedAttributeObject{Attributes: templateDetailAttributes(false)},
			},
		},
	}
}

func (d *catalogTemplatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *catalogTemplatesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config catalogTemplatesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	source := catalog.CatalogTemplateSourceOfficial
	if !config.Source.IsNull() && !config.Source.IsUnknown() {
		source = catalog.ListPublicTemplatesParamsSource(config.Source.ValueString())
	}

	apiResp, err := d.client.Catalog.ListPublicTemplatesWithResponse(ctx,
		&catalog.ListPublicTemplatesParams{Source: &source})
	if err != nil {
		resp.Diagnostics.AddError("Listing public templates failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Listing public templates failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	state, diags := flattenCatalogTemplates(ctx, string(source), apiResp.JSON200.Templates)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenCatalogTemplates(ctx context.Context, source string, catalogTemplates []sdktypes.Template) (catalogTemplatesDataSourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	state := catalogTemplatesDataSourceModel{
		Source:    types.StringValue(source),
		Templates: []templateDetailModel{},
	}
	for _, template := range catalogTemplates {
		item, itemDiags := flattenCatalogTemplate(ctx, template)
		diags.Append(itemDiags...)
		state.Templates = append(state.Templates, item)
	}
	return state, diags
}

func flattenCatalogTemplate(ctx context.Context, template sdktypes.Template) (templateDetailModel, diag.Diagnostics) {
	model := templateDetailModel{
		Id:           types.StringValue(template.Id),
		Name:         types.StringValue(template.Name),
		Image:        types.StringValue(template.Image),
		Category:     types.StringValue(string(template.Category)),
		Disk:         types.Int64Value(int64(template.Disk)),
		Args:         types.StringValue(template.Args),
		Public:       types.BoolValue(template.Public),
		Serverless:   types.BoolValue(template.Serverless),
		Registry:     types.StringPointerValue(template.Registry),
		StartJupyter: types.BoolValue(template.StartJupyter),
		StartSsh:     types.BoolValue(template.StartSsh),
		Mounts:       &templateMountsModel{},
	}
	if template.Mounts.Persistent != nil {
		model.Mounts.Persistent = &templatePersistentMountModel{
			Path: types.StringValue(template.Mounts.Persistent.Path),
			Size: types.Int64Value(int64(template.Mounts.Persistent.Size)),
		}
	}

	diags := templateCollections(ctx, template.Env, template.Ports, template.AllowedCudaVersions,
		&model.Env, &model.Ports, &model.AllowedCudaVersions)
	return model, diags
}

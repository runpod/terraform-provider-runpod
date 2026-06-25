package datasource_template

import (
	"os"
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewTemplateDataSource() datasource.DataSource {
	return &TemplateDataSource{}
}

type TemplateDataSource struct{}

func (d *TemplateDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_template"
}

func (d *TemplateDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = TemplateDataSourceSchema(ctx)
}

func (d *TemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TemplateModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		query GetTemplate($templateId: String!) {
			template(input: { templateId: $templateId }) {
				id
				name
				imageName
				category
				containerDiskInGb
				containerRegistryAuthId
				dockerEntrypoint
				dockerStartCmd
				env
				isPublic
				isServerless
				ports
				readme
				volumeInGb
				volumeMountPath
				earned
				isRunpod
				runtimeInMin
			}
		}
	`

	variables := map[string]interface{}{
		"templateId": config.Id.ValueString(),
	}

	apiKey := os.Getenv("RUNPOD_API_KEY")
	runpodClient := client.NewRunPodClient(apiKey, client.GetGraphQLEndpoint())
	result, err := runpodClient.Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		if template, ok := data["template"].(map[string]interface{}); ok {
			envMap := make(map[string]attr.Value)
			if val, ok := template["env"].(map[string]interface{}); ok {
				for key, v := range val {
					if strVal, ok := v.(string); ok {
						envMap[key] = types.StringValue(strVal)
					}
				}
			}

			dockerEntrypoint := make([]attr.Value, 0)
			if val, ok := template["dockerEntrypoint"].([]interface{}); ok {
				for _, v := range val {
					if vStr, ok := v.(string); ok {
						dockerEntrypoint = append(dockerEntrypoint, types.StringValue(vStr))
					}
				}
			}

			dockerStartCmd := make([]attr.Value, 0)
			if val, ok := template["dockerStartCmd"].([]interface{}); ok {
				for _, v := range val {
					if vStr, ok := v.(string); ok {
						dockerStartCmd = append(dockerStartCmd, types.StringValue(vStr))
					}
				}
			}

			ports := make([]attr.Value, 0)
			if val, ok := template["ports"].([]interface{}); ok {
				for _, v := range val {
					if vStr, ok := v.(string); ok {
						ports = append(ports, types.StringValue(vStr))
					}
				}
			}

			dockerEntrypointList, diags := types.ListValue(types.StringType, dockerEntrypoint)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}

			dockerStartCmdList, diags := types.ListValue(types.StringType, dockerStartCmd)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}

			envObj, diags := types.ObjectValue(map[string]attr.Type{
				"key": types.StringType,
			}, envMap)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}

			portsList, diags := types.ListValue(types.StringType, ports)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}

			model := TemplateModel{
				Id:                      config.Id,
				Name:                    types.StringValue(template["name"].(string)),
				ImageName:               types.StringValue(template["imageName"].(string)),
				Category:                types.StringValue(template["category"].(string)),
				ContainerDiskInGb:       types.Int64Value(int64(template["containerDiskInGb"].(float64))),
				ContainerRegistryAuthId: types.StringValue(template["containerRegistryAuthId"].(string)),
				DockerEntrypoint:        dockerEntrypointList,
				DockerStartCmd:          dockerStartCmdList,
				Env:                     envObj,
				IsPublic:                types.BoolValue(template["isPublic"].(bool)),
				IsServerless:            types.BoolValue(template["isServerless"].(bool)),
				Ports:                   portsList,
				Readme:                  types.StringValue(template["readme"].(string)),
				VolumeInGb:              types.Int64Value(int64(template["volumeInGb"].(float64))),
				VolumeMountPath:         types.StringValue(template["volumeMountPath"].(string)),
				Earned:                  types.Float64Value(template["earned"].(float64)),
				IsRunpod:                types.BoolValue(template["isRunpod"].(bool)),
				RuntimeInMin:            types.Int64Value(int64(template["runtimeInMin"].(float64))),
			}
			diags = resp.State.Set(ctx, &model)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
		} else {
			resp.Diagnostics.AddError("API Error", "Template not found in response")
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to get data from response")
	}
}

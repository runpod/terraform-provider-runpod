package datasource_pod

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewPodDataSource() datasource.DataSource {
	return &PodDataSource{}
}

type PodDataSource struct {
	rlClient *client.RunPodClient
}

func (d *PodDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *PodDataSource) getClient() *client.RunPodClient {
	if d.rlClient != nil {
		return d.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	endpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if endpoint == "" {
		endpoint = "https://api.runpod.io/graphql"
	}
	d.rlClient = client.NewRunPodClient(apiKey, endpoint, "")
	return d.rlClient
}

func (d *PodDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_pod"
}

func (d *PodDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = PodDataSourceSchema(ctx)
}

func (d *PodDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PodModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	query := `
		query GetPod($podId: String!) {
			pod(input: { podId: $podId }) {
				id
				name
				desiredStatus
				imageName
				machineId
				machineType
				gpuCount
				costPerHr
				memoryInGb
				volumeInGb
				volumeMountPath
				volumeKey
				ports
				lastStatusChange
				dockerArgs
				env
				templateId
				containerDiskInGb
				machine { gpuType { id } }
			}
		}
	`

variables := map[string]interface{}{
		"podId": config.Id.ValueString(),
	}

	if d.rlClient == nil {
		d.rlClient = d.getClient()
	}
	result, err := d.rlClient.Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if pod, ok := result["pod"].(map[string]interface{}); ok {
		envListValue := types.ListNull(types.StringType)
		if envArr, ok := pod["env"].([]interface{}); ok && len(envArr) > 0 {
			elements := make([]attr.Value, 0, len(envArr))
			for _, e := range envArr {
				if s, ok := e.(string); ok {
					elements = append(elements, types.StringValue(s))
				}
			}
			if len(elements) > 0 {
				if lv, d := types.ListValue(types.StringType, elements); !d.HasError() {
					envListValue = lv
				} else {
					resp.Diagnostics.Append(d...)
					return
				}
			}
		}
		// Only 'name' is guaranteed on a pod; all other fields may be null
		// (e.g. no machine while transitioning, no volume attached, no template).
		strOr := func(key string) string {
			if v, ok := pod[key].(string); ok {
				return v
			}
			return ""
		}
		f64Or := func(key string) float64 {
			if v, ok := pod[key].(float64); ok {
				return v
			}
			return 0
		}

		if strOr("name") == "" {
			resp.Diagnostics.AddError("API Error", "Field 'name' is missing or not a string in pod response")
			return
		}

		// v2 pods report GPU type on the owning machine; null while pod is transitioning
		gpuTypeId := ""
		if m, ok := pod["machine"].(map[string]interface{}); ok {
			if gt, ok := m["gpuType"].(map[string]interface{}); ok {
				if v, ok := gt["id"].(string); ok {
					gpuTypeId = v
				}
			}
		}

		name := strOr("name")
		status := strOr("desiredStatus")     // GraphQL has no 'status'; desiredStatus is closest
		desiredStatus := strOr("desiredStatus")
		imageName := strOr("imageName")
		machineId := strOr("machineId")
		machineType := strOr("machineType")
		ports := strOr("ports")
		createdAt := strOr("lastStatusChange") // GraphQL has no created_at; nearest lifecycle stamp
		dockerArgs := strOr("dockerArgs")
		templateId := strOr("templateId")
		volumeMountPath := strOr("volumeMountPath")
		gpuCount := f64Or("gpuCount")
		costPerHr := f64Or("costPerHr")
		memoryInGb := f64Or("memoryInGb")
		volumeInGb := f64Or("volumeInGb")
		containerDiskInGb := f64Or("containerDiskInGb")


		model := PodModel{
			Id:                config.Id,
			Name:              types.StringValue(name),
			Status:            types.StringValue(status),
			DesiredStatus:     types.StringValue(desiredStatus),
			ImageName:         types.StringValue(imageName),
			MachineId:         types.StringValue(machineId),
			MachineType:       types.StringValue(machineType),
			GpuTypeId:         types.StringValue(gpuTypeId),
			GpuCount:          types.Int64Value(int64(gpuCount)),
			CostPerHr:         types.Float64Value(costPerHr),
			MemoryInGb:        types.Float64Value(memoryInGb),
			VolumeInGb:        types.Float64Value(volumeInGb),
			VolumeMountPath:   types.StringValue(volumeMountPath),
			Ports:             types.StringValue(ports),
			CreatedAt:         types.StringValue(createdAt),
			DockerArgs:        types.StringValue(dockerArgs),
			Env:               envListValue,
			TemplateId:        types.StringValue(templateId),
			ContainerDiskInGb: types.Int64Value(int64(containerDiskInGb)),
		}
		diags = resp.State.Set(ctx, &model)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Pod not found in response")
	}
}

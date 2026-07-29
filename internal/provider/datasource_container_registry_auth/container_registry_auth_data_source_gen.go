// Schema/model definitions. Hand-maintained: terraform-provider-spec.json documents this
// logical schema, but the installed generator emits custom types for list_nested
// attributes, so this file is not byte-regenerable. Keep the spec in sync manually.

package datasource_container_registry_auth

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func ContainerRegistryAuthDataSourceSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"container_registry_auths": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "List of container registry auths",
				MarkdownDescription: "List of container registry auths",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							Description:         "Container registry auth ID",
							MarkdownDescription: "Container registry auth ID",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							Description:         "Container registry auth name",
							MarkdownDescription: "Container registry auth name",
						},
						"username": schema.StringAttribute{
							Computed:            true,
							Description:         "Container registry username",
							MarkdownDescription: "Container registry username",
						},
						"password": schema.StringAttribute{
							Computed:            true,
							Sensitive:           true,
							Description:         "Container registry password",
							MarkdownDescription: "Container registry password",
						},
						"created_at": schema.StringAttribute{
							Computed:            true,
							Description:         "Container registry auth creation timestamp",
							MarkdownDescription: "Container registry auth creation timestamp",
						},
						"updated_at": schema.StringAttribute{
							Computed:            true,
							Description:         "Container registry auth last update timestamp",
							MarkdownDescription: "Container registry auth last update timestamp",
						},
					},
				},
			},
		},
	}
}

type ContainerRegistryAuthModel struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Username   types.String `tfsdk:"username"`
	Password   types.String `tfsdk:"password"`
	CreatedAt  types.String `tfsdk:"created_at"`
	UpdatedAt  types.String `tfsdk:"updated_at"`
}

type ContainerRegistryAuthDataSourceModel struct {
	ContainerRegistryAuths []ContainerRegistryAuthModel `tfsdk:"container_registry_auths"`
}

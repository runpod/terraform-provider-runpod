// Schema/model definitions. Hand-maintained: terraform-provider-spec.json documents this
// logical schema, but the installed generator emits custom types for list_nested
// attributes, so this file is not byte-regenerable. Keep the spec in sync manually.

package datasource_ecr_delegations

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func EcrDelegationsDataSourceSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"ecr_delegations": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "List of ECR delegations",
				MarkdownDescription: "List of ECR delegations",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							Description:         "ECR delegation identifier",
							MarkdownDescription: "ECR delegation identifier",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							Description:         "Optional name for the delegation",
							MarkdownDescription: "Optional name for the delegation",
						},
						"resource": schema.StringAttribute{
							Computed:            true,
							Description:         "ECR resource ARN (e.g., arn:aws:ecr:us-east-2:123456789:repository/myapp)",
							MarkdownDescription: "ECR resource ARN (e.g., `arn:aws:ecr:us-east-2:123456789:repository/myapp`)",
						},
						"aws_user": schema.StringAttribute{
							Computed:            true,
							Description:         "AWS user/role being delegated",
							MarkdownDescription: "AWS user/role being delegated",
						},
						"repository": schema.StringAttribute{
							Computed:            true,
							Description:         "ECR repository name",
							MarkdownDescription: "ECR repository name",
						},
						"tag": schema.StringAttribute{
							Computed:            true,
							Description:         "ECR image tag",
							MarkdownDescription: "ECR image tag",
						},
						"aws_region": schema.StringAttribute{
							Computed:            true,
							Description:         "AWS region",
							MarkdownDescription: "AWS region",
						},
						"docker_registry_uri": schema.StringAttribute{
							Computed:            true,
							Description:         "Formatted ECR registry URI for Docker login",
							MarkdownDescription: "Formatted ECR registry URI for Docker login",
						},
						"created_at": schema.StringAttribute{
							Computed:            true,
							Description:         "When the delegation was created",
							MarkdownDescription: "When the delegation was created",
						},
					},
				},
			},
		},
	}
}

type EcrDelegationModel struct {
	Id                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	Resource            types.String `tfsdk:"resource"`
	AwsUser             types.String `tfsdk:"aws_user"`
	Repository          types.String `tfsdk:"repository"`
	Tag                 types.String `tfsdk:"tag"`
	AwsRegion           types.String `tfsdk:"aws_region"`
	DockerRegistryUri   types.String `tfsdk:"docker_registry_uri"`
	CreatedAt           types.String `tfsdk:"created_at"`
}

type EcrDelegationsDataSourceModel struct {
	EcrDelegations []EcrDelegationModel `tfsdk:"ecr_delegations"`
}

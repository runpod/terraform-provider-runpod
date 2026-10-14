package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/pods"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

var (
	_ resource.Resource              = (*podActionResource)(nil)
	_ resource.ResourceWithConfigure = (*podActionResource)(nil)
)

func NewPodActionResource() resource.Resource { return &podActionResource{} }

type podActionResource struct {
	client *runpod.Client
}

type podActionResourceModel struct {
	Id       types.String `tfsdk:"id"`
	PodId    types.String `tfsdk:"pod_id"`
	Action   types.String `tfsdk:"action"`
	Triggers types.Map    `tfsdk:"triggers"`
	Status   types.String `tfsdk:"status"`
}

func (r *podActionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pod_action"
}

func (r *podActionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Runs one state transition on an existing pod. This resource is imperative, not " +
			"declarative: it holds no pod state and reconciles nothing. The action runs when the " +
			"resource is created, and again whenever action or triggers change. A refresh calls " +
			"nothing, so a pod that changes state on its own produces no diff, and destroying the " +
			"resource only forgets it: the pod is left as it is.\n\n" +
			"Do not use it on a pod whose runpod_pod resource sets desired_state: that attribute " +
			"manages the same transitions declaratively and would fight this one, each apply undoing " +
			"the other. Pick desired_state for a run state Terraform should keep, and this resource " +
			"for a one-off restart or a stop tied to something else changing.\n\n" +
			"There is nothing to import. Declaring the resource again, for example after it was removed " +
			"from state, runs the action again.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The pod this resource acts on; there is one action runner per pod.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"pod_id": schema.StringAttribute{
				Required: true,
				Description: "Pod to act on. Pointing the resource at a different pod replaces it, " +
					"which runs the action on the new pod and leaves the old one untouched.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"action": schema.StringAttribute{
				Required: true,
				Description: "Transition to run: start boots a stopped pod, stop releases its compute " +
					"but keeps the disk, restart restarts the container in place, terminate deletes the " +
					"pod permanently. Which ones are valid depends on the pod's current status, and the API " +
					"rejects the rest with a conflict. Changing this re-runs the action.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(sdktypes.PodActionStart),
						string(sdktypes.PodActionStop),
						string(sdktypes.PodActionRestart),
						string(sdktypes.PodActionTerminate),
					),
				},
			},
			"status": schema.StringAttribute{
				Computed: true,
				Description: "Pod status the action returned, as the 1.x resource reported it: PROVISIONING, " +
					"STARTING, RUNNING, EXITED, ERROR, or TERMINATED. It is the status at the moment the " +
					"action was accepted, not a live value: a transition the API takes asynchronously " +
					"still reads as the status before it lands, and a refresh does not update it. " +
					"terminate answers with no body, so it leaves this null.",
			},
			"triggers": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Arbitrary values that re-run the action when any of them changes. This is the " +
					"usual way to restart a pod after an image tag, template, or config value moves. " +
					"Adding, removing, or changing an entry runs the action again.",
			},
		},
	}
}

func (r *podActionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (r *podActionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan podActionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.runAction(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read keeps state as it is. The resource records an action that already ran,
// not a pod's current state. There is nothing to refresh, and calling the API
// here would only let a pod's own state changes churn the plan.
func (r *podActionResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

// Update re-runs the action: the plan only reaches here when action or triggers
// changed, since pod_id carries RequiresReplace.
func (r *podActionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan podActionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.runAction(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete calls nothing: an action that already ran cannot be undone, and
// reversing it would be a guess about what the pod should be doing instead.
func (r *podActionResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// expandPodAction builds the one-field action body. The enum is gated by the
// schema validator, so the value passes through as configured.
func expandPodAction(plan podActionResourceModel) pods.PodActionJSONRequestBody {
	return pods.PodActionJSONRequestBody{Action: sdktypes.PodAction(plan.Action.ValueString())}
}

// flattenPodActionStatus reads the status off the pod the action returned.
// The REST v2 action answers 200 with the updated pod, so no follow-up read is
// needed; terminate answers 204 with no body, and its status is null.
func flattenPodActionStatus(pod *sdktypes.Pod) types.String {
	if pod == nil {
		return types.StringNull()
	}
	return types.StringValue(string(pod.Status))
}

func (r *podActionResource) runAction(ctx context.Context, plan *podActionResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	podId := plan.PodId.ValueString()
	apiResp, err := r.client.Pods.PodActionWithResponse(ctx, podId, expandPodAction(*plan))
	if err != nil {
		diags.AddError("Pod action failed", err.Error())
		return diags
	}
	// start, stop, and restart answer 200 with the updated pod; terminate
	// answers 204 with no body.
	switch apiResp.HTTPResponse.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		plan.Id = types.StringValue(podId)
		plan.Status = flattenPodActionStatus(apiResp.JSON200)
	default:
		diags.AddError(
			fmt.Sprintf("Pod action %q failed", plan.Action.ValueString()),
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
	}
	return diags
}

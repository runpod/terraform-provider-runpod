package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/account"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

var (
	_ resource.Resource                = (*accountSshKeysResource)(nil)
	_ resource.ResourceWithConfigure   = (*accountSshKeysResource)(nil)
	_ resource.ResourceWithImportState = (*accountSshKeysResource)(nil)
)

// accountSshKeysId is the fixed id of the singleton: the account's SSH keys are
// one document behind GET/PUT /v2/account/ssh-keys, not a collection, so there
// is nothing per-key to address.
const accountSshKeysId = "account"

func NewAccountSshKeysResource() resource.Resource { return &accountSshKeysResource{} }

type accountSshKeysResource struct {
	client *runpod.Client
}

type accountSshKeysResourceModel struct {
	Id              types.String `tfsdk:"id"`
	Keys            types.Set    `tfsdk:"keys"`
	RemoveOnDestroy types.Bool   `tfsdk:"remove_on_destroy"`
}

func (r *accountSshKeysResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_ssh_keys"
}

func (r *accountSshKeysResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The account's registered SSH public keys. The API stores them as one document " +
			"that every write replaces in full, so this resource is a singleton: declare it at most " +
			"once per account, and expect its keys attribute to own every key on the account: a key " +
			"added in the console is removed on the next apply. The keys reach a pod only if the pod " +
			"was created with SSH enabled (runpod_pod does so through its ports and the account keys; " +
			"runpod_cluster through start_ssh).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				Description: "Always \"account\": the keys are one account-wide document, and this is " +
					"the id to import it by.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"keys": schema.SetAttribute{
				ElementType: types.StringType,
				Required:    true,
				Description: "The complete set of SSH public keys to register, one authorized_keys-style " +
					"line each (\"<type> <base64-key> [comment]\", e.g. the contents of ~/.ssh/id_ed25519.pub). " +
					"A set, not a list: the API returns the keys in no guaranteed order. An empty set " +
					"removes every key from the account.",
			},
			"remove_on_destroy": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				Description: "Whether destroying this resource also clears the account's keys. Default " +
					"false: a destroy then leaves the keys registered and only forgets them in state, so " +
					"removing the resource cannot lock you out of running pods. Set true to have destroy " +
					"replace the document with an empty set.",
			},
		},
	}
}

func (r *accountSshKeysResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

// Create and Update are the same PUT: the endpoint replaces the whole
// document, so there is no create-versus-modify distinction to make.
func (r *accountSshKeysResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan accountSshKeysResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.putKeys(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accountSshKeysResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accountSshKeysResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Account.GetSshKeysWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading account SSH keys failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading account SSH keys failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenAccountSshKeys(ctx, apiResp.JSON200, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *accountSshKeysResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan accountSshKeysResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.putKeys(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete clears the account's keys only when asked to. The default leaves them
// in place: the keys are how you reach every running pod, and a document this
// resource happens to manage is not one a terraform destroy should empty by
// surprise.
func (r *accountSshKeysResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state accountSshKeysResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.RemoveOnDestroy.ValueBool() {
		return
	}

	apiResp, err := r.client.Account.UpdateSshKeysWithResponse(ctx,
		account.UpdateSshKeysJSONRequestBody{Keys: []string{}})
	if err != nil {
		resp.Diagnostics.AddError("Removing account SSH keys failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Removing account SSH keys failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
	}
}

// ImportState takes the literal id "account" and fills the attributes the
// following read cannot: the id itself and remove_on_destroy, which has no
// API-side representation.
func (r *accountSshKeysResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID != accountSshKeysId {
		resp.Diagnostics.AddError(
			"Unexpected import identifier",
			fmt.Sprintf("The account SSH keys are a singleton; import them as %q, got %q.", accountSshKeysId, req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), accountSshKeysId)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("remove_on_destroy"), false)...)
}

func (r *accountSshKeysResource) putKeys(ctx context.Context, plan *accountSshKeysResourceModel) diag.Diagnostics {
	body, diags := expandAccountSshKeys(ctx, *plan)
	if diags.HasError() {
		return diags
	}

	apiResp, err := r.client.Account.UpdateSshKeysWithResponse(ctx, body)
	if err != nil {
		diags.AddError("Writing account SSH keys failed", err.Error())
		return diags
	}
	if apiResp.JSON200 == nil {
		diags.AddError(
			"Writing account SSH keys failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return diags
	}

	diags.Append(flattenAccountSshKeys(ctx, apiResp.JSON200, plan)...)
	return diags
}

func expandAccountSshKeys(ctx context.Context, plan accountSshKeysResourceModel) (account.UpdateSshKeysJSONRequestBody, diag.Diagnostics) {
	var diags diag.Diagnostics
	// An empty set is a meaningful request (remove every key), so the body
	// always carries a non-nil slice rather than omitting the field.
	keys := []string{}
	if !plan.Keys.IsNull() && !plan.Keys.IsUnknown() {
		diags.Append(plan.Keys.ElementsAs(ctx, &keys, false)...)
	}
	return account.UpdateSshKeysJSONRequestBody{Keys: keys}, diags
}

func flattenAccountSshKeys(ctx context.Context, sshKeys *sdktypes.SshKeys, model *accountSshKeysResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.Id = types.StringValue(accountSshKeysId)
	// An account with no keys must flatten to an empty set, not a null one:
	// keys is Required, and a null would not match the configured empty set.
	registered := sshKeys.Keys
	if registered == nil {
		registered = []string{}
	}
	keys, keyDiags := types.SetValueFrom(ctx, types.StringType, registered)
	diags.Append(keyDiags...)
	model.Keys = keys
	// remove_on_destroy is provider-side only; the default fills it on create
	// and the import path sets it explicitly, so a read never has to guess.
	if model.RemoveOnDestroy.IsNull() || model.RemoveOnDestroy.IsUnknown() {
		model.RemoveOnDestroy = types.BoolValue(false)
	}

	return diags
}

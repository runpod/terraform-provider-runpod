package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

const testSshKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILXGDN/SclOozk1xsDztpmhGiKkkrfQB9SKoO8dSIQQZ me@example.com"

func TestExpandAccountSshKeys(t *testing.T) {
	ctx := context.Background()
	keys, diags := types.SetValueFrom(ctx, types.StringType, []string{testSshKey})
	if diags.HasError() {
		t.Fatal(diags)
	}

	body, diags := expandAccountSshKeys(ctx, accountSshKeysResourceModel{Keys: keys})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if len(body.Keys) != 1 || body.Keys[0] != testSshKey {
		t.Fatalf("keys: %v", body.Keys)
	}
}

func TestExpandAccountSshKeysEmptySetClearsKeys(t *testing.T) {
	ctx := context.Background()
	keys, diags := types.SetValueFrom(ctx, types.StringType, []string{})
	if diags.HasError() {
		t.Fatal(diags)
	}

	// The endpoint replaces the whole document, so an empty set is the request
	// that removes every key: the body must carry keys: [], never omit it.
	body, diags := expandAccountSshKeys(ctx, accountSshKeysResourceModel{Keys: keys})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if body.Keys == nil {
		t.Fatal("an empty set must send an empty array, not omit the field")
	}
	if len(body.Keys) != 0 {
		t.Fatalf("keys: %v", body.Keys)
	}
}

func TestFlattenAccountSshKeys(t *testing.T) {
	ctx := context.Background()
	var model accountSshKeysResourceModel
	if diags := flattenAccountSshKeys(ctx, &sdktypes.SshKeys{Keys: []string{testSshKey}}, &model); diags.HasError() {
		t.Fatal(diags)
	}

	if model.Id.ValueString() != accountSshKeysId {
		t.Fatalf("id = %q, want %q", model.Id.ValueString(), accountSshKeysId)
	}
	var keys []string
	if diags := model.Keys.ElementsAs(ctx, &keys, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(keys) != 1 || keys[0] != testSshKey {
		t.Fatalf("keys: %v", keys)
	}
	if model.RemoveOnDestroy.IsNull() || model.RemoveOnDestroy.ValueBool() {
		t.Fatalf("remove_on_destroy must default to false, got %+v", model.RemoveOnDestroy)
	}
}

func TestFlattenAccountSshKeysKeepsRemoveOnDestroy(t *testing.T) {
	ctx := context.Background()
	// remove_on_destroy is provider-side only: a read must not reset the
	// configured value, or the destroy behaviour would silently change.
	model := accountSshKeysResourceModel{RemoveOnDestroy: types.BoolValue(true)}
	if diags := flattenAccountSshKeys(ctx, &sdktypes.SshKeys{Keys: []string{testSshKey}}, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.RemoveOnDestroy.ValueBool() {
		t.Fatal("remove_on_destroy was reset by a read")
	}
}

func TestFlattenAccountSshKeysEmptyAccount(t *testing.T) {
	ctx := context.Background()
	var model accountSshKeysResourceModel
	// An account with no keys must produce an empty set: keys is Required, so a
	// null would not match a configured empty set.
	if diags := flattenAccountSshKeys(ctx, &sdktypes.SshKeys{}, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Keys.IsNull() {
		t.Fatal("no registered keys must flatten to an empty set, not null")
	}
	if len(model.Keys.Elements()) != 0 {
		t.Fatalf("keys: %+v", model.Keys)
	}
}

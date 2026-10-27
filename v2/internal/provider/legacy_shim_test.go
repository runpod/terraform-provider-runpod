package provider

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAppendLegacyReadFailureWarnsWithTheCause(t *testing.T) {
	var diags diag.Diagnostics
	appendLegacyReadFailure(&diags, "pod", errors.New("graphql is down"))
	if diags.HasError() {
		t.Fatalf("a shim read failure must not break the apply: %v", diags)
	}
	if diags.WarningsCount() != 1 {
		t.Fatalf("want one warning, got %v", diags)
	}
	if !strings.Contains(diags.Warnings()[0].Detail(), "graphql is down") {
		t.Fatalf("the warning must carry the cause: %q", diags.Warnings()[0].Detail())
	}
}

func TestAppendLegacyWriteFailureErrors(t *testing.T) {
	var diags diag.Diagnostics
	appendLegacyWriteFailure(&diags, "template", errors.New("saveTemplate rejected the input"))
	if !diags.HasError() {
		t.Fatalf("a configured shim input that cannot be applied must be an error: %v", diags)
	}
	if !strings.Contains(diags.Errors()[0].Detail(), "saveTemplate rejected the input") {
		t.Fatalf("the error must carry the cause: %q", diags.Errors()[0].Detail())
	}
}

func TestAppendLegacyWriteFailureNamesTheWayOutForAnInvisibleTemplate(t *testing.T) {
	var diags diag.Diagnostics
	appendLegacyWriteFailure(&diags, "template", fmt.Errorf("saving the readme: %w", errLegacyTemplateNotVisible))
	if !diags.HasError() {
		t.Fatalf("want an error, got %v", diags)
	}
	if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "Drop the deprecated attribute") {
		t.Fatalf("the error must say what to do: %q", detail)
	}
}

func TestWarnUnappliedLegacyInputWarnsOnlyWhenConfigured(t *testing.T) {
	attribute := path.Root("gpu_type_priority")

	var unset diag.Diagnostics
	warnUnappliedLegacyInput(&unset, attribute, types.StringNull(), "no API can set it")
	if len(unset) != 0 {
		t.Fatalf("an unset attribute must not warn: %v", unset)
	}

	for name, value := range map[string]types.String{
		"set":     types.StringValue("availability"),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			warnUnappliedLegacyInput(&diags, attribute, value, "no API can set it")
			if diags.HasError() || diags.WarningsCount() != 1 {
				t.Fatalf("want one warning, got %v", diags)
			}
			warning := diags.Warnings()[0]
			withPath, ok := warning.(diag.DiagnosticWithPath)
			if !ok || !withPath.Path().Equal(attribute) {
				t.Fatalf("the warning must point at %s: %v", attribute, warning)
			}
			if !strings.Contains(warning.Detail(), "no API can set it") {
				t.Fatalf("the warning must carry the reason: %q", warning.Detail())
			}
		})
	}
}

func TestLegacyTruncatedInt64AndNonEmptyString(t *testing.T) {
	fractional := 90.7
	if got := legacyTruncatedInt64(&fractional); got.ValueInt64() != 90 {
		t.Errorf("truncated = %v, want 90", got)
	}
	if got := legacyTruncatedInt64(nil); !got.IsNull() {
		t.Errorf("absent = %v, want null", got)
	}

	empty, value := "", "m-1"
	if got := legacyNonEmptyString(&empty); !got.IsNull() {
		t.Errorf("empty = %v, want null", got)
	}
	if got := legacyNonEmptyString(&value); got.ValueString() != "m-1" {
		t.Errorf("value = %v, want m-1", got)
	}
}

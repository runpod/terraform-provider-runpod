package provider

import (
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The deprecated 1.x attributes on runpod_pod, runpod_template and
// runpod_serverless_endpoint. Each worked in the 1.0.8 provider through REST
// v1. Where REST v2 has the field it is read from there; where only GraphQL
// has it, the GraphQL shim in legacy_graphql.go serves it; where neither has
// it, the attribute is always null, or, for an input, kept in state and never
// sent. All of them go away when REST v2 grows the field or the attribute is
// dropped in a later major release.

// legacyShimDeprecation is the DeprecationMessage of an attribute served by
// the GraphQL shim.
const legacyShimDeprecation = "GraphQL shim: REST v2 has no equivalent yet; removed when it does"

// legacyShimDescriptionSuffix ends the Description of an attribute served by
// the GraphQL shim.
const legacyShimDescriptionSuffix = "Served by the deprecated GraphQL shim, because REST v2 does not report it yet. " +
	"Removed once REST v2 reports it."

// legacyNullDeprecation is the DeprecationMessage of a read-back no API
// reports any more.
const legacyNullDeprecation = "Always null: neither REST v2 nor the GraphQL API reports it; remove references to it"

// legacyNullDescriptionSuffix ends the Description of such a read-back.
const legacyNullDescriptionSuffix = "Always null: neither REST v2 nor the GraphQL API reports this value. " +
	"Kept so a 1.x configuration that references it still plans; removed in a later major release."

// legacyIgnoredInputDeprecation is the DeprecationMessage of an input no API
// can apply any more.
const legacyIgnoredInputDeprecation = "Not applied: neither REST v2 nor the GraphQL API can set it; remove it from the configuration"

// legacyIgnoredInputDescriptionSuffix ends the Description of such an input.
const legacyIgnoredInputDescriptionSuffix = "Not applied: neither REST v2 nor the GraphQL API can set this value, so " +
	"a configured value is kept in state as written, never sent, and reported by a warning at plan time. " +
	"Removed in a later major release."

// appendLegacyReadFailure records a GraphQL shim read that did not answer. A
// read must not break an apply over attributes nothing configured: it warns
// and the attributes come back null. An object the GraphQL API does not know
// is not a failure and does not come through here; the caller nulls the
// attributes silently, since warning about it on every refresh would be noise
// the user cannot act on.
func appendLegacyReadFailure(diags *diag.Diagnostics, subject string, err error) {
	diags.AddWarning(
		fmt.Sprintf("Reading the deprecated %s attributes from the GraphQL API failed", subject),
		fmt.Sprintf(
			"%s\n\nThe deprecated attributes served by the GraphQL shim are reported as null. Remove them from the configuration to stop depending on that API.",
			err.Error(),
		),
	)
}

// appendLegacyWriteFailure records a GraphQL shim write that did not land.
// Unlike a read, this is an error: the configuration asked for something only
// that API can apply, and the REST v2 change has already been applied.
func appendLegacyWriteFailure(diags *diag.Diagnostics, subject string, err error) {
	if errors.Is(err, errLegacyTemplateNotVisible) {
		diags.AddError(
			fmt.Sprintf("Applying the deprecated %s settings through the GraphQL API failed", subject),
			fmt.Sprintf(
				"%s\n\nThe deprecated attributes only the GraphQL API can apply cannot be applied to this %s. The REST v2 %s itself was applied. Drop the deprecated attribute from the configuration.",
				err.Error(), subject, subject,
			),
		)
		return
	}
	diags.AddError(
		fmt.Sprintf("Applying the deprecated %s settings through the GraphQL API failed", subject),
		fmt.Sprintf("%s\n\nThese attributes are served by the GraphQL shim; the REST v2 %s itself was applied.", err.Error(), subject),
	)
}

// warnUnappliedLegacyInput is the plan-time warning for a deprecated input no
// API can apply any more. Call it from ValidateConfig with the configured
// value: a null value is not configured and warns nothing. An unknown value
// warns too, because whatever it resolves to will not be sent either.
func warnUnappliedLegacyInput(diags *diag.Diagnostics, attribute path.Path, value attr.Value, reason string) {
	if value.IsNull() {
		return
	}
	diags.AddAttributeWarning(
		attribute,
		"Deprecated attribute is not applied",
		fmt.Sprintf(
			"%s is set, but %s. The value is kept in state as written and never sent to the API. Remove it from the configuration.",
			attribute, reason,
		),
	)
}

// legacyTruncatedInt64 truncates a JSON number the way 1.0.8 did: it read
// these as a float64 and truncated with int64(val), so a fractional value is
// data to round rather than a decode error. An absent field is null rather
// than zero.
func legacyTruncatedInt64(value *float64) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*value))
}

// legacyNonEmptyString carries a string that 1.0.8 wrote only when the
// response reported it non-empty (`ok && val != ""`), leaving the attribute
// null otherwise, so a 1.x configuration's `!= null` check keeps answering
// what it did.
func legacyNonEmptyString(value *string) types.String {
	if value == nil || *value == "" {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

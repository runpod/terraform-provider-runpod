package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// clients is what provider.Configure hands to every resource and data source:
// the v2 REST client everything is built on, plus the legacy GraphQL client
// the deprecated shims need (runpod_machine, runpod_user, and the attributes
// REST v2 does not report).
type clients struct {
	API    *runpod.Client
	Legacy *legacy.Client
}

// maxDiagnosticBody caps how much of an API response body a diagnostic quotes,
// matching the bound the SDK's legacy package puts on its own error bodies:
// an HTML error page or a large JSON payload would otherwise flood the CLI.
const maxDiagnosticBody = 4096

// boundedBody renders a response body for a diagnostic, truncated to
// maxDiagnosticBody.
func boundedBody(body []byte) string {
	if len(body) <= maxDiagnosticBody {
		return string(body)
	}
	return string(body[:maxDiagnosticBody]) + "… (truncated)"
}

// listAllPages walks a cursor-paginated REST v2 list route to exhaustion and
// returns every item. fetchPage reads one page and returns its items together
// with the route's pagination block; cursor is nil for the first page and the
// previous page's nextCursor thereafter.
//
// Reading only the first page would report a truncated list as the complete
// set: every v2 list route caps a page at a server-side default, 1000 items
// for most and 100 for endpoint builds, and a data source that promises all of
// something has to ask for the rest.
//
// A route that reports hasNextPage without advancing the cursor would loop
// forever, so a repeated cursor is an error rather than a stopping condition:
// the result is incomplete either way, and a diagnostic says so instead of
// hanging the plan.
func listAllPages[T any](ctx context.Context, fetchPage func(context.Context, *string) ([]T, sdktypes.Pagination, error)) ([]T, error) {
	items := []T{}
	var cursor *string
	for {
		page, pagination, err := fetchPage(ctx, cursor)
		if err != nil {
			return nil, err
		}
		items = append(items, page...)
		if !pagination.HasNextPage || pagination.NextCursor == nil {
			return items, nil
		}
		if cursor != nil && *pagination.NextCursor == *cursor {
			return nil, fmt.Errorf("the API repeated pagination cursor %q instead of advancing; stopped after %d items",
				*cursor, len(items))
		}
		cursor = pagination.NextCursor
	}
}

// configuredClient extracts the shared API client handed over by
// provider.Configure. Returns nil with no diagnostic when provider data is
// absent: Terraform calls Configure once with nil provider data during
// provider validation, before the provider itself is configured, and issues no
// read or CRUD call until provider Configure has succeeded, so a nil client
// here is never reached by a request.
func configuredClient(providerData any, diags *diag.Diagnostics) *runpod.Client {
	if providerData == nil {
		return nil
	}
	configured, ok := providerData.(*clients)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *clients, got %T", providerData))
		return nil
	}
	return configured.API
}

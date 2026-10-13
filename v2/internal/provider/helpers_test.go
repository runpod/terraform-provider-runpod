package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

func TestListAllPagesWalksEveryPage(t *testing.T) {
	ctx := context.Background()
	pages := []struct {
		items      []string
		pagination sdktypes.Pagination
	}{
		{items: []string{"a", "b"}, pagination: sdktypes.Pagination{HasNextPage: true, NextCursor: pointerTo("cur_2")}},
		{items: []string{"c"}, pagination: sdktypes.Pagination{HasNextPage: true, NextCursor: pointerTo("cur_3")}},
		{items: []string{"d"}, pagination: sdktypes.Pagination{HasNextPage: false}},
	}

	var cursors []*string
	items, err := listAllPages(ctx, func(_ context.Context, cursor *string) ([]string, sdktypes.Pagination, error) {
		page := pages[len(cursors)]
		cursors = append(cursors, cursor)
		return page.items, page.pagination, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(items, "") != "abcd" {
		t.Errorf("items = %q, want every page in order", items)
	}
	if len(cursors) != 3 || cursors[0] != nil ||
		*cursors[1] != "cur_2" || *cursors[2] != "cur_3" {
		t.Errorf("cursors = %v, want nil then each nextCursor verbatim", cursors)
	}
}

func TestListAllPagesStopsOnACursorThatDoesNotAdvance(t *testing.T) {
	// A route that keeps reporting hasNextPage without moving the cursor would
	// otherwise spin forever inside a plan.
	calls := 0
	_, err := listAllPages(context.Background(), func(_ context.Context, _ *string) ([]string, sdktypes.Pagination, error) {
		calls++
		return []string{"a"}, sdktypes.Pagination{HasNextPage: true, NextCursor: pointerTo("stuck")}, nil
	})
	if err == nil {
		t.Fatal("a cursor that never advances must be an error, not a stopping condition")
	}
	if !strings.Contains(err.Error(), "stuck") {
		t.Errorf("error = %q, want the offending cursor quoted", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want the walk to stop on the repeat", calls)
	}
}

func TestListAllPagesReportsAPageError(t *testing.T) {
	pageError := errors.New("API returned status 500")
	items, err := listAllPages(context.Background(), func(_ context.Context, _ *string) ([]string, sdktypes.Pagination, error) {
		return nil, sdktypes.Pagination{}, pageError
	})
	if !errors.Is(err, pageError) {
		t.Errorf("err = %v, want the page error through unchanged", err)
	}
	if items != nil {
		t.Errorf("items = %v, want none when a page failed", items)
	}
}

func pointerTo[T any](value T) *T { return &value }

func TestBoundedBodyCapsWhatADiagnosticQuotes(t *testing.T) {
	// An HTML error page or a large JSON payload would otherwise go into the
	// diagnostic whole and flood the CLI; the SDK's legacy package caps its own
	// error bodies at the same size.
	short := []byte(`{"error":"not found"}`)
	if got := boundedBody(short); got != string(short) {
		t.Errorf("boundedBody(%q) = %q", short, got)
	}

	long := []byte(strings.Repeat("a", maxDiagnosticBody+100))
	got := boundedBody(long)
	if len(got) <= maxDiagnosticBody || !strings.HasSuffix(got, "(truncated)") {
		t.Errorf("a long body must be cut at %d bytes and say so, got %d bytes", maxDiagnosticBody, len(got))
	}
	if !strings.HasPrefix(got, strings.Repeat("a", maxDiagnosticBody)) {
		t.Error("the quoted prefix must be the start of the body")
	}
}

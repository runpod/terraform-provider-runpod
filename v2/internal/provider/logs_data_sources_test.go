package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/pods"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// collectFromString reads a complete, non-blocking body: the idle stop is off
// because a strings.Reader is never quiet, it just ends.
func collectFromString(t *testing.T, body string, limit int) logStreamRead {
	t.Helper()
	read, err := collectLogEvents(context.Background(), strings.NewReader(body), limit, 0)
	if err != nil {
		t.Fatal(err)
	}
	return read
}

func TestCollectLogEvents(t *testing.T) {
	read := collectFromString(t, ": keep-alive\n\n"+
		"id: 2026-06-01T12:02:03Z/000000000001\n"+
		"data: {\"ts\":\"2026-06-01T12:02:03Z\",\"source\":\"container\",\"line\":\"Model loaded.\"}\n\n"+
		"id: 2026-06-01T12:02:04Z/000000000002\n"+
		"data: {\"ts\":\"2026-06-01T12:02:04Z\",\"source\":\"system\",\"line\":\"Pod started.\"}\n\n", 0)

	if len(read.Entries) != 2 {
		t.Fatalf("entries = %d, want 2: %+v", len(read.Entries), read.Entries)
	}
	if read.Entries[0].Timestamp.ValueString() != "2026-06-01T12:02:03Z" ||
		read.Entries[0].Source.ValueString() != "container" ||
		read.Entries[0].Line.ValueString() != "Model loaded." {
		t.Fatalf("first entry: %+v", read.Entries[0])
	}
	if read.Entries[1].Source.ValueString() != "system" || read.Entries[1].Line.ValueString() != "Pod started." {
		t.Fatalf("second entry: %+v", read.Entries[1])
	}
	if read.LastId != "2026-06-01T12:02:04Z/000000000002" {
		t.Fatalf("last id = %q", read.LastId)
	}
	if read.HitDeadline {
		t.Fatal("a stream that ended on its own must not report a deadline")
	}
}

func TestCollectLogEventsStopsAtLimit(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&body, "id: %d\ndata: {\"line\":\"line %d\"}\n\n", i, i)
	}

	read := collectFromString(t, body.String(), 2)
	if len(read.Entries) != 2 {
		t.Fatalf("entries = %d, want the read to stop at the limit of 2", len(read.Entries))
	}
	if read.LastId != "1" {
		t.Fatalf("last id = %q, want the cursor of the second event", read.LastId)
	}
}

func TestCollectLogEventsMultiLinePayloadAndCrlf(t *testing.T) {
	// A JSON payload split over several data: lines is rejoined with newlines
	// before decoding, per the SSE format.
	read := collectFromString(t, "id: 1\r\n"+
		"data: {\"ts\":\"2026-06-01T12:00:00Z\",\r\n"+
		"data:  \"source\":\"container\",\"line\":\"split payload\"}\r\n\r\n", 0)

	if len(read.Entries) != 1 {
		t.Fatalf("entries = %d, want 1: %+v", len(read.Entries), read.Entries)
	}
	if read.Entries[0].Line.ValueString() != "split payload" {
		t.Fatalf("entry: %+v", read.Entries[0])
	}
}

func TestCollectLogEventsUndecodablePayloadKeepsRawText(t *testing.T) {
	read := collectFromString(t, "data: not json at all\n\n", 0)
	if len(read.Entries) != 1 {
		t.Fatalf("entries = %d, want 1: %+v", len(read.Entries), read.Entries)
	}
	if read.Entries[0].Line.ValueString() != "not json at all" {
		t.Fatalf("line = %q", read.Entries[0].Line.ValueString())
	}
	if !read.Entries[0].Timestamp.IsNull() || !read.Entries[0].Source.IsNull() {
		t.Fatalf("timestamp/source must be null for an undecodable payload: %+v", read.Entries[0])
	}
}

func TestCollectLogEventsTrailingEventWithoutBlankLine(t *testing.T) {
	read := collectFromString(t, "data: {\"line\":\"last\"}\n", 0)
	if len(read.Entries) != 1 || read.Entries[0].Line.ValueString() != "last" {
		t.Fatalf("entries: %+v", read.Entries)
	}
}

func TestCollectLogEventsEmptyBody(t *testing.T) {
	read := collectFromString(t, "", 0)
	if read.Entries == nil || len(read.Entries) != 0 {
		t.Fatalf("entries = %v, want an empty list", read.Entries)
	}
	if read.LastId != "" {
		t.Fatalf("last id = %q, want empty", read.LastId)
	}
}

// logStreamServer serves an SSE response: it writes the events, flushes each
// one, then either closes the stream or holds the connection open the way the
// live endpoints do. The write errors are discarded rather than reported: a
// reader that stops at its deadline, which is what the timeout tests do, ends
// the stream mid-write, and that is the case under test, not a failure.
func logStreamServer(t *testing.T, status int, events []string, hold bool) *httptest.Server {
	t.Helper()
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("test server response writer cannot flush, so it cannot stream")
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = fmt.Fprint(w, `{"status":404,"title":"Not Found","detail":"pod not found"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for i, event := range events {
			_, _ = fmt.Fprintf(w, "id: cursor-%d\ndata: %s\n\n", i, event)
			flusher.Flush()
		}
		if hold {
			<-release
		}
	}))

	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	return server
}

func logStreamClient(t *testing.T, server *httptest.Server) *runpod.Client {
	t.Helper()
	client, err := runpod.New(runpod.WithAPIKey("test-key"), runpod.WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func fetchPodLogs(t *testing.T, client *runpod.Client, timeout, idle time.Duration, limit int) (logStreamResult, []string, []string) {
	t.Helper()
	params := pods.GetPodLogsParams{}
	result, diags := fetchLogStream(context.Background(), "Reading pod logs", timeout, idle, limit,
		func(streamCtx context.Context) (*http.Response, error) {
			return client.Pods.GetPodLogs(streamCtx, "pod_abc123", &params)
		})

	var errorSummaries, warningSummaries []string
	for _, d := range diags.Errors() {
		errorSummaries = append(errorSummaries, d.Summary()+": "+d.Detail())
	}
	for _, d := range diags.Warnings() {
		warningSummaries = append(warningSummaries, d.Summary())
	}
	return result, errorSummaries, warningSummaries
}

func sampleLogEvents(n int) []string {
	events := make([]string, 0, n)
	for i := 0; i < n; i++ {
		events = append(events, fmt.Sprintf(`{"ts":"2026-06-01T12:00:0%dZ","source":"container","line":"line %d"}`, i, i))
	}
	return events
}

func TestFetchLogStreamServerClosesTheStream(t *testing.T) {
	server := logStreamServer(t, http.StatusOK, sampleLogEvents(3), false)

	result, errs, warnings := fetchPodLogs(t, logStreamClient(t, server), 5*time.Second, 2*time.Second, 0)
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("entries = %d, want 3: %+v", len(result.Entries), result.Entries)
	}
	if result.Entries[2].Line.ValueString() != "line 2" {
		t.Fatalf("last entry: %+v", result.Entries[2])
	}
	if result.LastId.ValueString() != "cursor-2" {
		t.Fatalf("last_id = %v, want cursor-2", result.LastId)
	}
}

func TestFetchLogStreamReturnsAtTailWhileServerStillStreams(t *testing.T) {
	// The server sends the backfill and then holds the connection open, which
	// is what the live endpoints do. The read must not wait for it.
	server := logStreamServer(t, http.StatusOK, sampleLogEvents(2), true)

	started := time.Now()
	result, errs, warnings := fetchPodLogs(t, logStreamClient(t, server), 30*time.Second, 30*time.Second, 2)
	elapsed := time.Since(started)

	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %d, want 2: %+v", len(result.Entries), result.Entries)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("the read took %s against a server that never closed the stream; it must return at the tail", elapsed)
	}
	if result.LastId.ValueString() != "cursor-1" {
		t.Fatalf("last_id = %v, want cursor-1", result.LastId)
	}
}

func TestFetchLogStreamReturnsWhenTheStreamGoesQuiet(t *testing.T) {
	// A pod with less history than the tail cap: three events arrive, then the
	// server goes silent without closing. The idle stop ends the read at the
	// quiet gap, with no waiting out read_timeout_seconds and no warning, because
	// a short backfill is a complete read.
	server := logStreamServer(t, http.StatusOK, sampleLogEvents(3), true)

	started := time.Now()
	result, errs, warnings := fetchPodLogs(t, logStreamClient(t, server), 30*time.Second, 300*time.Millisecond, defaultLogTail)
	elapsed := time.Since(started)

	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(warnings) != 0 {
		t.Fatalf("a short backfill is a complete read, not a warning: %v", warnings)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("entries = %d, want 3: %+v", len(result.Entries), result.Entries)
	}
	if result.LastId.ValueString() != "cursor-2" {
		t.Fatalf("last_id = %v, want cursor-2", result.LastId)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("the read took %s; the idle stop should have ended it at about the 300ms gap", elapsed)
	}
}

func TestFetchLogStreamTimesOutWithoutEventsAndWarns(t *testing.T) {
	// No event ever arrives, so the idle stop never arms and the hard deadline
	// is what ends the read.
	server := logStreamServer(t, http.StatusOK, nil, true)

	started := time.Now()
	result, errs, warnings := fetchPodLogs(t, logStreamClient(t, server), 300*time.Millisecond, 10*time.Second, 0)
	elapsed := time.Since(started)

	if len(errs) != 0 {
		t.Fatalf("a read timeout must not be an error: %v", errs)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "read timeout") {
		t.Fatalf("warnings = %v, want one read-timeout warning", warnings)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("entries = %+v, want none", result.Entries)
	}
	if !result.LastId.IsNull() {
		t.Fatalf("last_id = %v, want null", result.LastId)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("the read took %s, want it bounded by the 300ms timeout", elapsed)
	}
}

func TestFetchLogStreamWarnsWhenDeadlineCutsLiveStream(t *testing.T) {
	// An idle gap longer than the hard bound means the deadline is what stops
	// the read. Events were still expected, so this one does warn, and what
	// arrived is kept.
	server := logStreamServer(t, http.StatusOK, sampleLogEvents(1), true)

	result, errs, warnings := fetchPodLogs(t, logStreamClient(t, server), 300*time.Millisecond, 10*time.Second, 0)
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one", warnings)
	}
	if len(result.Entries) != 1 || result.Entries[0].Line.ValueString() != "line 0" {
		t.Fatalf("entries: %+v", result.Entries)
	}
	if result.LastId.ValueString() != "cursor-0" {
		t.Fatalf("last_id = %v, want cursor-0", result.LastId)
	}
}

func TestFetchLogStreamNotFoundIsAnError(t *testing.T) {
	server := logStreamServer(t, http.StatusNotFound, nil, false)

	result, errs, _ := fetchPodLogs(t, logStreamClient(t, server), 5*time.Second, 2*time.Second, 0)
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want exactly one", errs)
	}
	if !strings.Contains(errs[0], "status 404") || !strings.Contains(errs[0], "pod not found") {
		t.Fatalf("error must carry the status and body: %q", errs[0])
	}
	if len(result.Entries) != 0 {
		t.Fatalf("entries = %+v, want none", result.Entries)
	}
}

func TestLogStreamParamsResolve(t *testing.T) {
	params, diags := logStreamParams{
		Source:      types.StringValue(string(sdktypes.LogSourceContainer)),
		Tail:        types.Int64Value(50),
		Since:       types.StringValue("2026-06-01T12:00:00Z"),
		LastEventID: types.StringValue("2026-06-01T12:30:00Z"),
	}.resolve()
	if diags.HasError() {
		t.Fatal(diags)
	}

	if params.Source == nil || *params.Source != sdktypes.LogSourceContainer {
		t.Fatalf("source = %v", params.Source)
	}
	if params.Tail == nil || *params.Tail != 50 {
		t.Fatalf("tail = %v", params.Tail)
	}
	if params.Since == nil || params.Since.Format(time.RFC3339) != "2026-06-01T12:00:00Z" {
		t.Fatalf("since = %v", params.Since)
	}
	if params.LastEventID == nil || params.LastEventID.Format(time.RFC3339) != "2026-06-01T12:30:00Z" {
		t.Fatalf("last_event_id = %v", params.LastEventID)
	}
}

func TestLogStreamParamsResolveOmitsUnsetParameters(t *testing.T) {
	params, diags := logStreamParams{
		Source:      types.StringNull(),
		Tail:        types.Int64Null(),
		Since:       types.StringNull(),
		LastEventID: types.StringNull(),
	}.resolve()
	if diags.HasError() {
		t.Fatal(diags)
	}
	if params.Source != nil || params.Tail != nil || params.Since != nil || params.LastEventID != nil {
		t.Fatalf("unset parameters must stay out of the request: %+v", params)
	}
}

func TestLogStreamParamsRejectsMalformedTimestamp(t *testing.T) {
	_, diags := logStreamParams{
		Source: types.StringNull(),
		Tail:   types.Int64Null(),
		Since:  types.StringValue("yesterday"),
	}.resolve()
	if !diags.HasError() {
		t.Fatal("a non-RFC-3339 since must produce an error diagnostic")
	}
}

func TestLogStreamEventLimit(t *testing.T) {
	unset := logStreamParams{
		Tail:        types.Int64Null(),
		Since:       types.StringNull(),
		LastEventID: types.StringNull(),
	}
	if got := unset.eventLimit(); got != defaultLogTail {
		t.Fatalf("unset tail limit = %d, want the API default %d", got, defaultLogTail)
	}

	tailed := unset
	tailed.Tail = types.Int64Value(25)
	if got := tailed.eventLimit(); got != 25 {
		t.Fatalf("tail limit = %d, want 25", got)
	}

	live := unset
	live.Tail = types.Int64Value(0)
	if got := live.eventLimit(); got != 0 {
		t.Fatalf("tail 0 limit = %d, want 0 so only the idle gap and the deadline bound the read", got)
	}

	resumedBySince := tailed
	resumedBySince.Since = types.StringValue("2026-06-01T12:00:00Z")
	if got := resumedBySince.eventLimit(); got != 0 {
		t.Fatalf("since limit = %d, want 0: a resumed stream has no known event count", got)
	}

	resumedByCursor := tailed
	resumedByCursor.LastEventID = types.StringValue("2026-06-01T12:00:00Z")
	if got := resumedByCursor.eventLimit(); got != 0 {
		t.Fatalf("last_event_id limit = %d, want 0: a resumed stream has no known event count", got)
	}
}

func TestLogStreamTimeouts(t *testing.T) {
	unset := logStreamParams{IdleTimeoutSeconds: types.Int64Null(), ReadTimeoutSeconds: types.Int64Null()}
	if got := unset.idleTimeout(); got != defaultLogIdleTimeoutSeconds*time.Second {
		t.Fatalf("unset idle_timeout_seconds = %s, want %ds", got, defaultLogIdleTimeoutSeconds)
	}
	if got := unset.readTimeout(); got != defaultLogReadTimeoutSeconds*time.Second {
		t.Fatalf("unset read_timeout_seconds = %s, want %ds", got, defaultLogReadTimeoutSeconds)
	}

	set := logStreamParams{IdleTimeoutSeconds: types.Int64Value(5), ReadTimeoutSeconds: types.Int64Value(3)}
	if got := set.idleTimeout(); got != 5*time.Second {
		t.Fatalf("idle_timeout_seconds = %s, want 5s", got)
	}
	if got := set.readTimeout(); got != 3*time.Second {
		t.Fatalf("read_timeout_seconds = %s, want 3s", got)
	}
}

func TestLogTailValidatorMatchesTheSpecRange(t *testing.T) {
	ctx := context.Background()

	var schemaResp datasource.SchemaResponse
	NewPodLogsDataSource().Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	attribute, ok := schemaResp.Schema.Attributes["tail"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("tail is %T, want schema.Int64Attribute", schemaResp.Schema.Attributes["tail"])
	}

	rejects := func(value int64) bool {
		var resp validator.Int64Response
		for _, attributeValidator := range attribute.Int64Validators() {
			attributeValidator.ValidateInt64(ctx, validator.Int64Request{
				Path:           path.Root("tail"),
				PathExpression: path.MatchRoot("tail"),
				ConfigValue:    types.Int64Value(value),
			}, &resp)
		}
		return resp.Diagnostics.HasError()
	}

	// The spec puts tail at minimum 0, maximum 5000.
	for _, accepted := range []int64{0, 100, 5000} {
		if rejects(accepted) {
			t.Fatalf("tail = %d must be accepted", accepted)
		}
	}
	for _, refused := range []int64{-1, 5001, 10000} {
		if !rejects(refused) {
			t.Fatalf("tail = %d must be rejected at plan time", refused)
		}
	}
}

func TestLogDataSourceModelsFitTheirSchemas(t *testing.T) {
	ctx := context.Background()
	read := collectFromString(t, "id: 2026-06-01T12:02:03Z/000000000001\n"+
		"data: {\"ts\":\"2026-06-01T12:02:03Z\",\"source\":\"container\",\"line\":\"Model loaded.\"}\n\n", 0)
	lines, diags := logLines(ctx, read.Entries)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if got := len(lines.Elements()); got != 1 {
		t.Fatalf("lines = %d, want 1", got)
	}

	assertDataSourceModelFits(t, NewPodLogsDataSource, podLogsDataSourceModel{
		PodId:   types.StringValue("pod_abc123"),
		Entries: read.Entries,
		Lines:   lines,
		LastId:  types.StringValue(read.LastId),
	})
	assertDataSourceModelFits(t, NewEndpointWorkerLogsDataSource, endpointWorkerLogsDataSourceModel{
		EndpointId: types.StringValue("ep_abc123"),
		WorkerId:   types.StringValue("worker_abc"),
		Entries:    read.Entries,
		Lines:      lines,
		LastId:     types.StringValue(read.LastId),
	})
}

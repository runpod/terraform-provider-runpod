package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/pods"
	"github.com/runpod/go-sdk/v2/lib/serverless"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// runpod_pod_logs and runpod_endpoint_worker_logs read the log endpoints, which
// serve Server-Sent Events rather than a JSON document.
//
// Both endpoints keep streaming live lines after the backfill, so a read that
// waited for the server to close would never return. These data sources go
// around the generated read-it-all helper: they call the raw client method,
// decode SSE frames off the open body as they arrive, and stop at the first of:
// the backfill is complete (tail events decoded), the stream went quiet for
// idle_timeout_seconds, the server closed the stream, or read_timeout_seconds
// elapsed. Only the last of those is worth a diagnostic, and only a warning:
// whatever arrived still lands in state.

const (
	// defaultLogTail mirrors the API's own default backfill size, which is
	// what an unset tail asks for and therefore the most events a complete
	// backfill can hold. A pod with less history than that never reaches the
	// count, which is what the idle stop is for.
	defaultLogTail = 100

	// defaultLogIdleTimeoutSeconds ends a read whose backfill is shorter than
	// the cap: once events have started arriving, a gap this long means the
	// history is exhausted and the rest would be live output.
	defaultLogIdleTimeoutSeconds = 2

	// defaultLogReadTimeoutSeconds is the hard bound, for a stream that keeps
	// producing output faster than the idle gap.
	defaultLogReadTimeoutSeconds = 10

	initialLogScanBuffer = 64 * 1024
	maxLogLineBytes      = 1024 * 1024
	maxErrorBodyBytes    = 4 * 1024
)

// logEntryModel is one decoded log event.
type logEntryModel struct {
	Timestamp types.String `tfsdk:"timestamp"`
	Source    types.String `tfsdk:"source"`
	Line      types.String `tfsdk:"line"`
}

// logStreamAttributes are the query parameters and results shared by both log
// data sources; the resource identifiers are declared per data source.
func logStreamAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"source": schema.StringAttribute{
			Optional:    true,
			Description: "Log source to read: \"container\" for the container's stdout and stderr, or \"system\" for the host lifecycle stream. Omit for both.",
			Validators: []validator.String{
				stringvalidator.OneOf(string(sdktypes.LogSourceContainer), string(sdktypes.LogSourceSystem)),
			},
		},
		"tail": schema.Int64Attribute{
			Optional: true,
			Description: "Number of historical lines to backfill, 0 to 5000. The API defaults to 100. The read returns as soon as that many " +
				"events have arrived, or once the stream goes quiet for idle_timeout_seconds if the pod has less history than that. Set 0 to " +
				"skip the backfill and collect only live lines. Ignored by the API when since or last_event_id is set.",
			Validators: []validator.Int64{int64validator.Between(0, 5000)},
		},
		"since": schema.StringAttribute{
			Optional:    true,
			Description: "RFC 3339 timestamp to resume from. When set, tail is ignored and the read is bounded only by read_timeout_seconds.",
		},
		"last_event_id": schema.StringAttribute{
			Optional: true,
			Description: "SSE reconnect cursor: a timestamp this endpoint emitted earlier, in RFC 3339 format, as reported by last_id. Takes " +
				"precedence over since and tail, and the read is bounded only by read_timeout_seconds.",
		},
		"idle_timeout_seconds": schema.Int64Attribute{
			Optional: true,
			Description: "How long a gap in the stream ends the read, once at least one event has arrived. Defaults to 2. This is what returns " +
				"a short backfill promptly instead of waiting out read_timeout_seconds, and it is not an error or a warning: a quiet stream " +
				"means the history is exhausted.",
			Validators: []validator.Int64{int64validator.AtLeast(1)},
		},
		"read_timeout_seconds": schema.Int64Attribute{
			Optional: true,
			Description: "Hard bound on the read, for a stream that keeps producing output faster than idle_timeout_seconds. Defaults to 10, " +
				"1 to 60. Reaching it is not an error; the events collected so far land in state behind a warning. The ceiling is the " +
				"provider's HTTP client, which closes the connection at 60 seconds.",
			Validators: []validator.Int64{int64validator.Between(1, 60)},
		},
		"entries": schema.ListNestedAttribute{
			Computed:    true,
			Description: "Log events in the order the API streamed them.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"timestamp": schema.StringAttribute{Computed: true, Description: "Event timestamp in RFC 3339 format; null for an event the API sent without one."},
					"source":    schema.StringAttribute{Computed: true, Description: "Log source that produced the line: container or system. Null for an event the API sent without one."},
					"line":      schema.StringAttribute{Computed: true, Description: "The log line itself. An event whose payload is not the documented JSON object is reported here as its raw text."},
				},
			},
		},
		"lines": schema.ListAttribute{
			ElementType: types.StringType,
			Computed:    true,
			Description: "The line text of every entry, in the same order, for use with join() or file().",
		},
		"last_id": schema.StringAttribute{
			Computed:    true,
			Description: "Cursor of the last event received; pass it back as last_event_id to resume where this read stopped. Null when the read collected no event that carried one.",
		},
	}
}

// logStreamParams is the request half of a log data source model, shared by
// both so the parameter handling is written once.
type logStreamParams struct {
	Source             types.String
	Tail               types.Int64
	Since              types.String
	LastEventID        types.String
	IdleTimeoutSeconds types.Int64
	ReadTimeoutSeconds types.Int64
}

// resolve converts the configured parameters into the SDK's pod-log params.
// The serverless params struct is field-identical and is derived from these.
func (p logStreamParams) resolve() (pods.GetPodLogsParams, diag.Diagnostics) {
	var diags diag.Diagnostics
	params := pods.GetPodLogsParams{}

	if !p.Source.IsNull() && !p.Source.IsUnknown() {
		source := sdktypes.LogSourceParam(p.Source.ValueString())
		params.Source = &source
	}
	if !p.Tail.IsNull() && !p.Tail.IsUnknown() {
		tail := sdktypes.LogTail(p.Tail.ValueInt64())
		params.Tail = &tail
	}
	if since, ok := parseRFC3339Attribute(p.Since, "since", &diags); ok {
		params.Since = &since
	}
	if cursor, ok := parseRFC3339Attribute(p.LastEventID, "last_event_id", &diags); ok {
		params.LastEventID = &cursor
	}
	return params, diags
}

// eventLimit is how many events complete the read. A resume cursor makes the
// backfill size unknowable (the API replays from a point in time, not a line
// count), so those reads are bounded by the timeout alone, which is what 0
// means here.
func (p logStreamParams) eventLimit() int {
	if isSet(p.Since) || isSet(p.LastEventID) {
		return 0
	}
	if !p.Tail.IsNull() && !p.Tail.IsUnknown() {
		return int(p.Tail.ValueInt64())
	}
	return defaultLogTail
}

func (p logStreamParams) idleTimeout() time.Duration {
	if !p.IdleTimeoutSeconds.IsNull() && !p.IdleTimeoutSeconds.IsUnknown() {
		return time.Duration(p.IdleTimeoutSeconds.ValueInt64()) * time.Second
	}
	return defaultLogIdleTimeoutSeconds * time.Second
}

func (p logStreamParams) readTimeout() time.Duration {
	if !p.ReadTimeoutSeconds.IsNull() && !p.ReadTimeoutSeconds.IsUnknown() {
		return time.Duration(p.ReadTimeoutSeconds.ValueInt64()) * time.Second
	}
	return defaultLogReadTimeoutSeconds * time.Second
}

func isSet(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// parseRFC3339Attribute reads a timestamp-typed configuration attribute. The
// second return reports whether a value was set at all, so an unset attribute
// stays out of the request.
func parseRFC3339Attribute(value types.String, attribute string, diags *diag.Diagnostics) (time.Time, bool) {
	if !isSet(value) {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value.ValueString())
	if err != nil {
		diags.AddError(
			"Invalid timestamp",
			fmt.Sprintf("%s must be an RFC 3339 timestamp, e.g. 2026-06-01T12:00:00Z: %s", attribute, err.Error()),
		)
		return time.Time{}, false
	}
	return parsed, true
}

type logStreamResult struct {
	Entries []logEntryModel
	LastId  types.String
}

// fetchLogStream issues the request under a deadline and decodes events off
// the open body. The body is closed and the request cancelled on every path,
// including the one where the server is still willing to send more.
func fetchLogStream(
	ctx context.Context,
	action string,
	timeout, idleAfter time.Duration,
	limit int,
	send func(context.Context) (*http.Response, error),
) (logStreamResult, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := logStreamResult{Entries: []logEntryModel{}, LastId: types.StringNull()}

	streamCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	apiResp, err := send(streamCtx)
	if err != nil {
		detail := err.Error()
		if errors.Is(err, context.DeadlineExceeded) {
			detail += fmt.Sprintf(" (the API sent no response within read_timeout_seconds = %s)", timeout)
		}
		diags.AddError(action+" failed", detail)
		return result, diags
	}
	// Nothing can act on a close error here: the frames that arrived are
	// already in state, and the stream is one the server never closes itself.
	defer func() { _ = apiResp.Body.Close() }()

	if apiResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(apiResp.Body, maxErrorBodyBytes))
		diags.AddError(action+" failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.StatusCode, body))
		return result, diags
	}

	read, readErr := collectLogEvents(streamCtx, apiResp.Body, limit, idleAfter)
	result.Entries = read.Entries
	if read.LastId != "" {
		result.LastId = types.StringValue(read.LastId)
	}

	// The hard deadline is the only stop worth reporting: a complete backfill,
	// a quiet stream, and a closed stream are all ordinary reads.
	timedOut := read.HitDeadline || errors.Is(readErr, context.DeadlineExceeded) ||
		(readErr != nil && streamCtx.Err() != nil)

	switch {
	case readErr == nil:
	case timedOut:
		detail := fmt.Sprintf("The API was still streaming after %s, so the %d event(s) received so far are what landed in state. "+
			"Raise read_timeout_seconds to wait longer.", timeout, len(read.Entries))
		if !result.LastId.IsNull() {
			detail += fmt.Sprintf(" Set last_event_id to %q on the next read to continue from here.", result.LastId.ValueString())
		}
		diags.AddWarning(action+" stopped at the read timeout", detail)
	case len(read.Entries) == 0:
		diags.AddError(action+" failed", readErr.Error())
	default:
		diags.AddWarning(
			action+" ended early",
			fmt.Sprintf("The stream failed after %d event(s), which are what landed in state: %s", len(read.Entries), readErr.Error()),
		)
	}
	return result, diags
}

// logFrame is one decoded SSE event: its payload when it carried one, and the
// resume cursor from its id: line.
type logFrame struct {
	Entry    logEntryModel
	HasEntry bool
	Id       string
}

// logStreamRead is what one bounded read collected.
type logStreamRead struct {
	Entries     []logEntryModel
	LastId      string
	HitDeadline bool
}

// collectLogEvents reads decoded events until the first stop condition: the
// backfill is complete (limit events; 0 means no count bound), the stream went
// quiet for idleAfter with at least one event already in hand (0 disables that
// stop), the server ended the stream, or ctx's deadline expired. Only the
// deadline sets HitDeadline; the rest are complete reads.
func collectLogEvents(ctx context.Context, body io.Reader, limit int, idleAfter time.Duration) (logStreamRead, error) {
	frames := make(chan logFrame)
	errc := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)

	go scanLogFrames(body, frames, errc, done)

	read := logStreamRead{Entries: []logEntryModel{}}

	// The idle timer starts at the first event: a gap before anything has
	// arrived says nothing about whether the backfill is done.
	var idle *time.Timer
	var quiet <-chan time.Time
	defer func() {
		if idle != nil {
			idle.Stop()
		}
	}()

	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				select {
				case err := <-errc:
					return read, err
				default:
					return read, nil
				}
			}
			if frame.Id != "" {
				read.LastId = frame.Id
			}
			if !frame.HasEntry {
				continue
			}
			read.Entries = append(read.Entries, frame.Entry)
			if limit > 0 && len(read.Entries) >= limit {
				return read, nil
			}
			if idleAfter > 0 {
				if idle == nil {
					idle = time.NewTimer(idleAfter)
					quiet = idle.C
					continue
				}
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(idleAfter)
			}
		case <-quiet:
			return read, nil
		case <-ctx.Done():
			read.HitDeadline = true
			return read, ctx.Err()
		}
	}
}

// scanLogFrames decodes SSE frames off an open stream and sends one logFrame
// per event: a blank line ends an event, data: lines carry its JSON payload,
// id: lines carry the resume cursor, and event:/retry:/comment lines are
// ignored. frames is closed when the stream ends; the scan error, nil on a
// clean end, goes to the buffered errc before that. done lets the consumer
// walk away from a stream the server is still writing to.
func scanLogFrames(body io.Reader, frames chan<- logFrame, errc chan<- error, done <-chan struct{}) {
	defer close(frames)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, initialLogScanBuffer), maxLogLineBytes)

	var payload []string
	var id string

	send := func() bool {
		frame := logFrame{Id: id}
		frame.Entry, frame.HasEntry = decodeLogEvent(strings.Join(payload, "\n"))
		payload, id = nil, ""
		if !frame.HasEntry && frame.Id == "" {
			return true
		}
		select {
		case frames <- frame:
			return true
		case <-done:
			return false
		}
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		switch {
		case line == "":
			if !send() {
				return
			}
		case strings.HasPrefix(line, ":"):
			// SSE comment / keep-alive.
		case strings.HasPrefix(line, "data:"):
			payload = append(payload, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		case strings.HasPrefix(line, "id:"):
			id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		default:
			// event: and retry: carry no log content.
		}
	}

	if err := scanner.Err(); err != nil {
		// A half-received event is dropped: its payload cannot be decoded and
		// its text would be a truncated fragment.
		errc <- err
		return
	}
	// A stream that ends without a trailing blank line still has one event
	// pending.
	if !send() {
		return
	}
	errc <- nil
}

func decodeLogEvent(data string) (logEntryModel, bool) {
	if strings.TrimSpace(data) == "" {
		return logEntryModel{}, false
	}

	var event struct {
		Ts     *string `json:"ts"`
		Source *string `json:"source"`
		Line   *string `json:"line"`
	}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return logEntryModel{
			Timestamp: types.StringNull(),
			Source:    types.StringNull(),
			Line:      types.StringValue(data),
		}, true
	}
	return logEntryModel{
		Timestamp: types.StringPointerValue(event.Ts),
		Source:    types.StringPointerValue(event.Source),
		Line:      types.StringPointerValue(event.Line),
	}, true
}

func logLines(ctx context.Context, entries []logEntryModel) (types.List, diag.Diagnostics) {
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, entry.Line.ValueString())
	}
	return types.ListValueFrom(ctx, types.StringType, lines)
}

var (
	_ datasource.DataSource              = (*podLogsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*podLogsDataSource)(nil)
)

func NewPodLogsDataSource() datasource.DataSource { return &podLogsDataSource{} }

type podLogsDataSource struct {
	client *runpod.Client
}

type podLogsDataSourceModel struct {
	PodId              types.String    `tfsdk:"pod_id"`
	Source             types.String    `tfsdk:"source"`
	Tail               types.Int64     `tfsdk:"tail"`
	Since              types.String    `tfsdk:"since"`
	LastEventID        types.String    `tfsdk:"last_event_id"`
	IdleTimeoutSeconds types.Int64     `tfsdk:"idle_timeout_seconds"`
	ReadTimeoutSeconds types.Int64     `tfsdk:"read_timeout_seconds"`
	Entries            []logEntryModel `tfsdk:"entries"`
	Lines              types.List      `tfsdk:"lines"`
	LastId             types.String    `tfsdk:"last_id"`
}

func (m podLogsDataSourceModel) params() logStreamParams {
	return logStreamParams{
		Source:             m.Source,
		Tail:               m.Tail,
		Since:              m.Since,
		LastEventID:        m.LastEventID,
		IdleTimeoutSeconds: m.IdleTimeoutSeconds,
		ReadTimeoutSeconds: m.ReadTimeoutSeconds,
	}
}

func (d *podLogsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pod_logs"
}

func (d *podLogsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := logStreamAttributes()
	attributes["pod_id"] = schema.StringAttribute{Required: true, Description: "Pod identifier to read logs from."}
	resp.Schema = schema.Schema{
		Description: "Log lines from a pod. The API streams these live, so the read collects the backfill (tail lines, 100 by default) and " +
			"returns as soon as it has them, or as soon as the stream goes quiet for idle_timeout_seconds, and in any case at " +
			"read_timeout_seconds. Logs change between runs, so every plan shows this data source as needing a refresh.",
		Attributes: attributes,
	}
}

func (d *podLogsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *podLogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state podLogsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := state.params()
	params, diags := config.resolve()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, readDiags := fetchLogStream(ctx, "Reading pod logs",
		config.readTimeout(), config.idleTimeout(), config.eventLimit(),
		func(streamCtx context.Context) (*http.Response, error) {
			return d.client.Pods.GetPodLogs(streamCtx, state.PodId.ValueString(), &params)
		})
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Entries = result.Entries
	state.LastId = result.LastId
	lines, lineDiags := logLines(ctx, state.Entries)
	resp.Diagnostics.Append(lineDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Lines = lines
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

var (
	_ datasource.DataSource              = (*endpointWorkerLogsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*endpointWorkerLogsDataSource)(nil)
)

func NewEndpointWorkerLogsDataSource() datasource.DataSource { return &endpointWorkerLogsDataSource{} }

type endpointWorkerLogsDataSource struct {
	client *runpod.Client
}

type endpointWorkerLogsDataSourceModel struct {
	EndpointId         types.String    `tfsdk:"endpoint_id"`
	WorkerId           types.String    `tfsdk:"worker_id"`
	Source             types.String    `tfsdk:"source"`
	Tail               types.Int64     `tfsdk:"tail"`
	Since              types.String    `tfsdk:"since"`
	LastEventID        types.String    `tfsdk:"last_event_id"`
	IdleTimeoutSeconds types.Int64     `tfsdk:"idle_timeout_seconds"`
	ReadTimeoutSeconds types.Int64     `tfsdk:"read_timeout_seconds"`
	Entries            []logEntryModel `tfsdk:"entries"`
	Lines              types.List      `tfsdk:"lines"`
	LastId             types.String    `tfsdk:"last_id"`
}

func (m endpointWorkerLogsDataSourceModel) params() logStreamParams {
	return logStreamParams{
		Source:             m.Source,
		Tail:               m.Tail,
		Since:              m.Since,
		LastEventID:        m.LastEventID,
		IdleTimeoutSeconds: m.IdleTimeoutSeconds,
		ReadTimeoutSeconds: m.ReadTimeoutSeconds,
	}
}

func (d *endpointWorkerLogsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_worker_logs"
}

func (d *endpointWorkerLogsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := logStreamAttributes()
	attributes["endpoint_id"] = schema.StringAttribute{Required: true, Description: "Serverless endpoint identifier the worker belongs to."}
	attributes["worker_id"] = schema.StringAttribute{Required: true, Description: "Worker identifier, from runpod_endpoint_workers."}
	resp.Schema = schema.Schema{
		Description: "Log lines from one Serverless worker. The API streams these live, so the read collects the backfill (tail lines, 100 by " +
			"default) and returns as soon as it has them, or as soon as the stream goes quiet for idle_timeout_seconds, and in any case at " +
			"read_timeout_seconds. Logs change between runs, so every plan shows this data source as needing a refresh.",
		Attributes: attributes,
	}
}

func (d *endpointWorkerLogsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *endpointWorkerLogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state endpointWorkerLogsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := state.params()
	podParams, diags := config.resolve()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := serverless.GetWorkerLogsParams{
		Tail:        podParams.Tail,
		Since:       podParams.Since,
		LastEventID: podParams.LastEventID,
	}
	if podParams.Source != nil {
		source := sdktypes.LogSourceParam(*podParams.Source)
		params.Source = &source
	}

	result, readDiags := fetchLogStream(ctx, "Reading worker logs",
		config.readTimeout(), config.idleTimeout(), config.eventLimit(),
		func(streamCtx context.Context) (*http.Response, error) {
			return d.client.Serverless.GetWorkerLogs(streamCtx,
				state.EndpointId.ValueString(), state.WorkerId.ValueString(), &params)
		})
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Entries = result.Entries
	state.LastId = result.LastId
	lines, lineDiags := logLines(ctx, state.Entries)
	resp.Diagnostics.Append(lineDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Lines = lines
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// TestEndpointWorkersDataSourceReadsConfiguredEndpointId drives a Read with
// only endpoint_id configured: everything else is computed, so Terraform sends
// it null, and decoding the whole state model out of configuration would
// reject the null worker summary before any request went out. The releases
// view has the same shape in its rollout object.
func TestEndpointWorkersDataSourceReadsConfiguredEndpointId(t *testing.T) {
	ctx := context.Background()

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"endpointVersion":4,` +
			`"summary":{"total":1,"idle":0,"initializing":0,"running":1,"throttled":0,"unhealthy":0},` +
			`"workers":[{"id":"worker_abc","status":"RUNNING","gpuCount":1,"isStale":false}]}`))
	}))
	defer srv.Close()

	var state endpointWorkersDataSourceModel
	readDataSource(ctx, t, &endpointWorkersDataSource{client: shimTestClient(t, srv.URL)},
		map[string]tftypes.Value{"endpoint_id": tftypes.NewValue(tftypes.String, "ep_abc123")}, &state)

	if gotPath != "/v2/serverless/ep_abc123/workers" {
		t.Errorf("path = %q", gotPath)
	}
	if state.EndpointId.ValueString() != "ep_abc123" || state.EndpointVersion.ValueInt64() != 4 {
		t.Errorf("identity: %+v", state)
	}
	if state.Summary.Total.ValueInt64() != 1 || state.Summary.Running.ValueInt64() != 1 {
		t.Errorf("summary: %+v", state.Summary)
	}
	if len(state.Workers) != 1 || state.Workers[0].Id.ValueString() != "worker_abc" {
		t.Errorf("workers: %+v", state.Workers)
	}
}

// The two per-endpoint histories are cursor-paginated, and builds cap a page
// at 100, which a busy endpoint reaches. If Read stopped after the first page,
// the older builds would drop out of state without a word.
func TestEndpointBuildsDataSourceFollowsPaginationCursor(t *testing.T) {
	ctx := context.Background()

	var queries []string
	srv := pagingServer(t, &queries, "cur_builds_2",
		`{"builds":[{"id":"bld_first","status":"COMPLETED","imageName":"runpod/built:1"}],`+
			`"pagination":{"hasNextPage":true,"nextCursor":"cur_builds_2"}}`,
		`{"builds":[{"id":"bld_second","status":"FAILED","error":"build step exited 1"}],`+
			`"pagination":{"hasNextPage":false,"nextCursor":null}}`)

	var state endpointBuildsDataSourceModel
	readDataSource(ctx, t, &endpointBuildsDataSource{client: shimTestClient(t, srv.URL)},
		map[string]tftypes.Value{"endpoint_id": tftypes.NewValue(tftypes.String, "ep_abc123")}, &state)

	if want := wantPagedQueries("cur_builds_2"); !slices.Equal(queries, want) {
		t.Fatalf("queries = %q, want %q", queries, want)
	}
	if state.EndpointId.ValueString() != "ep_abc123" {
		t.Errorf("endpoint_id = %v", state.EndpointId)
	}
	if len(state.Builds) != 2 ||
		state.Builds[0].Id.ValueString() != "bld_first" ||
		state.Builds[1].Id.ValueString() != "bld_second" {
		t.Fatalf("builds = %+v, want both pages in order", state.Builds)
	}
	if state.Builds[1].Error.ValueString() != "build step exited 1" {
		t.Errorf("second page entry: %+v", state.Builds[1])
	}
}

func TestEndpointReleasesDataSourceFollowsPaginationCursor(t *testing.T) {
	ctx := context.Background()

	var queries []string
	srv := pagingServer(t, &queries, "cur_releases_2",
		`{"endpointVersion":4,"rollout":{"inProgress":true,"percentOnLatest":50,"workersOnLatest":1,"workersTotal":2},`+
			`"releases":[{"id":"rel_first","source":"MANUAL","createdAt":"2026-06-02T12:00:00Z","workerCount":1,"diff":[]}],`+
			`"pagination":{"hasNextPage":true,"nextCursor":"cur_releases_2"}}`,
		`{"endpointVersion":4,"rollout":{"inProgress":true,"percentOnLatest":50,"workersOnLatest":1,"workersTotal":2},`+
			`"releases":[{"id":"rel_second","source":"GIT_BUILD","createdAt":"2026-06-01T12:00:00Z","workerCount":1,"diff":[]}],`+
			`"pagination":{"hasNextPage":false,"nextCursor":null}}`)

	var state endpointReleasesDataSourceModel
	readDataSource(ctx, t, &endpointReleasesDataSource{client: shimTestClient(t, srv.URL)},
		map[string]tftypes.Value{"endpoint_id": tftypes.NewValue(tftypes.String, "ep_abc123")}, &state)

	if want := wantPagedQueries("cur_releases_2"); !slices.Equal(queries, want) {
		t.Fatalf("queries = %q, want %q", queries, want)
	}
	if len(state.Releases) != 2 ||
		state.Releases[0].Id.ValueString() != "rel_first" ||
		state.Releases[1].Id.ValueString() != "rel_second" {
		t.Fatalf("releases = %+v, want both pages in order", state.Releases)
	}
	// The rollout summary and endpoint version repeat on every page and must
	// survive the walk rather than being overwritten by an empty envelope.
	if state.EndpointVersion.ValueInt64() != 4 || !state.Rollout.InProgress.ValueBool() ||
		state.Rollout.WorkersTotal.ValueInt64() != 2 {
		t.Errorf("rollout: version=%v %+v", state.EndpointVersion, state.Rollout)
	}
}

func TestFlattenEndpointWorkers(t *testing.T) {
	startedAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	version := 7
	uptime := 120
	image := "runpod/worker-vllm:1.0.0"
	gpuType := "NVIDIA GeForce RTX 4090"
	dataCenter := "US-TX-3"

	state := flattenEndpointWorkers("ep_abc123", sdktypes.ListEndpointWorkersResponse{
		EndpointVersion: &version,
		Summary: sdktypes.WorkerSummary{
			Total:        3,
			Idle:         1,
			Initializing: 1,
			Running:      1,
			Throttled:    0,
			Unhealthy:    0,
		},
		Workers: []sdktypes.Worker{
			{
				Id:            "worker_abc",
				Status:        sdktypes.WorkerStatusRunning,
				Image:         &image,
				GpuTypeId:     &gpuType,
				GpuCount:      1,
				DataCenterId:  &dataCenter,
				IsStale:       false,
				StartedAt:     &startedAt,
				UptimeSeconds: &uptime,
				Version:       &version,
			},
			{
				Id:       "worker_init",
				Status:   sdktypes.WorkerStatusInitializing,
				GpuCount: 2,
				IsStale:  true,
			},
		},
	})

	if state.EndpointId.ValueString() != "ep_abc123" || state.EndpointVersion.ValueInt64() != 7 {
		t.Fatalf("endpoint: %+v", state)
	}
	if state.Summary.Total.ValueInt64() != 3 || state.Summary.Idle.ValueInt64() != 1 ||
		state.Summary.Running.ValueInt64() != 1 || state.Summary.Unhealthy.ValueInt64() != 0 {
		t.Fatalf("summary: %+v", state.Summary)
	}
	if len(state.Workers) != 2 {
		t.Fatalf("workers = %d, want 2", len(state.Workers))
	}
	running := state.Workers[0]
	if running.Id.ValueString() != "worker_abc" || running.Status.ValueString() != "RUNNING" ||
		running.GpuTypeId.ValueString() != gpuType || running.GpuCount.ValueInt64() != 1 ||
		running.StartedAt.ValueString() != "2026-06-01T12:00:00Z" ||
		running.UptimeSeconds.ValueInt64() != 120 || running.Version.ValueInt64() != 7 {
		t.Fatalf("running worker: %+v", running)
	}
	initializing := state.Workers[1]
	if !initializing.IsStale.ValueBool() || !initializing.StartedAt.IsNull() ||
		!initializing.UptimeSeconds.IsNull() || !initializing.Version.IsNull() ||
		!initializing.Image.IsNull() || !initializing.GpuTypeId.IsNull() ||
		!initializing.DataCenterId.IsNull() {
		t.Fatalf("initializing worker: %+v", initializing)
	}

	assertDataSourceModelFits(t, NewEndpointWorkersDataSource, state)
}

func TestFlattenEndpointWorkersWithoutWorkers(t *testing.T) {
	state := flattenEndpointWorkers("ep_idle", sdktypes.ListEndpointWorkersResponse{})
	if !state.EndpointVersion.IsNull() {
		t.Fatalf("endpoint_version = %v, want null", state.EndpointVersion)
	}
	if state.Workers == nil || len(state.Workers) != 0 {
		t.Fatalf("workers = %v, want an empty list", state.Workers)
	}
	assertDataSourceModelFits(t, NewEndpointWorkersDataSource, state)
}

func sampleBuild() sdktypes.Build {
	startedAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(5 * time.Minute)
	commitDate := startedAt.Add(-time.Hour)
	branch := "main"
	commitHash := "0b1d2c3"
	commitMessage := "bump the handler"
	imageName := "registry.runpod.net/ep_abc123:0b1d2c3"

	return sdktypes.Build{
		Id:            "build_abc",
		Status:        sdktypes.BuildStateCompleted,
		ImageName:     &imageName,
		Branch:        &branch,
		CommitHash:    &commitHash,
		CommitMessage: &commitMessage,
		CommitDate:    &commitDate,
		StartedAt:     &startedAt,
		CompletedAt:   &completedAt,
	}
}

func TestFlattenEndpointBuilds(t *testing.T) {
	build := flattenEndpointBuild(sampleBuild())

	if build.Id.ValueString() != "build_abc" || build.Status.ValueString() != "COMPLETED" {
		t.Fatalf("id/status: %+v", build)
	}
	if build.Branch.ValueString() != "main" || build.CommitHash.ValueString() != "0b1d2c3" {
		t.Fatalf("git fields: %+v", build)
	}
	if build.StartedAt.ValueString() != "2026-06-01T10:00:00Z" ||
		build.CompletedAt.ValueString() != "2026-06-01T10:05:00Z" ||
		build.CommitDate.ValueString() != "2026-06-01T09:00:00Z" {
		t.Fatalf("timestamps: %+v", build)
	}
	if !build.Error.IsNull() {
		t.Fatalf("error = %v, want null on a completed build", build.Error)
	}

	assertDataSourceModelFits(t, NewEndpointBuildsDataSource, endpointBuildsDataSourceModel{
		EndpointId: build.Id,
		Builds:     []endpointBuildModel{build},
	})
}

func TestFlattenEndpointBuildDetail(t *testing.T) {
	failure := "docker build exited 1"
	failed := sampleBuild()
	failed.Status = sdktypes.BuildStateFailed
	failed.CompletedAt = nil
	failed.Error = &failure

	state := flattenEndpointBuildDetail("ep_abc123", failed)
	if state.EndpointId.ValueString() != "ep_abc123" || state.Id.ValueString() != "build_abc" {
		t.Fatalf("keys: %+v", state)
	}
	if state.Status.ValueString() != "FAILED" || state.Error.ValueString() != failure {
		t.Fatalf("failure: %+v", state)
	}
	if !state.CompletedAt.IsNull() {
		t.Fatalf("completed_at = %v, want null", state.CompletedAt)
	}

	assertDataSourceModelFits(t, NewEndpointBuildDataSource, state)
}

func TestFlattenEndpointReleases(t *testing.T) {
	createdAt := time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)
	version := 7
	buildId := "build_abc"
	userId := "user_abc"

	state := flattenEndpointReleases("ep_abc123", sdktypes.ListEndpointReleasesResponse{
		EndpointVersion: &version,
		Rollout: sdktypes.RolloutSummary{
			InProgress:      true,
			PercentOnLatest: 67,
			WorkersOnLatest: 2,
			WorkersTotal:    3,
		},
		Releases: []sdktypes.Release{
			{
				Id:              "rel_abc",
				Version:         &version,
				Source:          sdktypes.ReleaseSourceGitBuild,
				BuildId:         &buildId,
				CreatedAt:       createdAt,
				CreatedByUserId: &userId,
				WorkerCount:     2,
				Diff: []sdktypes.ReleaseDiffEntry{
					{Field: "template.imageName", Old: "old:1", New: "new:2"},
					{Field: "gpuCount", Old: nil, New: float64(2)},
					{Field: "template.env", Old: map[string]any{"A": "1"}, New: nil},
				},
			},
			{
				Id:          "rel_manual",
				Source:      sdktypes.ReleaseSourceManual,
				CreatedAt:   createdAt,
				WorkerCount: 0,
				Diff:        nil,
			},
		},
	})

	if state.EndpointId.ValueString() != "ep_abc123" || state.EndpointVersion.ValueInt64() != 7 {
		t.Fatalf("endpoint: %+v", state)
	}
	if !state.Rollout.InProgress.ValueBool() || state.Rollout.PercentOnLatest.ValueInt64() != 67 ||
		state.Rollout.WorkersOnLatest.ValueInt64() != 2 || state.Rollout.WorkersTotal.ValueInt64() != 3 {
		t.Fatalf("rollout: %+v", state.Rollout)
	}
	if len(state.Releases) != 2 {
		t.Fatalf("releases = %d, want 2", len(state.Releases))
	}
	gitRelease := state.Releases[0]
	if gitRelease.Source.ValueString() != "GIT_BUILD" || gitRelease.BuildId.ValueString() != buildId ||
		gitRelease.CreatedAt.ValueString() != "2026-06-01T11:00:00Z" ||
		gitRelease.CreatedByUserId.ValueString() != userId {
		t.Fatalf("git release: %+v", gitRelease)
	}
	if len(gitRelease.Diff) != 3 {
		t.Fatalf("diff = %d entries, want 3", len(gitRelease.Diff))
	}
	if gitRelease.Diff[0].Old.ValueString() != "old:1" || gitRelease.Diff[0].New.ValueString() != "new:2" {
		t.Fatalf("string diff: %+v", gitRelease.Diff[0])
	}
	if !gitRelease.Diff[1].Old.IsNull() || gitRelease.Diff[1].New.ValueString() != "2" {
		t.Fatalf("number diff: %+v", gitRelease.Diff[1])
	}
	if gitRelease.Diff[2].Old.ValueString() != `{"A":"1"}` || !gitRelease.Diff[2].New.IsNull() {
		t.Fatalf("object diff: %+v", gitRelease.Diff[2])
	}
	manual := state.Releases[1]
	if !manual.Version.IsNull() || !manual.BuildId.IsNull() || !manual.CreatedByUserId.IsNull() ||
		len(manual.Diff) != 0 {
		t.Fatalf("manual release: %+v", manual)
	}

	assertDataSourceModelFits(t, NewEndpointReleasesDataSource, state)
}

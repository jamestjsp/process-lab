package studio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSimulationHistoryIsNewestFirstBoundedAndReportsFreshness(t *testing.T) {
	ctx := context.Background()
	service := openTestStudio(t, ":memory:")
	snapshot, err := service.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}

	oldestCreated := snapshot.Flow.ModelUpdatedAt.Add(-time.Minute)
	newerCreated := snapshot.Flow.ModelUpdatedAt.Add(time.Minute)
	for index := 0; index < defaultSimulationHistoryLimit+1; index++ {
		created := newerCreated.Add(time.Duration(index) * time.Second)
		seriesCount := 2
		if index == 0 {
			created = oldestCreated
			seriesCount = 1
		}
		insertSimulationHistoryRun(t, service, snapshot.Flow.ID, created, seriesCount)
	}

	bounded, err := service.SimulationHistory(ctx, snapshot.Flow.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(bounded) != defaultSimulationHistoryLimit {
		t.Fatalf("default history length = %d, want %d", len(bounded), defaultSimulationHistoryLimit)
	}
	if bounded[0].ID <= bounded[len(bounded)-1].ID {
		t.Fatalf("history ids are not newest-first: first %d, last %d", bounded[0].ID, bounded[len(bounded)-1].ID)
	}
	if bounded[0].ChannelCount != 2 || bounded[0].Stale {
		t.Fatalf("newest summary = %#v", bounded[0])
	}

	all, err := service.SimulationHistory(ctx, snapshot.Flow.ID, maxSimulationHistoryLimit)
	if err != nil {
		t.Fatal(err)
	}
	oldest := all[len(all)-1]
	if len(all) != defaultSimulationHistoryLimit+1 || oldest.ChannelCount != 1 || !oldest.Stale {
		t.Fatalf("full history length = %d, oldest = %#v", len(all), oldest)
	}
	if oldest.Duration != 2 || oldest.SampleTime != 0.25 || oldest.CreatedAt != oldestCreated {
		t.Fatalf("oldest metadata = %#v", oldest)
	}

	one, err := service.SimulationHistory(ctx, snapshot.Flow.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].ID != bounded[0].ID {
		t.Fatalf("one-run history = %#v", one)
	}
}

func TestSimulationHistoryRejectsInvalidLimitsBeforeDatabaseAccess(t *testing.T) {
	service := openTestStudio(t, ":memory:")
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{-1, maxSimulationHistoryLimit + 1} {
		_, err := service.SimulationHistory(context.Background(), 1, limit)
		var validation *ValidationError
		if !errors.As(err, &validation) || !strings.Contains(validation.Message, "history limit") {
			t.Fatalf("limit %d error = %v, want validation before database access", limit, err)
		}
	}
}

func TestSimulationRunLoadsLegacyJSONAndHidesForeignFlowRuns(t *testing.T) {
	ctx := context.Background()
	service := openTestStudio(t, ":memory:")
	current, err := service.CurrentWorkspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	created := current.Snapshot.Flow.ModelUpdatedAt.Add(-time.Minute)
	result, err := service.db.ExecContext(ctx, `
		INSERT INTO simulation_runs(flow_id, created_at, duration, sample_time, result_json)
		VALUES(?, ?, ?, ?, ?)`,
		current.Snapshot.Flow.ID,
		created.Format(time.RFC3339Nano),
		2.5,
		0.5,
		`{"id":999,"duration":99,"sampleTime":9,"times":[0,0.5],"series":[{"blockId":4,"name":"Temperature","values":[0,1]}],"metrics":[]}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	runID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	stored, err := service.SimulationRun(ctx, current.Snapshot.Flow.ID, runID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != runID || stored.Duration != 2.5 || stored.SampleTime != 0.5 || stored.CreatedAt != created {
		t.Fatalf("stored metadata = %#v", stored)
	}
	if !stored.Stale || len(stored.Series) != 1 || stored.Series[0].Name != "Temperature" {
		t.Fatalf("legacy stored run = %#v", stored)
	}
	if len(stored.Spectra) != 0 {
		t.Fatalf("legacy empty spectra = %#v", stored.Spectra)
	}

	foreign, err := service.CreateFlow(ctx, current.Project.ID, "Foreign history")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SimulationRun(ctx, foreign.Snapshot.Flow.ID, runID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign flow lookup error = %v, want ErrNotFound", err)
	}
	if _, err := service.SimulationRun(ctx, current.Snapshot.Flow.ID, runID+1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing run lookup error = %v, want ErrNotFound", err)
	}
}

func insertSimulationHistoryRun(
	t *testing.T,
	service *Studio,
	flowID int64,
	created time.Time,
	seriesCount int,
) int64 {
	t.Helper()
	series := make([]string, seriesCount)
	for index := range series {
		series[index] = fmt.Sprintf(`{"blockId":%d,"name":"signal %d","values":[0,1]}`, index+1, index+1)
	}
	encoded := `{"times":[0,1],"series":[` + strings.Join(series, ",") + `],"metrics":[]}`
	result, err := service.db.ExecContext(context.Background(), `
		INSERT INTO simulation_runs(flow_id, created_at, duration, sample_time, result_json)
		VALUES(?, ?, ?, ?, ?)`,
		flowID, created.Format(time.RFC3339Nano), 2, 0.25, encoded,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	defaultSimulationHistoryLimit = 20
	maxSimulationHistoryLimit     = 100
)

type SimulationSummary struct {
	ID           int64     `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	Duration     float64   `json:"duration"`
	SampleTime   float64   `json:"sampleTime"`
	ChannelCount int       `json:"channelCount"`
	Stale        bool      `json:"stale"`
}

type StoredSimulation struct {
	Simulation
	Stale bool `json:"stale"`
}

// SimulationHistory returns a bounded, newest-first summary of stored runs.
// A zero limit selects the default; callers cannot request an unbounded scan.
func (s *Studio) SimulationHistory(
	ctx context.Context,
	flowID int64,
	limit int,
) ([]SimulationSummary, error) {
	switch {
	case limit == 0:
		limit = defaultSimulationHistoryLimit
	case limit < 0:
		return nil, invalid("simulation history limit must be positive")
	case limit > maxSimulationHistoryLimit:
		return nil, invalid(
			"simulation history limit must not exceed %d",
			maxSimulationHistoryLimit,
		)
	}

	var modelUpdatedText string
	if err := s.db.QueryRowContext(ctx,
		"SELECT model_updated_at FROM flows WHERE id = ?", flowID,
	).Scan(&modelUpdatedText); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load simulation history flow: %w", err)
	}
	modelUpdatedAt, err := time.Parse(time.RFC3339Nano, modelUpdatedText)
	if err != nil {
		return nil, fmt.Errorf("parse simulation history model revision: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, created_at, duration, sample_time, result_json
		FROM simulation_runs
		WHERE flow_id = ?
		ORDER BY id DESC LIMIT ?`, flowID, limit)
	if err != nil {
		return nil, fmt.Errorf("load simulation history: %w", err)
	}
	defer rows.Close()

	summaries := make([]SimulationSummary, 0, limit)
	for rows.Next() {
		var summary SimulationSummary
		var createdText, resultJSON string
		if err := rows.Scan(
			&summary.ID,
			&createdText,
			&summary.Duration,
			&summary.SampleTime,
			&resultJSON,
		); err != nil {
			return nil, fmt.Errorf("scan simulation history: %w", err)
		}
		summary.CreatedAt, err = time.Parse(time.RFC3339Nano, createdText)
		if err != nil {
			return nil, fmt.Errorf("parse simulation history timestamp: %w", err)
		}
		var shape struct {
			Series  []struct{} `json:"series"`
			Spectra []struct{} `json:"spectra"`
		}
		if err := json.Unmarshal([]byte(resultJSON), &shape); err != nil {
			return nil, fmt.Errorf("decode simulation history run %d: %w", summary.ID, err)
		}
		summary.ChannelCount = len(shape.Series) + len(shape.Spectra)
		summary.Stale = summary.CreatedAt.Before(modelUpdatedAt)
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate simulation history: %w", err)
	}
	return summaries, nil
}

// SimulationRun returns one stored run only when it belongs to flowID. The
// joined ownership check prevents a run id from disclosing another flow's data.
func (s *Studio) SimulationRun(
	ctx context.Context,
	flowID int64,
	runID int64,
) (StoredSimulation, error) {
	var run Simulation
	var createdText, modelUpdatedText, resultJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT runs.id, runs.created_at, runs.duration, runs.sample_time,
			runs.result_json, flows.model_updated_at
		FROM simulation_runs AS runs
		JOIN flows ON flows.id = runs.flow_id
		WHERE runs.flow_id = ? AND runs.id = ?`, flowID, runID,
	).Scan(
		&run.ID,
		&createdText,
		&run.Duration,
		&run.SampleTime,
		&resultJSON,
		&modelUpdatedText,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredSimulation{}, ErrNotFound
	}
	if err != nil {
		return StoredSimulation{}, fmt.Errorf("load simulation run: %w", err)
	}

	runID = run.ID
	duration := run.Duration
	sampleTime := run.SampleTime
	if err := json.Unmarshal([]byte(resultJSON), &run); err != nil {
		return StoredSimulation{}, fmt.Errorf("decode simulation run: %w", err)
	}
	run.ID = runID
	run.Duration = duration
	run.SampleTime = sampleTime
	run.CreatedAt, err = time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return StoredSimulation{}, fmt.Errorf("parse simulation run timestamp: %w", err)
	}
	modelUpdatedAt, err := time.Parse(time.RFC3339Nano, modelUpdatedText)
	if err != nil {
		return StoredSimulation{}, fmt.Errorf("parse simulation model revision: %w", err)
	}
	return StoredSimulation{
		Simulation: run,
		Stale:      run.CreatedAt.Before(modelUpdatedAt),
	}, nil
}

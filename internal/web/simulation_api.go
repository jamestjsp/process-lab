package web

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/jamestjsp/process-lab/internal/studio"
)

type simulationRunAPIRequest struct {
	Duration   float64 `json:"duration"`
	SampleTime float64 `json:"sampleTime"`
}

type latestSimulationAPIRecord struct {
	studio.Simulation
	Stale bool `json:"stale"`
}

func (s *Server) simulationRunAPI(r *http.Request) (apiResponse, error) {
	flowID, err := parsePathInt(r, "flowID")
	if err != nil {
		return apiResponse{}, err
	}
	var input simulationRunAPIRequest
	if err := decodeAPIJSON(r, &input); err != nil {
		return apiResponse{}, err
	}
	snapshot, err := s.studio.Run(r.Context(), flowID, studio.SimulationRequest{
		Duration: input.Duration, SampleTime: input.SampleTime,
	})
	if err != nil {
		return apiResponse{}, err
	}
	if snapshot.LastRun == nil {
		return apiResponse{}, errors.New("simulation completed without a stored result")
	}
	return apiResponse{Status: http.StatusCreated, Value: snapshot.LastRun}, nil
}

func (s *Server) simulationShowAPI(r *http.Request) (apiResponse, error) {
	flowID, err := parsePathInt(r, "flowID")
	if err != nil {
		return apiResponse{}, err
	}
	run, err := s.studio.LatestSimulation(r.Context(), flowID)
	if errors.Is(err, studio.ErrNotFound) {
		return apiResponse{}, &studio.ValidationError{Message: "no simulation run is stored; run one first."}
	}
	if err != nil {
		return apiResponse{}, err
	}
	workspace, err := s.workspaceForFlow(r.Context(), flowID)
	if err != nil {
		return apiResponse{}, err
	}
	return apiResponse{Value: latestSimulationAPIRecord{
		Simulation: run, Stale: workspace.Snapshot.Flow.NeedsRun,
	}}, nil
}

func (s *Server) simulationHistoryAPI(r *http.Request) (apiResponse, error) {
	flowID, err := parsePathInt(r, "flowID")
	if err != nil {
		return apiResponse{}, err
	}
	limit := 0
	if requested, err := optionalQueryInt(r, "limit"); err != nil {
		return apiResponse{}, err
	} else if requested != nil {
		limit = int(*requested)
	}
	history, err := s.studio.SimulationHistory(r.Context(), flowID, limit)
	if err != nil {
		return apiResponse{}, err
	}
	return apiResponse{Value: history}, nil
}

func (s *Server) simulationRunShowAPI(r *http.Request) (apiResponse, error) {
	flowID, err := parsePathInt(r, "flowID")
	if err != nil {
		return apiResponse{}, err
	}
	runID, err := parsePathInt(r, "runID")
	if err != nil {
		return apiResponse{}, err
	}
	run, err := s.studio.SimulationRun(r.Context(), flowID, runID)
	if err != nil {
		return apiResponse{}, err
	}
	return apiResponse{Value: run}, nil
}

func (s *Server) simulationCSV(w http.ResponseWriter, r *http.Request) {
	flowID, ok := pathID(w, r, "flowID")
	if !ok {
		return
	}
	runText, ok := strings.CutSuffix(r.PathValue("runFile"), ".csv")
	if !ok {
		http.NotFound(w, r)
		return
	}
	runID, err := strconv.ParseInt(runText, 10, 64)
	if err != nil || runID <= 0 {
		http.Error(w, "Invalid identifier.", http.StatusBadRequest)
		return
	}
	run, err := s.studio.SimulationRun(r.Context(), flowID, runID)
	if errors.Is(err, studio.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Process Lab could not export this simulation.", http.StatusInternalServerError)
		return
	}
	encoded, err := simulationCSVData(run.Simulation)
	if err != nil {
		http.Error(w, "Process Lab could not export this simulation.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set(
		"Content-Disposition",
		fmt.Sprintf(`attachment; filename="process-lab-flow-%d-simulation-%d.csv"`, flowID, runID),
	)
	_, _ = w.Write(encoded)
}

func simulationCSVData(run studio.Simulation) ([]byte, error) {
	columns, err := simulationCSVColumns(run)
	if err != nil {
		return nil, err
	}

	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.UseCRLF = true
	header := make([]string, len(columns))
	rowCount := 0
	for index, column := range columns {
		header[index] = column.Header
		rowCount = max(rowCount, len(column.Values))
	}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("write simulation CSV header: %w", err)
	}
	row := make([]string, len(columns))
	for sample := range rowCount {
		for index, column := range columns {
			row[index] = ""
			if sample < len(column.Values) {
				row[index] = strconv.FormatFloat(column.Values[sample], 'g', -1, 64)
			}
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("write simulation CSV row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("write simulation CSV: %w", err)
	}
	return output.Bytes(), nil
}

type simulationCSVColumn struct {
	Header string
	Values []float64
}

func simulationCSVColumns(run studio.Simulation) ([]simulationCSVColumn, error) {
	series := append([]studio.Series(nil), run.Series...)
	sort.SliceStable(series, func(i, j int) bool {
		return simulationResultChannelLess(series[i].ResultChannel, series[j].ResultChannel)
	})
	spectra := append([]studio.Spectrum(nil), run.Spectra...)
	sort.SliceStable(spectra, func(i, j int) bool {
		return simulationResultChannelLess(spectra[i].ResultChannel, spectra[j].ResultChannel)
	})
	for index, value := range run.Times {
		if !finite(value) {
			return nil, fmt.Errorf("simulation time %d is not finite", index)
		}
	}
	for _, signal := range series {
		if len(signal.Values) != len(run.Times) {
			return nil, fmt.Errorf("simulation series %q has %d values for %d samples",
				signal.Name, len(signal.Values), len(run.Times))
		}
		for index, value := range signal.Values {
			if !finite(value) {
				return nil, fmt.Errorf("simulation series %q sample %d is not finite", signal.Name, index)
			}
		}
	}
	for _, spectrum := range spectra {
		if len(spectrum.Frequencies) != len(spectrum.Magnitudes) {
			return nil, fmt.Errorf(
				"simulation spectrum %q has %d frequencies for %d magnitudes",
				spectrum.Name, len(spectrum.Frequencies), len(spectrum.Magnitudes),
			)
		}
		for index, value := range spectrum.Frequencies {
			if !finite(value) {
				return nil, fmt.Errorf("simulation spectrum %q frequency %d is not finite", spectrum.Name, index)
			}
		}
		for index, value := range spectrum.Magnitudes {
			if !finite(value) {
				return nil, fmt.Errorf("simulation spectrum %q magnitude %d is not finite", spectrum.Name, index)
			}
		}
	}

	columns := make([]simulationCSVColumn, 0, 1+len(series)+2*len(spectra))
	if len(series) > 0 || len(spectra) == 0 {
		columns = append(columns, simulationCSVColumn{Header: "time [unit=s]", Values: run.Times})
	}
	for _, signal := range series {
		name := signal.Name
		if name == "" {
			name = "signal"
		}
		columns = append(columns, simulationCSVColumn{
			Header: simulationCSVChannelHeader(name, "unspecified", signal.ResultChannel),
			Values: signal.Values,
		})
	}
	for _, spectrum := range spectra {
		name := spectrum.Name
		if name == "" {
			name = "spectrum"
		}
		columns = append(columns,
			simulationCSVColumn{
				Header: simulationCSVChannelHeader("frequency", "Hz", spectrum.ResultChannel),
				Values: spectrum.Frequencies,
			},
			simulationCSVColumn{
				Header: simulationCSVChannelHeader(name, "magnitude", spectrum.ResultChannel),
				Values: spectrum.Magnitudes,
			},
		)
	}
	return columns, nil
}

func simulationResultChannelLess(left, right studio.ResultChannel) bool {
	if left.BlockID != right.BlockID {
		return left.BlockID < right.BlockID
	}
	if left.Port != right.Port {
		return left.Port < right.Port
	}
	if left.Channel != right.Channel {
		return left.Channel < right.Channel
	}
	return left.Name < right.Name
}

func simulationCSVChannelHeader(name, unit string, channel studio.ResultChannel) string {
	return spreadsheetSafeCSVCell(fmt.Sprintf(
		"%s [unit=%s;blockId=%d;port=%d;channel=%d]",
		name, unit, channel.BlockID, channel.Port, channel.Channel,
	))
}

func spreadsheetSafeCSVCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

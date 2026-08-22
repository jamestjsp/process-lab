package web

import (
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/jamestjsp/process-lab/internal/studio"
)

func TestSimulationAPIStoresAndShowsLatestRunsIncludingStaleData(t *testing.T) {
	server, service := openTestServer(t)
	current, err := service.CurrentWorkspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	flowID := current.Snapshot.Flow.ID

	run := requestJSONAPI(t, server, http.MethodPost, fmt.Sprintf("/api/v1/flows/%d/simulations", flowID), simulationRunAPIRequest{
		Duration: 1, SampleTime: 0.1,
	})
	if run.Code != http.StatusCreated {
		t.Fatalf("simulation run status = %d: %s", run.Code, run.Body.String())
	}
	var simulation studio.Simulation
	decodeJSONResponse(t, run, &simulation)
	if simulation.ID == 0 || len(simulation.Times) != 11 || len(simulation.Series) == 0 {
		t.Fatalf("simulation = id %d, %d times, %d series", simulation.ID, len(simulation.Times), len(simulation.Series))
	}

	show := requestAPI(t, server, http.MethodGet, fmt.Sprintf("/api/v1/flows/%d/simulations/latest", flowID))
	if show.Code != http.StatusOK {
		t.Fatalf("simulation show status = %d: %s", show.Code, show.Body.String())
	}
	var latest latestSimulationAPIRecord
	decodeJSONResponse(t, show, &latest)
	if latest.ID != simulation.ID || latest.Stale {
		t.Fatalf("latest simulation = %#v", latest)
	}

	if _, _, err := service.AddBlock(context.Background(), flowID, studio.BlockConstant, studio.Point{X: 2200, Y: 1000}); err != nil {
		t.Fatal(err)
	}
	stale := requestAPI(t, server, http.MethodGet, fmt.Sprintf("/api/v1/flows/%d/simulations/latest", flowID))
	if stale.Code != http.StatusOK {
		t.Fatalf("stale simulation status = %d: %s", stale.Code, stale.Body.String())
	}
	var staleLatest latestSimulationAPIRecord
	decodeJSONResponse(t, stale, &staleLatest)
	if staleLatest.ID != simulation.ID || !staleLatest.Stale {
		t.Fatalf("stale latest simulation = %#v", staleLatest)
	}
}

func TestSimulationAPIReportsMissingRunAsUsage(t *testing.T) {
	server, service := openTestServer(t)
	current, err := service.CurrentWorkspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateFlow(context.Background(), current.Project.ID, "No run")
	if err != nil {
		t.Fatal(err)
	}
	response := requestAPI(t, server, http.MethodGet, fmt.Sprintf("/api/v1/flows/%d/simulations/latest", created.Snapshot.Flow.ID))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "run one first") {
		t.Fatalf("missing run response = %d: %s", response.Code, response.Body.String())
	}
}

func TestSimulationHistoryAPIListsAndShowsOwnedRuns(t *testing.T) {
	server, service := openTestServer(t)
	current, err := service.CurrentWorkspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	flowID := current.Snapshot.Flow.ID
	var runs []studio.Simulation
	for range 2 {
		response := requestJSONAPI(t, server, http.MethodPost,
			fmt.Sprintf("/api/v1/flows/%d/simulations", flowID),
			simulationRunAPIRequest{Duration: 1, SampleTime: 0.1},
		)
		if response.Code != http.StatusCreated {
			t.Fatalf("simulation status = %d: %s", response.Code, response.Body.String())
		}
		var run studio.Simulation
		decodeJSONResponse(t, response, &run)
		runs = append(runs, run)
	}

	list := requestAPI(t, server, http.MethodGet,
		fmt.Sprintf("/api/v1/flows/%d/simulations?limit=1", flowID))
	if list.Code != http.StatusOK {
		t.Fatalf("history status = %d: %s", list.Code, list.Body.String())
	}
	var history []studio.SimulationSummary
	decodeJSONResponse(t, list, &history)
	if len(history) != 1 || history[0].ID != runs[1].ID || history[0].Stale || history[0].ChannelCount == 0 {
		t.Fatalf("history = %#v", history)
	}

	detail := requestAPI(t, server, http.MethodGet,
		fmt.Sprintf("/api/v1/flows/%d/simulations/%d", flowID, runs[0].ID))
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d: %s", detail.Code, detail.Body.String())
	}
	var stored studio.StoredSimulation
	decodeJSONResponse(t, detail, &stored)
	if stored.ID != runs[0].ID || stored.Stale || len(stored.Times) != len(runs[0].Times) {
		t.Fatalf("stored run = %#v", stored)
	}

	for _, query := range []string{"limit=nope", "limit=101"} {
		response := requestAPI(t, server, http.MethodGet,
			fmt.Sprintf("/api/v1/flows/%d/simulations?%s", flowID, query))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("history query %q status = %d: %s", query, response.Code, response.Body.String())
		}
	}

	foreign, err := service.CreateFlow(context.Background(), current.Project.ID, "Foreign API history")
	if err != nil {
		t.Fatal(err)
	}
	missing := requestAPI(t, server, http.MethodGet,
		fmt.Sprintf("/api/v1/flows/%d/simulations/%d", foreign.Snapshot.Flow.ID, runs[0].ID))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("foreign detail status = %d: %s", missing.Code, missing.Body.String())
	}
}

func TestSimulationCSVRouteReturnsAnOwnedAttachment(t *testing.T) {
	server, service := openTestServer(t)
	current, err := service.CurrentWorkspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	flowID := current.Snapshot.Flow.ID
	runResponse := requestJSONAPI(t, server, http.MethodPost,
		fmt.Sprintf("/api/v1/flows/%d/simulations", flowID),
		simulationRunAPIRequest{Duration: 1, SampleTime: 0.1},
	)
	var run studio.Simulation
	decodeJSONResponse(t, runResponse, &run)

	response := requestAPI(t, server, http.MethodGet,
		fmt.Sprintf("/flows/%d/simulations/%d.csv", flowID, run.ID))
	if response.Code != http.StatusOK {
		t.Fatalf("CSV status = %d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("CSV content type = %q", got)
	}
	if got := response.Header().Get("Content-Disposition"); !strings.Contains(got, fmt.Sprintf("simulation-%d.csv", run.ID)) {
		t.Fatalf("CSV content disposition = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("CSV cache policy = %q", got)
	}
	records, err := csv.NewReader(strings.NewReader(response.Body.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != len(run.Times)+1 || records[0][0] != "time [unit=s]" || !strings.Contains(records[0][1], "blockId=") {
		t.Fatalf("CSV records = %#v", records)
	}

	foreign, err := service.CreateFlow(context.Background(), current.Project.ID, "Foreign CSV history")
	if err != nil {
		t.Fatal(err)
	}
	foreignResponse := requestAPI(t, server, http.MethodGet,
		fmt.Sprintf("/flows/%d/simulations/%d.csv", foreign.Snapshot.Flow.ID, run.ID))
	if foreignResponse.Code != http.StatusNotFound {
		t.Fatalf("foreign CSV status = %d: %s", foreignResponse.Code, foreignResponse.Body.String())
	}
}

func TestSimulationCSVQuotesMultichannelIdentityAndRejectsNonFiniteValues(t *testing.T) {
	run := studio.Simulation{
		Times: []float64{0, 0.5},
		Series: []studio.Series{
			{
				ResultChannel: studio.ResultChannel{
					BlockID: 9, Port: 1, Channel: 1,
					Name: "pressure, secondary",
				},
				Values: []float64{3, 4},
			},
			{
				ResultChannel: studio.ResultChannel{
					BlockID: 2, Port: 0, Channel: 0,
					Name: "temperature \"primary\"\nloop",
				},
				Values: []float64{1, 2},
			},
		},
	}
	encoded, err := simulationCSVData(run)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "\r\n") {
		t.Fatalf("CSV does not use CRLF: %q", encoded)
	}
	records, err := csv.NewReader(strings.NewReader(string(encoded))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || len(records[0]) != 3 {
		t.Fatalf("CSV shape = %#v", records)
	}
	if !strings.HasPrefix(records[0][1], "temperature \"primary\"\nloop") ||
		!strings.Contains(records[0][1], "unit=unspecified;blockId=2;port=0;channel=0") ||
		!strings.HasPrefix(records[0][2], "pressure, secondary") {
		t.Fatalf("CSV headers = %#v", records[0])
	}
	if records[1][0] != "0" || records[1][1] != "1" || records[1][2] != "3" ||
		records[2][0] != "0.5" || records[2][1] != "2" || records[2][2] != "4" {
		t.Fatalf("CSV sample order = %#v", records[1:])
	}

	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		bad := run
		bad.Series = append([]studio.Series(nil), run.Series...)
		bad.Series[0].Values = append([]float64(nil), run.Series[0].Values...)
		bad.Series[0].Values[0] = invalid
		if encoded, err := simulationCSVData(bad); err == nil || len(encoded) != 0 {
			t.Fatalf("non-finite %v produced %q with error %v", invalid, encoded, err)
		}
	}
}

func TestSimulationCSVIncludesSpectrumFrequencyDomains(t *testing.T) {
	run := studio.Simulation{
		Times: []float64{0, 0.5},
		Series: []studio.Series{{
			ResultChannel: studio.ResultChannel{
				BlockID: 9, Port: 0, Channel: 0, Name: "temperature",
			},
			Values: []float64{1, 2},
		}},
		Spectra: []studio.Spectrum{
			{
				ResultChannel: studio.ResultChannel{
					BlockID: 7, Port: 0, Channel: 1, Name: "secondary spectrum",
				},
				Frequencies: []float64{0, 1}, Magnitudes: []float64{3, 4},
			},
			{
				ResultChannel: studio.ResultChannel{
					BlockID: 3, Port: 0, Channel: 0, Name: "primary spectrum",
				},
				Frequencies: []float64{0, 1, 2}, Magnitudes: []float64{5, 6, 7},
			},
		},
	}

	encoded, err := simulationCSVData(run)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(string(encoded))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 || len(records[0]) != 6 {
		t.Fatalf("mixed CSV shape = %#v", records)
	}
	for index, expected := range []string{
		"time [unit=s]",
		"temperature [unit=unspecified;blockId=9;port=0;channel=0]",
		"frequency [unit=Hz;blockId=3;port=0;channel=0]",
		"primary spectrum [unit=magnitude;blockId=3;port=0;channel=0]",
		"frequency [unit=Hz;blockId=7;port=0;channel=1]",
		"secondary spectrum [unit=magnitude;blockId=7;port=0;channel=1]",
	} {
		if records[0][index] != expected {
			t.Fatalf("mixed CSV header %d = %q, want %q", index, records[0][index], expected)
		}
	}
	if got := records[3]; got[0] != "" || got[1] != "" || got[2] != "2" || got[3] != "7" || got[4] != "" || got[5] != "" {
		t.Fatalf("mixed CSV final row = %#v", got)
	}

	spectrumOnly := run
	spectrumOnly.Series = nil
	encoded, err = simulationCSVData(spectrumOnly)
	if err != nil {
		t.Fatal(err)
	}
	records, err = csv.NewReader(strings.NewReader(string(encoded))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records[0]) != 4 || strings.HasPrefix(records[0][0], "time ") ||
		!strings.HasPrefix(records[0][0], "frequency ") {
		t.Fatalf("spectrum-only CSV = %#v", records)
	}
}

func TestSimulationCSVNeutralizesFormulaLeadingNames(t *testing.T) {
	for index, name := range []string{"=formula", "+formula", "-formula", "@formula"} {
		run := studio.Simulation{
			Times: []float64{0},
			Series: []studio.Series{{
				ResultChannel: studio.ResultChannel{
					BlockID: int64(index + 1), Name: name,
				},
				Values: []float64{1},
			}},
		}
		encoded, err := simulationCSVData(run)
		if err != nil {
			t.Fatal(err)
		}
		records, err := csv.NewReader(strings.NewReader(string(encoded))).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		if got := records[0][1]; !strings.HasPrefix(got, "'"+name) {
			t.Fatalf("header for %q = %q, want spreadsheet-safe prefix", name, got)
		}
	}
}

func TestSimulationCSVRejectsMalformedSpectrumData(t *testing.T) {
	for _, spectrum := range []studio.Spectrum{
		{Frequencies: []float64{0, 1}, Magnitudes: []float64{1}},
		{Frequencies: []float64{math.NaN()}, Magnitudes: []float64{1}},
		{Frequencies: []float64{0}, Magnitudes: []float64{math.Inf(1)}},
	} {
		run := studio.Simulation{Spectra: []studio.Spectrum{spectrum}}
		if encoded, err := simulationCSVData(run); err == nil || len(encoded) != 0 {
			t.Fatalf("malformed spectrum produced %q with error %v", encoded, err)
		}
	}
}

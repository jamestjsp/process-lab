package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/jamestjsp/process-lab/internal/studio"
)

func TestWorkbenchModesUseCanonicalDocumentAndFragmentURLs(t *testing.T) {
	server, _ := openTestServer(t)

	for _, path := range []string{
		"/projects/1/flows/1",
		"/projects/1/flows/1?view=unknown",
	} {
		response := request(t, server, http.MethodGet, path, nil)
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/projects/1/flows/1?view=simulation" {
			t.Fatalf("%s = %d %q", path, response.Code, response.Header().Get("Location"))
		}
	}

	for _, mode := range []string{"simulation", "design", "dynamics", "frequency", "loop", "compare"} {
		response := request(t, server, http.MethodGet, "/projects/1/flows/1?view="+mode, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d", mode, response.Code)
		}
		body := response.Body.String()
		for _, expected := range []string{
			`data-workbench-mode="` + mode + `"`,
			`href="/projects/1/flows/1?view=` + mode + `"`,
			`hx-get="/flows/1/workbench?view=` + mode + `"`,
			`hx-push-url="/projects/1/flows/1?view=` + mode + `"`,
		} {
			if !strings.Contains(body, expected) {
				t.Errorf("%s page does not contain %q", mode, expected)
			}
		}
	}
}

func TestWorkbenchModeSurvivesHXCurrentURLFallback(t *testing.T) {
	server, _ := openTestServer(t)
	values := url.Values{"duration": {"1"}, "sample_time": {"0.1"}}
	req := httptest.NewRequest(http.MethodPost, "/flows/1/simulations", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "http://process-lab.test/projects/1/flows/1?view=frequency")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `data-workbench-mode="frequency"`) {
		t.Fatalf("frequency mode was lost: %s", response.Body.String())
	}
}

func TestCompareAllowsAStaleBaselineAfterTheEditedModelIsRerun(t *testing.T) {
	server, service := openTestServer(t)
	ctx := context.Background()
	snapshot, err := service.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Run(ctx, snapshot.Flow.ID, studio.SimulationRequest{Duration: 1, SampleTime: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	baselineID := first.LastRun.ID

	gain := findKindBlock(t, first.Blocks, "gain")
	if _, err := service.UpdateBlock(ctx, gain.ID, studio.BlockUpdate{
		Name: gain.Name, Parameters: map[string]string{"gain": "2.1"},
	}); err != nil {
		t.Fatal(err)
	}
	staleCurrent := request(t, server, http.MethodGet,
		"/projects/1/flows/1?view=compare&baseline="+strconv.FormatInt(baselineID, 10), nil)
	if !strings.Contains(staleCurrent.Body.String(), "latest simulation is stale") ||
		strings.Contains(staleCurrent.Body.String(), "Current and baseline") {
		t.Fatalf("stale current run was not blocked: %s", staleCurrent.Body.String())
	}
	if _, err := service.Run(ctx, snapshot.Flow.ID, studio.SimulationRequest{Duration: 1, SampleTime: 0.1}); err != nil {
		t.Fatal(err)
	}

	response := request(t, server, http.MethodGet,
		"/projects/1/flows/1?view=compare&baseline="+strconv.FormatInt(baselineID, 10), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{
		"Baseline predates the current model revision; comparison is historical.",
		"Current and baseline", "Difference", `stroke-dasharray="6 4"`,
		`data-plot-group="simulation-compare"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("comparison does not contain %q", expected)
		}
	}
}

func TestCompareRejectsARunOwnedByAnotherFlow(t *testing.T) {
	server, service := openTestServer(t)
	ctx := context.Background()
	current, err := service.CurrentWorkspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Run(ctx, current.Snapshot.Flow.ID, studio.SimulationRequest{Duration: 1, SampleTime: 0.1}); err != nil {
		t.Fatal(err)
	}
	copy, err := service.DuplicateFlow(ctx, current.Snapshot.Flow.ID)
	if err != nil {
		t.Fatal(err)
	}
	copyRun, err := service.Run(ctx, copy.Snapshot.Flow.ID, studio.SimulationRequest{Duration: 1, SampleTime: 0.1})
	if err != nil {
		t.Fatal(err)
	}

	response := request(t, server, http.MethodGet,
		"/projects/1/flows/1?view=compare&baseline="+strconv.FormatInt(copyRun.LastRun.ID, 10), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "selected baseline is unavailable for this flowsheet") {
		t.Fatalf("foreign run was not rejected: %s", body)
	}
	if strings.Contains(body, "Current and baseline") {
		t.Fatal("foreign run data was rendered")
	}
}

func TestEngineeringPlotMarkupExposesAccessibleControls(t *testing.T) {
	server, service := openTestServer(t)
	if _, err := service.Run(context.Background(), 1, studio.SimulationRequest{Duration: 1, SampleTime: 0.1}); err != nil {
		t.Fatal(err)
	}
	body := request(t, server, http.MethodGet, "/projects/1/flows/1?view=simulation", nil).Body.String()
	for _, expected := range []string{
		`data-engineering-plot`, `data-x-scale="linear"`, `data-chart-readout`,
		`data-chart-characteristics`, `data-chart-zoom-in`, `data-chart-zoom-out`,
		`data-chart-clear`, `data-chart-reset`, `tabindex="0"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("plot markup does not contain %q", expected)
		}
	}
}

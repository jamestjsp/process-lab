package web

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/jamestjsp/process-lab/internal/studio"
)

type workbenchMode string

const (
	workbenchModeSimulation workbenchMode = "simulation"
	workbenchModeDesign     workbenchMode = "design"
	workbenchModeDynamics   workbenchMode = "dynamics"
	workbenchModeFrequency  workbenchMode = "frequency"
	workbenchModeLoop       workbenchMode = "loop"
	workbenchModeCompare    workbenchMode = "compare"
)

var workbenchModes = []struct {
	Name  workbenchMode
	Label string
}{
	{Name: workbenchModeSimulation, Label: "Simulation"},
	{Name: workbenchModeDesign, Label: "Design"},
	{Name: workbenchModeDynamics, Label: "Dynamics"},
	{Name: workbenchModeFrequency, Label: "Frequency"},
	{Name: workbenchModeLoop, Label: "Loop"},
	{Name: workbenchModeCompare, Label: "Compare"},
}

type workbenchModeView struct {
	Name          string
	Label         string
	CanonicalPath string
	Links         []workbenchModeLinkView
	Simulation    bool
	Design        bool
	Analysis      bool
	Compare       bool
}

type workbenchModeLinkView struct {
	Name     string
	Label    string
	Href     string
	Fragment string
	Active   bool
}

type simulationRunView struct {
	ID           int64
	Created      string
	Duration     string
	SampleTime   string
	ChannelCount int
	Stale        bool
	Current      bool
	CSVPath      string
	CompareHref  string
	CompareHTMX  string
}

type simulationComparisonView struct {
	CurrentID    int64
	BaselineID   int64
	Current      string
	Baseline     string
	Message      string
	Warning      string
	Plots        []analysisPlotView
	Unmatched    []string
	MatchedCount int
}

type simulationChannelIdentity struct {
	BlockID int64
	Port    int
	Channel int
}

func parseWorkbenchMode(raw string) (workbenchMode, bool) {
	if raw == "" {
		return workbenchModeSimulation, true
	}
	mode := workbenchMode(raw)
	for _, candidate := range workbenchModes {
		if mode == candidate.Name {
			return mode, true
		}
	}
	return workbenchModeSimulation, false
}

func workbenchModeFromRequest(r *http.Request) workbenchMode {
	raw := strings.TrimSpace(r.FormValue("view"))
	if raw == "" {
		raw = currentURLQueryValue(r, "view")
	}
	mode, _ := parseWorkbenchMode(raw)
	return mode
}

func workbenchBaselineFromRequest(r *http.Request) string {
	if value := strings.TrimSpace(r.FormValue("baseline")); value != "" {
		return value
	}
	return currentURLQueryValue(r, "baseline")
}

func currentURLQueryValue(r *http.Request, name string) string {
	current := strings.TrimSpace(r.Header.Get("HX-Current-URL"))
	if current == "" {
		return ""
	}
	parsed, err := url.Parse(current)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get(name))
}

func (s *Server) requestWorkbenchView(
	r *http.Request,
	workspace studio.Workspace,
	selectedID int64,
	errorMessage string,
) (workbenchView, error) {
	view := s.newWorkbenchView(workspace, selectedID, errorMessage)
	mode := workbenchModeFromRequest(r)
	baseline := workbenchBaselineFromRequest(r)
	if err := s.populateWorkbenchMode(r.Context(), &view, mode, baseline); err != nil {
		return workbenchView{}, err
	}
	return view, nil
}

func (s *Server) populateWorkbenchMode(
	ctx context.Context,
	view *workbenchView,
	mode workbenchMode,
	baseline string,
) error {
	projectID := view.Workspace.Project.ID
	flowID := view.Snapshot.Flow.ID
	view.Mode = newWorkbenchModeView(projectID, flowID, mode, baseline)
	if mode != workbenchModeSimulation {
		view.Title = view.Mode.Label + " · " + view.Title
	}
	for index := range view.Tabs {
		view.Tabs[index].Href = workbenchDocumentPath(
			projectID, view.Tabs[index].ID, mode, "",
		)
		view.Tabs[index].Fragment = workbenchFragmentPath(
			view.Tabs[index].ID, mode, "",
		)
	}

	if view.Mode.Analysis {
		filtered := view.Analysis.Results[:0]
		for _, result := range view.Analysis.Results {
			if result.Kind == string(mode) {
				filtered = append(filtered, result)
			}
		}
		view.Analysis.Results = filtered
		view.Analysis.Stale = false
		for _, result := range filtered {
			view.Analysis.Stale = view.Analysis.Stale || result.Stale
		}
	}
	if mode != workbenchModeSimulation && mode != workbenchModeCompare {
		return nil
	}

	history, err := s.studio.SimulationHistory(ctx, flowID, 20)
	if errors.Is(err, studio.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	view.RecentRuns = simulationRunViews(projectID, flowID, history)
	if mode != workbenchModeCompare {
		return nil
	}
	view.Comparison = s.simulationComparison(ctx, flowID, history, baseline)
	return nil
}

func newWorkbenchModeView(
	projectID int64,
	flowID int64,
	mode workbenchMode,
	baseline string,
) workbenchModeView {
	view := workbenchModeView{
		Name:          string(mode),
		CanonicalPath: workbenchDocumentPath(projectID, flowID, mode, baseline),
		Simulation:    mode == workbenchModeSimulation,
		Design:        mode == workbenchModeDesign,
		Analysis: mode == workbenchModeDynamics ||
			mode == workbenchModeFrequency || mode == workbenchModeLoop,
		Compare: mode == workbenchModeCompare,
	}
	for _, candidate := range workbenchModes {
		candidateBaseline := ""
		if candidate.Name == workbenchModeCompare && mode == workbenchModeCompare {
			candidateBaseline = baseline
		}
		link := workbenchModeLinkView{
			Name:     string(candidate.Name),
			Label:    candidate.Label,
			Href:     workbenchDocumentPath(projectID, flowID, candidate.Name, candidateBaseline),
			Fragment: workbenchFragmentPath(flowID, candidate.Name, candidateBaseline),
			Active:   candidate.Name == mode,
		}
		view.Links = append(view.Links, link)
		if link.Active {
			view.Label = link.Label
		}
	}
	return view
}

func workbenchDocumentPath(
	projectID int64,
	flowID int64,
	mode workbenchMode,
	baseline string,
) string {
	path := fmt.Sprintf("/projects/%d/flows/%d?view=%s", projectID, flowID, mode)
	if mode == workbenchModeCompare && baseline != "" {
		path += "&baseline=" + url.QueryEscape(baseline)
	}
	return path
}

func workbenchFragmentPath(flowID int64, mode workbenchMode, baseline string) string {
	path := fmt.Sprintf("/flows/%d/workbench?view=%s", flowID, mode)
	if mode == workbenchModeCompare && baseline != "" {
		path += "&baseline=" + url.QueryEscape(baseline)
	}
	return path
}

func simulationRunViews(
	projectID int64,
	flowID int64,
	history []studio.SimulationSummary,
) []simulationRunView {
	views := make([]simulationRunView, 0, len(history))
	for index, run := range history {
		baseline := strconv.FormatInt(run.ID, 10)
		views = append(views, simulationRunView{
			ID: run.ID, Created: run.CreatedAt.Local().Format("Jan 2, 15:04:05"),
			Duration:     fmt.Sprintf("%.4g s", run.Duration),
			SampleTime:   fmt.Sprintf("%.4g s", run.SampleTime),
			ChannelCount: run.ChannelCount, Stale: run.Stale, Current: index == 0,
			CSVPath: fmt.Sprintf("/flows/%d/simulations/%d.csv", flowID, run.ID),
			CompareHref: workbenchDocumentPath(
				projectID, flowID, workbenchModeCompare, baseline,
			),
			CompareHTMX: workbenchFragmentPath(flowID, workbenchModeCompare, baseline),
		})
	}
	return views
}

func (s *Server) simulationComparison(
	ctx context.Context,
	flowID int64,
	history []studio.SimulationSummary,
	baselineText string,
) simulationComparisonView {
	view := simulationComparisonView{}
	if len(history) == 0 {
		view.Message = "Run the model before choosing a comparison baseline."
		return view
	}
	currentSummary := history[0]
	view.CurrentID = currentSummary.ID
	view.Current = currentSummary.CreatedAt.Local().Format("Jan 2, 15:04:05")
	if currentSummary.Stale {
		view.Message = "The latest simulation is stale because the model changed. Run the current model before comparing."
		return view
	}
	if baselineText == "" {
		view.Message = "Choose a recent run as the baseline."
		return view
	}
	baselineID, err := strconv.ParseInt(baselineText, 10, 64)
	if err != nil || baselineID <= 0 {
		view.Message = "The selected baseline is not a valid simulation run."
		return view
	}
	current, err := s.studio.SimulationRun(ctx, flowID, currentSummary.ID)
	if err != nil {
		view.Message = "The latest simulation is no longer available. Run the model again."
		return view
	}
	baseline, err := s.studio.SimulationRun(ctx, flowID, baselineID)
	if errors.Is(err, studio.ErrNotFound) {
		view.Message = "The selected baseline is unavailable for this flowsheet."
		return view
	}
	if err != nil {
		view.Message = "Process Lab could not load the selected baseline."
		return view
	}
	view.BaselineID = baselineID
	view.Baseline = baseline.CreatedAt.Local().Format("Jan 2, 15:04:05")
	if baseline.Stale {
		view.Warning = "Baseline predates the current model revision; comparison is historical."
	}
	view.Plots, view.Unmatched, view.MatchedCount = compareSimulationRuns(
		current.Simulation, baseline.Simulation,
	)
	if view.MatchedCount == 0 {
		view.Message = "The selected runs have no matching block, port, and channel identities."
	}
	return view
}

func compareSimulationRuns(
	current studio.Simulation,
	baseline studio.Simulation,
) ([]analysisPlotView, []string, int) {
	baselineByChannel := make(map[simulationChannelIdentity]studio.Series, len(baseline.Series))
	for _, series := range baseline.Series {
		baselineByChannel[simulationSeriesIdentity(series)] = series
	}
	currentChannels := make(map[simulationChannelIdentity]struct{}, len(current.Series))
	overlay := make([]analysisSeries, 0, len(current.Series)*2)
	difference := make([]analysisSeries, 0, len(current.Series))
	var unmatched []string
	matched := 0
	baselineColors := []string{"#9a492c", "#17645d", "#86681f", "#304d76"}
	for index, series := range current.Series {
		identity := simulationSeriesIdentity(series)
		currentChannels[identity] = struct{}{}
		baselineSeries, ok := baselineByChannel[identity]
		if !ok {
			unmatched = append(unmatched, "Current only: "+simulationSeriesName(series))
			continue
		}
		matched++
		identityKey := fmt.Sprintf("%d:%d:%d", identity.BlockID, identity.Port, identity.Channel)
		overlay = append(overlay,
			analysisSeries{
				Name:  simulationSeriesName(series) + " · current",
				Key:   "compare:" + identityKey + ":current",
				Color: chartColors[index%len(chartColors)], X: current.Times, Y: series.Values,
			},
			analysisSeries{
				Name:  simulationSeriesName(baselineSeries) + " · baseline",
				Key:   "compare:" + identityKey + ":baseline",
				Color: baselineColors[index%len(baselineColors)], Dash: "6 4",
				X: baseline.Times, Y: baselineSeries.Values,
			},
		)
		difference = append(difference, analysisSeries{
			Name:  simulationSeriesName(series) + " · current − baseline",
			Key:   "compare:" + identityKey + ":difference",
			Color: chartColors[index%len(chartColors)], X: current.Times,
			Y: simulationDifference(
				current.Times, series.Values, baseline.Times, baselineSeries.Values,
			),
		})
	}
	for _, series := range baseline.Series {
		if _, ok := currentChannels[simulationSeriesIdentity(series)]; !ok {
			unmatched = append(unmatched, "Baseline only: "+simulationSeriesName(series))
		}
	}
	if matched == 0 {
		return nil, unmatched, 0
	}
	reference := []plotReferenceSpec{{
		Axis: plotAxisY, Value: 0, Kind: "y-zero", IncludeInDomain: true,
	}}
	plots := []analysisPlotView{
		newAnalysisPlot(engineeringPlotSpec{
			ID: "simulation-compare-overlay", GroupID: "simulation-compare",
			Title: "Current and baseline", XLabel: "time (s)", YLabel: "output",
			Rect: analysisPlotRect(), XScaleKind: plotScaleLinear, YScaleKind: plotScaleLinear,
			Series: overlay, References: reference,
		}),
		newAnalysisPlot(engineeringPlotSpec{
			ID: "simulation-compare-difference", GroupID: "simulation-compare",
			Title: "Difference", XLabel: "time (s)", YLabel: "current − baseline",
			Rect: analysisPlotRect(), XScaleKind: plotScaleLinear, YScaleKind: plotScaleLinear,
			Series: difference, References: reference,
		}),
	}
	return plots, unmatched, matched
}

func simulationSeriesIdentity(series studio.Series) simulationChannelIdentity {
	return simulationChannelIdentity{
		BlockID: series.BlockID, Port: series.Port, Channel: series.Channel,
	}
}

func simulationSeriesName(series studio.Series) string {
	if strings.TrimSpace(series.Name) != "" {
		return series.Name
	}
	return fmt.Sprintf("block %d · port %d · channel %d", series.BlockID, series.Port, series.Channel)
}

func simulationDifference(
	currentTimes []float64,
	currentValues []float64,
	baselineTimes []float64,
	baselineValues []float64,
) []float64 {
	count := min(len(currentTimes), len(currentValues))
	difference := make([]float64, count)
	for index := range count {
		baseline, ok := interpolatedSimulationValue(
			baselineTimes, baselineValues, currentTimes[index],
		)
		if !ok || !finiteViewNumber(currentValues[index]) {
			difference[index] = math.NaN()
			continue
		}
		difference[index] = currentValues[index] - baseline
	}
	return difference
}

func interpolatedSimulationValue(times, values []float64, target float64) (float64, bool) {
	count := min(len(times), len(values))
	if count == 0 || !finiteViewNumber(target) || target < times[0] || target > times[count-1] {
		return 0, false
	}
	right := sort.Search(count, func(index int) bool { return times[index] >= target })
	if right < count && times[right] == target {
		return values[right], finiteViewNumber(values[right])
	}
	if right == 0 || right == count {
		return 0, false
	}
	left := right - 1
	if !finiteViewNumber(times[left]) || !finiteViewNumber(times[right]) ||
		!finiteViewNumber(values[left]) || !finiteViewNumber(values[right]) ||
		times[right] <= times[left] {
		return 0, false
	}
	fraction := (target - times[left]) / (times[right] - times[left])
	value := values[left] + fraction*(values[right]-values[left])
	return value, finiteViewNumber(value)
}

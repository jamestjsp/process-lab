package web

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jamestjsp/process-lab/internal/studio"
)

type pageView struct {
	Workbench workbenchView
}

// registerView is the projects home — the drawing register. Every project the
// database holds arrives with the flowsheets its row expands to reveal, so
// expanding a row costs no request.
type registerView struct {
	Projects     []registerRowView
	ProjectCount int
	ProjectLabel string
	SheetCount   int
	SheetLabel   string
}

// registerRowView is one ruled line of the register.
type registerRowView struct {
	ID   int64
	Name string
	// Href is the project's own address. `GET /projects/{id}` already redirects
	// to the project's first flowsheet, so the name opens the project without
	// the register having to name a particular sheet — and it stays correct
	// when the first sheet changes.
	Href       string
	SheetCount int
	Edited     string
	Sheets     []registerSheetView
	// CanDelete carries the domain's refusal to delete the last project into
	// the interface, so the control is absent rather than present and doomed.
	CanDelete bool
	// Confirm names the project and its sheet count, which is what makes the
	// confirmation worth reading.
	Confirm string
}

// registerSheetView is one flowsheet chip under an expanded row.
type registerSheetView struct {
	// Ordinal is the sheet's place in the project's tab order, zero padded, the
	// way a drawing register numbers the sheets in a set. It is the tab strip's
	// own order, so the register and the workbench count sheets alike.
	Ordinal  string
	Name     string
	Href     string
	NeedsRun bool
}

func newRegisterView(register studio.Register) registerView {
	view := registerView{Projects: make([]registerRowView, 0, len(register.Projects))}
	// One project left is the one the domain refuses to delete.
	deletable := len(register.Projects) > 1
	for _, entry := range register.Projects {
		row := registerRowView{
			ID:         entry.Project.ID,
			Name:       entry.Project.Name,
			Href:       fmt.Sprintf("/projects/%d", entry.Project.ID),
			SheetCount: entry.FlowCount(),
			Edited:     relativeTime(entry.EditedAt),
			CanDelete:  deletable,
			Sheets:     make([]registerSheetView, 0, entry.FlowCount()),
		}
		row.Confirm = fmt.Sprintf("Delete “%s” and its %d %s? This cannot be undone.",
			row.Name, row.SheetCount, plural(row.SheetCount, "flowsheet", "flowsheets"),
		)
		for index, flow := range entry.Flows {
			row.Sheets = append(row.Sheets, registerSheetView{
				Ordinal:  fmt.Sprintf("%02d", index+1),
				Name:     flow.Name,
				Href:     fmt.Sprintf("/projects/%d/flows/%d", flow.ProjectID, flow.ID),
				NeedsRun: flow.NeedsRun,
			})
		}
		view.SheetCount += row.SheetCount
		view.Projects = append(view.Projects, row)
	}
	view.ProjectCount = len(view.Projects)
	view.ProjectLabel = plural(view.ProjectCount, "project", "projects")
	view.SheetLabel = plural(view.SheetCount, "sheet", "sheets")
	return view
}

func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}

type workbenchView struct {
	Workspace             studio.Workspace
	Snapshot              studio.Snapshot
	Blocks                []blockView
	Connections           []connectionView
	Selected              *blockView
	SelectedLinks         []inspectorLink
	Palette               []paletteItem
	Sheet                 sheetGeometry
	Tabs                  []flowTabView
	Chart                 chartView
	Analysis              analysisView
	Mode                  workbenchModeView
	RecentRuns            []simulationRunView
	Comparison            simulationComparisonView
	ControllerCandidate   *controllerCandidateView
	Error                 string
	Updated               string
	BlockCount            int
	ConnectionCount       int
	SimulationLimits      string
	SimulationMinDuration float64
	SimulationMinSample   float64
	BoundedEdit           bool

	// Title names the flowsheet the page is showing. A tab swap pushes a new
	// URL, so the title has to move with it or every history entry reads the
	// same and the change is announced to nobody. The workbench fragment
	// carries it as a root-level <title>, which htmx lifts out of a partial
	// and applies to the document.
	Title string
}

// flowTabView is one sheet in the tab strip, in the project's `position`
// order.
//
// Both addresses are built here rather than in the template because a tab
// needs both and they are not interchangeable: Fragment is the workbench
// markup htmx swaps into the page, while Href is the canonical address the
// tab pushes, links to, and hands to a user without JavaScript. Pushing
// Fragment instead would put a bare <main> with no stylesheet in the address
// bar, which is what a reader gets back if they reload or share it.
type flowTabView struct {
	ID       int64
	Name     string
	Href     string
	Fragment string
	Active   bool
	// NeedsRun is the amber dot: the model changed after its last simulation.
	// It is the same flag the simulation dock reads, so the two cannot
	// disagree about whether the sheet is current.
	NeedsRun bool
}

// sheetGeometry hands the domain's sheet constants to the client so the
// viewport, the grid, and the snap step cannot drift from the server.
type sheetGeometry struct {
	Width       int
	Height      int
	Grid        int
	BlockWidth  int
	BlockHeight int
}

type blockView struct {
	studio.Block
	Definition    studio.BlockDefinition
	Fields        []studio.ParameterField
	InputPorts    []portView
	OutputPorts   []portView
	Selected      bool
	ParameterText string
}

type portView struct {
	Index     int
	Top       int
	Center    int
	HitHeight int
	Size      int
	Label     string
	Name      string
	Width     int
	Channels  []string
}

type connectionView struct {
	studio.Connection
	SourceName   string
	TargetName   string
	SourceCenter int
	TargetCenter int
}

type inspectorLink struct {
	ID        int64
	Direction string
	OtherName string
	PortName  string
}

type paletteItem struct {
	studio.BlockDefinition
	X int
	Y int
}

type chartView struct {
	Present    bool
	Plot       engineeringPlotView
	Paths      []chartPath
	SplitPlots []trendPlotView
	YGrid      []chartGrid
	XGrid      []chartGrid
	Duration   string
	SampleTime string
	CreatedAt  string
	Metrics    []studio.Metric
	Spectra    []spectrumView
	Fidelity   fidelityView
}

type trendPlotView struct {
	Title     string
	SeriesKey string
	Plot      engineeringPlotView
	Paths     []chartPath
	YGrid     []chartGrid
	XGrid     []chartGrid
}

type fidelityView struct {
	Driver     string
	Domain     string
	BaseStep   string
	SourceHold string
	Segments   int
	Rates      []string
	Delays     []string
	Note       string
}

type chartPath struct {
	Name  string
	Key   string
	D     string
	Color string
	Dash  string
}

type chartGrid struct {
	Position float64
	Label    string
}

type spectrumView struct {
	Name          string
	D             string
	PeakFrequency string
	PeakMagnitude string
	MaxFrequency  string
}

type analysisView struct {
	Available bool
	Inputs    []analysisChannelOptionView
	Outputs   []analysisChannelOptionView
	Results   []analysisResultView
	Stale     bool
	Revision  string
}

type analysisChannelOptionView struct {
	Value    string
	Name     string
	Selected bool
}

type analysisResultView struct {
	Kind     string
	Title    string
	Created  string
	Revision string
	Channel  string
	Stale    bool
	Metrics  []analysisMetricView
	Plots    []analysisPlotView
	Notices  []string
}

type analysisMetricView struct {
	Label string
	Value string
}

type analysisPlotView struct {
	Title   string
	XLabel  string
	YLabel  string
	Plot    engineeringPlotView
	Paths   []chartPath
	Markers []analysisMarkerView
}

type engineeringPlotView struct {
	ID         string
	GroupID    string
	Rect       plotRectView
	XScale     plotScaleView
	YScale     plotScaleView
	XTicks     []plotTickView
	YTicks     []plotTickView
	References []plotReferenceView
}

type plotRectView struct {
	Left   float64
	Top    float64
	Right  float64
	Bottom float64
}

type plotScaleView struct {
	Kind      string
	DomainMin float64
	DomainMax float64
}

type plotTickView struct {
	Position float64
	Value    float64
	Label    string
}

type plotReferenceView struct {
	X1    float64
	Y1    float64
	X2    float64
	Y2    float64
	Label string
	Kind  string
}

type analysisMarkerView struct {
	X     float64
	Y     float64
	Label string
	Kind  string
}

// workbenchTitle names the sheet before the project so a browser tab, a
// history entry, and a bookmark stay distinguishable when the label is
// truncated to its first few characters.
func workbenchTitle(workspace studio.Workspace) string {
	flow := strings.TrimSpace(workspace.Snapshot.Flow.Name)
	project := strings.TrimSpace(workspace.Project.Name)
	switch {
	case flow == "" && project == "":
		return "Process Lab"
	case flow == "":
		return project + " · Process Lab"
	case project == "":
		return flow + " · Process Lab"
	}
	return flow + " · " + project + " · Process Lab"
}

func newWorkbenchView(workspace studio.Workspace, selectedID int64, errorMessage string) workbenchView {
	snapshot := workspace.Snapshot
	view := workbenchView{
		Workspace:             workspace,
		Snapshot:              snapshot,
		Title:                 workbenchTitle(workspace),
		Error:                 errorMessage,
		Updated:               relativeTime(snapshot.Flow.UpdatedAt),
		BlockCount:            len(snapshot.Blocks),
		ConnectionCount:       len(snapshot.Connections),
		SimulationLimits:      studio.SimulationLimitsText(),
		SimulationMinDuration: studio.MinSimulationDuration,
		SimulationMinSample:   studio.MinSimulationSampleTime,
		Sheet: sheetGeometry{
			Width:       studio.SheetWidth,
			Height:      studio.SheetHeight,
			Grid:        studio.GridPitch,
			BlockWidth:  studio.BlockWidth,
			BlockHeight: studio.BlockHeight,
		},
	}
	view.Mode = newWorkbenchModeView(
		workspace.Project.ID, snapshot.Flow.ID, workbenchModeSimulation, "",
	)
	for _, flow := range workspace.Flows {
		view.Tabs = append(view.Tabs, flowTabView{
			ID:       flow.ID,
			Name:     flow.Name,
			Href:     fmt.Sprintf("/projects/%d/flows/%d", flow.ProjectID, flow.ID),
			Fragment: fmt.Sprintf("/flows/%d/workbench", flow.ID),
			Active:   flow.ID == snapshot.Flow.ID,
			NeedsRun: flow.NeedsRun,
		})
	}

	for _, definition := range studio.BlockLibrary() {
		view.Palette = append(view.Palette, paletteItem{
			BlockDefinition: definition,
			X:               60,
			Y:               80,
		})
	}

	blockNames := make(map[int64]string, len(snapshot.Blocks))
	for _, block := range snapshot.Blocks {
		blockNames[block.ID] = block.Name
		item := blockView{
			Block:         block,
			Definition:    block.Kind.Definition(),
			Fields:        block.EditorFields(),
			InputPorts:    inputPortViews(block),
			OutputPorts:   outputPortViews(block),
			Selected:      block.ID == selectedID,
			ParameterText: block.Summary(),
		}
		view.Blocks = append(view.Blocks, item)
		if item.Selected {
			copy := item
			view.Selected = &copy
		}
	}

	for _, connection := range snapshot.Connections {
		source := blockByID(snapshot.Blocks, connection.SourceID)
		target := blockByID(snapshot.Blocks, connection.TargetID)
		view.Connections = append(view.Connections, connectionView{
			Connection:   connection,
			SourceName:   blockNames[connection.SourceID],
			TargetName:   blockNames[connection.TargetID],
			SourceCenter: portCenterOffset(source.OutputPortCount(), connection.SourcePort),
			TargetCenter: portCenterOffset(target.InputPortCount(), connection.TargetPort),
		})
		portName := connectionPortName(source, connection.SourcePort, target, connection.TargetPort)
		if connection.SourceID == selectedID {
			view.SelectedLinks = append(view.SelectedLinks, inspectorLink{
				ID: connection.ID, Direction: "to", OtherName: blockNames[connection.TargetID],
				PortName: portName,
			})
		}
		if connection.TargetID == selectedID {
			view.SelectedLinks = append(view.SelectedLinks, inspectorLink{
				ID: connection.ID, Direction: "from", OtherName: blockNames[connection.SourceID],
				PortName: portName,
			})
		}
	}
	view.Chart = newChartView(snapshot.LastRun)
	view.Analysis = newAnalysisView(workspace.Analysis)
	return view
}

func newAnalysisView(workspace studio.AnalysisWorkspace) analysisView {
	view := analysisView{
		Available: len(workspace.Inputs) > 0 && len(workspace.Outputs) > 0,
		Revision:  workspace.ModelUpdatedAt.Local().Format("15:04:05.000"),
	}
	for _, channel := range workspace.Inputs {
		view.Inputs = append(view.Inputs, analysisChannelOptionView{
			Value:    channelRefValue(channel.ChannelRef),
			Name:     channel.Name,
			Selected: channel.ChannelRef == workspace.SelectedInput,
		})
	}
	for _, channel := range workspace.Outputs {
		view.Outputs = append(view.Outputs, analysisChannelOptionView{
			Value:    channelRefValue(channel.ChannelRef),
			Name:     channel.Name,
			Selected: channel.ChannelRef == workspace.SelectedOutput,
		})
	}
	if workspace.Dynamics != nil {
		view.Results = append(view.Results, dynamicsResultView(*workspace.Dynamics))
		view.Stale = view.Stale || workspace.Dynamics.Stale
	}
	if workspace.Frequency != nil {
		view.Results = append(view.Results, frequencyResultView(*workspace.Frequency))
		view.Stale = view.Stale || workspace.Frequency.Stale
	}
	if workspace.Loop != nil {
		view.Results = append(view.Results, loopResultView(*workspace.Loop))
		view.Stale = view.Stale || workspace.Loop.Stale
	}
	return view
}

func dynamicsResultView(record studio.DynamicsAnalysisRecord) analysisResultView {
	result := record.Result
	view := analysisResultView{
		Kind:     "dynamics",
		Title:    "Dynamics & time",
		Created:  record.CreatedAt.Local().Format("15:04:05"),
		Revision: result.ModelUpdatedAt.Local().Format("15:04:05.000"),
		Channel:  result.Input.Name + " → " + result.Output.Name,
		Stale:    record.Stale,
	}
	if result.Stable != nil {
		value := "unstable"
		if *result.Stable {
			value = "stable"
		}
		view.Metrics = append(view.Metrics, analysisMetricView{Label: "Stability", Value: value})
	}
	if result.DCGain != nil {
		view.Metrics = append(view.Metrics, analysisMetricView{
			Label: "DC gain", Value: formatAnalysisNumber(*result.DCGain),
		})
	}
	view.Metrics = append(view.Metrics,
		analysisMetricView{Label: "Poles", Value: fmt.Sprintf("%d", len(result.Poles))},
		analysisMetricView{Label: "Zeros", Value: fmt.Sprintf("%d", len(result.Zeros))},
	)
	if result.StepExperiment != nil {
		step := result.StepExperiment
		references := make([]plotReferenceSpec, 0, 3)
		if step.Metrics.SteadyStateValue != nil {
			references = append(references, plotReferenceSpec{
				Axis: plotAxisY, Value: *step.Metrics.SteadyStateValue,
				Label: "steady state", Kind: "steady-state", IncludeInDomain: true,
			})
		}
		if step.Metrics.RiseTime != nil {
			references = append(references, plotReferenceSpec{
				Axis: plotAxisX, Value: *step.Metrics.RiseTime,
				Label: "rise time", Kind: "rise-time",
			})
		}
		if step.Metrics.SettlingTime != nil {
			references = append(references, plotReferenceSpec{
				Axis: plotAxisX, Value: *step.Metrics.SettlingTime,
				Label: "settling time", Kind: "settling-time",
			})
		}
		view.Plots = append(view.Plots, newAnalysisPlot(engineeringPlotSpec{
			ID:    "analysis-dynamics-step",
			Title: "Step response", XLabel: "time (s)", YLabel: "output",
			Rect: analysisPlotRect(), XScaleKind: plotScaleLinear, YScaleKind: plotScaleLinear,
			Series: []analysisSeries{{
				Name: "step", Color: "#e17845",
				X: step.Times, Y: step.Values,
			}},
			References: references,
		}))
		if step.Metrics.RiseTime != nil {
			view.Metrics = append(view.Metrics, analysisMetricView{
				Label: "Rise time", Value: formatAnalysisNumber(*step.Metrics.RiseTime) + " s",
			})
		}
		if step.Metrics.SettlingTime != nil {
			view.Metrics = append(view.Metrics, analysisMetricView{
				Label: "Settling", Value: formatAnalysisNumber(*step.Metrics.SettlingTime) + " s",
			})
		}
	}
	if len(result.Poles) > 0 || len(result.Zeros) > 0 {
		var markers []analysisPoint
		for _, pole := range result.Poles {
			markers = append(markers, analysisPoint{
				X: pole.Real, Y: pole.Imag, Label: "×", Kind: "pole",
			})
		}
		for _, zero := range result.Zeros {
			markers = append(markers, analysisPoint{
				X: zero.Real, Y: zero.Imag, Label: "○", Kind: "zero",
			})
		}
		view.Plots = append(view.Plots, newAnalysisPlot(engineeringPlotSpec{
			ID:    "analysis-dynamics-pole-zero",
			Title: "Pole-zero map", XLabel: "real", YLabel: "imaginary",
			Rect: analysisPlotRect(), XScaleKind: plotScaleLinear, YScaleKind: plotScaleLinear,
			Points: markers,
			References: []plotReferenceSpec{{
				Axis: plotAxisX, Value: 0, Label: "stability boundary",
				Kind: "stability-boundary", IncludeInDomain: true,
			}},
		}))
	}
	for _, issue := range result.Issues {
		view.Notices = append(view.Notices, issue.Operation+": "+issue.Message)
	}
	return view
}

func frequencyResultView(record studio.FrequencyAnalysisRecord) analysisResultView {
	result := record.Result
	view := analysisResultView{
		Kind:     "frequency",
		Title:    "Frequency response",
		Created:  record.CreatedAt.Local().Format("15:04:05"),
		Revision: result.ModelUpdatedAt.Local().Format("15:04:05.000"),
		Stale:    record.Stale,
	}
	if len(result.Inputs) > 0 && len(result.Outputs) > 0 {
		if len(result.Inputs) == 1 && len(result.Outputs) == 1 {
			view.Channel = result.Inputs[0].Name + " → " + result.Outputs[0].Name
		} else {
			view.Channel = fmt.Sprintf(
				"%d named inputs → %d named outputs",
				len(result.Inputs), len(result.Outputs),
			)
		}
	}
	view.Metrics = append(view.Metrics,
		analysisMetricView{Label: "Grid", Value: fmt.Sprintf("%d points", len(result.Grid.Omega))},
		analysisMetricView{Label: "Frequency", Value: result.Units.Frequency},
		analysisMetricView{Label: "Magnitude", Value: result.Units.Magnitude},
	)
	if len(result.Bode) > 0 {
		magnitudeSeries := make([]analysisSeries, 0, len(result.Bode))
		phaseSeries := make([]analysisSeries, 0, len(result.Bode))
		for index, trace := range result.Bode {
			if trace.InputIndex < 0 || trace.InputIndex >= len(result.Inputs) ||
				trace.OutputIndex < 0 || trace.OutputIndex >= len(result.Outputs) {
				continue
			}
			input := result.Inputs[trace.InputIndex]
			output := result.Outputs[trace.OutputIndex]
			name := output.Name + " ← " + input.Name
			key := fmt.Sprintf(
				"frequency:%d:%d:%d:%d:%d:%d",
				output.BlockID, output.Port, output.Channel,
				input.BlockID, input.Port, input.Channel,
			)
			color := chartColors[index%len(chartColors)]
			x := result.Grid.Omega
			magnitudeSeries = append(magnitudeSeries, analysisSeries{
				Name: name, Key: key, Color: color, X: x,
				Y: pointerValues(trace.MagnitudeDB),
			})
			phaseSeries = append(phaseSeries, analysisSeries{
				Name: name, Key: key, Color: color, X: x,
				Y: pointerValues(trace.PhaseDegrees),
			})
		}
		view.Plots = append(view.Plots,
			newAnalysisPlot(engineeringPlotSpec{
				ID: "analysis-frequency-bode-magnitude", GroupID: "analysis-frequency-bode",
				Title: "Bode magnitude", XLabel: "ω (rad/s)", YLabel: "dB",
				Rect: analysisPlotRect(), XScaleKind: plotScaleLog10, YScaleKind: plotScaleLinear,
				Series: magnitudeSeries,
				References: []plotReferenceSpec{{
					Axis: plotAxisY, Value: 0, Label: "0 dB",
					Kind: "magnitude-zero", IncludeInDomain: true,
				}},
			}),
			newAnalysisPlot(engineeringPlotSpec{
				ID: "analysis-frequency-bode-phase", GroupID: "analysis-frequency-bode",
				Title: "Bode phase", XLabel: "ω (rad/s)", YLabel: "degrees",
				Rect: analysisPlotRect(), XScaleKind: plotScaleLog10, YScaleKind: plotScaleLinear,
				Series: phaseSeries,
				References: []plotReferenceSpec{{
					Axis: plotAxisY, Value: -180, Label: "−180°",
					Kind: "phase-critical", IncludeInDomain: true,
				}},
			}),
		)
	}
	if result.Nyquist != nil {
		x, y := complexSampleValues(result.Nyquist.Positive)
		view.Plots = append(view.Plots, newAnalysisPlot(engineeringPlotSpec{
			ID:    "analysis-frequency-nyquist",
			Title: "Nyquist", XLabel: "real", YLabel: "imaginary",
			Rect: analysisPlotRect(), XScaleKind: plotScaleLinear, YScaleKind: plotScaleLinear,
			Series: []analysisSeries{{
				Name: "Nyquist", Key: "frequency:nyquist", Color: "#2a8f83", X: x, Y: y,
			}},
			Points: []analysisPoint{{X: -1, Y: 0, Label: "×", Kind: "critical"}},
			References: []plotReferenceSpec{{
				Axis: plotAxisX, Value: -1, Label: "−1",
				Kind: "nyquist-critical", IncludeInDomain: true,
			}},
		}))
	}
	if result.Nichols != nil {
		view.Plots = append(view.Plots, newAnalysisPlot(engineeringPlotSpec{
			ID:    "analysis-frequency-nichols",
			Title: "Nichols", XLabel: "phase (deg)", YLabel: "magnitude (dB)",
			Rect: analysisPlotRect(), XScaleKind: plotScaleLinear, YScaleKind: plotScaleLinear,
			Series: []analysisSeries{{
				Name: "Nichols", Key: "frequency:nichols", Color: "#c9a13b",
				X: pointerValues(result.Nichols.PhaseDegrees),
				Y: pointerValues(result.Nichols.MagnitudeDB),
			}},
			References: []plotReferenceSpec{
				{Axis: plotAxisX, Value: -180, Label: "−180°", Kind: "phase-critical", IncludeInDomain: true},
				{Axis: plotAxisY, Value: 0, Label: "0 dB", Kind: "magnitude-zero", IncludeInDomain: true},
			},
		}))
	}
	if result.SingularValues != nil {
		var series []analysisSeries
		for index, values := range result.SingularValues.Values {
			series = append(series, analysisSeries{
				Name:  fmt.Sprintf("σ%d", index+1),
				Key:   fmt.Sprintf("frequency:sigma:%d", index+1),
				Color: chartColors[index%len(chartColors)],
				X:     result.Grid.Omega,
				Y:     pointerValues(values),
			})
		}
		view.Plots = append(view.Plots, newAnalysisPlot(engineeringPlotSpec{
			ID:    "analysis-frequency-singular-values",
			Title: "Singular values", XLabel: "ω (rad/s)", YLabel: "absolute gain",
			Rect: analysisPlotRect(), XScaleKind: plotScaleLog10, YScaleKind: plotScaleLinear,
			Series: series,
		}))
	}
	for _, issue := range result.Issues {
		view.Notices = append(view.Notices, issue.Operation+": "+issue.Message)
	}
	return view
}

func loopResultView(record studio.LoopAnalysisRecord) analysisResultView {
	result := record.Result
	view := analysisResultView{
		Kind:     "loop",
		Title:    "Loop robustness",
		Created:  record.CreatedAt.Local().Format("15:04:05"),
		Revision: result.ModelUpdatedAt.Local().Format("15:04:05.000"),
		Channel:  result.Input.Name + " → " + result.Output.Name,
		Stale:    record.Stale,
		Metrics: []analysisMetricView{
			{Label: "Basis", Value: "explicit SISO"},
			{Label: "Domain", Value: result.Domain},
		},
	}
	if result.Margins != nil {
		view.Metrics = append(view.Metrics,
			analysisMetricView{
				Label: "Gain margin", Value: formatOptionalAnalysisNumber(result.Margins.GainMarginDB, "unbounded", " dB"),
			},
			analysisMetricView{
				Label: "Phase margin", Value: formatOptionalAnalysisNumber(result.Margins.PhaseMarginDegrees, "unbounded", "°"),
			},
		)
	}
	if result.Bandwidth != nil {
		value := formatOptionalAnalysisNumber(result.Bandwidth.RadPerSecond, "unbounded", " rad/s")
		view.Metrics = append(view.Metrics, analysisMetricView{Label: "Bandwidth", Value: value})
	}
	if result.DiskMargin != nil {
		view.Metrics = append(view.Metrics, analysisMetricView{
			Label: "Peak sensitivity",
			Value: formatOptionalAnalysisNumber(result.DiskMargin.PeakSensitivity, "undefined", ""),
		})
	}
	if result.Passivity != nil {
		view.Metrics = append(view.Metrics, analysisMetricView{
			Label: "Passivity evidence", Value: result.Passivity.Status,
		})
	}
	if result.RootLocus != nil {
		var series []analysisSeries
		for index, branch := range result.RootLocus.Branches {
			x := make([]float64, len(branch))
			y := make([]float64, len(branch))
			for sample, value := range branch {
				x[sample], y[sample] = value.Real, value.Imag
			}
			series = append(series, analysisSeries{
				Name:  fmt.Sprintf("branch %d", index+1),
				Key:   fmt.Sprintf("loop:root-locus:%d", index+1),
				Color: chartColors[index%len(chartColors)],
				X:     x, Y: y,
			})
		}
		view.Plots = append(view.Plots, newAnalysisPlot(engineeringPlotSpec{
			ID: "analysis-loop-root-locus", GroupID: "analysis-loop",
			Title: "Root locus", XLabel: "real", YLabel: "imaginary",
			Rect: analysisPlotRect(), XScaleKind: plotScaleLinear, YScaleKind: plotScaleLinear,
			Series: series,
			References: []plotReferenceSpec{{
				Axis: plotAxisX, Value: 0, Label: "stability boundary",
				Kind: "stability-boundary", IncludeInDomain: true,
			}},
		}))
	}
	for _, applicability := range result.Applicability {
		if applicability.Status != "available" {
			view.Notices = append(view.Notices,
				applicability.Operation+": "+applicability.Detail,
			)
		}
	}
	return view
}

const analysisPlotWidth = 400.0
const analysisPlotHeight = 140.0

const (
	plotScaleLinear = "linear"
	plotScaleLog10  = "log10"
	plotAxisX       = "x"
	plotAxisY       = "y"
)

var chartColors = []string{"#e17845", "#2a8f83", "#c9a13b", "#5277a8"}

type analysisSeries struct {
	Name  string
	Key   string
	Color string
	Dash  string
	X     []float64
	Y     []float64
}

type analysisPoint struct {
	X     float64
	Y     float64
	Label string
	Kind  string
}

type plotReferenceSpec struct {
	Axis            string
	Value           float64
	Label           string
	Kind            string
	IncludeInDomain bool
}

type engineeringPlotSpec struct {
	ID         string
	GroupID    string
	Title      string
	XLabel     string
	YLabel     string
	Rect       plotRectView
	XScaleKind string
	YScaleKind string
	Series     []analysisSeries
	Points     []analysisPoint
	References []plotReferenceSpec
}

type engineeringPlotResult struct {
	View    engineeringPlotView
	Paths   []chartPath
	Markers []analysisMarkerView
}

type plotAxis struct {
	view       plotScaleView
	pixelStart float64
	pixelEnd   float64
	ticks      []plotTickView
}

type plotRange struct {
	minimum float64
	maximum float64
	present bool
}

func (bounds *plotRange) add(value float64) {
	if !finiteViewNumber(value) {
		return
	}
	if !bounds.present {
		bounds.minimum = value
		bounds.maximum = value
		bounds.present = true
		return
	}
	bounds.minimum = math.Min(bounds.minimum, value)
	bounds.maximum = math.Max(bounds.maximum, value)
}

func analysisLinePlot(
	title string,
	xLabel string,
	yLabel string,
	series []analysisSeries,
	points []analysisPoint,
) analysisPlotView {
	return newAnalysisPlot(engineeringPlotSpec{
		ID: stablePlotID(title), Title: title, XLabel: xLabel, YLabel: yLabel,
		Rect:       analysisPlotRect(),
		XScaleKind: plotScaleLinear,
		YScaleKind: plotScaleLinear,
		Series:     series,
		Points:     points,
	})
}

func analysisPlotRect() plotRectView {
	return plotRectView{Left: 54, Top: 10, Right: analysisPlotWidth - 24, Bottom: analysisPlotHeight - 16}
}

func newAnalysisPlot(spec engineeringPlotSpec) analysisPlotView {
	result := buildEngineeringPlot(spec)
	return analysisPlotView{
		Title: spec.Title, XLabel: spec.XLabel, YLabel: spec.YLabel,
		Plot: result.View, Paths: result.Paths, Markers: result.Markers,
	}
}

func buildEngineeringPlot(spec engineeringPlotSpec) engineeringPlotResult {
	if spec.Rect == (plotRectView{}) {
		spec.Rect = analysisPlotRect()
	}
	if spec.XScaleKind == "" {
		spec.XScaleKind = plotScaleLinear
	}
	if spec.YScaleKind == "" {
		spec.YScaleKind = plotScaleLinear
	}
	if spec.GroupID == "" {
		spec.GroupID = spec.ID
	}
	result := engineeringPlotResult{View: engineeringPlotView{
		ID: spec.ID, GroupID: spec.GroupID, Rect: spec.Rect,
	}}

	var xBounds, yBounds plotRange
	add := func(x, y float64) {
		if !validPlotValue(spec.XScaleKind, x) || !validPlotValue(spec.YScaleKind, y) {
			return
		}
		xBounds.add(x)
		yBounds.add(y)
	}
	for _, values := range spec.Series {
		for index := 0; index < len(values.X) && index < len(values.Y); index++ {
			add(values.X[index], values.Y[index])
		}
	}
	for _, point := range spec.Points {
		add(point.X, point.Y)
	}
	if !xBounds.present || !yBounds.present {
		return result
	}
	for _, reference := range spec.References {
		if !reference.IncludeInDomain {
			continue
		}
		switch reference.Axis {
		case plotAxisX:
			if validPlotValue(spec.XScaleKind, reference.Value) {
				xBounds.add(reference.Value)
			}
		case plotAxisY:
			if validPlotValue(spec.YScaleKind, reference.Value) {
				yBounds.add(reference.Value)
			}
		}
	}

	xAxis, okX := newPlotAxis(
		spec.XScaleKind, xBounds.minimum, xBounds.maximum,
		spec.Rect.Left, spec.Rect.Right,
	)
	yAxis, okY := newPlotAxis(
		spec.YScaleKind, yBounds.minimum, yBounds.maximum,
		spec.Rect.Bottom, spec.Rect.Top,
	)
	if !okX || !okY {
		return result
	}
	result.View.XScale = xAxis.view
	result.View.YScale = yAxis.view
	result.View.XTicks = xAxis.ticks
	result.View.YTicks = yAxis.ticks

	for index, values := range spec.Series {
		var path strings.Builder
		started := false
		for sample := 0; sample < len(values.X) && sample < len(values.Y); sample++ {
			x, xOK := xAxis.position(values.X[sample])
			y, yOK := yAxis.position(values.Y[sample])
			if !xOK || !yOK {
				started = false
				continue
			}
			command := "L"
			if !started {
				command = "M"
				started = true
			}
			fmt.Fprintf(&path, "%s %.2f %.2f ", command, x, y)
		}
		if path.Len() == 0 {
			continue
		}
		key := values.Key
		if key == "" {
			key = fmt.Sprintf("%s:series:%d", spec.ID, index)
		}
		result.Paths = append(result.Paths, chartPath{
			Name: values.Name, Key: key,
			D: strings.TrimSpace(path.String()), Color: values.Color, Dash: values.Dash,
		})
	}
	for _, point := range spec.Points {
		x, xOK := xAxis.position(point.X)
		y, yOK := yAxis.position(point.Y)
		if !xOK || !yOK {
			continue
		}
		result.Markers = append(result.Markers, analysisMarkerView{
			X: x, Y: y, Label: point.Label, Kind: point.Kind,
		})
	}

	references := append([]plotReferenceSpec(nil), spec.References...)
	if spec.XScaleKind == plotScaleLinear &&
		xAxis.contains(0) && !hasPlotReference(references, plotAxisX, 0) {
		references = append(references, plotReferenceSpec{Axis: plotAxisX, Kind: "x-zero"})
	}
	if spec.YScaleKind == plotScaleLinear &&
		yAxis.contains(0) && !hasPlotReference(references, plotAxisY, 0) {
		references = append(references, plotReferenceSpec{Axis: plotAxisY, Kind: "y-zero"})
	}
	for _, reference := range references {
		switch reference.Axis {
		case plotAxisX:
			position, ok := xAxis.position(reference.Value)
			if ok {
				result.View.References = append(result.View.References, plotReferenceView{
					X1: position, Y1: spec.Rect.Top, X2: position, Y2: spec.Rect.Bottom,
					Label: reference.Label, Kind: reference.Kind,
				})
			}
		case plotAxisY:
			position, ok := yAxis.position(reference.Value)
			if ok {
				result.View.References = append(result.View.References, plotReferenceView{
					X1: spec.Rect.Left, Y1: position, X2: spec.Rect.Right, Y2: position,
					Label: reference.Label, Kind: reference.Kind,
				})
			}
		}
	}
	return result
}

func newPlotAxis(kind string, minimum, maximum, pixelStart, pixelEnd float64) (plotAxis, bool) {
	if !validPlotValue(kind, minimum) || !validPlotValue(kind, maximum) || minimum > maximum ||
		!finiteViewNumber(pixelStart) || !finiteViewNumber(pixelEnd) || pixelStart == pixelEnd {
		return plotAxis{}, false
	}
	var domainMin, domainMax float64
	var values []float64
	switch kind {
	case plotScaleLinear:
		domainMin, domainMax, values = linearPlotTicks(minimum, maximum)
	case plotScaleLog10:
		domainMin, domainMax, values = logarithmicPlotTicks(minimum, maximum)
	default:
		return plotAxis{}, false
	}
	if !validPlotValue(kind, domainMin) || !validPlotValue(kind, domainMax) || domainMin >= domainMax {
		return plotAxis{}, false
	}
	axis := plotAxis{
		view:       plotScaleView{Kind: kind, DomainMin: domainMin, DomainMax: domainMax},
		pixelStart: pixelStart, pixelEnd: pixelEnd,
	}
	for _, value := range values {
		position, ok := axis.position(value)
		if !ok {
			continue
		}
		axis.ticks = append(axis.ticks, plotTickView{
			Position: position, Value: value, Label: formatPlotTick(value),
		})
	}
	return axis, len(axis.ticks) > 0
}

func linearPlotTicks(minimum, maximum float64) (float64, float64, []float64) {
	if minimum == maximum {
		value := minimum
		switch {
		case value == 0:
			minimum, maximum = -1, 1
		case value > 0:
			minimum = value * 0.9
			maximum = value * 1.1
			if !finiteViewNumber(maximum) || maximum <= minimum {
				maximum = value
			}
		case value < 0:
			minimum = value * 1.1
			maximum = value * 0.9
			if !finiteViewNumber(minimum) || maximum <= minimum {
				minimum = value
			}
		}
		if minimum >= maximum {
			if value > 0 {
				minimum, maximum = 0, value
			} else {
				minimum, maximum = value, 0
			}
		}
	}
	span := maximum - minimum
	if !finiteViewNumber(span) || span <= 0 {
		values := []float64{minimum}
		if minimum < 0 && maximum > 0 {
			values = append(values, 0)
		}
		values = append(values, maximum)
		return minimum, maximum, values
	}
	step := nicePlotStep(span / 5)
	if !finiteViewNumber(step) || step <= 0 {
		return minimum, maximum, []float64{minimum, maximum}
	}
	padding := span * 0.05
	paddedMin, paddedMax := minimum, maximum
	if minimum != 0 && finiteViewNumber(minimum-padding) {
		paddedMin -= padding
	}
	if maximum != 0 && finiteViewNumber(maximum+padding) {
		paddedMax += padding
	}
	domainMin := math.Floor(paddedMin/step) * step
	domainMax := math.Ceil(paddedMax/step) * step
	if !finiteViewNumber(domainMin) || !finiteViewNumber(domainMax) || domainMin >= domainMax {
		return minimum, maximum, []float64{minimum, maximum}
	}
	count := int(math.Round((domainMax-domainMin)/step)) + 1
	if count < 2 || count > 100 {
		return domainMin, domainMax, []float64{domainMin, domainMax}
	}
	values := make([]float64, 0, count)
	for index := 0; index < count; index++ {
		value := domainMin + float64(index)*step
		if math.Abs(value) < step*1e-10 {
			value = 0
		}
		values = append(values, value)
	}
	return domainMin, domainMax, values
}

func nicePlotStep(value float64) float64 {
	if !finiteViewNumber(value) || value <= 0 {
		return 0
	}
	exponent := math.Floor(math.Log10(value))
	power := math.Pow(10, exponent)
	fraction := value / power
	var nice float64
	switch {
	case fraction <= 1:
		nice = 1
	case fraction <= 2:
		nice = 2
	case fraction <= 5:
		nice = 5
	default:
		nice = 10
	}
	return nice * power
}

func logarithmicPlotTicks(minimum, maximum float64) (float64, float64, []float64) {
	minimumExponent := int(math.Floor(math.Log10(minimum)))
	maximumExponent := int(math.Ceil(math.Log10(maximum)))
	if minimumExponent == maximumExponent {
		minimumExponent--
		maximumExponent++
	}
	domainMin := math.Pow(10, float64(minimumExponent))
	domainMax := math.Pow(10, float64(maximumExponent))
	if !validPlotValue(plotScaleLog10, domainMin) {
		domainMin = minimum
	}
	if !validPlotValue(plotScaleLog10, domainMax) {
		domainMax = maximum
	}
	decades := maximumExponent - minimumExponent
	stride := max(1, int(math.Ceil(float64(decades)/10)))
	values := make([]float64, 0, decades/stride+2)
	for exponent := minimumExponent; exponent <= maximumExponent; exponent += stride {
		value := math.Pow(10, float64(exponent))
		if validPlotValue(plotScaleLog10, value) && value >= domainMin && value <= domainMax {
			values = append(values, value)
		}
	}
	if len(values) == 0 || values[len(values)-1] != domainMax {
		values = append(values, domainMax)
	}
	return domainMin, domainMax, values
}

func (axis plotAxis) position(value float64) (float64, bool) {
	if !validPlotValue(axis.view.Kind, value) || !axis.contains(value) {
		return 0, false
	}
	var fraction float64
	switch axis.view.Kind {
	case plotScaleLinear:
		span := axis.view.DomainMax - axis.view.DomainMin
		if finiteViewNumber(span) {
			fraction = (value - axis.view.DomainMin) / span
		} else {
			scale := math.Max(math.Abs(axis.view.DomainMin), math.Abs(axis.view.DomainMax))
			fraction = (value/scale - axis.view.DomainMin/scale) /
				(axis.view.DomainMax/scale - axis.view.DomainMin/scale)
		}
	case plotScaleLog10:
		minimum := math.Log10(axis.view.DomainMin)
		fraction = (math.Log10(value) - minimum) / (math.Log10(axis.view.DomainMax) - minimum)
	default:
		return 0, false
	}
	position := axis.pixelStart + fraction*(axis.pixelEnd-axis.pixelStart)
	return position, finiteViewNumber(position)
}

func (axis plotAxis) contains(value float64) bool {
	return value >= axis.view.DomainMin && value <= axis.view.DomainMax
}

func validPlotValue(kind string, value float64) bool {
	if !finiteViewNumber(value) {
		return false
	}
	return kind != plotScaleLog10 || value > 0
}

func hasPlotReference(references []plotReferenceSpec, axis string, value float64) bool {
	for _, reference := range references {
		if reference.Axis == axis && reference.Value == value {
			return true
		}
	}
	return false
}

func formatPlotTick(value float64) string {
	if value == 0 {
		return "0"
	}
	return fmt.Sprintf("%.4g", value)
}

func stablePlotID(title string) string {
	var id strings.Builder
	separator := false
	for _, character := range strings.ToLower(title) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && id.Len() > 0 {
				id.WriteByte('-')
			}
			id.WriteRune(character)
			separator = false
		} else {
			separator = true
		}
	}
	if id.Len() == 0 {
		return "plot"
	}
	return id.String()
}

func complexSampleValues(values []studio.ComplexSample) ([]float64, []float64) {
	x := make([]float64, len(values))
	y := make([]float64, len(values))
	for i, value := range values {
		x[i], y[i] = math.NaN(), math.NaN()
		if value.Real != nil && value.Imag != nil {
			x[i], y[i] = *value.Real, *value.Imag
		}
	}
	return x, y
}

func pointerValues(values []*float64) []float64 {
	result := make([]float64, len(values))
	for i, value := range values {
		result[i] = math.NaN()
		if value != nil {
			result[i] = *value
		}
	}
	return result
}

func finiteViewNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func channelRefValue(ref studio.ChannelRef) string {
	return fmt.Sprintf("%d:%d:%d", ref.BlockID, ref.Port, ref.Channel)
}

func formatAnalysisNumber(value float64) string {
	return fmt.Sprintf("%.4g", value)
}

func formatOptionalAnalysisNumber(value *float64, fallback, suffix string) string {
	if value == nil {
		return fallback
	}
	return formatAnalysisNumber(*value) + suffix
}

func blockByID(blocks []studio.Block, id int64) studio.Block {
	for _, block := range blocks {
		if block.ID == id {
			return block
		}
	}
	return studio.Block{}
}

func inputPortViews(block studio.Block) []portView {
	ports := make([]portView, block.InputPortCount())
	for index := range ports {
		center := portCenterOffset(len(ports), index)
		size := portSize(len(ports))
		label := ""
		if (block.Kind == studio.BlockSum || block.Kind == studio.BlockVectorSum) &&
			index < len(block.Parameters.Signs) {
			label = string(block.Parameters.Signs[index])
		}
		schema, _ := block.InputPort(index)
		if schema.Width > 1 {
			label = fmt.Sprintf("%d", schema.Width)
		}
		ports[index] = portView{
			Index: index, Top: portTop(center, size), Center: center,
			HitHeight: portHitHeight(len(ports)), Size: size,
			Label: label, Name: inputPortName(block, index),
			Width: schema.Width, Channels: schema.Channels,
		}
	}
	return ports
}

func outputPortViews(block studio.Block) []portView {
	ports := make([]portView, block.OutputPortCount())
	for index := range ports {
		center := portCenterOffset(len(ports), index)
		size := portSize(len(ports))
		schema, _ := block.OutputPort(index)
		label := ""
		if schema.Width > 1 {
			label = fmt.Sprintf("%d", schema.Width)
		}
		ports[index] = portView{
			Index: index, Top: portTop(center, size), Center: center,
			HitHeight: portHitHeight(len(ports)), Size: size,
			Label: label, Name: outputPortName(block, index),
			Width: schema.Width, Channels: schema.Channels,
		}
	}
	return ports
}

func inputPortName(block studio.Block, port int) string {
	if (block.Kind == studio.BlockSum || block.Kind == studio.BlockVectorSum) &&
		port >= 0 && port < len(block.Parameters.Signs) {
		return fmt.Sprintf("input %s (port %d)", string(block.Parameters.Signs[port]), port+1)
	}
	return portName("input", block, port)
}

func outputPortName(block studio.Block, port int) string {
	return portName("output", block, port)
}

func portName(direction string, block studio.Block, port int) string {
	var (
		schema studio.SignalPort
		ok     bool
	)
	if direction == "input" {
		schema, ok = block.InputPort(port)
	} else {
		schema, ok = block.OutputPort(port)
	}
	if !ok || schema.Width == 1 {
		return fmt.Sprintf("%s port %d", direction, port+1)
	}
	return fmt.Sprintf(
		"%s port %d (%d channels: %s)",
		direction, port+1, schema.Width, strings.Join(schema.Channels, ", "),
	)
}

func connectionPortName(source studio.Block, sourcePort int, target studio.Block, targetPort int) string {
	return fmt.Sprintf("%s ← %s",
		inputPortName(target, targetPort),
		outputPortName(source, sourcePort),
	)
}

func portCenterOffset(count, index int) int {
	if count <= 0 || index < 0 || index >= count {
		return studio.BlockHeight / 2
	}
	return int(math.Round(float64(studio.BlockHeight) * float64(index+1) / float64(count+1)))
}

func portHitHeight(count int) int {
	if count <= 1 {
		return studio.BlockHeight
	}
	return max(1, studio.BlockHeight/(count+1))
}

func portSize(count int) int {
	return min(14, portHitHeight(count))
}

func portTop(center, size int) int {
	if size == 14 {
		return center - 8
	}
	return center - size/2
}

func newChartView(run *studio.Simulation) chartView {
	if run == nil || len(run.Times) == 0 || len(run.Series) == 0 && len(run.Spectra) == 0 {
		return chartView{}
	}
	const (
		width  = 780.0
		height = 228.0
		left   = 48.0
		right  = 18.0
		top    = 18.0
		bottom = 32.0
	)
	colors := []string{"#e17845", "#2a8f83", "#c9a13b", "#5277a8"}

	view := chartView{
		Present:    true,
		Duration:   fmt.Sprintf("%.1f", run.Duration),
		SampleTime: fmt.Sprintf("%.3f", run.SampleTime),
		CreatedAt:  run.CreatedAt.Local().Format("15:04:05"),
		Metrics:    run.Metrics,
		Fidelity:   newFidelityView(run.Fidelity, run.SampleTime),
	}
	if len(run.Series) > 0 {
		seriesValues := make([]analysisSeries, 0, len(run.Series))
		for index, series := range run.Series {
			seriesValues = append(seriesValues, analysisSeries{
				Name: series.Name,
				Key: fmt.Sprintf(
					"%d:%d:%d", series.BlockID, series.Port, series.Channel,
				),
				X: run.Times, Y: series.Values, Color: colors[index%len(colors)],
			})
		}
		plot := buildEngineeringPlot(engineeringPlotSpec{
			ID: "simulation-trend", GroupID: "simulation",
			Rect:       plotRectView{Left: left, Top: top, Right: width - right, Bottom: height - bottom},
			XScaleKind: plotScaleLinear, YScaleKind: plotScaleLinear,
			Series: seriesValues,
			References: []plotReferenceSpec{{
				Axis: plotAxisY, Value: 0, Kind: "y-zero", IncludeInDomain: true,
			}},
		})
		view.Plot = plot.View
		view.Paths = plot.Paths
		for _, tick := range plot.View.YTicks {
			view.YGrid = append(view.YGrid, chartGrid{
				Position: tick.Position,
				Label:    tick.Label,
			})
		}
		for _, tick := range plot.View.XTicks {
			view.XGrid = append(view.XGrid, chartGrid{
				Position: tick.Position,
				Label:    tick.Label,
			})
		}
		for index, series := range seriesValues {
			const splitHeight = 180.0
			split := buildEngineeringPlot(engineeringPlotSpec{
				ID: fmt.Sprintf(
					"simulation-trend-%d-%d-%d",
					run.Series[index].BlockID, run.Series[index].Port, run.Series[index].Channel,
				),
				GroupID: "simulation",
				Rect: plotRectView{
					Left: left, Top: top, Right: width - right, Bottom: splitHeight - bottom,
				},
				XScaleKind: plotScaleLinear, YScaleKind: plotScaleLinear,
				Series: []analysisSeries{series},
				References: []plotReferenceSpec{{
					Axis: plotAxisY, Value: 0, Kind: "y-zero", IncludeInDomain: true,
				}},
			})
			panel := trendPlotView{
				Title: series.Name, SeriesKey: series.Key,
				Plot: split.View, Paths: split.Paths,
			}
			for _, tick := range split.View.YTicks {
				panel.YGrid = append(panel.YGrid, chartGrid{
					Position: tick.Position, Label: tick.Label,
				})
			}
			for _, tick := range split.View.XTicks {
				panel.XGrid = append(panel.XGrid, chartGrid{
					Position: tick.Position, Label: tick.Label,
				})
			}
			view.SplitPlots = append(view.SplitPlots, panel)
		}
	}
	for _, spectrum := range run.Spectra {
		view.Spectra = append(view.Spectra, newSpectrumView(spectrum))
	}
	return view
}

func newFidelityView(fidelity studio.Fidelity, fallbackBaseStep float64) fidelityView {
	if fidelity.BaseStep == 0 {
		fidelity.BaseStep = fallbackBaseStep
	}
	if fidelity.ModelDomain == "" {
		fidelity.ModelDomain = "continuous"
	}
	if fidelity.SegmentCount == 0 {
		fidelity.SegmentCount = 1
	}
	if fidelity.Driver == "" {
		fidelity.Driver = "batch-lsim"
	}
	if fidelity.SourceHold == "" {
		fidelity.SourceHold = "piecewise-constant"
	}
	view := fidelityView{
		Driver:     fidelity.Driver,
		Domain:     fidelity.ModelDomain,
		BaseStep:   fmt.Sprintf("%.3g s", fidelity.BaseStep),
		SourceHold: strings.ReplaceAll(fidelity.SourceHold, "-", " "),
		Segments:   fidelity.SegmentCount,
	}
	switch fidelity.Driver {
	case "batch-lsim":
		view.Driver = "Batch LTI · Lsim"
	case "delay-aware-simulate":
		view.Driver = "Delay-aware · Simulate"
	case "per-sample-simulate":
		view.Driver = "Stateful discrete · Simulate"
	}
	for _, rate := range fidelity.BlockRates {
		timing := fmt.Sprintf("%.3g s · %s", rate.SampleTime, rate.Mode)
		if rate.UpdateEvery > 1 {
			timing = fmt.Sprintf("%.3g s · every %d base steps", rate.SampleTime, rate.UpdateEvery)
		}
		view.Rates = append(view.Rates, fmt.Sprintf("%s · %s", rate.BlockName, timing))
	}
	for _, delay := range fidelity.Delays {
		switch delay.Representation {
		case "exact":
			view.Delays = append(view.Delays, fmt.Sprintf(
				"%s · exact %.3g s · aligned at %.3g s",
				delay.BlockName, delay.Delay, delay.SampleTime,
			))
		case "pade":
			view.Delays = append(view.Delays, fmt.Sprintf(
				"%s · Padé %d · %.3g s",
				delay.BlockName, delay.ApproximationOrder, delay.Delay,
			))
		case "thiran":
			view.Delays = append(view.Delays, fmt.Sprintf(
				"%s · Thiran %d · %.3g s at %.3g s",
				delay.BlockName, delay.ApproximationOrder,
				delay.Delay, delay.SampleTime,
			))
		}
	}
	switch {
	case fidelity.SourceHold == "sampled-zero-order-hold":
		view.Note = "Sampled source values are held between run points."
	case fidelity.SegmentCount > 1:
		view.Note = "Segment boundaries use a zero-order hold."
	case hasApproximateDelay(fidelity.Delays):
		view.Note = "Delay behavior includes an explicit finite-order approximation."
	case fidelity.Driver == "per-sample-simulate":
		view.Note = "controlsys carries discrete state between samples."
	default:
		view.Note = "One composed LTI segment with piecewise-constant excitation."
	}
	return view
}

func hasApproximateDelay(delays []studio.DelayProvenance) bool {
	for _, delay := range delays {
		if delay.Representation != "exact" {
			return true
		}
	}
	return false
}

func newSpectrumView(spectrum studio.Spectrum) spectrumView {
	const (
		left   = 48.0
		right  = 18.0
		top    = 18.0
		bottom = 30.0
		width  = 780.0
		height = 190.0
	)
	view := spectrumView{
		Name:          spectrum.Name,
		PeakFrequency: fmt.Sprintf("%.3g Hz", spectrum.PeakFrequency),
		PeakMagnitude: fmt.Sprintf("%.3g", spectrum.PeakMagnitude),
	}
	if len(spectrum.Frequencies) == 0 || len(spectrum.Magnitudes) == 0 {
		return view
	}
	maxFrequency := spectrum.Frequencies[len(spectrum.Frequencies)-1]
	maxMagnitude := spectrum.PeakMagnitude
	if maxFrequency <= 0 || maxMagnitude <= 0 {
		return view
	}
	view.MaxFrequency = fmt.Sprintf("%.3g Hz", maxFrequency)
	plotWidth := width - left - right
	plotHeight := height - top - bottom
	var path strings.Builder
	for i, frequency := range spectrum.Frequencies {
		x := left + frequency/maxFrequency*plotWidth
		y := top + (1-spectrum.Magnitudes[i]/maxMagnitude)*plotHeight
		if i == 0 {
			fmt.Fprintf(&path, "M %.2f %.2f", x, y)
		} else {
			fmt.Fprintf(&path, " L %.2f %.2f", x, y)
		}
	}
	view.D = path.String()
	return view
}

func relativeTime(value time.Time) string {
	if value.IsZero() {
		return "just now"
	}
	delta := time.Since(value)
	if delta < time.Minute {
		return "just now"
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	}
	return value.Local().Format("Jan 2, 15:04")
}

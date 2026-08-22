package web

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jamestjsp/process-lab/internal/studio"
)

func TestPlotLinearScaleProducesFiniteTicksForEngineeringRanges(t *testing.T) {
	tests := []struct {
		name    string
		minimum float64
		maximum float64
	}{
		{name: "constant zero", minimum: 0, maximum: 0},
		{name: "constant nonzero", minimum: 3, maximum: 3},
		{name: "positive", minimum: 0.2, maximum: 9},
		{name: "negative", minimum: -9, maximum: -0.2},
		{name: "mixed", minimum: -2, maximum: 3},
		{name: "wide", minimum: -1e120, maximum: 1e120},
		{name: "largest constant", minimum: math.MaxFloat64, maximum: math.MaxFloat64},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			axis, ok := newPlotAxis(plotScaleLinear, test.minimum, test.maximum, 20, 380)
			if !ok {
				t.Fatal("linear plot axis was not constructed")
			}
			if axis.view.Kind != plotScaleLinear ||
				!finiteViewNumber(axis.view.DomainMin) ||
				!finiteViewNumber(axis.view.DomainMax) ||
				axis.view.DomainMin >= axis.view.DomainMax ||
				axis.view.DomainMin > test.minimum ||
				axis.view.DomainMax < test.maximum {
				t.Fatalf("linear plot scale = %#v for [%g, %g]", axis.view, test.minimum, test.maximum)
			}
			if len(axis.ticks) < 2 {
				t.Fatalf("linear plot ticks = %#v", axis.ticks)
			}
			for _, tick := range axis.ticks {
				if !finiteViewNumber(tick.Position) || !finiteViewNumber(tick.Value) || tick.Label == "" {
					t.Fatalf("non-finite linear plot tick = %#v", tick)
				}
			}
		})
	}
}

func TestPlotScaleMetadataInvertsLinearAndLogarithmicCoordinates(t *testing.T) {
	tests := []struct {
		kind    string
		minimum float64
		maximum float64
		value   float64
	}{
		{kind: plotScaleLinear, minimum: -4, maximum: 8, value: 2},
		{kind: plotScaleLog10, minimum: 0.01, maximum: 100, value: 1},
	}
	for _, test := range tests {
		axis, ok := newPlotAxis(test.kind, test.minimum, test.maximum, 18, 390)
		if !ok {
			t.Fatalf("%s plot axis was not constructed", test.kind)
		}
		position, ok := axis.position(test.value)
		if !ok {
			t.Fatalf("%s plot value %g was not mapped", test.kind, test.value)
		}
		fraction := (position - 18) / (390 - 18)
		var inverted float64
		if axis.view.Kind == plotScaleLog10 {
			minimum := math.Log10(axis.view.DomainMin)
			maximum := math.Log10(axis.view.DomainMax)
			inverted = math.Pow(10, minimum+fraction*(maximum-minimum))
		} else {
			inverted = axis.view.DomainMin + fraction*(axis.view.DomainMax-axis.view.DomainMin)
		}
		if math.Abs(inverted-test.value) > math.Abs(test.value)*1e-12+1e-12 {
			t.Fatalf("%s inverted value = %g, want %g", test.kind, inverted, test.value)
		}
	}
}

func TestPlotLogarithmicScaleUsesActualDecadeValues(t *testing.T) {
	axis, ok := newPlotAxis(plotScaleLog10, 0.1, 100, 18, 390)
	if !ok {
		t.Fatal("logarithmic plot axis was not constructed")
	}
	if axis.view.Kind != plotScaleLog10 || axis.view.DomainMin != 0.1 || axis.view.DomainMax != 100 {
		t.Fatalf("logarithmic plot scale = %#v", axis.view)
	}
	want := []struct {
		value float64
		label string
	}{{0.1, "0.1"}, {1, "1"}, {10, "10"}, {100, "100"}}
	if len(axis.ticks) != len(want) {
		t.Fatalf("logarithmic plot ticks = %#v", axis.ticks)
	}
	for index, expected := range want {
		if axis.ticks[index].Value != expected.value || axis.ticks[index].Label != expected.label {
			t.Fatalf("logarithmic plot tick %d = %#v, want %g %q", index, axis.ticks[index], expected.value, expected.label)
		}
	}
}

func TestPlotPathsSplitAtNonFiniteAndInvalidLogSamples(t *testing.T) {
	plot := buildEngineeringPlot(engineeringPlotSpec{
		ID: "test-log", Rect: analysisPlotRect(),
		XScaleKind: plotScaleLog10, YScaleKind: plotScaleLinear,
		Series: []analysisSeries{{
			Name: "response", X: []float64{0.1, 1, 0, 10, 100, 1000},
			Y: []float64{1, math.NaN(), 2, 3, math.Inf(1), 4},
		}},
		Points: []analysisPoint{
			{X: 1, Y: 1, Label: "finite"},
			{X: -1, Y: 1, Label: "invalid log"},
			{X: 10, Y: math.NaN(), Label: "non-finite"},
		},
	})
	if len(plot.Paths) != 1 {
		t.Fatalf("plot paths = %#v", plot.Paths)
	}
	if strings.Count(plot.Paths[0].D, "M ") != 3 ||
		strings.Contains(plot.Paths[0].D, "NaN") ||
		strings.Contains(plot.Paths[0].D, "Inf") {
		t.Fatalf("discontinuous plot path = %q", plot.Paths[0].D)
	}
	if len(plot.Markers) != 1 || plot.Markers[0].Label != "finite" {
		t.Fatalf("plot markers = %#v", plot.Markers)
	}
	if plot.Paths[0].Key != "test-log:series:0" {
		t.Fatalf("fallback series key = %q", plot.Paths[0].Key)
	}
}

func TestAnalysisPlotsExposeStableIDsReferencesAndFrequencyDomains(t *testing.T) {
	value := func(number float64) *float64 { return &number }
	record := studio.FrequencyAnalysisRecord{
		CreatedAt: time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC),
		Result: studio.FrequencyAnalysis{
			Inputs:  []studio.AnalyzedChannel{{ChannelRef: studio.ChannelRef{BlockID: 1}, Name: "feed"}},
			Outputs: []studio.AnalyzedChannel{{ChannelRef: studio.ChannelRef{BlockID: 2}, Name: "temperature"}},
			Grid:    studio.FrequencyGrid{Omega: []float64{0.1, 1, 10}},
			Units: studio.FrequencyUnits{
				Frequency: "rad/s", Magnitude: "dB", Phase: "degrees",
			},
			Bode: []studio.BodeTrace{{
				MagnitudeDB:  []*float64{value(10), value(0), value(-10)},
				PhaseDegrees: []*float64{value(-90), value(-180), value(-270)},
			}},
			Nyquist: &studio.NyquistAnalysis{Positive: []studio.ComplexSample{
				{Real: value(-2), Imag: value(-1)},
				{Real: value(0), Imag: value(1)},
			}},
			Nichols: &studio.NicholsAnalysis{
				PhaseDegrees: []*float64{value(-240), value(-120)},
				MagnitudeDB:  []*float64{value(-10), value(10)},
			},
			SingularValues: &studio.SingularValueAnalysis{Values: [][]*float64{
				{value(2), value(1), value(0.5)},
			}},
		},
	}
	first := frequencyResultView(record)
	second := frequencyResultView(record)
	if len(first.Plots) != 5 || len(second.Plots) != 5 {
		t.Fatalf("frequency plots = %d and %d, want 5", len(first.Plots), len(second.Plots))
	}
	for index := range first.Plots {
		if first.Plots[index].Plot.ID == "" ||
			first.Plots[index].Plot.ID != second.Plots[index].Plot.ID ||
			first.Plots[index].Plot.GroupID != second.Plots[index].Plot.GroupID {
			t.Fatalf("unstable plot metadata at %d: %#v %#v", index, first.Plots[index].Plot, second.Plots[index].Plot)
		}
		for pathIndex := range first.Plots[index].Paths {
			if first.Plots[index].Paths[pathIndex].Key == "" ||
				first.Plots[index].Paths[pathIndex].Key != second.Plots[index].Paths[pathIndex].Key {
				t.Fatalf("unstable series key at plot %d path %d", index, pathIndex)
			}
		}
	}
	for _, bode := range first.Plots[:2] {
		if bode.XLabel != "ω (rad/s)" || bode.Plot.XScale.Kind != plotScaleLog10 ||
			bode.Plot.XScale.DomainMin != 0.1 || bode.Plot.XScale.DomainMax != 10 {
			t.Fatalf("Bode frequency scale = %#v, label %q", bode.Plot.XScale, bode.XLabel)
		}
	}
	if first.Plots[0].Plot.GroupID != "analysis-frequency-bode" ||
		first.Plots[1].Plot.GroupID != "analysis-frequency-bode" ||
		first.Plots[2].Plot.GroupID != first.Plots[2].Plot.ID ||
		first.Plots[3].Plot.GroupID != first.Plots[3].Plot.ID ||
		first.Plots[4].Plot.GroupID != first.Plots[4].Plot.ID ||
		!hasReferenceKind(first.Plots[0].Plot.References, "magnitude-zero") ||
		!hasReferenceKind(first.Plots[1].Plot.References, "phase-critical") ||
		!hasReferenceKind(first.Plots[2].Plot.References, "nyquist-critical") ||
		!hasReferenceKind(first.Plots[3].Plot.References, "phase-critical") ||
		!hasReferenceKind(first.Plots[3].Plot.References, "magnitude-zero") {
		t.Fatalf("frequency plot references = %#v", first.Plots)
	}
}

func TestAnalysisAndSimulationPlotsShareEngineeringMetadata(t *testing.T) {
	steady, rise, settling := 1.0, 0.4, 1.5
	dynamics := dynamicsResultView(studio.DynamicsAnalysisRecord{Result: studio.DynamicsAnalysis{
		StepExperiment: &studio.StepExperimentResult{
			Times: []float64{0, 1, 2}, Values: []float64{0, 0.8, 1},
			Metrics: studio.StepMetrics{
				SteadyStateValue: &steady, RiseTime: &rise, SettlingTime: &settling,
			},
		},
		Poles: []studio.ComplexValue{{Real: -1, Imag: 1}, {Real: -1, Imag: -1}},
	}})
	if len(dynamics.Plots) != 2 ||
		dynamics.Plots[0].Plot.ID != "analysis-dynamics-step" ||
		dynamics.Plots[1].Plot.ID != "analysis-dynamics-pole-zero" ||
		dynamics.Plots[0].Plot.GroupID != dynamics.Plots[0].Plot.ID ||
		dynamics.Plots[1].Plot.GroupID != dynamics.Plots[1].Plot.ID ||
		!hasReferenceKind(dynamics.Plots[0].Plot.References, "steady-state") ||
		!hasReferenceKind(dynamics.Plots[0].Plot.References, "rise-time") ||
		!hasReferenceKind(dynamics.Plots[0].Plot.References, "settling-time") ||
		!hasReferenceKind(dynamics.Plots[1].Plot.References, "stability-boundary") {
		t.Fatalf("dynamics plots = %#v", dynamics.Plots)
	}

	loop := loopResultView(studio.LoopAnalysisRecord{Result: studio.LoopAnalysis{
		RootLocus: &studio.RootLocusAnalysis{Branches: [][]studio.ComplexValue{{
			{Real: -2, Imag: 0}, {Real: 1, Imag: 0},
		}}},
	}})
	if len(loop.Plots) != 1 || loop.Plots[0].Plot.ID != "analysis-loop-root-locus" ||
		!hasReferenceKind(loop.Plots[0].Plot.References, "stability-boundary") ||
		loop.Plots[0].Paths[0].Key != "loop:root-locus:1" {
		t.Fatalf("root-locus plot = %#v", loop.Plots)
	}

	chart := newChartView(&studio.Simulation{
		Duration: 2, SampleTime: 1, Times: []float64{0, 1, 2},
		Series: []studio.Series{{
			ResultChannel: studio.ResultChannel{BlockID: 7, Name: "temperature"},
			Values:        []float64{2, math.NaN(), 3},
		}},
	})
	if chart.Plot.ID != "simulation-trend" ||
		chart.Plot.XScale.Kind != plotScaleLinear || chart.Plot.YScale.Kind != plotScaleLinear ||
		chart.Plot.Rect != (plotRectView{Left: 48, Top: 18, Right: 762, Bottom: 196}) ||
		!hasReferenceKind(chart.Plot.References, "y-zero") ||
		len(chart.Paths) != 1 || strings.Count(chart.Paths[0].D, "M ") != 2 ||
		len(chart.XGrid) != len(chart.Plot.XTicks) || len(chart.YGrid) != len(chart.Plot.YTicks) {
		t.Fatalf("simulation engineering plot = %#v", chart)
	}
}

func hasReferenceKind(references []plotReferenceView, kind string) bool {
	for _, reference := range references {
		if reference.Kind == kind {
			return true
		}
	}
	return false
}

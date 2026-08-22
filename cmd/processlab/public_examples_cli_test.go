package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type publicExampleFixture struct {
	SchemaVersion int    `json:"schemaVersion"`
	ID            string `json:"id"`
	Source        struct {
		Title     string `json:"title"`
		URL       string `json:"url"`
		Section   string `json:"section"`
		CheckedAt string `json:"checkedAt"`
	} `json:"source"`
	Simulation struct {
		Duration       float64 `json:"duration"`
		SampleTime     float64 `json:"sampleTime"`
		ExpectedDriver string  `json:"expectedDriver"`
	} `json:"simulation"`
	ExpectedSeries          string   `json:"expectedSeries"`
	ExpectedAnalysisOutputs []string `json:"expectedAnalysisOutputs"`
	Oracle                  string   `json:"oracle"`
	Tolerance               float64  `json:"tolerance"`
	Document                struct {
		Version int              `json:"version"`
		Blocks  []map[string]any `json:"blocks"`
		Wires   []map[string]any `json:"wires"`
	} `json:"document"`
}

func TestPublicControlExamplesThroughCLI(t *testing.T) {
	fixtures := loadPublicExampleFixtures(t)
	if len(fixtures) != 5 {
		t.Fatalf("public example fixture count = %d, want 5", len(fixtures))
	}

	harness := newCLIHarness(t)
	defer harness.Close()

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			project := requireSkillCommand(t, harness, "project", "create", fixture.Source.Title)
			projectID := parseSkillID(t, project.stdout)
			flows := requireSkillCommand(t, harness, "flow", "list", "--project", strconv.FormatInt(projectID, 10), "--json")
			var flowRecords []flowClientRecord
			decodeSkillJSON(t, flows.stdout, &flowRecords)
			if len(flowRecords) != 1 {
				t.Fatalf("new project flows = %#v", flowRecords)
			}
			flowID := flowRecords[0].ID
			flowIDText := strconv.FormatInt(flowID, 10)

			documentJSON, err := json.Marshal(fixture.Document)
			if err != nil {
				t.Fatal(err)
			}
			dryRun := requireSkillCommandInput(t, harness, string(documentJSON), "flow", "apply", "--flow", flowIDText, "--dry-run", "--json")
			var dryRunResponse flowApplyResponseClient
			decodeSkillJSON(t, dryRun.stdout, &dryRunResponse)
			if !dryRunResponse.Result.DryRun ||
				len(dryRunResponse.Result.Added) != len(fixture.Document.Blocks) ||
				dryRunResponse.Result.WiresAdded != len(fixture.Document.Wires) {
				t.Fatalf("dry-run result = %#v", dryRunResponse.Result)
			}

			apply := requireSkillCommandInput(t, harness, string(documentJSON), "flow", "apply", "--flow", flowIDText, "--json")
			var applyResponse flowApplyResponseClient
			decodeSkillJSON(t, apply.stdout, &applyResponse)
			if applyResponse.Result.DryRun ||
				len(applyResponse.Result.Added) != len(fixture.Document.Blocks) ||
				applyResponse.Result.WiresAdded != len(fixture.Document.Wires) {
				t.Fatalf("apply result = %#v", applyResponse.Result)
			}

			blocks := requireSkillCommand(t, harness, "block", "list", "--flow", flowIDText, "--json")
			var blockRecords []blockRecordClient
			decodeSkillJSON(t, blocks.stdout, &blockRecords)
			if len(blockRecords) != len(fixture.Document.Blocks) {
				t.Fatalf("applied block count = %d, want %d", len(blockRecords), len(fixture.Document.Blocks))
			}
			wires := requireSkillCommand(t, harness, "wire", "list", "--flow", flowIDText, "--json")
			var wireRecords []wireRecordClient
			decodeSkillJSON(t, wires.stdout, &wireRecords)
			if len(wireRecords) != len(fixture.Document.Wires) {
				t.Fatalf("applied wire count = %d, want %d", len(wireRecords), len(fixture.Document.Wires))
			}

			channels := requireSkillCommand(t, harness, "analyze", "channels", "--flow", flowIDText, "--json")
			var analysis analysisWorkspaceClient
			decodeSkillJSON(t, channels.stdout, &analysis)
			for _, name := range fixture.ExpectedAnalysisOutputs {
				if !hasAnalysisOutput(analysis, name) {
					t.Fatalf("analysis omitted %q: %#v", name, analysis.Outputs)
				}
			}

			run := requireSkillCommand(t, harness, "sim", "run", "--flow", flowIDText,
				"--duration", strconv.FormatFloat(fixture.Simulation.Duration, 'g', -1, 64),
				"--sample-time", strconv.FormatFloat(fixture.Simulation.SampleTime, 'g', -1, 64), "--json")
			var result simulationClient
			decodeSkillJSON(t, run.stdout, &result)
			if result.Stale || len(result.Times) != int(math.Round(fixture.Simulation.Duration/fixture.Simulation.SampleTime))+1 {
				t.Fatalf("simulation metadata = %#v", result)
			}
			var fidelity struct {
				Fidelity struct {
					Driver string `json:"driver"`
				} `json:"fidelity"`
			}
			decodeSkillJSON(t, run.stdout, &fidelity)
			if fidelity.Fidelity.Driver != fixture.Simulation.ExpectedDriver {
				t.Fatalf("simulation driver = %q, want %q", fidelity.Fidelity.Driver, fixture.Simulation.ExpectedDriver)
			}
			series := publicExampleSeries(t, result, fixture.ExpectedSeries)
			assertPublicExampleOracle(t, fixture, result.Times, series.Values)
			assertPublicExampleLimitations(t, harness, fixture.ID, flowIDText)
		})
	}
}

func assertPublicExampleLimitations(t *testing.T, harness *cliHarness, fixtureID, flowID string) {
	t.Helper()
	var result cliResult
	var required []string
	switch fixtureID {
	case "mathworks-fopdt":
		result = harness.Run("--server", harness.URL(), "sim", "run", "--flow", flowID,
			"--duration", "4", "--sample-time", "0.04", "--json")
		required = []string{"Input delay", "not aligned", "nearest aligned delay", "Padé or Thiran"}
	case "mathworks-unit-delay":
		result = harness.Run("--server", harness.URL(), "sim", "run", "--flow", flowID,
			"--duration", "1.2", "--sample-time", "0.06", "--json")
		required = []string{"One sample memory", "not an integer multiple", "use 0.06 s or 0.12 s"}
	default:
		return
	}
	if result.code != 1 || result.stdout != "" {
		t.Fatalf("unsupported-grid result = %s", result)
	}
	for _, text := range required {
		if !strings.Contains(result.stderr, text) {
			t.Fatalf("unsupported-grid message omitted %q: %s", text, result)
		}
	}
}

func loadPublicExampleFixtures(t *testing.T) []publicExampleFixture {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(sourceFile), "testdata", "public_examples", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	fixtures := make([]publicExampleFixture, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		encoded, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var fixture publicExampleFixture
		if err := json.Unmarshal(encoded, &fixture); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if fixture.SchemaVersion != 1 || fixture.ID == "" || seen[fixture.ID] {
			t.Fatalf("invalid fixture identity in %s: %#v", path, fixture)
		}
		if fixture.Source.Title == "" || !strings.HasPrefix(fixture.Source.URL, "https://") ||
			fixture.Source.Section == "" || fixture.Source.CheckedAt == "" {
			t.Fatalf("incomplete source attribution in %s: %#v", path, fixture.Source)
		}
		if fixture.Document.Version != 1 || len(fixture.Document.Blocks) == 0 || len(fixture.Document.Wires) == 0 ||
			fixture.ExpectedSeries == "" || len(fixture.ExpectedAnalysisOutputs) == 0 || fixture.Tolerance <= 0 {
			t.Fatalf("incomplete public example in %s: %#v", path, fixture)
		}
		seen[fixture.ID] = true
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func hasAnalysisOutput(analysis analysisWorkspaceClient, name string) bool {
	for _, output := range analysis.Outputs {
		if output.Name == name {
			return true
		}
	}
	return false
}

func publicExampleSeries(t *testing.T, result simulationClient, name string) simulationSeriesClient {
	t.Helper()
	for _, series := range result.Series {
		if series.Name == name {
			if len(series.Values) != len(result.Times) {
				t.Fatalf("series %q has %d values, want %d", name, len(series.Values), len(result.Times))
			}
			return series
		}
	}
	t.Fatalf("simulation omitted series %q: %#v", name, result.Series)
	return simulationSeriesClient{}
}

func assertPublicExampleOracle(t *testing.T, fixture publicExampleFixture, times, values []float64) {
	t.Helper()
	var expected []float64
	switch fixture.ID {
	case "ctms-cruise-pi":
		expected = make([]float64, len(times))
		for index, currentTime := range times {
			expected[index] = 10 * (1 - math.Exp(-0.8*currentTime))
		}
	case "ctms-dc-motor":
		expected = dcMotorStepOracle(times)
	case "ctms-aircraft-pitch":
		expected = aircraftPitchStepOracle(times)
	case "mathworks-fopdt":
		expected = make([]float64, len(times))
		for index, currentTime := range times {
			if currentTime > 2.1 {
				expected[index] = 0.1 * (1 - math.Exp(-10*(currentTime-2.1)))
			}
		}
	case "mathworks-unit-delay":
		expected = make([]float64, len(times))
		expected[0] = -1
		for index := 1; index < len(times); index++ {
			if times[index-1]+1e-12 >= 0.2 {
				expected[index] = 2
			}
		}
	default:
		t.Fatalf("fixture %q has no oracle", fixture.ID)
	}
	for index, want := range expected {
		if difference := math.Abs(values[index] - want); difference > fixture.Tolerance {
			t.Fatalf("%s at t=%.6g = %.12g, want %.12g (difference %.3g, tolerance %.3g)",
				fixture.ID, times[index], values[index], want, difference, fixture.Tolerance)
		}
	}
}

func dcMotorStepOracle(times []float64) []float64 {
	const dcGain = 0.01 / 0.1001
	discriminant := math.Sqrt(12*12 - 4*20.02)
	slowPole := (-12 + discriminant) / 2
	fastPole := (-12 - discriminant) / 2
	slowCoefficient := dcGain * fastPole / (slowPole - fastPole)
	fastCoefficient := -dcGain - slowCoefficient
	expected := make([]float64, len(times))
	for index, currentTime := range times {
		expected[index] = dcGain + slowCoefficient*math.Exp(slowPole*currentTime) +
			fastCoefficient*math.Exp(fastPole*currentTime)
	}
	return expected
}

func aircraftPitchStepOracle(times []float64) []float64 {
	expected := make([]float64, len(times))
	state := [3]float64{}
	currentTime := 0.0
	for index, targetTime := range times {
		for currentTime+1e-12 < targetTime {
			step := math.Min(0.0001, targetTime-currentTime)
			state = aircraftPitchRK4Step(state, step)
			currentTime += step
		}
		expected[index] = state[2]
	}
	return expected
}

func aircraftPitchRK4Step(state [3]float64, step float64) [3]float64 {
	k1 := aircraftPitchDerivative(state)
	k2 := aircraftPitchDerivative(addAircraftState(state, k1, step/2))
	k3 := aircraftPitchDerivative(addAircraftState(state, k2, step/2))
	k4 := aircraftPitchDerivative(addAircraftState(state, k3, step))
	for index := range state {
		state[index] += step / 6 * (k1[index] + 2*k2[index] + 2*k3[index] + k4[index])
	}
	return state
}

func aircraftPitchDerivative(state [3]float64) [3]float64 {
	feedback := -0.6435*state[0] + 169.695*state[1] + 7.0711*state[2]
	elevator := 7.0711*0.2 - feedback
	return [3]float64{
		-0.313*state[0] + 56.7*state[1] + 0.232*elevator,
		-0.0139*state[0] - 0.426*state[1] + 0.0203*elevator,
		56.7 * state[1],
	}
}

func addAircraftState(state, derivative [3]float64, scale float64) [3]float64 {
	for index := range state {
		state[index] += scale * derivative[index]
	}
	return state
}

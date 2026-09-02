package studio

import (
	"context"
	"math/cmplx"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jamestjsp/controlsys"
)

func TestPIDInheritedSampleTimeEditorContract(t *testing.T) {
	for _, kind := range []BlockKind{BlockPID, BlockPID2} {
		t.Run(string(kind), func(t *testing.T) {
			parameters := defaultParameters(kind)
			if normalizedModelDomain(parameters) != modelDomainContinuous ||
				normalizedSampleTimeMode(parameters) != sampleTimeExplicit ||
				parameters.SampleTime != 0.1 {
				t.Fatalf("defaults = %#v", parameters)
			}

			schema, ok := kind.Schema()
			if !ok {
				t.Fatal("missing schema")
			}
			fields := make(map[string]ParameterSchema, len(schema.Parameters))
			for _, field := range schema.Parameters {
				fields[field.Name] = field
			}
			wantModeActivation := []ParameterActivation{
				parameterActivation("time_domain", modelDomainDiscrete),
			}
			wantTimeActivation := []ParameterActivation{
				parameterActivation("time_domain", modelDomainDiscrete),
				parameterActivation("sample_time_mode", string(sampleTimeExplicit)),
			}
			if !fields["sample_time_mode"].Optional {
				t.Fatal("sample-time mode must remain optional for legacy update payloads")
			}
			if !reflect.DeepEqual(fields["sample_time_mode"].ActiveWhen, wantModeActivation) {
				t.Fatalf("sample-time mode activation = %#v, want %#v", fields["sample_time_mode"].ActiveWhen, wantModeActivation)
			}
			if !reflect.DeepEqual(fields["sample_time"].ActiveWhen, wantTimeActivation) {
				t.Fatalf("sample-time activation = %#v, want %#v", fields["sample_time"].ActiveWhen, wantTimeActivation)
			}

			legacyValues := pidSchemaValues(t, kind)
			delete(legacyValues, "sample_time_mode")
			legacyValues["time_domain"] = modelDomainDiscrete
			legacyValues["sample_time"] = "0.2"
			legacy, err := validateBlockUpdate(Block{
				Kind: kind, Name: "Controller", Parameters: parameters,
			}, BlockUpdate{Name: "Controller", Parameters: legacyValues})
			if err != nil {
				t.Fatalf("legacy update without sample-time mode: %v", err)
			}
			if normalizedSampleTimeMode(legacy.Parameters) != sampleTimeExplicit ||
				legacy.Parameters.SampleTime != 0.2 {
				t.Fatalf("legacy explicit parameters = %#v", legacy.Parameters)
			}

			values := pidSchemaValues(t, kind)
			values["time_domain"] = modelDomainDiscrete
			values["sample_time_mode"] = string(sampleTimeInherited)
			values["sample_time"] = "-2"
			updated, err := validateBlockUpdate(Block{
				Kind: kind, Name: "Controller", Parameters: parameters,
			}, BlockUpdate{Name: "Controller", Parameters: values})
			if err != nil {
				t.Fatalf("inherited update: %v", err)
			}
			if normalizedSampleTimeMode(updated.Parameters) != sampleTimeInherited ||
				updated.Parameters.SampleTime != -2 {
				t.Fatalf("inherited parameters = %#v", updated.Parameters)
			}

			values["sample_time_mode"] = string(sampleTimeExplicit)
			if _, err := validateBlockUpdate(Block{
				Kind: kind, Name: "Controller", Parameters: parameters,
			}, BlockUpdate{Name: "Controller", Parameters: values}); err == nil ||
				!strings.Contains(err.Error(), "sample time") {
				t.Fatalf("explicit invalid sample-time error = %v", err)
			}
			values["sample_time_mode"] = "automatic"
			values["sample_time"] = "0.1"
			if _, err := validateBlockUpdate(Block{
				Kind: kind, Name: "Controller", Parameters: parameters,
			}, BlockUpdate{Name: "Controller", Parameters: values}); err == nil ||
				!strings.Contains(err.Error(), "explicit or inherited") {
				t.Fatalf("unknown sample-time mode error = %v", err)
			}
		})
	}
}

func TestInheritedPIDMatchesExplicitPublicAnalysisAndSimulation(t *testing.T) {
	fixture := loadR2026aPIDFixture(t)
	for _, kind := range []BlockKind{BlockPID, BlockPID2} {
		t.Run(string(kind), func(t *testing.T) {
			explicit := newPublicPIDRateFlow(t, kind, sampleTimeExplicit, fixture)
			inherited := newPublicPIDRateFlow(t, kind, sampleTimeInherited, fixture)

			for input := range explicit.sourceIDs {
				explicitFrequency, err := explicit.studio.AnalyzeFrequency(
					explicit.ctx,
					explicit.flowID,
					FrequencyAnalysisRequest{
						Inputs:  []ChannelRef{{BlockID: explicit.sourceIDs[input]}},
						Outputs: []ChannelRef{{BlockID: explicit.controllerID}},
						Omega:   fixture.Scenario.Omega,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				inheritedFrequency, err := inherited.studio.AnalyzeFrequency(
					inherited.ctx,
					inherited.flowID,
					FrequencyAnalysisRequest{
						Inputs:  []ChannelRef{{BlockID: inherited.sourceIDs[input]}},
						Outputs: []ChannelRef{{BlockID: inherited.controllerID}},
						Omega:   fixture.Scenario.Omega,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(inheritedFrequency.Grid, explicitFrequency.Grid) ||
					!reflect.DeepEqual(inheritedFrequency.Bode, explicitFrequency.Bode) {
					t.Fatalf("inherited frequency differs from explicit:\ninherited=%#v\nexplicit=%#v", inheritedFrequency.Bode, explicitFrequency.Bode)
				}
				for index, omega := range fixture.Scenario.Omega {
					assertBodeSample(t, inheritedFrequency, index, discretePIDOracle(fixture, kind, input, omega))
				}
			}

			explicitRun, err := explicit.studio.Run(explicit.ctx, explicit.flowID, SimulationRequest{
				Duration: 1, SampleTime: fixture.Scenario.SampleTime,
			})
			if err != nil {
				t.Fatal(err)
			}
			inheritedRun, err := inherited.studio.Run(inherited.ctx, inherited.flowID, SimulationRequest{
				Duration: 1, SampleTime: fixture.Scenario.SampleTime,
			})
			if err != nil {
				t.Fatal(err)
			}
			if explicitRun.LastRun == nil || inheritedRun.LastRun == nil ||
				!reflect.DeepEqual(inheritedRun.LastRun.Times, explicitRun.LastRun.Times) ||
				!reflect.DeepEqual(inheritedRun.LastRun.Series, explicitRun.LastRun.Series) {
				t.Fatalf("inherited simulation differs from explicit:\ninherited=%#v\nexplicit=%#v", inheritedRun.LastRun, explicitRun.LastRun)
			}

			controller := findBlock(t, inheritedRun.Blocks, inherited.controllerID)
			if normalizedSampleTimeMode(controller.Parameters) != sampleTimeInherited ||
				controller.Parameters.SampleTime != inheritedPIDFallbackSampleTime {
				t.Fatalf("authored inherited controller = %#v", controller.Parameters)
			}
			var rate *BlockRate
			for index := range inheritedRun.LastRun.Fidelity.BlockRates {
				candidate := &inheritedRun.LastRun.Fidelity.BlockRates[index]
				if candidate.BlockID == inherited.controllerID {
					rate = candidate
					break
				}
			}
			if rate == nil || rate.Mode != string(sampleTimeInherited) ||
				rate.SampleTime != fixture.Scenario.SampleTime || rate.UpdateEvery != 1 {
				t.Fatalf("inherited PID fidelity rate = %#v", rate)
			}
		})
	}
}

func TestInheritedPID2RejectsConflictingInputRates(t *testing.T) {
	controller := defaultParameters(BlockPID2)
	controller.TimeDomain = modelDomainDiscrete
	controller.SampleTime = inheritedPIDFallbackSampleTime
	controller.SampleTimeMode = string(sampleTimeInherited)
	fast := defaultParameters(BlockDiscreteTransfer)
	fast.SampleTime = 0.1
	slow := defaultParameters(BlockDiscreteTransfer)
	slow.SampleTime = 0.2
	_, err := compileModel([]Block{
		{ID: 1, Kind: BlockConstant, Name: "Reference"},
		{ID: 2, Kind: BlockDiscreteTransfer, Name: "Fast reference", Parameters: fast},
		{ID: 3, Kind: BlockConstant, Name: "Measurement"},
		{ID: 4, Kind: BlockDiscreteTransfer, Name: "Slow measurement", Parameters: slow},
		{ID: 5, Kind: BlockPID2, Name: "Controller", Parameters: controller},
		{ID: 6, Kind: BlockScope, Name: "Output"},
	}, []Connection{
		{SourceID: 1, TargetID: 2},
		{SourceID: 2, TargetID: 5, TargetPort: 0},
		{SourceID: 3, TargetID: 4},
		{SourceID: 4, TargetID: 5, TargetPort: 1},
		{SourceID: 5, TargetID: 6},
	})
	if err == nil || !strings.Contains(err.Error(), "Controller") ||
		!strings.Contains(err.Error(), "Fast reference") ||
		!strings.Contains(err.Error(), "Slow measurement") {
		t.Fatalf("conflicting PID2 rate error = %v", err)
	}
}

func TestInheritedPIDSampleTimePersistsAcrossSQLiteReopen(t *testing.T) {
	for _, kind := range []BlockKind{BlockPID, BlockPID2} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "pid-rate.db")
			first, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := first.Current(ctx)
			if err != nil {
				t.Fatal(err)
			}
			values := pidSchemaValues(t, kind)
			values["time_domain"] = modelDomainDiscrete
			values["sample_time_mode"] = string(sampleTimeInherited)
			values["sample_time"] = formatFloat(inheritedPIDFallbackSampleTime)
			_, blockID, err := first.AddConfiguredBlock(
				ctx, snapshot.Flow.ID, kind, Point{X: 1200, Y: 900}, values,
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}

			second, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			reread, err := second.Snapshot(ctx, snapshot.Flow.ID)
			if err != nil {
				t.Fatal(err)
			}
			controller := findBlock(t, reread.Blocks, blockID)
			if normalizedSampleTimeMode(controller.Parameters) != sampleTimeInherited ||
				controller.Parameters.SampleTime != inheritedPIDFallbackSampleTime {
				t.Fatalf("reopened inherited PID = %#v", controller.Parameters)
			}
		})
	}
}

func TestPIDDesignUsesDiscretePlantRateForInheritedController(t *testing.T) {
	for _, kind := range []BlockKind{BlockPID, BlockPID2} {
		t.Run(string(kind), func(t *testing.T) {
			service, flowID, _, controllerID := pidDesignStudio(t, BlockDiscreteTransfer, kind)
			values := pidSchemaValues(t, kind)
			values["time_domain"] = modelDomainDiscrete
			values["sample_time_mode"] = string(sampleTimeInherited)
			values["sample_time"] = formatFloat(inheritedPIDFallbackSampleTime)
			if _, err := service.UpdateBlock(context.Background(), controllerID, BlockUpdate{
				Name: "Controller", Parameters: values,
			}); err != nil {
				t.Fatal(err)
			}

			candidate, err := service.DesignPIDController(context.Background(), flowID, PIDDesignRequest{
				Type: controlsys.PidtunePI, CrossoverFrequency: 0.5, PhaseMargin: 50,
			})
			if err != nil {
				t.Fatal(err)
			}
			if candidate.Gains.SampleTime != 0.1 {
				t.Fatalf("candidate sample time = %g, want plant rate 0.1", candidate.Gains.SampleTime)
			}
			application, err := service.ApplyPIDDesignCandidate(context.Background(), candidate)
			if err != nil {
				t.Fatal(err)
			}
			controller := findBlock(t, application.Snapshot.Blocks, controllerID)
			if normalizedSampleTimeMode(controller.Parameters) != sampleTimeInherited ||
				controller.Parameters.SampleTime != inheritedPIDFallbackSampleTime {
				t.Fatalf("applied inherited controller = %#v", controller.Parameters)
			}

			_, err = service.DesignPIDController(context.Background(), flowID, PIDDesignRequest{
				Type: controlsys.PidtunePI, CrossoverFrequency: 0.5,
				PhaseMargin: 50, BaseStep: 0.2,
			})
			if err == nil || !strings.Contains(err.Error(), "sample time") {
				t.Fatalf("conflicting design base-step error = %v", err)
			}
		})
	}
}

func TestUnanchoredInheritedPIDUsesAnalysisBaseStep(t *testing.T) {
	fixture := loadR2026aPIDFixture(t)
	for _, kind := range []BlockKind{BlockPID, BlockPID2} {
		t.Run(string(kind), func(t *testing.T) {
			flow := newPublicPIDRateFlow(t, kind, sampleTimeInherited, fixture)
			values := pidSchemaValues(t, BlockDiscreteTransfer)
			values["sample_time_mode"] = string(sampleTimeInherited)
			values["sample_time"] = "0.2"
			if _, err := flow.studio.UpdateBlock(flow.ctx, flow.anchorID, BlockUpdate{
				Name: "Inherited downstream", Parameters: values,
			}); err != nil {
				t.Fatal(err)
			}

			result, err := flow.studio.AnalyzeFrequency(
				flow.ctx, flow.flowID, FrequencyAnalysisRequest{
					Inputs:  []ChannelRef{{BlockID: flow.sourceIDs[0]}},
					Outputs: []ChannelRef{{BlockID: flow.controllerID}},
					Omega:   fixture.Scenario.Omega, BaseStep: fixture.Scenario.SampleTime,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			for index, omega := range fixture.Scenario.Omega {
				assertBodeSample(t, result, index, discretePIDOracle(fixture, kind, 0, omega))
			}

			_, err = flow.studio.AnalyzeFrequency(
				flow.ctx, flow.flowID, FrequencyAnalysisRequest{
					Inputs:  []ChannelRef{{BlockID: flow.sourceIDs[0]}},
					Outputs: []ChannelRef{{BlockID: flow.controllerID}},
					Omega:   fixture.Scenario.Omega,
				},
			)
			if err == nil || !strings.Contains(err.Error(), "inherited sample time requires") {
				t.Fatalf("unanchored inherited PID error = %v", err)
			}
		})
	}
}

const inheritedPIDFallbackSampleTime = 0.37

type publicPIDRateFlow struct {
	studio       *Studio
	ctx          context.Context
	flowID       int64
	sourceIDs    []int64
	controllerID int64
	anchorID     int64
}

func newPublicPIDRateFlow(
	t *testing.T,
	kind BlockKind,
	mode sampleTimeMode,
	fixture pidCompatibilityFixture,
) publicPIDRateFlow {
	t.Helper()
	ctx := context.Background()
	service := openTestStudio(t, filepath.Join(t.TempDir(), "pid-rate.db"))
	current, err := service.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateFlow(ctx, current.Flow.ProjectID, "PID inherited rate")
	if err != nil {
		t.Fatal(err)
	}
	flowID := created.Snapshot.Flow.ID
	sourceCount := 1
	if kind == BlockPID2 {
		sourceCount = 2
	}
	sourceIDs := make([]int64, sourceCount)
	for index := range sourceIDs {
		_, sourceIDs[index], err = service.AddBlock(
			ctx, flowID, BlockConstant, Point{X: 100, Y: 100 + index*160},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, controllerID, err := service.AddBlock(ctx, flowID, kind, Point{X: 400, Y: 180})
	if err != nil {
		t.Fatal(err)
	}
	values := pidSchemaValues(t, kind)
	values["proportional"] = formatFloat(fixture.Scenario.Proportional)
	values["integral"] = formatFloat(fixture.Scenario.Integral)
	values["derivative"] = formatFloat(fixture.Scenario.Derivative)
	values["filter_coefficient"] = formatFloat(fixture.Scenario.FilterCoefficient)
	values["time_domain"] = modelDomainDiscrete
	values["sample_time_mode"] = string(mode)
	values["sample_time"] = formatFloat(fixture.Scenario.SampleTime)
	if mode == sampleTimeInherited {
		values["sample_time"] = formatFloat(inheritedPIDFallbackSampleTime)
	}
	if kind == BlockPID2 {
		values["setpoint_weight"] = formatFloat(fixture.Scenario.SetpointWeight)
		values["derivative_weight"] = formatFloat(fixture.Scenario.DerivativeWeight)
	}
	if _, err := service.UpdateBlock(ctx, controllerID, BlockUpdate{
		Name: "Controller", Parameters: values,
	}); err != nil {
		t.Fatal(err)
	}
	for index, sourceID := range sourceIDs {
		if _, err := service.Connect(ctx, flowID, Wire{
			SourceID: sourceID, TargetID: controllerID, TargetPort: index,
		}); err != nil {
			t.Fatal(err)
		}
	}
	_, anchorID, err := service.AddBlock(ctx, flowID, BlockDiscreteTransfer, Point{X: 700, Y: 180})
	if err != nil {
		t.Fatal(err)
	}
	_, scopeID, err := service.AddBlock(ctx, flowID, BlockScope, Point{X: 1000, Y: 180})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Connect(ctx, flowID, Wire{SourceID: controllerID, TargetID: anchorID}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Connect(ctx, flowID, Wire{SourceID: anchorID, TargetID: scopeID}); err != nil {
		t.Fatal(err)
	}
	return publicPIDRateFlow{
		studio: service, ctx: ctx, flowID: flowID,
		sourceIDs: sourceIDs, controllerID: controllerID, anchorID: anchorID,
	}
}

func pidSchemaValues(t *testing.T, kind BlockKind) map[string]string {
	t.Helper()
	schema, ok := kind.Schema()
	if !ok {
		t.Fatalf("missing %s schema", kind)
	}
	values := make(map[string]string, len(schema.Parameters)+1)
	for _, field := range schema.Parameters {
		values[field.Name] = field.Default
	}
	return values
}

func discretePIDOracle(
	fixture pidCompatibilityFixture,
	kind BlockKind,
	input int,
	omega float64,
) complex128 {
	z := cmplx.Exp(complex(0, omega*fixture.Scenario.SampleTime))
	integrator := complex(fixture.Scenario.SampleTime, 0) / (z - 1)
	derivative := complex(fixture.Scenario.FilterCoefficient, 0) *
		(z - 1) /
		(z - complex(1-fixture.Scenario.FilterCoefficient*fixture.Scenario.SampleTime, 0))
	if kind == BlockPID {
		return complex(fixture.Scenario.Proportional, 0) +
			complex(fixture.Scenario.Integral, 0)*integrator +
			complex(fixture.Scenario.Derivative, 0)*derivative
	}
	if input == 0 {
		return complex(fixture.Scenario.Proportional*fixture.Scenario.SetpointWeight, 0) +
			complex(fixture.Scenario.Integral, 0)*integrator +
			complex(fixture.Scenario.Derivative*fixture.Scenario.DerivativeWeight, 0)*derivative
	}
	return -complex(fixture.Scenario.Proportional, 0) -
		complex(fixture.Scenario.Integral, 0)*integrator -
		complex(fixture.Scenario.Derivative, 0)*derivative
}

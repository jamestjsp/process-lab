package studio

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestCascadeExampleTracksSetpointWithMirroredRecycle(t *testing.T) {
	data, err := os.ReadFile("../../examples/cascade-reactor.json")
	if err != nil {
		t.Fatal(err)
	}
	var document FlowDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	service := openTestStudio(t, filepath.Join(t.TempDir(), "cascade.db"))
	initial, err := service.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, applied, err := service.ApplyFlow(ctx, initial.Flow.ID, document, false)
	if err != nil {
		t.Fatal(err)
	}
	mirrored := 0
	for _, block := range applied.Blocks {
		if block.Mirrored {
			mirrored++
		}
	}
	if mirrored != 4 {
		t.Fatalf("mirrored blocks = %d", mirrored)
	}
	result, err := service.Run(ctx, initial.Flow.ID, SimulationRequest{Duration: 200, SampleTime: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if result.LastRun == nil {
		t.Fatal("missing simulation")
	}
	found := false
	for _, series := range result.LastRun.Series {
		for _, value := range series.Values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatal("non-finite output")
			}
		}
		if series.Name == "Reactor temperature" {
			found = true
			if math.Abs(series.Values[len(series.Values)-1]-1) > .002 {
				t.Fatal("temperature did not recover to setpoint")
			}
		}
	}
	if !found {
		t.Fatal("missing temperature output")
	}
}

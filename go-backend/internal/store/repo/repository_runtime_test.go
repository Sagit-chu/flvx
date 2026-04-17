package repo

import "testing"

func TestRuntimeEngineDefaultsToGost(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	engine, err := r.GetRuntimeEngine()
	if err != nil {
		t.Fatalf("get runtime engine: %v", err)
	}
	if engine != RuntimeEngineGost {
		t.Fatalf("expected default runtime engine %q, got %q", RuntimeEngineGost, engine)
	}
}

func TestRuntimeSwitchStateRoundTrips(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	wantEngine := RuntimeEngineDash
	if err := r.SetRuntimeEngine(wantEngine, 101); err != nil {
		t.Fatalf("set runtime engine: %v", err)
	}

	wantState := RuntimeSwitchState{
		Status:     RuntimeSwitchStatusFailed,
		Generation: 7,
		Error:      "switch failed",
	}
	if err := r.SetRuntimeSwitchStatus(wantState.Status, 202); err != nil {
		t.Fatalf("set runtime switch status: %v", err)
	}
	if err := r.SetRuntimeSwitchGeneration(wantState.Generation, 203); err != nil {
		t.Fatalf("set runtime switch generation: %v", err)
	}
	if err := r.SetRuntimeSwitchError(wantState.Error, 204); err != nil {
		t.Fatalf("set runtime switch error: %v", err)
	}

	gotEngine, err := r.GetRuntimeEngine()
	if err != nil {
		t.Fatalf("get runtime engine: %v", err)
	}
	if gotEngine != wantEngine {
		t.Fatalf("expected runtime engine %q, got %q", wantEngine, gotEngine)
	}

	gotState, err := r.GetRuntimeSwitchState()
	if err != nil {
		t.Fatalf("get runtime switch state: %v", err)
	}
	if gotState != wantState {
		t.Fatalf("expected runtime switch state %+v, got %+v", wantState, gotState)
	}

	gotStatus, err := r.GetRuntimeSwitchStatus()
	if err != nil {
		t.Fatalf("get runtime switch status: %v", err)
	}
	if gotStatus != wantState.Status {
		t.Fatalf("expected runtime switch status %q, got %q", wantState.Status, gotStatus)
	}

	gotGeneration, err := r.GetRuntimeSwitchGeneration()
	if err != nil {
		t.Fatalf("get runtime switch generation: %v", err)
	}
	if gotGeneration != wantState.Generation {
		t.Fatalf("expected runtime switch generation %d, got %d", wantState.Generation, gotGeneration)
	}

	gotError, err := r.GetRuntimeSwitchError()
	if err != nil {
		t.Fatalf("get runtime switch error: %v", err)
	}
	if gotError != wantState.Error {
		t.Fatalf("expected runtime switch error %q, got %q", wantState.Error, gotError)
	}

	wantAggregateState := RuntimeSwitchState{
		Status:     RuntimeSwitchStatusSwitching,
		Generation: 8,
		Error:      "",
	}
	if err := r.SetRuntimeSwitchState(wantAggregateState, 205); err != nil {
		t.Fatalf("set runtime switch state: %v", err)
	}

	gotAggregateState, err := r.GetRuntimeSwitchState()
	if err != nil {
		t.Fatalf("get aggregate runtime switch state: %v", err)
	}
	if gotAggregateState != wantAggregateState {
		t.Fatalf("expected aggregate runtime switch state %+v, got %+v", wantAggregateState, gotAggregateState)
	}
}

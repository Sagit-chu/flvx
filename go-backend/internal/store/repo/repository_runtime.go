package repo

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

const (
	runtimeEngineConfigKey           = "runtime_engine"
	runtimeSwitchStatusConfigKey     = "runtime_switch_status"
	runtimeSwitchGenerationConfigKey = "runtime_switch_generation"
	runtimeSwitchErrorConfigKey      = "runtime_switch_error"
)

type RuntimeEngine string

const (
	RuntimeEngineGost RuntimeEngine = "gost"
	RuntimeEngineDash RuntimeEngine = "dash"
)

type RuntimeSwitchStatus string

const (
	RuntimeSwitchStatusIdle      RuntimeSwitchStatus = "idle"
	RuntimeSwitchStatusSwitching RuntimeSwitchStatus = "switching"
	RuntimeSwitchStatusFailed    RuntimeSwitchStatus = "failed"
)

type RuntimeSwitchState struct {
	Status     RuntimeSwitchStatus
	Generation int64
	Error      string
}

func (r *Repository) GetRuntimeEngine() (RuntimeEngine, error) {
	value, err := r.GetViteConfigValue(runtimeEngineConfigKey)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeEngineGost, nil
	}
	if err != nil {
		return "", err
	}
	return parseRuntimeEngine(value), nil
}

func (r *Repository) SetRuntimeEngine(engine RuntimeEngine, now int64) error {
	return r.UpsertConfig(runtimeEngineConfigKey, string(parseRuntimeEngine(string(engine))), now)
}

func (r *Repository) GetRuntimeSwitchState() (RuntimeSwitchState, error) {
	status, err := r.GetRuntimeSwitchStatus()
	if err != nil {
		return RuntimeSwitchState{}, err
	}

	generation, err := r.GetRuntimeSwitchGeneration()
	if err != nil {
		return RuntimeSwitchState{}, err
	}

	lastError, err := r.GetRuntimeSwitchError()
	if err != nil {
		return RuntimeSwitchState{}, err
	}

	return RuntimeSwitchState{
		Status:     status,
		Generation: generation,
		Error:      lastError,
	}, nil
}

func (r *Repository) GetRuntimeSwitchStatus() (RuntimeSwitchStatus, error) {
	value, err := r.getRuntimeConfigValue(runtimeSwitchStatusConfigKey)
	if err != nil {
		return "", err
	}
	return parseRuntimeSwitchStatus(value), nil
}

func (r *Repository) GetRuntimeSwitchGeneration() (int64, error) {
	value, err := r.getRuntimeConfigValue(runtimeSwitchGenerationConfigKey)
	if err != nil {
		return 0, err
	}
	generation, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, nil
	}
	return generation, nil
}

func (r *Repository) GetRuntimeSwitchError() (string, error) {
	return r.getRuntimeConfigValue(runtimeSwitchErrorConfigKey)
}

func (r *Repository) SetRuntimeSwitchStatus(status RuntimeSwitchStatus, now int64) error {
	return r.UpsertConfig(runtimeSwitchStatusConfigKey, string(parseRuntimeSwitchStatus(string(status))), now)
}

func (r *Repository) SetRuntimeSwitchGeneration(generation int64, now int64) error {
	return r.UpsertConfig(runtimeSwitchGenerationConfigKey, strconv.FormatInt(generation, 10), now)
}

func (r *Repository) SetRuntimeSwitchError(lastError string, now int64) error {
	return r.UpsertConfig(runtimeSwitchErrorConfigKey, lastError, now)
}

func (r *Repository) SetRuntimeSwitchState(state RuntimeSwitchState, now int64) error {
	if err := r.SetRuntimeSwitchStatus(state.Status, now); err != nil {
		return err
	}
	if err := r.SetRuntimeSwitchGeneration(state.Generation, now); err != nil {
		return err
	}
	return r.SetRuntimeSwitchError(state.Error, now)
}

func (r *Repository) getRuntimeConfigValue(name string) (string, error) {
	value, err := r.GetViteConfigValue(name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func parseRuntimeEngine(value string) RuntimeEngine {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(RuntimeEngineDash):
		return RuntimeEngineDash
	default:
		return RuntimeEngineGost
	}
}

func parseRuntimeSwitchStatus(value string) RuntimeSwitchStatus {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(RuntimeSwitchStatusSwitching):
		return RuntimeSwitchStatusSwitching
	case string(RuntimeSwitchStatusFailed):
		return RuntimeSwitchStatusFailed
	default:
		return RuntimeSwitchStatusIdle
	}
}

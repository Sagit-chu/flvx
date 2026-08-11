package handler

import (
	"fmt"
	"strings"

	"go-backend/internal/license"
)

const keygenAccountID = "1bc96cac-09de-4cf4-af34-26afdad63a90"

var newLicenseClient = license.NewKeygenClient

func licenseNeedsMachineActivation(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "NO_MACHINES", "NO_MACHINE", "MACHINE_SCOPE_REQUIRED", "FINGERPRINT_SCOPE_MISMATCH":
		return true
	default:
		return false
	}
}

func (h *Handler) validateLicenseForMachine(key string) (*license.ValidateResponse, error) {
	fingerprint, err := h.getOrCreateMachineFingerprint()
	if err != nil {
		return nil, fmt.Errorf("prepare machine fingerprint: %w", err)
	}

	client := newLicenseClient(keygenAccountID, "")
	validation, err := client.ValidateKeyWithFingerprint(key, fingerprint)
	if err != nil {
		return nil, err
	}
	if validation.Meta.Valid || !licenseNeedsMachineActivation(validation.Meta.Code) {
		return validation, nil
	}

	client.Token = key
	if err := client.ActivateMachine(validation.Data.ID, fingerprint); err != nil {
		return validation, err
	}

	validation, err = client.ValidateKeyWithFingerprint(key, fingerprint)
	if err != nil {
		return nil, err
	}
	return validation, nil
}

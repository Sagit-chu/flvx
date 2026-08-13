package handler

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"go-backend/internal/license"
)

var newLicenseClient = license.NewKeygenClient

func keygenAccountID() string {
	if value := strings.TrimSpace(license.AccountID); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("KEYGEN_ACCOUNT_ID"))
}

func licenseNeedsMachineActivation(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "NO_MACHINES", "NO_MACHINE", "MACHINE_SCOPE_REQUIRED", "FINGERPRINT_SCOPE_MISMATCH":
		return true
	default:
		return false
	}
}

func licenseValidationErrorIsDefinitive(validation *license.ValidateResponse, err error) bool {
	if err == nil {
		return validation != nil && !validation.Meta.Valid
	}
	var apiErr *license.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= http.StatusInternalServerError {
		return false
	}
	return apiErr.Operation == "activate machine" && validation != nil && !validation.Meta.Valid
}

func licenseValidationErrorMessage(err error) string {
	if strings.Contains(err.Error(), "keygen account id is not configured") {
		return "授权服务配置错误"
	}
	var apiErr *license.APIError
	if errors.As(err, &apiErr) {
		if apiErr.HasCode("MACHINE_LIMIT_EXCEEDED") {
			return "授权设备数量已达上限"
		}
		if apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden {
			return "授权码无效或无权绑定设备"
		}
	}
	return "连接授权服务器失败，请稍后重试"
}

func (h *Handler) validateLicenseForMachine(key string) (*license.ValidateResponse, error) {
	fingerprint, err := h.getOrCreateMachineFingerprint()
	if err != nil {
		return nil, fmt.Errorf("prepare machine fingerprint: %w", err)
	}
	storedMachineID, _ := h.repo.GetViteConfigValue("license_machine_id")

	accountID := keygenAccountID()
	if accountID == "" {
		return nil, fmt.Errorf("keygen account id is not configured")
	}
	client := newLicenseClient(accountID, "")
	var validation *license.ValidateResponse
	if storedMachineID != "" {
		validation, err = client.ValidateKeyWithMachine(key, fingerprint, storedMachineID)
	} else {
		validation, err = client.ValidateKeyWithFingerprint(key, fingerprint)
	}
	if err != nil {
		return nil, err
	}
	if validation.Meta.Valid || !licenseNeedsMachineActivation(validation.Meta.Code) {
		if validation.Meta.Valid {
			validation.MachineID = storedMachineID
		}
		return validation, nil
	}

	client.Token = key
	machineID, err := client.ActivateMachine(validation.Data.ID, fingerprint)
	if err != nil {
		return validation, err
	}
	if machineID == "" {
		machineID, err = client.GetMachineID(fingerprint)
		if err != nil {
			return validation, fmt.Errorf("retrieve activated machine: %w", err)
		}
	}

	validation, err = client.ValidateKeyWithMachine(key, fingerprint, machineID)
	if err != nil {
		return nil, err
	}
	if validation.Meta.Valid {
		validation.MachineID = machineID
	}
	return validation, nil
}

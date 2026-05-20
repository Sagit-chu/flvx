package repo

import (
	"errors"
	"strconv"
	"strings"
)

type LicenseClientConfig struct {
	CenterURL   string
	PublicKey   string
	LicenseKey  string
	Certificate string
	InstanceID  string
	LastError   string
	UpdatedAt   int64
}

const (
	ConfigLicenseCenterURL   = "license_center_url"
	ConfigLicensePublicKey   = "license_public_key"
	ConfigLicenseKey         = "license_key"
	ConfigLicenseCertificate = "license_certificate"
	ConfigLicenseInstanceID  = "license_instance_id"
	ConfigLicenseLastError   = "license_last_error"
	ConfigLicenseUpdatedAt   = "license_updated_at"
	ConfigIsCommercial       = "is_commercial"
)

func (r *Repository) GetLicenseClientConfig() (LicenseClientConfig, error) {
	if r == nil || r.db == nil {
		return LicenseClientConfig{}, errors.New("repository not initialized")
	}
	cfg, err := r.GetConfigsByNames([]string{
		ConfigLicenseCenterURL,
		ConfigLicensePublicKey,
		ConfigLicenseKey,
		ConfigLicenseCertificate,
		ConfigLicenseInstanceID,
		ConfigLicenseLastError,
		ConfigLicenseUpdatedAt,
	})
	if err != nil {
		return LicenseClientConfig{}, err
	}
	return LicenseClientConfig{
		CenterURL:   strings.TrimSpace(cfg[ConfigLicenseCenterURL]),
		PublicKey:   strings.TrimSpace(cfg[ConfigLicensePublicKey]),
		LicenseKey:  strings.TrimSpace(cfg[ConfigLicenseKey]),
		Certificate: strings.TrimSpace(cfg[ConfigLicenseCertificate]),
		InstanceID:  strings.TrimSpace(cfg[ConfigLicenseInstanceID]),
		LastError:   strings.TrimSpace(cfg[ConfigLicenseLastError]),
		UpdatedAt:   parseConfigInt64(cfg[ConfigLicenseUpdatedAt]),
	}, nil
}

func (r *Repository) SaveLicenseClientActivation(centerURL, publicKey, licenseKey, certificate, instanceID string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	values := map[string]string{
		ConfigLicenseCenterURL:   strings.TrimSpace(centerURL),
		ConfigLicensePublicKey:   strings.TrimSpace(publicKey),
		ConfigLicenseKey:         strings.TrimSpace(licenseKey),
		ConfigLicenseCertificate: strings.TrimSpace(certificate),
		ConfigLicenseInstanceID:  strings.TrimSpace(instanceID),
		ConfigLicenseLastError:   "",
		ConfigLicenseUpdatedAt:   formatConfigInt64(now),
		ConfigIsCommercial:       "true",
	}
	for name, value := range values {
		if err := r.UpsertConfig(name, value, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) SaveLicenseClientCertificate(certificate string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if err := r.UpsertConfig(ConfigLicenseCertificate, strings.TrimSpace(certificate), now); err != nil {
		return err
	}
	if err := r.UpsertConfig(ConfigLicenseLastError, "", now); err != nil {
		return err
	}
	return r.UpsertConfig(ConfigLicenseUpdatedAt, formatConfigInt64(now), now)
}

func (r *Repository) SaveLicenseClientError(message string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if strings.TrimSpace(message) == "" {
		message = "授权同步失败"
	}
	if err := r.UpsertConfig(ConfigLicenseLastError, message, now); err != nil {
		return err
	}
	return r.UpsertConfig(ConfigLicenseUpdatedAt, formatConfigInt64(now), now)
}

func (r *Repository) EnsureLicenseInstanceID(instanceID string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.UpsertConfig(ConfigLicenseInstanceID, strings.TrimSpace(instanceID), now)
}

func parseConfigInt64(value string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func formatConfigInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}

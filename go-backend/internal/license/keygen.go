package license

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var AccountID string

type KeygenClient struct {
	AccountID  string
	Token      string
	BaseURL    string
	HTTPClient *http.Client
}

type APIError struct {
	Operation  string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("keygen %s failed: status %d, response: %s", e.Operation, e.StatusCode, e.Body)
}

func (e *APIError) HasCode(code string) bool {
	return e != nil && hasKeygenErrorCode([]byte(e.Body), code)
}

const defaultAPIBaseURL = "https://api.keygen.sh/v1"

func NewKeygenClient(accountID, token string) *KeygenClient {
	return &KeygenClient{
		AccountID:  accountID,
		Token:      token,
		BaseURL:    defaultAPIBaseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *KeygenClient) apiURL(path string) string {
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}
	return fmt.Sprintf("%s/accounts/%s/%s", baseURL, c.AccountID, strings.TrimLeft(path, "/"))
}

type ValidateResponse struct {
	Meta struct {
		Valid bool   `json:"valid"`
		Code  string `json:"code"`
	} `json:"meta"`
	Data struct {
		ID         string `json:"id"`
		Attributes struct {
			Expiry string `json:"expiry"`
		} `json:"attributes"`
	} `json:"data"`
	MachineID string `json:"-"`
}

type ActivateMachineRequest struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Fingerprint string `json:"fingerprint"`
		} `json:"attributes"`
		Relationships struct {
			License struct {
				Data struct {
					Type string `json:"type"`
					ID   string `json:"id"`
				} `json:"data"`
			} `json:"license"`
		} `json:"relationships"`
	} `json:"data"`
}

type keygenErrorResponse struct {
	Errors []struct {
		Code string `json:"code"`
	} `json:"errors"`
}

type MachineResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

func hasKeygenErrorCode(body []byte, code string) bool {
	var resp keygenErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}
	for _, item := range resp.Errors {
		if strings.EqualFold(strings.TrimSpace(item.Code), code) {
			return true
		}
	}
	return false
}

func (c *KeygenClient) ValidateKeyWithFingerprint(key string, fingerprint string) (*ValidateResponse, error) {
	return c.ValidateKeyWithMachine(key, fingerprint, "")
}

func (c *KeygenClient) ValidateKeyWithMachine(key, fingerprint, machineID string) (*ValidateResponse, error) {
	url := c.apiURL("licenses/actions/validate-key")

	meta := map[string]interface{}{
		"key": key,
	}

	if fingerprint != "" {
		meta["scope"] = map[string]interface{}{
			"fingerprint": fingerprint,
		}
	}
	if machineID != "" {
		scope, _ := meta["scope"].(map[string]interface{})
		if scope == nil {
			scope = make(map[string]interface{})
			meta["scope"] = scope
		}
		scope["machine"] = machineID
	}

	reqBody := map[string]interface{}{
		"meta": meta,
	}

	bodyBytes, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		if !strings.HasPrefix(c.Token, "Bearer ") && !strings.HasPrefix(c.Token, "License ") {
			req.Header.Set("Authorization", "License "+c.Token)
		} else {
			req.Header.Set("Authorization", c.Token)
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{Operation: "validate license", StatusCode: resp.StatusCode, Body: string(body)}
	}

	var valResp ValidateResponse
	if err := json.NewDecoder(resp.Body).Decode(&valResp); err != nil {
		return nil, err
	}

	return &valResp, nil
}

func (c *KeygenClient) GetMachineID(fingerprint string) (string, error) {
	machineURL := c.apiURL("machines/" + url.PathEscape(fingerprint))
	req, err := http.NewRequest(http.MethodGet, machineURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.api+json")
	if c.Token != "" {
		if !strings.HasPrefix(c.Token, "Bearer ") && !strings.HasPrefix(c.Token, "License ") {
			req.Header.Set("Authorization", "License "+c.Token)
		} else {
			req.Header.Set("Authorization", c.Token)
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", &APIError{Operation: "retrieve machine", StatusCode: resp.StatusCode, Body: string(body)}
	}

	var machineResp MachineResponse
	if err := json.NewDecoder(resp.Body).Decode(&machineResp); err != nil {
		return "", err
	}
	machineID := strings.TrimSpace(machineResp.Data.ID)
	if machineID == "" {
		return "", fmt.Errorf("failed to retrieve machine: empty machine id")
	}
	return machineID, nil
}

func (c *KeygenClient) ValidateKey(key string) (*ValidateResponse, error) {
	url := c.apiURL("licenses/actions/validate-key")

	reqBody := map[string]interface{}{
		"meta": map[string]string{
			"key": key,
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		if !strings.HasPrefix(c.Token, "Bearer ") && !strings.HasPrefix(c.Token, "License ") {
			req.Header.Set("Authorization", "License "+c.Token)
		} else {
			req.Header.Set("Authorization", c.Token)
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{Operation: "validate license", StatusCode: resp.StatusCode, Body: string(body)}
	}

	var valResp ValidateResponse
	if err := json.NewDecoder(resp.Body).Decode(&valResp); err != nil {
		return nil, err
	}

	return &valResp, nil
}

func (c *KeygenClient) ActivateMachine(licenseID, fingerprint string) (string, error) {
	url := c.apiURL("machines")

	var reqBody ActivateMachineRequest
	reqBody.Data.Type = "machines"
	reqBody.Data.Attributes.Fingerprint = fingerprint
	reqBody.Data.Relationships.License.Data.Type = "licenses"
	reqBody.Data.Relationships.License.Data.ID = licenseID

	bodyBytes, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		if !strings.HasPrefix(c.Token, "Bearer ") && !strings.HasPrefix(c.Token, "License ") {
			req.Header.Set("Authorization", "License "+c.Token)
		} else {
			req.Header.Set("Authorization", c.Token)
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		var machineResp MachineResponse
		if json.Unmarshal(body, &machineResp) == nil {
			return strings.TrimSpace(machineResp.Data.ID), nil
		}
		return "", nil
	}

	if resp.StatusCode == http.StatusUnprocessableEntity && hasKeygenErrorCode(body, "FINGERPRINT_TAKEN") {
		// Machine activation is idempotent. Keygen scopes fingerprint uniqueness
		// to the target license, so this means the same machine is already bound.
		return "", nil
	}

	return "", &APIError{Operation: "activate machine", StatusCode: resp.StatusCode, Body: string(body)}
}

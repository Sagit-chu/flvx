package license

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type KeygenClient struct {
	AccountID  string
	Token      string
	HTTPClient *http.Client
}

func NewKeygenClient(accountID, token string) *KeygenClient {
	return &KeygenClient{
		AccountID: accountID,
		Token:     token,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type ValidateResponse struct {
	Meta struct {
		Valid bool   `json:"valid"`
		Code  string `json:"code"`
	} `json:"meta"`
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
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

func (c *KeygenClient) ValidateKeyWithFingerprint(key string, fingerprint string) (*ValidateResponse, error) {
	url := fmt.Sprintf("https://api.keygen.sh/v1/accounts/%s/licenses/actions/validate-key", c.AccountID)

	meta := map[string]interface{}{
		"key": key,
	}

	if fingerprint != "" {
		meta["scope"] = map[string]interface{}{
			"fingerprint": fingerprint,
		}
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
		return nil, fmt.Errorf("keygen api error: status %d", resp.StatusCode)
	}

	var valResp ValidateResponse
	if err := json.NewDecoder(resp.Body).Decode(&valResp); err != nil {
		return nil, err
	}

	return &valResp, nil
}

func (c *KeygenClient) ValidateKey(key string) (*ValidateResponse, error) {
	url := fmt.Sprintf("https://api.keygen.sh/v1/accounts/%s/licenses/actions/validate-key", c.AccountID)

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
		return nil, fmt.Errorf("keygen api error: status %d", resp.StatusCode)
	}

	var valResp ValidateResponse
	if err := json.NewDecoder(resp.Body).Decode(&valResp); err != nil {
		return nil, err
	}

	return &valResp, nil
}

func (c *KeygenClient) ActivateMachine(licenseID, fingerprint string) error {
	url := fmt.Sprintf("https://api.keygen.sh/v1/accounts/%s/machines", c.AccountID)

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
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnprocessableEntity {
		if strings.Contains(string(body), "FINGERPRINT_TAKEN") || strings.Contains(string(body), "MACHINE_LIMIT_EXCEEDED") {
			// Machine already registered to this license or limit reached because it's already us.
			// The subsequent ValidateKey check will determine if the existing machine is actually us.
			return nil
		}
	}

	return fmt.Errorf("failed to activate machine: status %d, response: %s", resp.StatusCode, string(body))
}
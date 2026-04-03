package license

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		req.Header.Set("Authorization", "Bearer "+c.Token)
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
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}

	if resp.StatusCode == http.StatusConflict { // 409 usually means fingerprint already exists
		return nil // Machine might already be registered
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to activate machine: status %d, response: %s", resp.StatusCode, string(body))
}
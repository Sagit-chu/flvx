package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type FederationClient struct {
	client *http.Client
}

type RemoteNodeInfo struct {
	ShareID        int64  `json:"shareId"`
	ShareName      string `json:"shareName"`
	NodeID         int64  `json:"nodeId"`
	NodeName       string `json:"nodeName"`
	ServerIP       string `json:"serverIp"`
	Status         int    `json:"status"`
	MaxBandwidth   int64  `json:"maxBandwidth"`
	CurrentFlow    int64  `json:"currentFlow"`
	ExpiryTime     int64  `json:"expiryTime"`
	PortRangeStart int    `json:"portRangeStart"`
	PortRangeEnd   int    `json:"portRangeEnd"`
}

type RemoteTunnelResponse struct {
	TunnelID int64 `json:"tunnelId"`
}

type RuntimeReservePortRequest struct {
	ResourceKey   string `json:"resourceKey"`
	Protocol      string `json:"protocol"`
	RequestedPort int    `json:"requestedPort"`
}

type RuntimeReservePortResponse struct {
	ReservationID string `json:"reservationId"`
	BindingID     string `json:"bindingId"`
	AllocatedPort int    `json:"allocatedPort"`
}

type RuntimeTarget struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type RuntimeApplyRoleRequest struct {
	ReservationID string          `json:"reservationId"`
	ResourceKey   string          `json:"resourceKey"`
	Role          string          `json:"role"`
	Protocol      string          `json:"protocol"`
	Strategy      string          `json:"strategy"`
	Targets       []RuntimeTarget `json:"targets"`
}

type RuntimeApplyRoleResponse struct {
	BindingID     string `json:"bindingId"`
	ReservationID string `json:"reservationId"`
	AllocatedPort int    `json:"allocatedPort"`
}

type RuntimeReleaseRoleRequest struct {
	BindingID     string `json:"bindingId"`
	ReservationID string `json:"reservationId"`
	ResourceKey   string `json:"resourceKey"`
}

type RuntimeDiagnoseRequest struct {
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	Count   int    `json:"count"`
	Timeout int    `json:"timeout"`
}

func NewFederationClient() *FederationClient {
	return &FederationClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func NewFederationClientWithTimeout(timeout time.Duration) *FederationClient {
	return &FederationClient{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *FederationClient) Connect(url, token, localDomain string) (*RemoteNodeInfo, error) {
	url = strings.TrimSuffix(url, "/")
	req, err := http.NewRequest("POST", url+"/api/v1/federation/connect", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if localDomain != "" {
		req.Header.Set("X-Panel-Domain", localDomain)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("remote error %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data RemoteNodeInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("remote api error: %s", res.Msg)
	}

	return &res.Data, nil
}

func (c *FederationClient) CreateTunnel(url, token, localDomain, protocol string, remotePort int, target string) (*RemoteTunnelResponse, error) {
	url = strings.TrimSuffix(url, "/")
	payload := map[string]interface{}{
		"protocol":   protocol,
		"remotePort": remotePort,
		"target":     target,
	}
	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url+"/api/v1/federation/tunnel/create", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if localDomain != "" {
		req.Header.Set("X-Panel-Domain", localDomain)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("remote error %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Code int                  `json:"code"`
		Msg  string               `json:"msg"`
		Data RemoteTunnelResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("remote api error: %s", res.Msg)
	}

	return &res.Data, nil
}

func (c *FederationClient) ReservePort(url, token, localDomain string, reqData RuntimeReservePortRequest) (*RuntimeReservePortResponse, error) {
	url = strings.TrimSuffix(url, "/")
	bodyBytes, _ := json.Marshal(reqData)
	req, err := http.NewRequest("POST", url+"/api/v1/federation/runtime/reserve-port", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if localDomain != "" {
		req.Header.Set("X-Panel-Domain", localDomain)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("remote error %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Code int                        `json:"code"`
		Msg  string                     `json:"msg"`
		Data RuntimeReservePortResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("remote api error: %s", res.Msg)
	}

	return &res.Data, nil
}

func (c *FederationClient) ApplyRole(url, token, localDomain string, reqData RuntimeApplyRoleRequest) (*RuntimeApplyRoleResponse, error) {
	url = strings.TrimSuffix(url, "/")
	bodyBytes, _ := json.Marshal(reqData)
	req, err := http.NewRequest("POST", url+"/api/v1/federation/runtime/apply-role", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if localDomain != "" {
		req.Header.Set("X-Panel-Domain", localDomain)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("remote error %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Code int                      `json:"code"`
		Msg  string                   `json:"msg"`
		Data RuntimeApplyRoleResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("remote api error: %s", res.Msg)
	}

	return &res.Data, nil
}

func (c *FederationClient) ReleaseRole(url, token, localDomain string, reqData RuntimeReleaseRoleRequest) error {
	url = strings.TrimSuffix(url, "/")
	bodyBytes, _ := json.Marshal(reqData)
	req, err := http.NewRequest("POST", url+"/api/v1/federation/runtime/release-role", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if localDomain != "" {
		req.Header.Set("X-Panel-Domain", localDomain)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remote error %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("remote api error: %s", res.Msg)
	}

	return nil
}

func (c *FederationClient) Diagnose(url, token, localDomain string, reqData RuntimeDiagnoseRequest) (map[string]interface{}, error) {
	url = strings.TrimSuffix(url, "/")
	bodyBytes, _ := json.Marshal(reqData)
	req, err := http.NewRequest("POST", url+"/api/v1/federation/runtime/diagnose", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if localDomain != "" {
		req.Header.Set("X-Panel-Domain", localDomain)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("remote error %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Code int                    `json:"code"`
		Msg  string                 `json:"msg"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("remote api error: %s", res.Msg)
	}

	if res.Data == nil {
		return nil, fmt.Errorf("remote api error: empty diagnosis payload")
	}

	return res.Data, nil
}

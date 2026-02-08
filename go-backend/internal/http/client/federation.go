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
	ExpiryTime     int64  `json:"expiryTime"`
	PortRangeStart int    `json:"portRangeStart"`
	PortRangeEnd   int    `json:"portRangeEnd"`
}

type RemoteTunnelResponse struct {
	TunnelID int64 `json:"tunnelId"`
}

func NewFederationClient() *FederationClient {
	return &FederationClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *FederationClient) Connect(url, token string) (*RemoteNodeInfo, error) {
	url = strings.TrimSuffix(url, "/")
	req, err := http.NewRequest("POST", url+"/api/v1/federation/connect", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
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

func (c *FederationClient) CreateTunnel(url, token string, protocol string, remotePort int, target string) (*RemoteTunnelResponse, error) {
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

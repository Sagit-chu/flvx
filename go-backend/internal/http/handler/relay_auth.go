package handler

import (
	"encoding/json"
	"fmt"
	"go-backend/internal/http/client"
	"go-backend/internal/relaysecurity"
	"go-backend/internal/store/repo"
	"gorm.io/gorm"
)

func (h *Handler) resolveTunnelRelayAuth(tx *gorm.DB, state *tunnelCreateState, create bool) error {
	if state.Type != 2 || state.RelayAuthMode == relaysecurity.Legacy {
		return nil
	}
	if state.RelayAuthMode != relaysecurity.Required {
		return fmt.Errorf("unsupported relay authentication mode")
	}
	for i := range state.OutNodes {
		c, err := h.repo.RelayEndpointCredentials(tx, state.TunnelID, state.OutNodes[i].NodeID, 3, create)
		if err != nil {
			return err
		}
		state.OutNodes[i].RelayAuth = c
	}
	for hop := range state.ChainHops {
		for i := range state.ChainHops[hop] {
			c, err := h.repo.RelayEndpointCredentials(tx, state.TunnelID, state.ChainHops[hop][i].NodeID, 2, create)
			if err != nil {
				return err
			}
			state.ChainHops[hop][i].RelayAuth = c
		}
	}
	return nil
}

func (h *Handler) resolveBestExitRelayAuth(tunnelID int64, targets []tunnelRuntimeNode) error {
	tunnel, err := h.repo.GetTunnelRecord(tunnelID)
	if err != nil {
		return err
	}
	if tunnel == nil {
		return fmt.Errorf("tunnel not found")
	}
	return h.resolveTunnelRelayAuth(nil, &tunnelCreateState{TunnelID: tunnelID, Type: tunnel.Type, RelayAuthMode: tunnel.RelayAuthMode, OutNodes: targets}, false)
}

func relayAuthVersion(c *relaysecurity.Credentials) int {
	if c == nil {
		return 0
	}
	return c.Version
}
func decodePersistedRelayAuth(raw string) (*relaysecurity.Credentials, error) {
	if raw == "" {
		return nil, nil
	}
	var c relaysecurity.Credentials
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, fmt.Errorf("invalid persisted relay authentication")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}
func validateFederationRelayAuth(persisted string, c *relaysecurity.Credentials, targets []federationRuntimeTarget) error {
	old, err := decodePersistedRelayAuth(persisted)
	if err != nil {
		return err
	}
	if old != nil && (c == nil || *old != *c) {
		return fmt.Errorf("authenticated relay cannot be downgraded or rotated by reapply")
	}
	if c != nil {
		if err := c.Validate(); err != nil {
			return err
		}
		for _, target := range targets {
			if err := target.RelayAuth.Validate(); err != nil {
				return err
			}
		}
	} else {
		for _, target := range targets {
			if target.RelayAuth != nil {
				if err := target.RelayAuth.Validate(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (h *Handler) preflightFederationRelayAuth(state *tunnelCreateState, domain string) error {
	if state.RelayAuthMode != relaysecurity.Required || state.Type != 2 {
		return nil
	}
	for _, node := range state.Nodes {
		if node == nil || node.IsRemote != 1 {
			continue
		}
		info, err := client.NewFederationClient().Connect(node.RemoteURL, node.RemoteToken, domain)
		if err != nil {
			return fmt.Errorf("relay authentication capability check failed for remote node %d", node.ID)
		}
		if info.RelayAuthVersion != relaysecurity.Version {
			return fmt.Errorf("remote node %d does not support required relay authentication", node.ID)
		}
	}
	return nil
}

func decodeRuntimeRelayAuth(runtime *repo.PeerShareRuntime) (*relaysecurity.Credentials, error) {
	c, err := decodePersistedRelayAuth(runtime.RelayAuth)
	if err != nil {
		return nil, err
	}
	if runtime.RelayAuthVersion != relayAuthVersion(c) {
		return nil, fmt.Errorf("persisted relay authentication version or credentials unavailable")
	}
	return c, nil
}

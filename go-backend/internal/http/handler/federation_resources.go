package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/store/repo"
	"go-backend/internal/ws"
)

const peerShareResourcePrefix = "peer-share-"

func peerShareResourceName(shareID int64, kind, original string) string {
	return fmt.Sprintf("%s%d-%s-%s", peerShareResourcePrefix, shareID, kind, base64.RawURLEncoding.EncodeToString([]byte(original)))
}

func parsePeerShareServiceName(name string) (shareID int64, originalName string, ok bool) {
	if !strings.HasPrefix(name, peerShareResourcePrefix) {
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(name, peerShareResourcePrefix), "-", 3)
	if len(parts) != 3 || parts[1] != "service" {
		return
	}
	shareID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || shareID <= 0 {
		return 0, "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(decoded) == 0 {
		return 0, "", false
	}
	return shareID, string(decoded), true
}

func federationResourceCommandKind(cmd string) (kind, action string) {
	lower := strings.ToLower(cmd)
	for _, entry := range []struct{ suffix, kind string }{{"service", "service"}, {"chains", "chain"}, {"climiters", "climiter"}, {"limiters", "limiter"}} {
		if strings.HasSuffix(lower, entry.suffix) {
			return entry.kind, strings.TrimSuffix(lower, entry.suffix)
		}
	}
	return "", ""
}

// Every peer-supplied reference is resolved within the same share namespace.
// Composite traffic limiters use a comma-separated list in GOST.
func scopePeerResourceReferences(value interface{}, shareID int64) {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, child := range v {
			if strings.EqualFold(key, "chains") {
				if names, ok := child.([]interface{}); ok {
					for i, name := range names {
						if raw, ok := name.(string); ok {
							names[i] = peerShareResourceName(shareID, "chain", raw)
						}
					}
					continue
				}
			}
			kind := ""
			switch strings.ToLower(key) {
			case "chain":
				kind = "chain"
			case "limiter":
				kind = "limiter"
			case "climiter":
				kind = "climiter"
			}
			if raw, ok := child.(string); ok && kind != "" && strings.TrimSpace(raw) != "" {
				names := strings.Split(raw, ",")
				for i, name := range names {
					names[i] = peerShareResourceName(shareID, kind, strings.TrimSpace(name))
				}
				v[key] = strings.Join(names, ",")
			} else {
				scopePeerResourceReferences(child, shareID)
			}
		}
	case []interface{}:
		for _, child := range v {
			scopePeerResourceReferences(child, shareID)
		}
	}
}

func peerResourceDeletePayload(item repo.PeerShareResource) interface{} {
	if item.Kind == "service" {
		return map[string]interface{}{"services": []string{item.RuntimeName}}
	}
	key := item.Kind
	if key == "climiter" {
		key = "limiter"
	}
	return map[string]interface{}{key: item.RuntimeName}
}
func peerResourceDeleteCommand(kind string) string {
	switch kind {
	case "service":
		return "DeleteService"
	case "chain":
		return "DeleteChains"
	case "climiter":
		return "DeleteCLimiters"
	default:
		return "DeleteLimiters"
	}
}

// preparePeerResourceCommand validates and writes desired state before the node
// sees any command, closing the flow-report race even without a reservation.
func (h *Handler) preparePeerResourceCommand(share *repo.PeerShare, cmd string, data interface{}) ([]repo.PeerShareResource, error) {
	kind, action := federationResourceCommandKind(cmd)
	if kind == "" {
		return nil, fmt.Errorf("command not allowed")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var decoded interface{}
	if err = json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	var configs []map[string]interface{}
	var names []string
	setting := action == "add" || action == "update"
	if setting {
		if kind == "service" {
			configs = extractFederationServiceEntries(decoded)
		} else if m, ok := decoded.(map[string]interface{}); ok {
			if nested, ok := m["data"].(map[string]interface{}); ok {
				configs = []map[string]interface{}{nested}
			} else {
				configs = []map[string]interface{}{m}
			}
		}
		if len(configs) == 0 {
			return nil, fmt.Errorf("resource configuration is required")
		}
		for _, c := range configs {
			names = append(names, strings.TrimSpace(asString(c["name"])))
		}
	} else {
		if kind == "service" {
			if m, ok := decoded.(map[string]interface{}); ok {
				for _, v := range asAnySlice(m["services"]) {
					names = append(names, strings.TrimSpace(asString(v)))
				}
			}
		} else {
			if s, ok := decoded.(string); ok {
				names = []string{strings.TrimSpace(s)}
			} else if m, ok := decoded.(map[string]interface{}); ok {
				key := kind
				if key == "climiter" {
					key = "limiter"
				}
				names = []string{strings.TrimSpace(asString(m[key]))}
			}
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("resource name is required")
		}
	}
	requestedNames := make(map[string]bool, len(names))
	for _, name := range names {
		requestedNames[name] = true
	}
	items := make([]repo.PeerShareResource, 0, len(names))
	seen := map[string]bool{}
	for i, name := range names {
		if name == "" || strings.HasPrefix(name, peerShareResourcePrefix) {
			return nil, fmt.Errorf("invalid peer resource name")
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate resource name")
		}
		seen[name] = true
		old, err := h.repo.GetPeerShareResource(share.ID, kind, name)
		if err != nil {
			return nil, err
		}
		item := repo.PeerShareResource{ShareID: share.ID, NodeID: share.NodeID, Kind: kind, OriginalName: name, RuntimeName: peerShareResourceName(share.ID, kind, name), DesiredState: "active", UpdatedTime: time.Now().UnixMilli()}
		if old != nil {
			item = *old
			item.Applied = 0
			item.UpdatedTime = time.Now().UnixMilli()
		}
		if setting {
			if old == nil && kind == "service" {
				legacy, err := h.peerShareLegacyServiceNames(share, name)
				if err != nil {
					return nil, err
				}
				if len(legacy) > 0 {
					encoded, _ := json.Marshal(legacy)
					item.LegacyNames = string(encoded)
					item.LegacyServiceBase = normalizeForwardRuntimeServiceName(name)
				}
			}
			config := configs[i]
			if err := validatePeerResourceReferences(config, kind); err != nil {
				return nil, err
			}
			scopePeerResourceReferences(config, share.ID)
			config["name"] = item.RuntimeName
			encoded, err := json.Marshal(config)
			if err != nil {
				return nil, err
			}
			item.Config = string(encoded)
			item.DesiredState = "active"
			item.ReleaseLegacyFamily = false
		} else {
			if old == nil {
				// Delete callers send candidate base/TCP/UDP names. Missing names
				// are safe no-ops; never forward a raw legacy name to the node.
				if action == "delete" {
					if kind != "service" {
						continue
					}
					legacy, err := h.peerShareLegacyServiceNames(share, name)
					if err != nil {
						return nil, err
					}
					if len(legacy) == 0 {
						continue
					}
					encoded, _ := json.Marshal(legacy)
					item.LegacyNames = string(encoded)
					item.LegacyServiceBase = normalizeForwardRuntimeServiceName(name)
				} else {
					return nil, fmt.Errorf("service %q not found", name)
				}
			}
			if old != nil && old.DesiredState == "deleted" && action != "delete" {
				return nil, fmt.Errorf("service %q not found", name)
			}
			switch action {
			case "delete":
				item.DesiredState = "deleted"
				base := normalizeForwardRuntimeServiceName(name)
				if requestedNames[base] && requestedNames[base+"_tcp"] && requestedNames[base+"_udp"] {
					item.ReleaseLegacyFamily = true
				}
			case "pause":
				item.DesiredState = "paused"
			case "resume":
				if item.Config == "" {
					return nil, fmt.Errorf("resource has no saved configuration")
				}
				item.DesiredState = "active"
			default:
				return nil, fmt.Errorf("command not allowed")
			}
		}
		items = append(items, item)
	}
	// Ownership and desired config commit atomically. A failed resource write
	// must not rename a legacy binding and expose its remaining transports.
	err = h.repo.WithPeerShareResourceTransaction(func(tx *repo.Repository) error {
		if setting && kind == "service" {
			scoped := make([]interface{}, 0, len(items))
			for _, item := range items {
				var c map[string]interface{}
				_ = json.Unmarshal([]byte(item.Config), &c)
				scoped = append(scoped, c)
			}
			binder := &Handler{repo: tx}
			if err := binder.bindPeerShareForwardRuntimeServices(share, scoped); err != nil {
				return err
			}
		}
		return tx.SavePeerShareResources(items)
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (h *Handler) applyPeerShareResource(item repo.PeerShareResource) (ws.CommandResult, error) {
	var result ws.CommandResult
	var err error
	if item.LegacyNames != "" {
		var names []string
		if err := json.Unmarshal([]byte(item.LegacyNames), &names); err != nil {
			return result, err
		}
		share, loadErr := h.repo.GetPeerShare(item.ShareID)
		if loadErr != nil {
			return result, loadErr
		}
		if share == nil {
			return result, fmt.Errorf("legacy resource ownership missing")
		}
		for _, name := range names {
			owned, checkErr := h.peerShareLegacyServiceNames(share, name)
			if checkErr != nil {
				return result, checkErr
			}
			if len(owned) == 0 {
				return result, fmt.Errorf("legacy resource ownership missing")
			}
		}
		if _, err = h.sendNodeCommand(item.NodeID, "DeleteService", map[string]interface{}{"services": names}, false, true); err != nil {
			return result, err
		}
		if err = h.repo.ClearPeerShareResourceLegacyNames(item.ShareID, item.Kind, item.OriginalName); err != nil {
			return result, err
		}
	}
	if item.DesiredState == "deleted" || item.DesiredState == "paused" {
		// The provider retains the paused configuration durably. Keep the
		// listener absent on reconnect instead of briefly starting it before
		// sending a second pause command. Resume reapplies the saved config.
		result, err = h.sendNodeCommand(item.NodeID, peerResourceDeleteCommand(item.Kind), peerResourceDeletePayload(item), false, true)
	} else {
		var config map[string]interface{}
		if err = json.Unmarshal([]byte(item.Config), &config); err != nil {
			return result, err
		}
		if item.Kind == "service" {
			result, err = h.sendNodeCommand(item.NodeID, "UpdateService", []interface{}{config}, false, false)
		} else {
			suffix := "Limiters"
			key := "limiter"
			if item.Kind == "chain" {
				suffix = "Chains"
				key = "chain"
			}
			if item.Kind == "climiter" {
				suffix = "CLimiters"
			}
			result, err = h.sendNodeCommand(item.NodeID, "Add"+suffix, config, false, false)
			if err != nil && isAlreadyExistsMessage(err.Error()) {
				result, err = h.sendNodeCommand(item.NodeID, "Update"+suffix, map[string]interface{}{key: item.RuntimeName, "data": config}, false, false)
			}
		}
	}
	if err != nil {
		return result, err
	}
	if item.Kind == "service" && item.DesiredState == "deleted" {
		// TCP and UDP may share one reservation. Release only after all variants
		// have an acknowledged tombstone.
		items, listErr := h.repo.ListPeerShareResourcesByNode(item.NodeID)
		if listErr != nil {
			return result, listErr
		}
		base := normalizeForwardRuntimeServiceName(item.OriginalName)
		for _, other := range items {
			if other.ShareID == item.ShareID && other.Kind == "service" && other.OriginalName != item.OriginalName && normalizeForwardRuntimeServiceName(other.OriginalName) == base && (other.DesiredState != "deleted" || other.Applied == 0) {
				return result, h.repo.MarkPeerShareResourceApplied(item.ShareID, item.Kind, item.OriginalName)
			}
		}
		legacyBase := item.LegacyServiceBase
		if legacyBase == "" {
			for _, other := range items {
				if other.ShareID == item.ShareID && normalizeForwardRuntimeServiceName(other.OriginalName) == base && other.LegacyServiceBase != "" {
					legacyBase = other.LegacyServiceBase
					break
				}
			}
		}
		if legacyBase != "" && item.ReleaseLegacyFamily {
			share, loadErr := h.repo.GetPeerShare(item.ShareID)
			if loadErr != nil {
				return result, loadErr
			}
			if share == nil {
				return result, fmt.Errorf("share ownership missing")
			}
			if _, checkErr := h.peerShareLegacyServiceNames(share, legacyBase); checkErr != nil {
				return result, checkErr
			}
			if _, deleteErr := h.sendNodeCommand(item.NodeID, "DeleteService", map[string]interface{}{"services": buildForwardServiceDeleteNames([]string{legacyBase})}, false, true); deleteErr != nil {
				return result, deleteErr
			}
		}
		if legacyBase != "" && !item.ReleaseLegacyFamily {
			return result, h.repo.MarkPeerShareResourceApplied(item.ShareID, item.Kind, item.OriginalName)
		}
		runtimes, listErr := h.repo.ListActivePeerShareRuntimesByShareID(item.ShareID)
		if listErr != nil {
			return result, listErr
		}
		return result, h.repo.WithPeerShareResourceTransaction(func(tx *repo.Repository) error {
			for _, runtime := range runtimes {
				sid, original, scoped := parsePeerShareServiceName(runtime.ServiceName)
				ownedScoped := scoped && sid == item.ShareID && normalizeForwardRuntimeServiceName(original) == base
				ownedLegacy := !scoped && runtime.Role == "forward" && item.ReleaseLegacyFamily && legacyBase != "" && normalizeForwardRuntimeServiceName(runtime.ServiceName) == legacyBase
				if ownedScoped || ownedLegacy {
					if err = tx.CompletePeerShareRuntimeRelease(runtime.ID); err != nil {
						return err
					}
				}
			}
			if legacyBase != "" && item.ReleaseLegacyFamily {
				if err := tx.ClearPeerShareResourceLegacyFamily(item.ShareID, legacyBase); err != nil {
					return err
				}
			}
			return tx.MarkPeerShareResourceApplied(item.ShareID, item.Kind, item.OriginalName)
		})
	}
	return result, h.repo.MarkPeerShareResourceApplied(item.ShareID, item.Kind, item.OriginalName)
}

func orderPeerShareResources(items []repo.PeerShareResource) {
	rank := func(item repo.PeerShareResource) int {
		if item.DesiredState == "deleted" {
			if item.Kind == "service" {
				return 0
			}
			return 1
		}
		if item.Kind == "service" {
			return 4
		}
		if item.Kind == "chain" {
			return 3
		}
		return 2
	}
	sort.SliceStable(items, func(i, j int) bool { return rank(items[i]) < rank(items[j]) })
}

func (h *Handler) reconcilePeerShareResourcesOnNode(nodeID int64) error {
	return h.reconcilePeerShareResources(nodeID, false)
}
func (h *Handler) retryPendingPeerShareResourcesOnNode(nodeID int64) error {
	return h.reconcilePeerShareResources(nodeID, true)
}
func (h *Handler) reconcilePeerShareResources(nodeID int64, pendingOnly bool) error {
	h.peerResourceMu.Lock()
	defer h.peerResourceMu.Unlock()
	items, err := h.repo.ListPeerShareResourcesByNode(nodeID)
	if err != nil {
		return err
	}
	runtimes, err := h.repo.ListActivePeerShareRuntimesByNode(nodeID)
	if err != nil {
		return err
	}
	pending := map[string]bool{}
	for _, runtime := range runtimes {
		if runtime.ReleasePending != 0 {
			sid, name, ok := parsePeerShareServiceName(runtime.ServiceName)
			if ok {
				pending[fmt.Sprintf("%d:%s", sid, normalizeForwardRuntimeServiceName(name))] = true
			}
		}
	}
	groups := map[int64][]repo.PeerShareResource{}
	var ids []int64
	for _, item := range items {
		if _, ok := groups[item.ShareID]; !ok {
			ids = append(ids, item.ShareID)
		}
		groups[item.ShareID] = append(groups[item.ShareID], item)
	}
	var failures []error
	for _, shareID := range ids {
		share, err := h.repo.GetPeerShare(shareID)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		expired := share == nil || share.IsActive != 1 || (share.ExpiryTime > 0 && share.ExpiryTime <= time.Now().UnixMilli()) || isPeerShareFlowExceeded(share)
		group := groups[shareID]
		var changed []repo.PeerShareResource
		for i := range group {
			item := &group[i]
			if (expired || (item.Kind == "service" && pending[fmt.Sprintf("%d:%s", item.ShareID, normalizeForwardRuntimeServiceName(item.OriginalName))])) && (item.DesiredState != "deleted" || item.Applied == 0 || item.LegacyServiceBase != "") {
				item.DesiredState = "deleted"
				item.ReleaseLegacyFamily = true
				item.Applied = 0
				changed = append(changed, *item)
			}
		}
		if err := h.repo.SavePeerShareResources(changed); err != nil {
			failures = append(failures, err)
			continue
		}
		orderPeerShareResources(group)
		dependencyFailed := false
		deletionFailed := false
		for _, item := range group {
			if item.Applied == 1 && (pendingOnly || item.DesiredState == "deleted") {
				continue
			}
			if dependencyFailed && item.Kind == "service" && item.DesiredState != "deleted" {
				continue
			}
			if deletionFailed && item.Kind != "service" && item.DesiredState == "deleted" {
				continue
			}
			if _, err := h.applyPeerShareResource(item); err != nil {
				failures = append(failures, fmt.Errorf("share %d %s %s: %w", item.ShareID, item.Kind, item.OriginalName, err))
				if item.Kind != "service" {
					dependencyFailed = true
				}
				if item.DesiredState == "deleted" {
					deletionFailed = true
				}
			}
		}
	}
	return errors.Join(failures...)
}

func (h *Handler) releasePeerShareResources(shareID int64) error {
	h.peerResourceMu.Lock()
	defer h.peerResourceMu.Unlock()
	share, err := h.repo.GetPeerShare(shareID)
	if err != nil {
		return err
	}
	if share == nil {
		return nil
	}
	all, err := h.repo.ListPeerShareResourcesByNode(share.NodeID)
	if err != nil {
		return err
	}
	var items []repo.PeerShareResource
	for _, item := range all {
		if item.ShareID == shareID && !(item.DesiredState == "deleted" && item.Applied == 1 && item.LegacyServiceBase == "") {
			item.DesiredState = "deleted"
			item.ReleaseLegacyFamily = true
			item.Applied = 0
			items = append(items, item)
		}
	}
	if err = h.repo.SavePeerShareResources(items); err != nil {
		return err
	}
	orderPeerShareResources(items)
	for _, item := range items {
		if _, err = h.applyPeerShareResource(item); err != nil {
			return err
		}
	}
	return nil
}

// A legacy name is eligible for migration only when its runtime registration
// proves unique ownership. An identically numbered local forward is ambiguous.
func (h *Handler) peerShareLegacyServiceNames(share *repo.PeerShare, original string) ([]string, error) {
	base := normalizeForwardRuntimeServiceName(original)
	runtimes, err := h.repo.ListActiveForwardPeerShareRuntimesByNodeAndServiceName(share.NodeID, base)
	if err != nil {
		return nil, err
	}
	owned := false
	for _, runtime := range runtimes {
		if runtime.ShareID != share.ID {
			return nil, fmt.Errorf("legacy resource %q has ambiguous ownership", original)
		}
		if runtime.ReleasePending != 0 {
			return nil, fmt.Errorf("runtime release is pending")
		}
		owned = true
	}
	if !owned {
		resources, err := h.repo.ListPeerShareResourcesByNode(share.NodeID)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			if resource.ShareID == share.ID && resource.LegacyServiceBase == base {
				owned = true
				break
			}
		}
	}
	if !owned {
		return nil, nil
	}
	if id, _, _, ok := parseFlowServiceIDs(base); ok {
		local, err := h.repo.GetForwardRecord(id)
		if err != nil {
			return nil, err
		}
		if local != nil {
			return nil, fmt.Errorf("legacy resource %q collides with a local forward", original)
		}
	}
	// Only delete the requested transport. Other variants remain registered to
	// their legacy runtime until they are independently migrated.
	return []string{original}, nil
}

// releasePeerShareForwardRuntimeResources removes exact persisted transport
// names belonging to a single forward reservation, retaining failed tombstones.
func (h *Handler) releasePeerShareForwardRuntimeResources(runtime *repo.PeerShareRuntime) error {
	h.peerResourceMu.Lock()
	defer h.peerResourceMu.Unlock()
	shareID, original, ok := parsePeerShareServiceName(runtime.ServiceName)
	if !ok || shareID != runtime.ShareID {
		return fmt.Errorf("runtime is not a scoped shared forward")
	}
	all, err := h.repo.ListPeerShareResourcesByNode(runtime.NodeID)
	if err != nil {
		return err
	}
	var items []repo.PeerShareResource
	for _, item := range all {
		if item.ShareID == runtime.ShareID && item.Kind == "service" && normalizeForwardRuntimeServiceName(item.OriginalName) == normalizeForwardRuntimeServiceName(original) {
			item.DesiredState = "deleted"
			item.ReleaseLegacyFamily = true
			item.Applied = 0
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return fmt.Errorf("shared forward resource ownership is missing")
	}
	if err = h.repo.SavePeerShareResources(items); err != nil {
		return err
	}
	for _, item := range items {
		if _, err = h.applyPeerShareResource(item); err != nil {
			return err
		}
	}
	return nil
}

// Registry kinds without an owned command API cannot be referenced by peers.
func validatePeerResourceReferences(value interface{}, kind string) error {
	var walk func(interface{}) error
	walk = func(value interface{}) error {
		switch v := value.(type) {
		case map[string]interface{}:
			for key, child := range v {
				switch strings.ToLower(key) {
				case "auther", "authers", "admission", "admissions", "bypass", "bypasses", "resolver", "hosts", "rlimiter", "logger", "loggers", "observer", "recorders", "hop", "sd":
					if child != nil {
						empty := false
						switch x := child.(type) {
						case string:
							empty = strings.TrimSpace(x) == ""
						case []interface{}:
							empty = len(x) == 0
						}
						if !empty {
							return fmt.Errorf("unsupported shared registry reference: %s", key)
						}
					}
				case "forwarder":
					if f, ok := child.(map[string]interface{}); ok {
						for field, value := range f {
							if strings.EqualFold(field, "name") && strings.TrimSpace(asString(value)) != "" {
								return fmt.Errorf("named shared forwarder references are unsupported")
							}
						}
					}
				case "hops":
					for _, hop := range asMapSlice(child) {
						if _, ok := hop["nodes"]; !ok {
							return fmt.Errorf("shared chains require inline hop nodes")
						}
						for _, loader := range []string{"file", "redis", "http", "plugin"} {
							if hop[loader] != nil {
								return fmt.Errorf("shared hop loaders are unsupported")
							}
						}
					}
				}
				if err := walk(child); err != nil {
					return err
				}
			}
		case []interface{}:
			for _, child := range v {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if kind == "limiter" || kind == "climiter" {
		if config, ok := value.(map[string]interface{}); ok {
			for _, loader := range []string{"file", "redis", "http", "plugin"} {
				if config[loader] != nil {
					return fmt.Errorf("shared limiter loaders are unsupported")
				}
			}
		}
	}
	return walk(value)
}

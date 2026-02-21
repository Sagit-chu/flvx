package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"go-backend/internal/http/response"
)

const (
	githubRepo     = "Sagit-chu/flvx"
	githubProxy    = "https://gcode.hostcentral.cc"
	githubAPIBase  = "https://api.github.com"
	githubHTMLBase = "https://github.com"
	upgradeTimeout = 5 * time.Minute
	batchWorkers   = 5

	releaseChannelStable = "stable"
	releaseChannelDev    = "dev"
)

var (
	stableVersionPattern = regexp.MustCompile(`^\d+(?:\.\d+)+$`)
	testKeywordPattern   = regexp.MustCompile(`(?i)(alpha|beta|rc)`)
)

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
	Draft       bool   `json:"draft"`
}

func normalizeReleaseChannel(channel string) string {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case releaseChannelDev:
		return releaseChannelDev
	default:
		return releaseChannelStable
	}
}

func releaseChannelFromTag(tag string) string {
	normalized := strings.ToLower(strings.TrimSpace(tag))
	if normalized == "" {
		return releaseChannelDev
	}
	if testKeywordPattern.MatchString(normalized) {
		return releaseChannelDev
	}
	if stableVersionPattern.MatchString(normalized) {
		return releaseChannelStable
	}

	return releaseChannelDev
}

func releaseChannelLabel(channel string) string {
	if normalizeReleaseChannel(channel) == releaseChannelDev {
		return "测试版"
	}

	return "正式版"
}

func fetchGitHubReleases(perPage int) ([]githubRelease, error) {
	if perPage <= 0 {
		perPage = 20
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/repos/%s/releases?per_page=%d", githubAPIBase, githubRepo, perPage))
	if err != nil {
		return nil, fmt.Errorf("请求GitHub API失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API返回 %d: %s", resp.StatusCode, string(body))
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("解析GitHub API响应失败: %v", err)
	}

	return releases, nil
}

func resolveLatestReleaseByChannel(channel string) (string, error) {
	normalizedChannel := normalizeReleaseChannel(channel)
	releases, err := fetchGitHubReleases(50)
	if err != nil {
		return "", err
	}

	for _, r := range releases {
		if r.Draft {
			continue
		}
		tag := strings.TrimSpace(r.TagName)
		if tag == "" {
			continue
		}
		if releaseChannelFromTag(tag) == normalizedChannel {
			return tag, nil
		}
	}

	return "", fmt.Errorf("未找到%s版本号", releaseChannelLabel(normalizedChannel))
}

func (h *Handler) nodeUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		ID      int64  `json:"id"`
		Version string `json:"version"`
		Channel string `json:"channel"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("节点ID无效"))
		return
	}

	channel := normalizeReleaseChannel(req.Channel)
	version := strings.TrimSpace(req.Version)
	if version == "" {
		var err error
		version, err = resolveLatestReleaseByChannel(channel)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, fmt.Sprintf("获取最新%s失败: %v", releaseChannelLabel(channel), err)))
			return
		}
	}

	downloadURL := fmt.Sprintf(
		githubProxy+"/%s/%s/releases/download/%s/gost-{ARCH}",
		githubHTMLBase, githubRepo, version,
	)
	checksumURL := fmt.Sprintf(
		githubProxy+"/%s/%s/releases/download/%s/gost-{ARCH}.sha256",
		githubHTMLBase, githubRepo, version,
	)

	result, err := h.wsServer.SendCommand(req.ID, "UpgradeAgent", map[string]interface{}{
		"downloadUrl": downloadURL,
		"checksumUrl": checksumURL,
	}, upgradeTimeout)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("升级失败: %v", err)))
		return
	}
	h.markNodePendingUpgradeRedeploy(req.ID)

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"version": version,
		"message": result.Message,
	}))
}

func resolveLatestRelease() (string, error) {
	return resolveLatestReleaseByChannel(releaseChannelStable)
}

func resolveLatestReleaseAPI() (string, error) {
	return resolveLatestReleaseByChannel(releaseChannelStable)
}

func (h *Handler) nodeBatchUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		IDs     []int64 `json:"ids"`
		Version string  `json:"version"`
		Channel string  `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if len(req.IDs) == 0 {
		response.WriteJSON(w, response.ErrDefault("ids不能为空"))
		return
	}

	channel := normalizeReleaseChannel(req.Channel)
	version := strings.TrimSpace(req.Version)
	if version == "" {
		var err error
		version, err = resolveLatestReleaseByChannel(channel)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, fmt.Sprintf("获取最新%s失败: %v", releaseChannelLabel(channel), err)))
			return
		}
	}

	downloadURL := fmt.Sprintf(
		githubProxy+"/%s/%s/releases/download/%s/gost-{ARCH}",
		githubHTMLBase, githubRepo, version,
	)
	checksumURL := fmt.Sprintf(
		githubProxy+"/%s/%s/releases/download/%s/gost-{ARCH}.sha256",
		githubHTMLBase, githubRepo, version,
	)

	type upgradeResult struct {
		ID      int64  `json:"id"`
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	results := make([]upgradeResult, len(req.IDs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup

	for i, id := range req.IDs {
		wg.Add(1)
		go func(index int, nodeID int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := h.wsServer.SendCommand(nodeID, "UpgradeAgent", map[string]interface{}{
				"downloadUrl": downloadURL,
				"checksumUrl": checksumURL,
			}, upgradeTimeout)
			if err != nil {
				results[index] = upgradeResult{ID: nodeID, Success: false, Message: err.Error()}
				return
			}
			h.markNodePendingUpgradeRedeploy(nodeID)
			results[index] = upgradeResult{ID: nodeID, Success: true, Message: result.Message}
		}(i, id)
	}
	wg.Wait()

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"version": version,
		"results": results,
	}))
}

func (h *Handler) listReleases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		Channel string `json:"channel"`
	}
	if err := decodeJSON(r.Body, &req); err != nil && err != io.EOF {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	channel := normalizeReleaseChannel(req.Channel)

	releases, err := fetchGitHubReleases(50)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("获取版本列表失败: %v", err)))
		return
	}

	type releaseItem struct {
		Version     string `json:"version"`
		Name        string `json:"name"`
		PublishedAt string `json:"publishedAt"`
		Prerelease  bool   `json:"prerelease"`
		Channel     string `json:"channel"`
	}

	items := make([]releaseItem, 0, len(releases))
	for _, r := range releases {
		if r.Draft {
			continue
		}
		tag := strings.TrimSpace(r.TagName)
		if tag == "" {
			continue
		}
		itemChannel := releaseChannelFromTag(tag)
		if itemChannel != channel {
			continue
		}
		items = append(items, releaseItem{
			Version:     tag,
			Name:        r.Name,
			PublishedAt: r.PublishedAt,
			Prerelease:  itemChannel == releaseChannelDev,
			Channel:     itemChannel,
		})
	}

	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) nodeRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		ID int64 `json:"id"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("节点ID无效"))
		return
	}

	result, err := h.wsServer.SendCommand(req.ID, "RollbackAgent", map[string]interface{}{}, 30*time.Second)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("回退失败: %v", err)))
		return
	}

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"message": result.Message,
	}))
}

func (h *Handler) markNodePendingUpgradeRedeploy(nodeID int64) {
	if h == nil || nodeID <= 0 {
		return
	}
	h.upgradeMu.Lock()
	h.pendingUpgradeRedeploy[nodeID] = struct{}{}
	h.upgradeMu.Unlock()
}

func (h *Handler) consumeNodePendingUpgradeRedeploy(nodeID int64) bool {
	if h == nil || nodeID <= 0 {
		return false
	}
	h.upgradeMu.Lock()
	_, ok := h.pendingUpgradeRedeploy[nodeID]
	if ok {
		delete(h.pendingUpgradeRedeploy, nodeID)
	}
	h.upgradeMu.Unlock()
	return ok
}

func (h *Handler) onNodeOnline(nodeID int64) {
	if !h.consumeNodePendingUpgradeRedeploy(nodeID) {
		return
	}
	h.redeployNodeRuntimeAfterUpgrade(nodeID)
}

func (h *Handler) redeployNodeRuntimeAfterUpgrade(nodeID int64) {
	tunnelIDs, err := h.repo.ListActiveTunnelIDsByNode(nodeID)
	if err != nil {
		fmt.Printf("post-upgrade redeploy: list tunnels for node %d failed: %v\n", nodeID, err)
		return
	}
	forwardIDs, err := h.repo.ListActiveForwardIDsByNode(nodeID)
	if err != nil {
		fmt.Printf("post-upgrade redeploy: list forwards for node %d failed: %v\n", nodeID, err)
		return
	}

	tunnelFailed := make(map[int64]struct{})
	for _, tunnelID := range tunnelIDs {
		if err := h.redeployTunnelAndForwards(tunnelID); err != nil {
			tunnelFailed[tunnelID] = struct{}{}
			fmt.Printf("post-upgrade redeploy: tunnel %d failed on node %d: %v\n", tunnelID, nodeID, err)
		}
	}

	for _, forwardID := range forwardIDs {
		forward, getErr := h.getForwardRecord(forwardID)
		if getErr != nil || forward == nil {
			continue
		}
		if _, skipped := tunnelFailed[forward.TunnelID]; skipped {
			continue
		}
		if err := h.syncForwardServices(forward, "UpdateService", true); err != nil {
			fmt.Printf("post-upgrade redeploy: forward %d failed on node %d: %v\n", forwardID, nodeID, err)
		}
	}
}

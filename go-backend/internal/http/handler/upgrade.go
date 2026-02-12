package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
)

func (h *Handler) nodeUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		ID      int64  `json:"id"`
		Version string `json:"version"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("节点ID无效"))
		return
	}

	version := strings.TrimSpace(req.Version)
	if version == "" {
		var err error
		version, err = resolveLatestRelease()
		if err != nil {
			response.WriteJSON(w, response.Err(-2, fmt.Sprintf("获取最新版本失败: %v", err)))
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

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"version": version,
		"message": result.Message,
	}))
}

func resolveLatestRelease() (string, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(githubProxy + "/" + githubHTMLBase + "/" + githubRepo + "/releases/latest")
	if err != nil {
		return "", fmt.Errorf("请求GitHub失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusMovedPermanently {
		return resolveLatestReleaseAPI()
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return resolveLatestReleaseAPI()
	}

	parts := strings.Split(location, "/")
	tag := parts[len(parts)-1]
	if tag == "" || tag == "latest" {
		return resolveLatestReleaseAPI()
	}

	return tag, nil
}

func resolveLatestReleaseAPI() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(githubAPIBase + "/repos/" + githubRepo + "/releases/latest")
	if err != nil {
		return "", fmt.Errorf("请求GitHub API失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("GitHub API返回 %d: %s", resp.StatusCode, string(body))
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("解析GitHub API响应失败: %v", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return "", fmt.Errorf("无法从GitHub获取最新版本号")
	}

	return release.TagName, nil
}

func (h *Handler) nodeBatchUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		IDs     []int64 `json:"ids"`
		Version string  `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if len(req.IDs) == 0 {
		response.WriteJSON(w, response.ErrDefault("ids不能为空"))
		return
	}

	version := strings.TrimSpace(req.Version)
	if version == "" {
		var err error
		version, err = resolveLatestRelease()
		if err != nil {
			response.WriteJSON(w, response.Err(-2, fmt.Sprintf("获取最新版本失败: %v", err)))
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

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(githubAPIBase + "/repos/" + githubRepo + "/releases?per_page=20")
	if err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("获取版本列表失败: %v", err)))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("获取版本列表失败: GitHub API返回 %d: %s", resp.StatusCode, string(body))))
		return
	}

	var releases []struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		PublishedAt string `json:"published_at"`
		Prerelease  bool   `json:"prerelease"`
		Draft       bool   `json:"draft"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("解析版本列表失败: %v", err)))
		return
	}

	type releaseItem struct {
		Version     string `json:"version"`
		Name        string `json:"name"`
		PublishedAt string `json:"publishedAt"`
		Prerelease  bool   `json:"prerelease"`
	}

	items := make([]releaseItem, 0, len(releases))
	for _, r := range releases {
		if r.Draft {
			continue
		}
		items = append(items, releaseItem{
			Version:     r.TagName,
			Name:        r.Name,
			PublishedAt: r.PublishedAt,
			Prerelease:  r.Prerelease,
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

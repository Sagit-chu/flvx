package panelupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

const helperTimeout = 30 * time.Minute

var verificationTimeout = 5 * time.Minute

type composeConfig struct {
	FullServices map[string]map[string]any `json:"-"`
	Services     map[string]struct {
		Image         string            `json:"image"`
		ContainerName string            `json:"container_name"`
		Volumes       []map[string]any  `json:"volumes"`
		Environment   map[string]string `json:"environment"`
	} `json:"services"`
	Volumes  map[string]any `json:"volumes"`
	Networks map[string]any `json:"networks"`
}

func composeArgs(p Plan, dir string, args ...string) []string {
	return append([]string{"compose", "--project-name", p.Project, "--project-directory", p.HostDir, "--env-file", filepath.Join(dir, ".env"), "-f", filepath.Join(dir, "docker-compose.yml")}, args...)
}
func config(ctx context.Context, p Plan, dir string) (composeConfig, error) {
	var c composeConfig
	b, e := docker(ctx, composeArgs(p, dir, "config", "--format", "json")...)
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	if e == nil {
		var raw struct {
			Services map[string]map[string]any `json:"services"`
		}
		e = json.Unmarshal(b, &raw)
		c.FullServices = raw.Services
	}
	return c, e
}
func validateConfig(old, next composeConfig, p Plan) error {
	// Compare complete normalized services, not a projection that can silently drop
	// host bindings, commands, security options, networks or dependency changes.
	if old.FullServices == nil || next.FullServices == nil {
		return errors.New("missing full Compose configuration")
	}
	raw, _ := json.Marshal(old.FullServices)
	var expected map[string]map[string]any
	if json.Unmarshal(raw, &expected) != nil {
		return errors.New("invalid configuration")
	}
	for _, service := range []string{"backend", "frontend"} {
		if expected[service] == nil || next.FullServices[service] == nil {
			return errors.New("missing service")
		}
		expected[service]["image"] = next.FullServices[service]["image"]
	}
	if env, ok := expected["backend"]["environment"].(map[string]any); ok {
		env["FLUX_VERSION"] = p.Job.TargetVersion
	} else {
		return errors.New("missing backend environment")
	}
	if !reflect.DeepEqual(expected, next.FullServices) {
		return errors.New("unsupported service configuration change; manual upgrade required")
	}

	for service, image := range map[string]string{"backend": "ghcr.io/sagit-chu/flux-panel-backend:", "frontend": "ghcr.io/sagit-chu/vite-frontend:"} {
		a, ok := old.Services[service]
		b, present := next.Services[service]
		if !ok || !present || b.Image != image+p.Job.TargetVersion || a.ContainerName != b.ContainerName || !reflect.DeepEqual(a.Volumes, b.Volumes) {
			return errors.New("service image, container identity or persistent mounts do not match the supported deployment")
		}
	}
	if next.Services["backend"].ContainerName != p.Backend || next.Services["backend"].Environment["FLUX_VERSION"] != p.Job.TargetVersion {
		return errors.New("backend identity/version mismatch")
	}

	for service := range next.Services {
		if service != "backend" && service != "frontend" && service != "postgres" {
			return errors.New("unsupported service")
		}
	}
	for key, value := range old.Services["backend"].Environment {
		if key != "FLUX_VERSION" && next.Services["backend"].Environment[key] != value {
			return errors.New("backend environment changed; manual upgrade required")
		}
	}
	for key, value := range next.Services["backend"].Environment {
		if key != "FLUX_VERSION" && old.Services["backend"].Environment[key] != value {
			return errors.New("backend environment changed; manual upgrade required")
		}
	}
	// Database configuration and shared resource identities are not upgrade targets.
	if !reflect.DeepEqual(old.Services["postgres"], next.Services["postgres"]) || !reflect.DeepEqual(old.Volumes, next.Volumes) || !reflect.DeepEqual(old.Networks, next.Networks) {
		return errors.New("database or shared resource configuration changed; manual upgrade required")
	}
	return nil
}

// validateLive proves the normalized source configuration still describes the running
// backend. In particular, Compose must not attach a fresh database volume by accident.
func validateLive(ctx context.Context, p Plan, c composeConfig) error {
	live, err := inspect(ctx, p.Backend)
	if err != nil {
		return err
	}
	if live.Image != p.Image || !live.State.Running || live.Config.Labels["com.docker.compose.project"] != p.Project || live.Config.Labels["com.docker.compose.service"] != "backend" {
		return errors.New("running backend changed")
	}
	if len(live.Mounts) != len(c.Services["backend"].Volumes) {
		return errors.New("unexpected live mounts; manual upgrade required")
	}
	for _, v := range c.Services["backend"].Volumes {
		typ, _ := v["type"].(string)
		src, _ := v["source"].(string)
		dst, _ := v["target"].(string)
		if typ == "volume" {
			spec, ok := c.Volumes[src].(map[string]any)
			if !ok {
				return errors.New("unknown volume")
			}
			src, _ = spec["name"].(string)
		}
		if src == "" || dst == "" || (typ != "volume" && typ != "bind") {
			return errors.New("unsupported mount")
		}
		found := false
		for _, mount := range live.Mounts {
			actual := mount.Source
			if typ == "volume" {
				actual = mount.Name
			}
			if mount.Type == typ && mount.Destination == dst && actual == src && mount.RW != (v["read_only"] == true) {
				found = true
			}
		}
		if !found {
			return errors.New("live persistent mount does not match Compose")
		}
	}
	env := map[string]string{}
	for _, v := range live.Config.Env {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	for _, key := range []string{"DB_TYPE", "DB_PATH", "DATABASE_URL", "PANEL_DEPLOY_DIR", "PANEL_BACKEND_CONTAINER"} {
		if env[key] != c.Services["backend"].Environment[key] {
			return errors.New("live database/deployment environment does not match Compose")
		}
	}
	return nil
}

// RunHelper runs before app/database initialization and does not depend on the old backend process.
func RunHelper(planPath string) error {
	b, e := os.ReadFile(planPath)
	if e != nil {
		return e
	}
	var p Plan
	if e = json.Unmarshal(b, &p); e != nil {
		return e
	}
	if !idPattern.MatchString(p.Job.ID) || ValidateVersion(p.Job.TargetVersion) != nil || !projectPattern.MatchString(p.Project) {
		return errors.New("invalid plan")
	}
	m := Manager{Dir: p.HostDir, Backend: p.Backend}
	dir := m.jobDir(p.Job.ID)
	if filepath.Clean(planPath) != filepath.Join(dir, "plan.json") {
		return errors.New("invalid plan location")
	}
	// Reject duplicate helper invocations without affecting the live job.
	f, e := os.OpenFile(filepath.Join(dir, "helper.started"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	f.Close()
	j, e := m.read(p.Job.ID)
	if e != nil {
		return e
	}
	if !j.Active {
		return errors.New("job is not active")
	}
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()
	fail := func(msg string) error {
		if e := m.write(j, "failed", msg); e != nil {
			return e
		}
		return errors.New(msg)
	}
	recovery := func(msg string) error {
		m.write(j, "recovery_required", msg+"；请管理员核对容器、数据库和任务备份后人工恢复，禁止自动回滚或重复升级")
		return errors.New(msg)
	}
	if e = m.write(j, "preparing", "校验部署配置和持久化资源"); e != nil {
		return e
	}
	old, e := config(ctx, p, p.HostDir)
	if e != nil {
		return fail("现有 Compose 配置校验失败，线上配置未变更")
	}
	next, e := config(ctx, p, dir)
	if e != nil {
		return fail("目标 Compose 配置校验失败，线上配置未变更")
	}
	if validateConfig(old, next, p) != nil {
		return fail("目标镜像、服务或持久化资源不匹配，需手动升级")
	}
	if validateLive(ctx, p, old) != nil {
		return fail("运行中后端与部署配置不一致，需手动升级")
	}
	if e = m.write(j, "pulling", "正在拉取并检查目标镜像，线上配置尚未变更"); e != nil {
		return e
	}
	if _, e = docker(ctx, composeArgs(p, dir, "pull", "backend", "frontend")...); e != nil {
		return fail("镜像拉取失败或超时，线上配置未变更")
	}
	targets := map[string]string{}
	for _, service := range []string{"backend", "frontend"} {
		out, err := docker(ctx, "image", "inspect", "--format", "{{.Id}}", next.Services[service].Image)
		if err != nil || !strings.HasPrefix(strings.TrimSpace(string(out)), "sha256:") {
			return fail("目标镜像校验失败，线上配置未变更")
		}
		targets[service] = strings.TrimSpace(string(out))
	}
	if validateLive(ctx, p, old) != nil {
		return fail("拉取期间运行中后端发生变化，已安全取消")
	}
	// Detect edits since snapshots before replacing either live file.
	for _, name := range []string{"docker-compose.yml", ".env"} {
		live, err := os.ReadFile(filepath.Join(p.HostDir, name))
		if err != nil {
			return fail("读取线上配置失败")
		}
		backup, err := os.ReadFile(filepath.Join(dir, name+".original"))
		if err != nil || string(live) != string(backup) {
			return fail("线上配置已被修改，本任务已安全取消")
		}
	}
	if e = m.write(j, "applying", "正在应用已校验配置"); e != nil {
		return e
	}
	for _, name := range []string{"docker-compose.yml", ".env"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			err = atomic(filepath.Join(p.HostDir, name), data)
		}
		if err != nil {
			for _, restore := range []string{"docker-compose.yml", ".env"} {
				original, re := os.ReadFile(filepath.Join(dir, restore+".original"))
				if re != nil || atomic(filepath.Join(p.HostDir, restore), original) != nil {
					return recovery("配置恢复失败，尚未执行容器重建")
				}
			}
			return fail("应用配置失败，已恢复原配置，未执行容器重建")
		}
	}
	// Persist the point of no automatic rollback BEFORE Docker can recreate any service.
	if e = m.write(j, "restarting", "正在重建面板服务"); e != nil {
		return recovery("无法记录重建状态，已停止执行")
	}
	if _, e = docker(ctx, composeArgs(p, p.HostDir, "up", "-d", "--no-deps", "--force-recreate", "--pull", "never", "backend", "frontend")...); e != nil {
		return recovery("容器重建失败或超时，可能已部分生效")
	}
	if e = m.write(j, "verifying", "正在验证服务版本及健康状态"); e != nil {
		return e
	}
	deadline := time.Now().Add(verificationTimeout)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		ready := true
		for _, service := range []string{"backend", "frontend"} {
			c, err := inspect(ctx, next.Services[service].ContainerName)
			if err != nil || !c.State.Running || c.Image != targets[service] || c.Config.Labels["com.docker.compose.project"] != p.Project || c.Config.Labels["com.docker.compose.service"] != service {
				ready = false
				continue
			}
			if c.State.Health != nil && c.State.Health.Status != "healthy" {
				ready = false
			}
			if service == "backend" && c.State.Health == nil {
				ready = false
			}
		}
		if ready { // Frontend has no Docker healthcheck in the stock template.
			probeCtx, probeCancel := context.WithTimeout(ctx, 10*time.Second)
			_, probeErr := docker(probeCtx, "exec", next.Services["frontend"].ContainerName, "wget", "-q", "-T", "5", "-O", "/dev/null", "http://127.0.0.1:80/")
			probeCancel()
			ready = probeErr == nil
		}
		if ready {
			return m.write(j, "succeeded", "升级完成，目标镜像和服务健康状态已验证")
		}
		select {
		case <-ctx.Done():
		case <-time.After(3 * time.Second):
		}
	}
	return recovery("升级后服务健康检查失败或超时")
}

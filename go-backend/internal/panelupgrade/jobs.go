// Package panelupgrade implements durable, conservative Docker Compose upgrades.
package panelupgrade

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var versionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9][A-Za-z0-9.-]*)?$`)
var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var projectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func ValidateVersion(v string) error {
	if !versionPattern.MatchString(v) || len(v) > 128 {
		return errors.New("invalid release version")
	}
	return nil
}

type Job struct {
	ID               string `json:"id"`
	TargetVersion    string `json:"targetVersion"`
	Stage            string `json:"stage"`
	Active           bool   `json:"active"`
	RequiresRecovery bool   `json:"requiresRecovery"`
	Message          string `json:"message"`
	StartedAt        int64  `json:"startedAt"`
	UpdatedAt        int64  `json:"updatedAt"`
	HelperContainer  string `json:"helperContainer,omitempty"`
	BackupDir        string `json:"backupDir,omitempty"`
}
type Plan struct {
	Job                                         Job
	DeployDir, HostDir, Project, Backend, Image string
}
type Manager struct{ Dir, Backend string }

func (m Manager) root() string            { return filepath.Join(m.Dir, ".panel-upgrades") }
func (m Manager) jobDir(id string) string { return filepath.Join(m.root(), id) }
func atomic(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".write-")
	if e != nil {
		return e
	}
	n := f.Name()
	defer os.Remove(n)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(n, path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func save(path string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return atomic(path, b)
}
func (m Manager) lock() (func(), error) {
	if e := os.MkdirAll(m.root(), 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(m.root(), "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("another upgrade operation is in progress")
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
func docker(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.WaitDelay = 2 * time.Second
	b, e := cmd.Output()
	if e != nil {
		return nil, errors.New("Docker operation failed (details suppressed to protect credentials)")
	}
	return b, nil
}

type container struct {
	Image  string
	Config struct {
		Labels map[string]string
		Env    []string
	}
	State struct {
		Running bool
		Health  *struct{ Status string }
	}
	Mounts []struct {
		Type, Source, Destination, Name string
		RW                              bool
	}
}

func inspect(ctx context.Context, name string) (container, error) {
	var c []container
	b, e := docker(ctx, "inspect", name)
	if e != nil {
		return container{}, e
	}
	if json.Unmarshal(b, &c) != nil || len(c) != 1 {
		return container{}, errors.New("invalid Docker inspection")
	}
	return c[0], nil
}
func (m Manager) Identity(ctx context.Context) (Plan, error) {
	p := Plan{DeployDir: m.Dir, Backend: m.Backend}
	if !filepath.IsAbs(m.Dir) || filepath.Clean(m.Dir) != m.Dir {
		return p, errors.New("deployment directory must be an absolute clean path")
	}
	c, e := inspect(ctx, m.Backend)
	if e != nil {
		return p, e
	}
	p.Image = c.Image
	p.Project = c.Config.Labels["com.docker.compose.project"]
	if !c.State.Running || !projectPattern.MatchString(p.Project) || c.Config.Labels["com.docker.compose.service"] != "backend" || !strings.HasPrefix(p.Image, "sha256:") {
		return p, errors.New("cannot establish original Compose identity")
	}
	for _, v := range c.Mounts {
		if v.Destination == m.Dir && v.Type == "bind" && v.RW {
			p.HostDir = v.Source
		}
	}
	if p.HostDir == "/" || !filepath.IsAbs(p.HostDir) || strings.ContainsAny(p.HostDir, ",\n\r") || filepath.Clean(p.HostDir) != p.HostDir {
		return p, errors.New("deployment directory must be a writable host bind mount")
	}

	for _, protected := range []string{"/app/paneld", "/usr/local/bin/docker", "/usr/local/libexec/docker/cli-plugins/docker-compose", "/var/run/docker.sock", "/bin", "/lib", "/etc", "/proc", "/sys", "/dev"} {
		if p.HostDir == protected || strings.HasPrefix(protected, p.HostDir+"/") || strings.HasPrefix(p.HostDir, protected+"/") {
			return p, errors.New("host deployment mount would shadow helper runtime paths")
		}
	}
	if c.Config.Labels["com.docker.compose.project.working_dir"] != p.HostDir || c.Config.Labels["com.docker.compose.project.config_files"] != filepath.Join(p.HostDir, "docker-compose.yml") {
		return p, errors.New("custom or multiple Compose paths require manual upgrade")
	}
	return p, nil
}
func (m Manager) read(id string) (*Job, error) {
	if !idPattern.MatchString(id) {
		return nil, errors.New("invalid job ID")
	}
	b, e := os.ReadFile(filepath.Join(m.jobDir(id), "status.json"))
	if e != nil {
		return nil, e
	}
	var j Job
	e = json.Unmarshal(b, &j)
	return &j, e
}
func (m Manager) Status(ctx context.Context, id string) (*Job, error) {
	if id == "" {
		b, e := os.ReadFile(filepath.Join(m.root(), "latest"))
		if os.IsNotExist(e) {
			return nil, nil
		}
		if e != nil {
			return nil, e
		}
		id = string(b)
	}
	j, e := m.read(id)
	if e != nil {
		return nil, e
	}
	if !j.Active || time.Now().UnixMilli()-j.UpdatedAt < 90000 {
		return j, nil
	}
	// An unavailable daemon, missing helper, or uncertain launch must never release the task lock.
	c, e := inspect(ctx, j.HelperContainer)
	if e == nil && c.State.Running {
		return j, nil
	}
	j.RequiresRecovery = true
	j.Message = "升级 helper 已停止或无法确认状态。请管理员检查任务备份和容器状态；在确认安全前禁止再次升级。"
	// This is a derived view: do not race the helper's durable state writes.
	j.Stage = "recovery_required"
	return j, nil
}
func (m Manager) write(j *Job, stage, msg string) error {
	j.Stage = stage
	j.Message = msg
	j.UpdatedAt = time.Now().UnixMilli()
	j.Active = stage != "succeeded" && stage != "failed"
	j.RequiresRecovery = stage == "recovery_required"
	if e := save(filepath.Join(m.jobDir(j.ID), "status.json"), j); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(m.jobDir(j.ID), "events.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = fmt.Fprintf(f, "%d %s %s\n", j.UpdatedAt, stage, msg)
	return e
}
func (m Manager) Start(ctx context.Context, version string, compose []byte) (*Job, string, error) {
	if e := ValidateVersion(version); e != nil {
		return nil, "", e
	}
	unlock, e := m.lock()
	if e != nil {
		return nil, "", e
	}
	defer unlock()
	old, e := m.Status(ctx, "")
	if e != nil {
		return nil, "", e
	}
	if old != nil && (old.Active || old.RequiresRecovery) {
		return old, "", errors.New("已有面板升级任务执行中或需要人工恢复")
	}
	p, e := m.Identity(ctx)
	if e != nil {
		return nil, "", e
	}
	b := make([]byte, 16)
	if _, e = rand.Read(b); e != nil {
		return nil, "", e
	}
	id := hex.EncodeToString(b)
	dir := m.jobDir(id)
	if e = os.Mkdir(dir, 0700); e != nil {
		return nil, "", e
	}
	j := Job{ID: id, TargetVersion: version, StartedAt: time.Now().UnixMilli(), HelperContainer: "flvx-upgrade-" + id, BackupDir: filepath.Join(p.HostDir, ".panel-upgrades", id)}
	p.Job = j
	if e = m.write(&j, "preparing", "正在准备独立升级任务"); e != nil {
		return nil, "", e
	}
	if e = atomic(filepath.Join(m.root(), "latest"), []byte(id)); e != nil {
		return nil, "", e
	}
	fail := func(err error) (*Job, string, error) {
		m.write(&j, "failed", "准备失败，线上配置未变更")
		return &j, p.Image, err
	}
	for _, name := range []string{"docker-compose.yml", ".env"} {
		data, err := os.ReadFile(filepath.Join(m.Dir, name))
		if err != nil {
			return fail(err)
		}
		if err = atomic(filepath.Join(dir, name+".original"), data); err != nil {
			return fail(err)
		}
	}
	env, e := os.ReadFile(filepath.Join(dir, ".env.original"))
	if e != nil {
		return fail(e)
	}
	lines := strings.Split(string(env), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "FLUX_VERSION=") || strings.HasPrefix(strings.TrimSpace(line), "export FLUX_VERSION=") {
			lines[i] = ""
		}
	}
	env = []byte(strings.Join(lines, "\n") + "\nFLUX_VERSION=" + version + "\n")
	if e = atomic(filepath.Join(dir, ".env"), env); e != nil {
		return fail(e)
	}
	if e = atomic(filepath.Join(dir, "docker-compose.yml"), compose); e != nil {
		return fail(e)
	}
	p.Job = j
	if e = save(filepath.Join(dir, "plan.json"), p); e != nil {
		return fail(e)
	}
	// No --rm: an exited helper is evidence needed after a daemon/client timeout.
	args := []string{"run", "-d", "--name", j.HelperContainer, "--label", "io.flvx.upgrade-job=" + id, "--mount", "type=bind,src=" + p.HostDir + ",dst=" + p.HostDir, "--mount", "type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock", "--entrypoint", "/app/paneld", p.Image, "panel-upgrade-helper", filepath.Join(p.HostDir, ".panel-upgrades", id, "plan.json")}
	if _, e = docker(ctx, args...); e != nil {
		c, ie := inspect(ctx, j.HelperContainer)
		if ie != nil || !c.State.Running {
			return &j, p.Image, errors.New("helper 启动结果不确定；任务已保留，请查询升级状态，勿重复启动")
		}
	}
	return &j, p.Image, nil
}

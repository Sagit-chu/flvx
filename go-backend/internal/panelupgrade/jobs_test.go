package panelupgrade

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeDocker(t *testing.T, body string) {
	t.Helper()
	d := t.TempDir()
	if e := os.WriteFile(filepath.Join(d, "docker"), []byte("#!/bin/sh\n"+body), 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
}
func fixture(t *testing.T) (Manager, Plan) {
	t.Helper()
	dir := t.TempDir()
	m := Manager{Dir: dir, Backend: "flux-panel-backend"}
	id := strings.Repeat("a", 32)
	os.MkdirAll(m.jobDir(id), 0700)
	j := Job{ID: id, TargetVersion: "2.5.0", HelperContainer: "helper", StartedAt: time.Now().UnixMilli()}
	m.write(&j, "preparing", "ready")
	atomic(filepath.Join(m.root(), "latest"), []byte(id))
	p := Plan{Job: j, HostDir: dir, DeployDir: "/opt/flvx-panel", Project: "original-project", Backend: m.Backend, Image: "sha256:old"}
	save(filepath.Join(m.jobDir(id), "plan.json"), p)
	for _, n := range []string{".env", "docker-compose.yml"} {
		atomic(filepath.Join(dir, n), []byte("original"))
		atomic(filepath.Join(m.jobDir(id), n+".original"), []byte("original"))
		atomic(filepath.Join(m.jobDir(id), n), []byte("target"))
	}
	return m, p
}
func TestVersionRejectsComposeAndPathInjection(t *testing.T) {
	for _, v := range []string{"latest", "../v1", "1.2.3\nSECRET=x", "1.2.3${TOKEN}", "1.2.3 x", "1.2.3;id"} {
		if ValidateVersion(v) == nil {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"2.5.0-rc10", "v2.5.0", "2.1.9-beta6"} {
		if ValidateVersion(v) != nil {
			t.Fatal(v)
		}
	}
}
func TestPersistentLockAcrossManagers(t *testing.T) {
	m, p := fixture(t)
	fakeDocker(t, "exit 1\n")
	_, _, e := m.Start(context.Background(), "2.5.0", nil)
	if e == nil {
		t.Fatal("accepted concurrent job")
	}
	b, _ := os.ReadFile(filepath.Join(m.root(), "latest"))
	if string(b) != p.Job.ID {
		t.Fatal("changed active job")
	}
}
func TestUncertainHelperNeverUnlocks(t *testing.T) {
	m, p := fixture(t)
	j, _ := m.read(p.Job.ID)
	j.UpdatedAt = 1
	save(filepath.Join(m.jobDir(j.ID), "status.json"), j)
	fakeDocker(t, "exit 1\n")
	j, e := m.Status(context.Background(), "")
	if e != nil || !j.Active || !j.RequiresRecovery || j.Stage != "recovery_required" {
		t.Fatalf("%+v %v", j, e)
	}
	if _, _, e = m.Start(context.Background(), "2.5.0", nil); e == nil {
		t.Fatal("uncertain launch unlocked")
	}
}
func configs(p Plan) (string, string) {
	old := composeConfig{}
	json.Unmarshal([]byte(`{"services":{"backend":{"image":"old","container_name":"flux-panel-backend","environment":{"FLUX_VERSION":"old"}},"frontend":{"image":"old","container_name":"vite-frontend"}}}`), &old)
	a, _ := json.Marshal(old)
	next := old
	next.Services = make(map[string]struct {
		Image         string            `json:"image"`
		ContainerName string            `json:"container_name"`
		Volumes       []map[string]any  `json:"volumes"`
		Environment   map[string]string `json:"environment"`
	})
	for k, v := range old.Services {
		next.Services[k] = v
	}
	v := next.Services["backend"]
	v.Image = "ghcr.io/sagit-chu/flux-panel-backend:" + p.Job.TargetVersion
	v.Environment = map[string]string{"FLUX_VERSION": p.Job.TargetVersion}
	next.Services["backend"] = v
	v = next.Services["frontend"]
	v.Image = "ghcr.io/sagit-chu/vite-frontend:" + p.Job.TargetVersion
	next.Services["frontend"] = v
	b, _ := json.Marshal(next)
	return string(a), string(b)
}
func TestHelperFailureBoundaries(t *testing.T) {
	for _, stage := range []string{"config", "pull", "apply", "up", "health", "success"} {
		t.Run(stage, func(t *testing.T) {
			m, p := fixture(t)
			old, next := configs(p)
			dir := m.jobDir(p.Job.ID)
			if stage == "apply" {
				os.Remove(filepath.Join(dir, ".env"))
				os.Mkdir(filepath.Join(dir, ".env"), 0700)
			}
			if stage == "health" {
				before := verificationTimeout
				verificationTimeout = 0
				t.Cleanup(func() { verificationTimeout = before })
			}
			script := `case "$*" in
 *"config --format json"*)
 case "$*" in *"` + p.Job.ID + `"*) echo '` + next + `';; *) echo '` + old + `';; esac
 ;;
 *"pull backend frontend"*) `
			if stage == "pull" {
				script += "echo SECRET >&2; exit 1"
			} else {
				script += "exit 0"
			}
			script += `;;
 "image inspect"*) echo sha256:target;;
 *"up -d"*) `
			if stage == "up" {
				script += "exit 1"
			} else {
				script += "touch \"" + filepath.Join(dir, "recreated") + "\"; exit 0"
			}
			script += `;;
 "inspect flux-panel-backend") if [ -f "` + filepath.Join(dir, "recreated") + `" ]; then IMG=sha256:target; else IMG=sha256:old; fi; echo '[{"Image":"'"$IMG"'","State":{"Running":true,"Health":{"Status":"healthy"}},"Config":{"Labels":{"com.docker.compose.project":"original-project","com.docker.compose.service":"backend"}}}]';;
 "inspect vite-frontend") echo '[{"Image":"sha256:target","State":{"Running":true},"Config":{"Labels":{"com.docker.compose.project":"original-project","com.docker.compose.service":"frontend"}}}]';;
 "exec vite-frontend wget"*) exit 0;;
 *) exit 1;; esac
`
			if stage == "config" {
				script = "echo SECRET >&2;exit 1\n"
			}
			fakeDocker(t, script)
			e := RunHelper(filepath.Join(dir, "plan.json"))
			j, _ := m.read(p.Job.ID)
			want := "failed"
			if stage == "up" || stage == "health" {
				want = "recovery_required"
			}
			if stage == "success" {
				want = "succeeded"
				if e != nil {
					t.Fatal(e)
				}
			}
			if j.Stage != want {
				t.Fatalf("stage=%s error=%v", j.Stage, e)
			}
			for _, n := range []string{".env", "docker-compose.yml"} {
				b, _ := os.ReadFile(filepath.Join(m.Dir, n))
				expected := "original"
				if stage == "up" || stage == "success" || stage == "health" {
					expected = "target"
				}
				if string(b) != expected {
					t.Fatalf("%s mutated unexpectedly", n)
				}
				b, _ = os.ReadFile(filepath.Join(dir, n+".original"))
				if string(b) != "original" {
					t.Fatal("backup overwritten")
				}
			}
			b, _ := os.ReadFile(filepath.Join(dir, "events.log"))
			if strings.Contains(string(b), "SECRET") {
				t.Fatal("secret leaked")
			}
		})
	}
}
func TestComposeUsesOriginalHostPathAndProject(t *testing.T) {
	p := Plan{Project: "my-project", HostDir: "/srv/panel"}
	s := strings.Join(composeArgs(p, "/srv/panel/.panel-upgrades/job", "pull", "backend", "frontend"), " ")
	if !strings.Contains(s, "--project-name my-project --project-directory /srv/panel") || strings.Contains(s, "remove-orphans") {
		t.Fatal(s)
	}
}
func TestStatusWithoutJobNeedsNoDocker(t *testing.T) {
	fakeDocker(t, "exit 99\n")
	j, e := (Manager{Dir: t.TempDir()}).Status(context.Background(), "")
	if e != nil || j != nil {
		t.Fatalf("%v %v", j, e)
	}
}

func TestFullConfigRejectsUnintendedChanges(t *testing.T) {
	for _, field := range []string{"ports", "network_mode", "command", "entrypoint", "privileged", "networks"} {
		t.Run(field, func(t *testing.T) {
			_, p := fixture(t)
			oldJSON, nextJSON := configs(p)
			var old, next composeConfig
			json.Unmarshal([]byte(oldJSON), &old)
			json.Unmarshal([]byte(nextJSON), &next)
			var a, b struct {
				Services map[string]map[string]any `json:"services"`
			}
			json.Unmarshal([]byte(oldJSON), &a)
			json.Unmarshal([]byte(nextJSON), &b)
			old.FullServices = a.Services
			next.FullServices = b.Services
			old.FullServices["frontend"][field] = "private-custom-setting"
			if validateConfig(old, next, p) == nil {
				t.Fatal("accepted service setting removal: " + field)
			}
		})
	}
}
func TestLiveRejectsShadowMount(t *testing.T) {
	_, p := fixture(t)
	fakeDocker(t, `echo '[{"Image":"sha256:old","State":{"Running":true},"Config":{"Labels":{"com.docker.compose.project":"original-project","com.docker.compose.service":"backend"}},"Mounts":[{"Type":"bind","Source":"/srv/db/gost.db","Destination":"/app/data/gost.db","RW":true}]}]'`)
	oldJSON, _ := configs(p)
	var old composeConfig
	json.Unmarshal([]byte(oldJSON), &old)
	if validateLive(context.Background(), p, old) == nil {
		t.Fatal("accepted undeclared shadow mount")
	}
}
func identityJSON(dir, host, project, files string) string {
	c := map[string]any{"Image": "sha256:old", "State": map[string]any{"Running": true}, "Config": map[string]any{"Labels": map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": "backend", "com.docker.compose.project.working_dir": host, "com.docker.compose.project.config_files": files}}, "Mounts": []map[string]any{{"Type": "bind", "Source": host, "Destination": dir, "RW": true}}}
	b, _ := json.Marshal([]any{c})
	return string(b)
}
func TestStartUsesHostMountAndKeepsUniqueBackups(t *testing.T) {
	dir := t.TempDir()
	m := Manager{Dir: dir, Backend: "backend"}
	for _, n := range []string{".env", "docker-compose.yml", ".env.upgrade.bak"} {
		os.WriteFile(filepath.Join(dir, n), []byte("original-secret"), 0600)
	}
	args := filepath.Join(t.TempDir(), "args")
	fakeDocker(t, `case "$1" in inspect) echo '`+identityJSON(dir, "/srv/original-panel", "original-project", "/srv/original-panel/docker-compose.yml")+`';; run) printf '%s\n' "$@" > '`+args+`'; echo container-id;; *) exit 1;; esac`)
	j, _, e := m.Start(context.Background(), "2.5.0-rc10", []byte("target"))
	if e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(args)
	s := string(b)
	for _, expected := range []string{"type=bind,src=/srv/original-panel,dst=/srv/original-panel", "/app/paneld", "sha256:old", "panel-upgrade-helper"} {
		if !strings.Contains(s, expected) {
			t.Fatal(s)
		}
	}
	if strings.Contains(s, "--rm") || strings.Contains(s, "--volumes-from") {
		t.Fatal(s)
	}
	first := j.ID
	m.write(j, "failed", "safe failure")
	j, _, e = m.Start(context.Background(), "2.5.1", []byte("target2"))
	if e != nil || j.ID == first {
		t.Fatalf("%v %v", j, e)
	}
	for _, n := range []string{".env", "docker-compose.yml", ".env.upgrade.bak"} {
		b, _ := os.ReadFile(filepath.Join(dir, n))
		if string(b) != "original-secret" {
			t.Fatal("live or legacy overwritten")
		}
	}
	b, _ = os.ReadFile(filepath.Join(m.jobDir(first), ".env.original"))
	if string(b) != "original-secret" {
		t.Fatal("old job backup changed")
	}
}
func TestIdentityFailsClosed(t *testing.T) {
	for _, host := range []string{"/", "/app", "/usr", "/var/run", "/proc", "relative", "/srv/bad,host"} {
		t.Run(host, func(t *testing.T) {
			dir := t.TempDir()
			fakeDocker(t, "echo '"+identityJSON(dir, host, "project", filepath.Join(host, "docker-compose.yml"))+"'\n")
			if _, e := (Manager{Dir: dir, Backend: "backend"}).Identity(context.Background()); e == nil {
				t.Fatal(host)
			}
		})
	}
}
func TestUncertainRunIsNotRetriedOrUnlocked(t *testing.T) {
	dir := t.TempDir()
	m := Manager{Dir: dir, Backend: "backend"}
	for _, n := range []string{".env", "docker-compose.yml"} {
		os.WriteFile(filepath.Join(dir, n), []byte("original"), 0600)
	}
	calls := filepath.Join(t.TempDir(), "runs")
	fakeDocker(t, `case "$*" in "inspect backend") echo '`+identityJSON(dir, "/srv/panel", "project", "/srv/panel/docker-compose.yml")+`';; run*) echo run >> '`+calls+`'; exit 1;; *) exit 1;; esac`)
	j, _, e := m.Start(context.Background(), "2.5.0", []byte("target"))
	if e == nil || j == nil || !j.Active {
		t.Fatalf("%v %v", j, e)
	}
	_, _, e = m.Start(context.Background(), "2.5.0", nil)
	if e == nil {
		t.Fatal("retry accepted")
	}
	b, _ := os.ReadFile(calls)
	if string(b) != "run\n" {
		t.Fatal("retried uncertain docker run")
	}
}

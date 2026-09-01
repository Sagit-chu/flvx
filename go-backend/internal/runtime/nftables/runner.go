package nftables

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Runner interface {
	ApplyScript(ctx context.Context, cfg SSHConfig, script string) error
	Test(ctx context.Context, cfg SSHConfig) error
	ListTableJSON(ctx context.Context, cfg SSHConfig) ([]byte, error)
}

type SSHRunner struct {
	Timeout time.Duration
}

func NewSSHRunner() *SSHRunner {
	return &SSHRunner{Timeout: 15 * time.Second}
}

func (r *SSHRunner) Test(ctx context.Context, cfg SSHConfig) error {
	nft := nftBinary(cfg)
	tableName := fmt.Sprintf("flvx_capability_%d", time.Now().UnixNano())
	return r.run(ctx, cfg, buildCapabilityCheckCommand(nft, tableName))
}

func buildCapabilityCheckCommand(nft, tableName string) string {
	script := RenderTable(NodePlan{
		Rules: []Rule{{
			ForwardID:  1,
			InPort:     12345,
			TargetHost: "192.0.2.1",
			TargetPort: 443,
			Protocols:  []string{"tcp", "udp"},
		}},
	})
	script = strings.Replace(script, "table inet flvx {", "table inet "+tableName+" {", 1)
	return "set -eu\n" +
		"command -v nft >/dev/null 2>&1\n" +
		nft + " --version >/dev/null 2>&1\n" +
		"tmp=$(mktemp /tmp/flvx-nft-capability-XXXXXX.nft)\n" +
		"trap 'rm -f \"$tmp\"' EXIT\n" +
		"cat > \"$tmp\" <<'EOF'\n" + script + "\nEOF\n" +
		"if ! " + nft + " -c -f \"$tmp\"; then\n" +
		"  echo 'nftables cannot validate the generated FLVX rules' >&2\n" +
		"  exit 1\n" +
		"fi"
}

func (r *SSHRunner) ApplyScript(ctx context.Context, cfg SSHConfig, script string) error {
	command := buildApplyCommand(nftBinary(cfg), script)
	return r.run(ctx, cfg, command)
}

func buildApplyCommand(nft, script string) string {
	return "set -eu\n" +
		"tmp=$(mktemp /tmp/flvx-nft-XXXXXX.nft)\n" +
		"batch=$(mktemp /tmp/flvx-nft-batch-XXXXXX.nft) || { rm -f \"$tmp\"; exit 1; }\n" +
		"cleanup() {\n" +
		"  rm -f \"$tmp\" \"$batch\"\n" +
		"}\n" +
		"trap cleanup EXIT\n" +
		"cat > \"$tmp\" <<'EOF'\n" + script + "\nEOF\n" +
		"if " + nft + " list table inet flvx >/dev/null 2>&1; then\n" +
		"  { printf '%s\\n' 'delete table inet flvx'; cat \"$tmp\"; } > \"$batch\"\n" +
		"else\n" +
		"  cp \"$tmp\" \"$batch\"\n" +
		"fi\n" +
		"if ! " + nft + " -c -f \"$batch\"; then\n" +
		"  echo 'nftables rule validation failed; active rules were preserved' >&2\n" +
		"  exit 1\n" +
		"fi\n" +
		nft + " -f \"$batch\""
}

func (r *SSHRunner) ListTableJSON(ctx context.Context, cfg SSHConfig) ([]byte, error) {
	return r.runOutput(ctx, cfg, nftBinary(cfg)+" -j list table inet flvx")
}

func (r *SSHRunner) run(ctx context.Context, cfg SSHConfig, command string) error {
	_, err := r.runOutput(ctx, cfg, command)
	return err
}

func (r *SSHRunner) runOutput(ctx context.Context, cfg SSHConfig, command string) ([]byte, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	clientConfig, err := buildSSHClientConfig(cfg)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(strings.TrimSpace(cfg.Host), fmt.Sprintf("%d", normalizedSSHPort(cfg.Port)))
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(runCtx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer conn.Close()

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH 认证失败: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("SSH 会话创建失败: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case <-runCtx.Done():
		_ = session.Close()
		return nil, fmt.Errorf("SSH 命令超时: %w", runCtx.Err())
	case err := <-done:
		if err != nil {
			message := strings.TrimSpace(stderr.String())
			if message != "" {
				return nil, fmt.Errorf("远程执行失败: %s: %w", message, err)
			}
			return nil, fmt.Errorf("远程执行失败: %w", err)
		}
		return stdout.Bytes(), nil
	}
}

func buildSSHClientConfig(cfg SSHConfig) (*ssh.ClientConfig, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, fmt.Errorf("SSH 主机不能为空")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return nil, fmt.Errorf("SSH 用户名不能为空")
	}

	auth, err := authMethods(cfg)
	if err != nil {
		return nil, err
	}
	if len(auth) == 0 {
		return nil, fmt.Errorf("SSH 认证方式不能为空")
	}

	return &ssh.ClientConfig{
		User:            strings.TrimSpace(cfg.Username),
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}, nil
}

func authMethods(cfg SSHConfig) ([]ssh.AuthMethod, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.AuthType)) {
	case "":
		if strings.TrimSpace(cfg.PrivateKey) == "" {
			return nil, fmt.Errorf("SSH 私钥不能为空")
		}
		signer, err := parsePrivateKey(cfg.PrivateKey, cfg.Passphrase)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	case "password":
		if cfg.Password == "" {
			return nil, fmt.Errorf("SSH 密码不能为空")
		}
		return []ssh.AuthMethod{ssh.Password(cfg.Password)}, nil
	case "private_key":
		if strings.TrimSpace(cfg.PrivateKey) == "" {
			return nil, fmt.Errorf("SSH 私钥不能为空")
		}
		signer, err := parsePrivateKey(cfg.PrivateKey, cfg.Passphrase)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default:
		return nil, fmt.Errorf("不支持的 SSH 认证方式: %s", cfg.AuthType)
	}
}

func parsePrivateKey(privateKey, passphrase string) (ssh.Signer, error) {
	if passphrase != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase([]byte(privateKey), []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("SSH 私钥解析失败: %w", err)
		}
		return signer, nil
	}
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return nil, fmt.Errorf("SSH 私钥解析失败: %w", err)
	}
	return signer, nil
}

func nftCommand(cfg SSHConfig, command string) string {
	return "sh -lc " + sshQuote(command)
}

func nftBinary(cfg SSHConfig) string {
	if strings.EqualFold(strings.TrimSpace(cfg.SudoMode), "sudo") {
		return "sudo -n nft"
	}
	return "nft"
}

func sshQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func normalizedSSHPort(port int) int {
	if port <= 0 {
		return 22
	}
	return port
}

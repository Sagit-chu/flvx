package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mcp-panel/internal/audit"
	"mcp-panel/internal/panelclient"
	"mcp-panel/internal/policy"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Registry struct {
	panel       *panelclient.Client
	confirm     *policy.ConfirmPolicy
	idempotency *policy.IdempotencyStore
	audit       *audit.Logger
}

func NewRegistry(panel *panelclient.Client, confirm *policy.ConfirmPolicy, idempotency *policy.IdempotencyStore, auditLogger *audit.Logger) *Registry {
	return &Registry{panel: panel, confirm: confirm, idempotency: idempotency, audit: auditLogger}
}

func (r *Registry) Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "auth.login",
		Description: "Login panel user and return JWT token from /api/v1/user/login",
	}, r.authLogin)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "auth.user_package",
		Description: "Get current user package from /api/v1/user/package",
	}, r.authUserPackage)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "node.list",
		Description: "List panel nodes via /api/v1/node/list",
	}, r.nodeList)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tunnel.list",
		Description: "List panel tunnels via /api/v1/tunnel/list",
	}, r.tunnelList)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "forward.list",
		Description: "List panel forwards via /api/v1/forward/list",
	}, r.forwardList)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "user.list",
		Description: "List panel users via /api/v1/user/list",
	}, r.userList)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "forward.delete",
		Description: "Delete forward by id via /api/v1/forward/delete (requires confirm_token)",
	}, r.forwardDelete)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "forward.pause",
		Description: "Pause forward by id via /api/v1/forward/pause (requires confirm_token)",
	}, r.forwardPause)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "forward.resume",
		Description: "Resume forward by id via /api/v1/forward/resume (requires confirm_token)",
	}, r.forwardResume)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "user.delete",
		Description: "Delete user by id via /api/v1/user/delete (requires confirm_token)",
	}, r.userDelete)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "node.delete",
		Description: "Delete node by id via /api/v1/node/delete (requires confirm_token)",
	}, r.nodeDelete)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tunnel.delete",
		Description: "Delete tunnel by id via /api/v1/tunnel/delete (requires confirm_token)",
	}, r.tunnelDelete)
}

type CommonAuthInput struct {
	PanelToken string `json:"panel_token" jsonschema:"Raw panel JWT token for Authorization header"`
}

type NodeListInput struct {
	CommonAuthInput
}

type UserListInput struct {
	CommonAuthInput
	Keyword string `json:"keyword,omitempty" jsonschema:"Optional keyword for user filtering"`
}

type LoginInput struct {
	Username  string `json:"username" jsonschema:"Panel username"`
	Password  string `json:"password" jsonschema:"Panel password"`
	CaptchaID string `json:"captcha_id,omitempty" jsonschema:"Optional captcha id"`
}

type LoginOutput struct {
	Token                 string `json:"token"`
	Name                  string `json:"name"`
	RoleID                int64  `json:"role_id"`
	RequirePasswordChange bool   `json:"requirePasswordChange"`
}

type UserPackageInput struct {
	CommonAuthInput
}

type DataOutput struct {
	Data map[string]any `json:"data"`
}

type ListOutput struct {
	Items []map[string]any `json:"items" jsonschema:"List response items"`
	Count int              `json:"count" jsonschema:"Total item count"`
}

type MutationByIDInput struct {
	CommonAuthInput
	ID             int64  `json:"id" jsonschema:"Resource ID"`
	ConfirmToken   string `json:"confirm_token" jsonschema:"Confirm token required for dangerous operations"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"Idempotency key for mutation replay protection"`
}

type MutationOutput struct {
	ID       int64  `json:"id"`
	Action   string `json:"action"`
	Replayed bool   `json:"replayed"`
}

func (r *Registry) authLogin(ctx context.Context, _ *mcp.CallToolRequest, in LoginInput) (result *mcp.CallToolResult, out LoginOutput, err error) {
	startedAt := time.Now()
	defer func() { r.audit.LogToolCall("auth.login", startedAt, err) }()

	if strings.TrimSpace(in.Username) == "" {
		return nil, LoginOutput{}, fmt.Errorf("username is required")
	}
	if strings.TrimSpace(in.Password) == "" {
		return nil, LoginOutput{}, fmt.Errorf("password is required")
	}

	login, err := r.panel.Login(ctx, in.Username, in.Password, in.CaptchaID)
	if err != nil {
		return nil, LoginOutput{}, err
	}

	out = LoginOutput{
		Token:                 login.Token,
		Name:                  login.Name,
		RoleID:                login.RoleID,
		RequirePasswordChange: login.RequirePasswordChange,
	}
	return textResult(out), out, nil
}

func (r *Registry) authUserPackage(ctx context.Context, _ *mcp.CallToolRequest, in UserPackageInput) (result *mcp.CallToolResult, out DataOutput, err error) {
	startedAt := time.Now()
	defer func() { r.audit.LogToolCall("auth.user_package", startedAt, err) }()

	if strings.TrimSpace(in.PanelToken) == "" {
		return nil, DataOutput{}, fmt.Errorf("panel_token is required")
	}

	pkg, err := r.panel.UserPackage(ctx, in.PanelToken)
	if err != nil {
		return nil, DataOutput{}, err
	}
	out = DataOutput{Data: pkg}
	return textResult(out), out, nil
}

func (r *Registry) nodeList(ctx context.Context, _ *mcp.CallToolRequest, in NodeListInput) (result *mcp.CallToolResult, out ListOutput, err error) {
	startedAt := time.Now()
	defer func() { r.audit.LogToolCall("node.list", startedAt, err) }()

	if strings.TrimSpace(in.PanelToken) == "" {
		return nil, ListOutput{}, fmt.Errorf("panel_token is required")
	}

	items, err := r.panel.ListNodes(ctx, in.PanelToken)
	if err != nil {
		return nil, ListOutput{}, err
	}
	out = ListOutput{Items: items, Count: len(items)}
	return textResult(out), out, nil
}

func (r *Registry) userList(ctx context.Context, _ *mcp.CallToolRequest, in UserListInput) (result *mcp.CallToolResult, out ListOutput, err error) {
	startedAt := time.Now()
	defer func() { r.audit.LogToolCall("user.list", startedAt, err) }()

	if strings.TrimSpace(in.PanelToken) == "" {
		return nil, ListOutput{}, fmt.Errorf("panel_token is required")
	}

	items, err := r.panel.ListUsers(ctx, in.PanelToken, in.Keyword)
	if err != nil {
		return nil, ListOutput{}, err
	}
	out = ListOutput{Items: items, Count: len(items)}
	return textResult(out), out, nil
}

func (r *Registry) tunnelList(ctx context.Context, _ *mcp.CallToolRequest, in NodeListInput) (result *mcp.CallToolResult, out ListOutput, err error) {
	startedAt := time.Now()
	defer func() { r.audit.LogToolCall("tunnel.list", startedAt, err) }()

	if strings.TrimSpace(in.PanelToken) == "" {
		return nil, ListOutput{}, fmt.Errorf("panel_token is required")
	}

	items, err := r.panel.ListTunnels(ctx, in.PanelToken)
	if err != nil {
		return nil, ListOutput{}, err
	}
	out = ListOutput{Items: items, Count: len(items)}
	return textResult(out), out, nil
}

func (r *Registry) forwardList(ctx context.Context, _ *mcp.CallToolRequest, in NodeListInput) (result *mcp.CallToolResult, out ListOutput, err error) {
	startedAt := time.Now()
	defer func() { r.audit.LogToolCall("forward.list", startedAt, err) }()

	if strings.TrimSpace(in.PanelToken) == "" {
		return nil, ListOutput{}, fmt.Errorf("panel_token is required")
	}

	items, err := r.panel.ListForwards(ctx, in.PanelToken)
	if err != nil {
		return nil, ListOutput{}, err
	}
	out = ListOutput{Items: items, Count: len(items)}
	return textResult(out), out, nil
}

func (r *Registry) forwardDelete(ctx context.Context, _ *mcp.CallToolRequest, in MutationByIDInput) (result *mcp.CallToolResult, out MutationOutput, err error) {
	return r.runDangerousMutation(ctx, "forward.delete", "forward_delete", in, func(ctx context.Context, token string, id int64) error {
		return r.panel.DeleteForward(ctx, token, id)
	})
}

func (r *Registry) forwardPause(ctx context.Context, _ *mcp.CallToolRequest, in MutationByIDInput) (result *mcp.CallToolResult, out MutationOutput, err error) {
	return r.runDangerousMutation(ctx, "forward.pause", "forward_pause", in, func(ctx context.Context, token string, id int64) error {
		return r.panel.PauseForward(ctx, token, id)
	})
}

func (r *Registry) forwardResume(ctx context.Context, _ *mcp.CallToolRequest, in MutationByIDInput) (result *mcp.CallToolResult, out MutationOutput, err error) {
	return r.runDangerousMutation(ctx, "forward.resume", "forward_resume", in, func(ctx context.Context, token string, id int64) error {
		return r.panel.ResumeForward(ctx, token, id)
	})
}

func (r *Registry) userDelete(ctx context.Context, _ *mcp.CallToolRequest, in MutationByIDInput) (result *mcp.CallToolResult, out MutationOutput, err error) {
	return r.runDangerousMutation(ctx, "user.delete", "user_delete", in, func(ctx context.Context, token string, id int64) error {
		return r.panel.DeleteUser(ctx, token, id)
	})
}

func (r *Registry) nodeDelete(ctx context.Context, _ *mcp.CallToolRequest, in MutationByIDInput) (result *mcp.CallToolResult, out MutationOutput, err error) {
	return r.runDangerousMutation(ctx, "node.delete", "node_delete", in, func(ctx context.Context, token string, id int64) error {
		return r.panel.DeleteNode(ctx, token, id)
	})
}

func (r *Registry) tunnelDelete(ctx context.Context, _ *mcp.CallToolRequest, in MutationByIDInput) (result *mcp.CallToolResult, out MutationOutput, err error) {
	return r.runDangerousMutation(ctx, "tunnel.delete", "tunnel_delete", in, func(ctx context.Context, token string, id int64) error {
		return r.panel.DeleteTunnel(ctx, token, id)
	})
}

func (r *Registry) runDangerousMutation(
	ctx context.Context,
	toolName string,
	action string,
	in MutationByIDInput,
	fn func(context.Context, string, int64) error,
) (result *mcp.CallToolResult, out MutationOutput, err error) {
	startedAt := time.Now()
	defer func() { r.audit.LogToolCall(toolName, startedAt, err) }()

	if strings.TrimSpace(in.PanelToken) == "" {
		return nil, MutationOutput{}, fmt.Errorf("panel_token is required")
	}
	if in.ID <= 0 {
		return nil, MutationOutput{}, fmt.Errorf("id must be greater than 0")
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return nil, MutationOutput{}, fmt.Errorf("idempotency_key is required")
	}
	if err := r.confirm.RequireDangerous(in.ConfirmToken); err != nil {
		return nil, MutationOutput{}, err
	}

	fingerprint := mutationFingerprint(action, in.PanelToken, in.ID)
	if data, replay, conflict := r.idempotency.Lookup(toolName, in.IdempotencyKey, fingerprint); conflict {
		return nil, MutationOutput{}, fmt.Errorf("idempotency_key conflict: request payload mismatch")
	} else if replay {
		var replayOut MutationOutput
		if err := json.Unmarshal(data, &replayOut); err != nil {
			return nil, MutationOutput{}, fmt.Errorf("decode idempotent replay response: %w", err)
		}
		replayOut.Replayed = true
		return textResult(replayOut), replayOut, nil
	}

	if err := fn(ctx, in.PanelToken, in.ID); err != nil {
		return nil, MutationOutput{}, err
	}
	out = MutationOutput{ID: in.ID, Action: action, Replayed: false}
	_ = r.idempotency.Save(toolName, in.IdempotencyKey, fingerprint, out)
	return textResult(out), out, nil
}

func mutationFingerprint(action, panelToken string, id int64) string {
	tokenHash := sha256.Sum256([]byte(strings.TrimSpace(panelToken)))
	raw := fmt.Sprintf("%s|%d|%s", action, id, hex.EncodeToString(tokenHash[:]))
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func textResult(v any) *mcp.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "{}"}}}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}
}

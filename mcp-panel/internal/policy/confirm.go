package policy

import (
	"fmt"
	"strings"
)

type ConfirmPolicy struct {
	confirmToken string
}

func NewConfirmPolicy(confirmToken string) *ConfirmPolicy {
	return &ConfirmPolicy{confirmToken: strings.TrimSpace(confirmToken)}
}

func (p *ConfirmPolicy) RequireDangerous(confirmToken string) error {
	if p == nil || p.confirmToken == "" {
		return fmt.Errorf("dangerous operation disabled: MCP_CONFIRM_TOKEN is not configured")
	}
	if strings.TrimSpace(confirmToken) != p.confirmToken {
		return fmt.Errorf("confirm_token is invalid")
	}
	return nil
}

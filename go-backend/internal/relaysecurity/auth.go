// Package relaysecurity defines the versioned, endpoint-scoped relay contract.
// It authenticates relay clients, not the confidentiality of their transport.
package relaysecurity

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

const Version = 1
const Required = "required"
const Legacy = "legacy"

type Credentials struct {
	Version  int    `json:"version"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func New() (*Credentials, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, errors.New("relay credential randomness unavailable")
	}
	u := make([]byte, 12)
	if _, err := rand.Read(u); err != nil {
		return nil, errors.New("relay credential randomness unavailable")
	}
	return &Credentials{Version: Version, Username: "relay-" + hex.EncodeToString(u), Password: hex.EncodeToString(b)}, nil
}
func (c *Credentials) Validate() error {
	if c == nil || c.Version != Version || strings.TrimSpace(c.Username) == "" || len(c.Username) > 255 || len(c.Password) < 32 || len(c.Password) > 255 {
		return errors.New("invalid required relay authentication")
	}
	return nil
}
func (c *Credentials) Config() map[string]interface{} {
	return map[string]interface{}{"username": c.Username, "password": c.Password}
}

package socket

import (
	"errors"
	"strings"

	"github.com/go-gost/core/logger"
	"github.com/go-gost/x/config"
	parser "github.com/go-gost/x/config/parsing/chain"
	"github.com/go-gost/x/registry"
)

func createChain(req createChainRequest) error {

	name := strings.TrimSpace(req.Data.Name)
	if name == "" {
		return errors.New("chain name is required")
	}
	req.Data.Name = name

	if registry.ChainRegistry().IsRegistered(name) {
		return errors.New("chain " + name + " already exists")
	}

	if !IsDashMode {
		v, err := parser.ParseChain(&req.Data, logger.Default())
		if err != nil {
			return errors.New("create chain " + name + " failed: " + err.Error())
		}

		if err := registry.ChainRegistry().Register(name, v); err != nil {
			return errors.New("chain " + name + " already exists")
		}
	}

	config.OnUpdate(func(c *config.Config) error {
		c.Chains = append(c.Chains, &req.Data)
		return nil
	})

	return nil
}

func updateChain(req updateChainRequest) error {

	name := strings.TrimSpace(req.Chain)

	if registry.ChainRegistry().IsRegistered(name) {
		registry.ChainRegistry().Unregister(name)
	}

	req.Data.Name = name

	if !IsDashMode {
		v, err := parser.ParseChain(&req.Data, logger.Default())
		if err != nil {
			return errors.New("create chain " + name + " failed: " + err.Error())
		}

		if err := registry.ChainRegistry().Register(name, v); err != nil {
			return errors.New("chain " + name + " already exists")
		}
	}

	config.OnUpdate(func(c *config.Config) error {
		found := false
		for i := range c.Chains {
			if c.Chains[i].Name == name {
				c.Chains[i] = &req.Data
				found = true
				break
			}
		}
		if !found {
			c.Chains = append(c.Chains, &req.Data)
		}
		return nil
	})

	return nil
}

func deleteChain(req deleteChainRequest) error {

	name := strings.TrimSpace(req.Chain)

	if registry.ChainRegistry().IsRegistered(name) {
		registry.ChainRegistry().Unregister(name)
	}

	config.OnUpdate(func(c *config.Config) error {
		chains := c.Chains
		c.Chains = nil
		for _, s := range chains {
			if s.Name == name {
				continue
			}
			c.Chains = append(c.Chains, s)
		}
		return nil
	})

	return nil
}

type createChainRequest struct {
	Data config.ChainConfig `json:"data"`
}

type updateChainRequest struct {
	Chain string             `json:"chain"`
	Data  config.ChainConfig `json:"data"`
}

type deleteChainRequest struct {
	Chain string `json:"chain"`
}

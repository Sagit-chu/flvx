package chain

import (
	"github.com/go-gost/core/chain"
	"github.com/go-gost/core/hop"
	"github.com/go-gost/core/logger"
	"github.com/go-gost/core/metadata"
	xchain "github.com/go-gost/x/chain"
	"github.com/go-gost/x/config"
	hop_parser "github.com/go-gost/x/config/parsing/hop"
	mdx "github.com/go-gost/x/metadata"
	"github.com/go-gost/x/registry"
)

func ParseChain(cfg *config.ChainConfig, log logger.Logger) (chain.Chainer, error) {
	if cfg == nil {
		return nil, nil
	}

	chainLogger := log.WithFields(map[string]any{
		"kind":  "chain",
		"chain": cfg.Name,
	})

	var md metadata.Metadata
	if cfg.Metadata != nil {
		md = mdx.NewMetadata(cfg.Metadata)
	}

	c := xchain.NewChain(cfg.Name,
		xchain.MetadataChainOption(md),
		xchain.LoggerChainOption(chainLogger),
	)

	for _, ch := range cfg.Hops {
		var hop hop.Hop
		var err error
		owned := false

		if ch.Nodes != nil || ch.Plugin != nil {
			if hop, err = hop_parser.ParseHop(ch, log); err != nil {
				return nil, err
			}
			owned = true
		} else {
			hop = registry.HopRegistry().Get(ch.Name)
		}
		if hop != nil {
			c.AddHop(hop, owned)
		}
	}

	return c, nil
}

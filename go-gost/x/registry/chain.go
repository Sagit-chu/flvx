package registry

import (
	"context"

	"github.com/go-gost/core/chain"
	"github.com/go-gost/core/metadata"
	"github.com/go-gost/core/selector"
)

type chainRegistry struct {
	registry[chain.Chainer]
}

func ReplaceChain(name string, v chain.Chainer) error {
	if name == "" {
		return nil
	}
	if r, ok := chainReg.(*chainRegistry); ok {
		r.replace(name, v)
		return nil
	}
	return chainReg.Register(name, v)
}

func (r *chainRegistry) Register(name string, v chain.Chainer) error {
	return r.registry.Register(name, v)
}

func (r *chainRegistry) replace(name string, v chain.Chainer) {
	r.m.Store(name, v)
}

func (r *chainRegistry) Get(name string) chain.Chainer {
	if name != "" {
		return &chainWrapper{name: name, r: r}
	}
	return nil
}

func (r *chainRegistry) get(name string) chain.Chainer {
	return r.registry.Get(name)
}

type chainWrapper struct {
	name string
	r    *chainRegistry
}

func (w *chainWrapper) Marker() selector.Marker {
	v := w.r.get(w.name)
	if v == nil {
		return nil
	}
	if mi, ok := v.(selector.Markable); ok {
		return mi.Marker()
	}
	return nil
}

func (w *chainWrapper) Metadata() metadata.Metadata {
	v := w.r.get(w.name)
	if v == nil {
		return nil
	}

	if mi, ok := v.(metadata.Metadatable); ok {
		return mi.Metadata()
	}
	return nil
}

func (w *chainWrapper) Route(ctx context.Context, network, address string, opts ...chain.RouteOption) chain.Route {
	v := w.r.get(w.name)
	if v == nil {
		return nil
	}
	return v.Route(ctx, network, address, opts...)
}

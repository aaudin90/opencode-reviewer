package pipeline

import "context"

type RuntimeLoader interface {
	LoadRuntime(ctx context.Context) (*RuntimeResources, error)
}

type RuntimeLoaderFunc func(ctx context.Context) (*RuntimeResources, error)

func (f RuntimeLoaderFunc) LoadRuntime(ctx context.Context) (*RuntimeResources, error) {
	return f(ctx)
}

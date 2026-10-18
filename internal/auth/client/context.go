package client

import "context"

type ctxKey struct{}

// WithClient returns ctx carrying the authenticated client.
func WithClient(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

// FromContext returns the client put in ctx by KeyFunc.
func FromContext(ctx context.Context) (Client, bool) {
	c, ok := ctx.Value(ctxKey{}).(Client)
	return c, ok
}

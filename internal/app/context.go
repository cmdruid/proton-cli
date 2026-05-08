package app

import "context"

type ctxKey struct{}

// WithApp stores a into ctx.
func WithApp(ctx context.Context, a *App) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

// From retrieves the App from ctx. Panics if absent (programmer error).
func From(ctx context.Context) *App {
	a, _ := ctx.Value(ctxKey{}).(*App)
	if a == nil {
		panic("app.From: no App in context")
	}
	return a
}

// FromOrNil returns the App in ctx, or nil if absent. Used by code
// paths that may run before PersistentPreRunE has installed an app
// (e.g. cmd/root.go's final-error formatter when arg-parsing fails).
func FromOrNil(ctx context.Context) *App {
	if ctx == nil {
		return nil
	}
	a, _ := ctx.Value(ctxKey{}).(*App)
	return a
}

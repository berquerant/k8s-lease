package logging

import (
	"context"
	"log/slog"
)

type prefixedHandler struct {
	slog.Handler
	prefix string
}

func newPrefixedHandler(h slog.Handler, prefix string) *prefixedHandler {
	return &prefixedHandler{
		Handler: h,
		prefix:  prefix,
	}
}

func (h *prefixedHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Message = h.prefix + r.Message
	return h.Handler.Handle(ctx, r)
}

func (h *prefixedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return newPrefixedHandler(h.Handler.WithAttrs(attrs), h.prefix)
}

func (h *prefixedHandler) WithGroup(name string) slog.Handler {
	return newPrefixedHandler(h.Handler.WithGroup(name), h.prefix)
}

func (h *prefixedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level)
}

package mserve

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// stackHandler wraps any slog.Handler and injects a stack trace on Warn+.
type stackHandler struct{ slog.Handler }

func NewStackHandler(inner slog.Handler) slog.Handler {
	return &stackHandler{inner}
}

func (h *stackHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.Handler.Enabled(ctx, l)
}

func (h *stackHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		r.AddAttrs(slog.String("stack", string(debug.Stack())))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *stackHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &stackHandler{h.Handler.WithAttrs(attrs)}
}

func (h *stackHandler) WithGroup(name string) slog.Handler {
	return &stackHandler{h.Handler.WithGroup(name)}
}

package log

import (
	"context"
	"io"
	"log/slog"
)

// Handler wraps slog.TextHandler with mode-awareness
type Handler struct {
	*slog.TextHandler
	w io.Writer
}

// NewHandler creates a new mode-aware slog handler
func NewHandler(w io.Writer, opts *slog.HandlerOptions) *Handler {
	return &Handler{
		TextHandler: slog.NewTextHandler(w, opts),
		w:           w,
	}
}

// Handle processes a log record
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	return h.TextHandler.Handle(ctx, r)
}

// WithAttrs returns a new Handler with the given attributes
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		TextHandler: h.TextHandler.WithAttrs(attrs).(*slog.TextHandler),
		w:           h.w,
	}
}

// WithGroup returns a new Handler with the given group
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		TextHandler: h.TextHandler.WithGroup(name).(*slog.TextHandler),
		w:           h.w,
	}
}

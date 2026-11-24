package obs

import (
	"io"
	"log/slog"
)

// NewJSONLogger returns a structured JSON logger for gateway events.
func NewJSONLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

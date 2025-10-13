package logging

import (
	"io"
	"log/slog"
)

func Setup(w io.Writer, debug bool) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	handler := newPrefixedHandler(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	}), "[k8s-lease] ")
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

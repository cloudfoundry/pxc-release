package test_helpers

import (
	"context"
	"log/slog"

	"github.com/onsi/gomega/gbytes"
)

type SlogHandler struct {
	handler  slog.Handler
	Buffer   *gbytes.Buffer
	Messages []string
}

func NewSlogHandler() *SlogHandler {
	buffer := gbytes.NewBuffer()

	return &SlogHandler{
		handler: slog.NewJSONHandler(buffer, nil),
		Buffer:  buffer,
	}
}

func (s *SlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return s.handler.Enabled(ctx, level)
}

func (s *SlogHandler) Handle(ctx context.Context, record slog.Record) error {
	s.Messages = append(s.Messages, record.Message)
	return s.handler.Handle(ctx, record)
}

func (s *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return s.handler.WithAttrs(attrs)
}

func (s *SlogHandler) WithGroup(name string) slog.Handler {
	return s.handler.WithGroup(name)
}

var _ slog.Handler = &SlogHandler{}

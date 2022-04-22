package debug

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
)

type contextKey struct{ string }

var (
	debugKey = contextKey{"context-debug"}
)

var (
	Nop    = log.New(io.Discard, "debug", 0)
	Logger = log.New(os.Stderr, "debug", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
)

func WithLog(ctx context.Context, l *log.Logger) context.Context {
	if l != nil {
		return context.WithValue(ctx, debugKey, l)
	}
	return context.WithValue(ctx, debugKey, Nop)
}

func FromContext(ctx context.Context) *log.Logger {
	if l, ok := ctx.Value(debugKey).(*log.Logger); ok {
		return l
	}
	return Nop
}

func Logf(ctx context.Context, format string, v ...any) {
	FromContext(ctx).Output(2, fmt.Sprintf(format, v...))
}

package command

import (
	"context"
	"os"

	"rafal.dev/reflow/pkg/debug"
)

type App struct {
	ctx context.Context
}

func NewApp(ctx context.Context) *App {
	if os.Getenv("REFLOW_DEBUG") == "1" {
		ctx = debug.WithLog(ctx, debug.Logger)
	}

	app := &App{
		ctx: ctx,
	}

	return app
}

func (app *App) Context() context.Context {
	return app.ctx
}

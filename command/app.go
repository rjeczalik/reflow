package command

import (
	"context"

	"github.com/spf13/pflag"
)

type App struct {
	ctx context.Context
}

func NewApp(ctx context.Context) *App {
	return &App{
		ctx: ctx,
	}
}

func (app *App) Context() context.Context {
	return app.ctx
}

func (app *App) Register(flags *pflag.FlagSet) {
}

func (app *App) Init(next CobraFunc) CobraFunc {
	return next
}

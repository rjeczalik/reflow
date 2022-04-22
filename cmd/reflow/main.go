package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"rafal.dev/reflow/command"
	"rafal.dev/reflow/command/reflow"
)

func main() {
	app := command.NewApp(contextProcess())
	cmd := reflow.NewCommand(app)

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func contextProcess() context.Context {
	ch := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-ch
		cancel()
	}()

	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	return ctx
}

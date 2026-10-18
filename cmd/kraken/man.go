package main

import (
	"os"
	"path"

	"github.com/kraken-hub/kraken/internal/app"
)

var version = "0.x-dev"

func main() {
	cmdName := path.Base(os.Args[0])
	kraken := app.New(cmdName, version)

	for _, m := range app.ModuleRegistry.List() {
		m.Init(kraken)
	}

	if err := kraken.RootCmd().Execute(); err != nil {
		if sigErr := app.InterruptedBy(kraken.Context()); sigErr != nil {
			os.Exit(sigErr.ExitCode())
		}
		kraken.Logger().Fatal("%s", err)
		os.Exit(1)
	}
}

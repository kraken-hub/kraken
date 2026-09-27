package main

import (
	_ "github.com/kraken-hub/kraken/internal/app/module/config"
	_ "github.com/kraken-hub/kraken/internal/app/module/create"
	_ "github.com/kraken-hub/kraken/internal/app/module/exec"
	_ "github.com/kraken-hub/kraken/internal/app/module/init"
	_ "github.com/kraken-hub/kraken/internal/app/module/list"
	_ "github.com/kraken-hub/kraken/internal/app/module/logs"
	_ "github.com/kraken-hub/kraken/internal/app/module/plugins"
	_ "github.com/kraken-hub/kraken/internal/app/module/rebuild"
	_ "github.com/kraken-hub/kraken/internal/app/module/remove"
	_ "github.com/kraken-hub/kraken/internal/app/module/restart"
	_ "github.com/kraken-hub/kraken/internal/app/module/run"
	_ "github.com/kraken-hub/kraken/internal/app/module/start"
	_ "github.com/kraken-hub/kraken/internal/app/module/status"
	_ "github.com/kraken-hub/kraken/internal/app/module/stop"
	_ "github.com/kraken-hub/kraken/internal/app/module/vars"
)

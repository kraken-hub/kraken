package rebuild

import (
	"github.com/kraken-hub/kraken/internal/app"
)

type Module struct{}

func (m *Module) Init(kraken *app.Kraken) {
	doInit(kraken)
}

func init() {
	app.ModuleRegistry.Add(&Module{})
}

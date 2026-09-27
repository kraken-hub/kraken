package app

type Module interface {
	Init(kraken *Kraken)
}

type moduleRegistry struct {
	items []Module
}

func (r *moduleRegistry) List() []Module {
	return r.items
}

func (r *moduleRegistry) Add(m Module) {
	r.items = append(r.items, m)
}

var ModuleRegistry = &moduleRegistry{}

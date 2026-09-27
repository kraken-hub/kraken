package app

type Payload struct {
	id   string
	Data interface{}
}

func (p *Payload) ID() string {
	return p.id
}

func NewPayload(id string) *Payload {
	return &Payload{id: id}
}

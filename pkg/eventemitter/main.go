package eventemitter

import (
	"reflect"
	"sync"
)

func New() *EventEmitter {
	var e = &EventEmitter{events: make(map[string]*Event)}
	e.Mutex = new(sync.Mutex)
	return e
}

type Emitter struct {
	*sync.Mutex
	handlers []reflect.Value
}

func (e *Emitter) Handlers() []reflect.Value {
	e.Lock()
	var handlers = e.handlers
	e.Unlock()
	return handlers
}

func (e *Emitter) Clear() {
	e.Lock()
	defer e.Unlock()
	e.handlers = []reflect.Value{}
}

func (e *Emitter) Add(fn interface{}) {
	e.Lock()
	defer e.Unlock()
	e.handlers = append(e.handlers, ValueOf(fn))
}

func (e *Emitter) Remove(fn interface{}) {
	e.Lock()
	defer e.Unlock()
	handler := ValueOf(fn)
	for i, h := range e.handlers {
		if h == handler {
			e.handlers = append(e.handlers[:i], e.handlers[i+1:]...)
			return
		}
	}
}

func (e *Emitter) Emit(argv ...interface{}) error {
	var handlers = e.Handlers()
	var callArgv []reflect.Value

	for _, a := range argv {
		callArgv = append(callArgv, reflect.ValueOf(a))
	}

	for _, h := range handlers {
		ret := h.Call(callArgv)

		if v := ret[0].Interface(); v != nil {
			return v.(error)
		}
	}

	return nil
}

type Event struct {
	*sync.Mutex
	emitters map[string]*Emitter
}

func (e *Event) Emitters() map[string]*Emitter {
	e.Lock()
	var emitters = e.emitters
	e.Unlock()
	return emitters
}

func (e *Event) GetEmitter(name string) *Emitter {
	e.Lock()
	defer e.Unlock()

	if emitter, ok := e.emitters[name]; ok {
		return emitter
	}

	emitter := &Emitter{}
	emitter.Mutex = new(sync.Mutex)

	e.emitters[name] = emitter

	return emitter
}

type EventEmitter struct {
	*sync.Mutex
	events map[string]*Event
}

func (e *EventEmitter) Events() map[string]*Event {
	e.Lock()
	var events = e.events
	e.Unlock()
	return events
}

func (e *EventEmitter) GetEvent(name string) *Event {
	e.Lock()
	defer e.Unlock()

	if m, ok := e.events[name]; ok {
		return m
	}

	event := &Event{emitters: make(map[string]*Emitter)}
	event.Mutex = new(sync.Mutex)

	e.events[name] = event

	return event
}

func (e *EventEmitter) Before(name string, fn interface{}) {
	e.GetEvent(name).GetEmitter("Before").Add(fn)
}

func (e *EventEmitter) On(name string, fn interface{}) {
	e.GetEvent(name).GetEmitter("On").Add(fn)
}

func (e *EventEmitter) After(name string, fn interface{}) {
	e.GetEvent(name).GetEmitter("After").Add(fn)
}

func (e *EventEmitter) Emit(name string, argv ...interface{}) error {
	event := e.GetEvent(name)

	if err := event.GetEmitter("Before").Emit(argv...); err != nil {
		return err
	}

	if err := event.GetEmitter("On").Emit(argv...); err != nil {
		return err
	}

	if err := event.GetEmitter("After").Emit(argv...); err != nil {
		return err
	}

	return nil
}

func ValueOf(v interface{}) reflect.Value {
	if value, ok := v.(reflect.Value); ok {
		return value
	}
	return reflect.ValueOf(v)
}

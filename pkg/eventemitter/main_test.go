package eventemitter

import (
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	e := New()
	if e == nil {
		t.Fatal("New() returned nil")
	}
	if e.events == nil {
		t.Fatal("events map not initialized")
	}
}

func TestOnEmit(t *testing.T) {
	e := New()
	called := false
	var gotArg string

	e.On("greet", func(name string) error {
		called = true
		gotArg = name
		return nil
	})

	if err := e.Emit("greet", "world"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if gotArg != "world" {
		t.Errorf("got arg %q, want %q", gotArg, "world")
	}
}

func TestMultipleListenersCalledInOrder(t *testing.T) {
	e := New()
	var order []int

	e.On("evt", func() error { order = append(order, 1); return nil })
	e.On("evt", func() error { order = append(order, 2); return nil })
	e.On("evt", func() error { order = append(order, 3); return nil })

	if err := e.Emit("evt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []int{1, 2, 3}
	if len(order) != len(want) {
		t.Fatalf("got %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %d, want %d", i, order[i], want[i])
		}
	}
}

func TestBeforeOnAfterOrdering(t *testing.T) {
	e := New()
	var order []string

	e.After("evt", func() error { order = append(order, "after"); return nil })
	e.On("evt", func() error { order = append(order, "on"); return nil })
	e.Before("evt", func() error { order = append(order, "before"); return nil })

	if err := e.Emit("evt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"before", "on", "after"}
	if len(order) != len(want) {
		t.Fatalf("got %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want[i])
		}
	}
}

func TestEmitErrorFromBeforeStopsOnAndAfter(t *testing.T) {
	e := New()
	boom := errors.New("boom")
	onCalled := false
	afterCalled := false

	e.Before("evt", func() error { return boom })
	e.On("evt", func() error { onCalled = true; return nil })
	e.After("evt", func() error { afterCalled = true; return nil })

	err := e.Emit("evt")
	if !errors.Is(err, boom) {
		t.Fatalf("got err %v, want %v", err, boom)
	}
	if onCalled {
		t.Error("On handler should not have been called after Before failed")
	}
	if afterCalled {
		t.Error("After handler should not have been called after Before failed")
	}
}

func TestEmitErrorFromOneListenerStopsRemaining(t *testing.T) {
	e := New()
	boom := errors.New("boom")
	var secondCalled bool

	e.On("evt", func() error { return boom })
	e.On("evt", func() error { secondCalled = true; return nil })

	err := e.Emit("evt")
	if !errors.Is(err, boom) {
		t.Fatalf("got err %v, want %v", err, boom)
	}
	if secondCalled {
		t.Error("second listener should not run after first returns an error")
	}
}

func TestEmitNoListenersReturnsNil(t *testing.T) {
	e := New()
	if err := e.Emit("nothing-registered"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestEmitterRemove(t *testing.T) {
	e := New()
	called := false
	fn := func() error { called = true; return nil }

	e.On("evt", fn)
	emitter := e.GetEvent("evt").GetEmitter("On")
	emitter.Remove(fn)

	if err := e.Emit("evt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("removed handler should not be called")
	}
}

func TestEmitterRemoveOnlyTargetHandler(t *testing.T) {
	e := New()
	var calledA, calledB bool
	fnA := func() error { calledA = true; return nil }
	fnB := func() error { calledB = true; return nil }

	e.On("evt", fnA)
	e.On("evt", fnB)

	emitter := e.GetEvent("evt").GetEmitter("On")
	emitter.Remove(fnA)

	if err := e.Emit("evt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledA {
		t.Error("fnA should have been removed")
	}
	if !calledB {
		t.Error("fnB should still be called")
	}
}

func TestEmitterClear(t *testing.T) {
	e := New()
	called := false
	e.On("evt", func() error { called = true; return nil })

	emitter := e.GetEvent("evt").GetEmitter("On")
	emitter.Clear()

	if err := e.Emit("evt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("handler should not be called after Clear")
	}
	if len(emitter.Handlers()) != 0 {
		t.Errorf("expected 0 handlers after Clear, got %d", len(emitter.Handlers()))
	}
}

func TestGetEventReturnsSameInstance(t *testing.T) {
	e := New()
	ev1 := e.GetEvent("evt")
	ev2 := e.GetEvent("evt")
	if ev1 != ev2 {
		t.Error("GetEvent should return the same *Event instance for the same name")
	}
}

func TestGetEmitterReturnsSameInstance(t *testing.T) {
	e := New()
	event := e.GetEvent("evt")
	em1 := event.GetEmitter("On")
	em2 := event.GetEmitter("On")
	if em1 != em2 {
		t.Error("GetEmitter should return the same *Emitter instance for the same name")
	}
}

func TestEventsAndEmittersAccessors(t *testing.T) {
	e := New()
	e.On("evt", func() error { return nil })

	events := e.Events()
	if _, ok := events["evt"]; !ok {
		t.Fatal("expected 'evt' to be present in Events()")
	}

	emitters := events["evt"].Emitters()
	if _, ok := emitters["On"]; !ok {
		t.Fatal("expected 'On' emitter to be present")
	}
}

func TestEmitMultipleArgs(t *testing.T) {
	e := New()
	var gotA string
	var gotB int

	e.On("evt", func(a string, b int) error {
		gotA = a
		gotB = b
		return nil
	})

	if err := e.Emit("evt", "hello", 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotA != "hello" || gotB != 42 {
		t.Errorf("got (%q, %d), want (%q, %d)", gotA, gotB, "hello", 42)
	}
}

func TestValueOfWithRawFunc(t *testing.T) {
	fn := func() error { return nil }
	v := ValueOf(fn)
	if v.Kind().String() != "func" {
		t.Errorf("expected func kind, got %s", v.Kind())
	}
}

func TestValueOfWithReflectValue(t *testing.T) {
	fn := func() error { return nil }
	rv := ValueOf(fn)
	v2 := ValueOf(rv)
	if v2 != rv {
		t.Error("ValueOf should return the same reflect.Value when passed one")
	}
}

func TestDifferentEventNamesAreIndependent(t *testing.T) {
	e := New()
	var calledA, calledB bool

	e.On("eventA", func() error { calledA = true; return nil })
	e.On("eventB", func() error { calledB = true; return nil })

	if err := e.Emit("eventA"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !calledA {
		t.Error("eventA handler should have been called")
	}
	if calledB {
		t.Error("eventB handler should not have been called")
	}
}

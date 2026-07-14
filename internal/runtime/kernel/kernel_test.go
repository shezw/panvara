/*
   Panvara
   internal/runtime/kernel/kernel_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package kernel

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestKernelStartsAndStopsInDependencySafeOrder(t *testing.T) {
	t.Parallel()

	events := make([]string, 0, 4)
	core := New()
	for _, name := range []string{"storage", "http"} {
		component := &fakeComponent{name: name, events: &events}
		if err := core.Register(component); err != nil {
			t.Fatal(err)
		}
	}
	if err := core.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !core.Ready() || core.State() != Running {
		t.Fatalf("kernel state = %s, ready = %v", core.State(), core.Ready())
	}
	if err := core.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"start:storage", "start:http", "stop:http", "stop:storage"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	if core.State() != Stopped {
		t.Fatalf("kernel state = %s, want stopped", core.State())
	}
	if err := core.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestKernelRollsBackPartialStart(t *testing.T) {
	t.Parallel()

	events := make([]string, 0, 3)
	core := New()
	first := &fakeComponent{name: "first", events: &events}
	second := &fakeComponent{name: "second", events: &events, startErr: errors.New("boom")}
	if err := core.Register(first); err != nil {
		t.Fatal(err)
	}
	if err := core.Register(second); err != nil {
		t.Fatal(err)
	}
	if err := core.Start(context.Background()); err == nil {
		t.Fatal("Start() unexpectedly succeeded")
	}
	if want := []string{"start:first", "start:second", "stop:second", "stop:first"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	if core.State() != Stopped || core.Ready() {
		t.Fatalf("kernel state = %s, ready = %v", core.State(), core.Ready())
	}
}

func TestKernelPreservesTerminalStopError(t *testing.T) {
	t.Parallel()

	stopErr := errors.New("cleanup failed")
	events := make([]string, 0, 2)
	core := New()
	if err := core.Register(&fakeComponent{name: "component", events: &events, stopErr: stopErr}); err != nil {
		t.Fatal(err)
	}
	if err := core.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := core.Stop(context.Background()); !errors.Is(err, stopErr) {
		t.Fatalf("first Stop() error = %v, want cleanup error", err)
	}
	if err := core.Stop(context.Background()); !errors.Is(err, stopErr) {
		t.Fatalf("second Stop() error = %v, want preserved cleanup error", err)
	}
	if want := []string{"start:component", "stop:component"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestKernelRejectsDuplicateComponent(t *testing.T) {
	t.Parallel()

	core := New()
	if err := core.Register(&fakeComponent{name: "same"}); err != nil {
		t.Fatal(err)
	}
	if err := core.Register(&fakeComponent{name: "same"}); err == nil {
		t.Fatal("Register() accepted a duplicate component")
	}
}

func TestKernelRejectsTypedNilComponent(t *testing.T) {
	t.Parallel()

	var component *fakeComponent
	if err := New().Register(component); err == nil {
		t.Fatal("Register() accepted a typed nil component")
	}
}

type fakeComponent struct {
	name     string
	events   *[]string
	startErr error
	stopErr  error
}

func (component *fakeComponent) Name() string {
	return component.name
}

func (component *fakeComponent) Start(_ context.Context) error {
	if component.events != nil {
		*component.events = append(*component.events, "start:"+component.name)
	}
	return component.startErr
}

func (component *fakeComponent) Stop(_ context.Context) error {
	if component.events != nil {
		*component.events = append(*component.events, "stop:"+component.name)
	}
	return component.stopErr
}

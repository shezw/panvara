/*
   Panvara
   internal/runtime/kernel/kernel.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package kernel coordinates process component lifecycle without owning domain
// or application logic.
package kernel

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

const rollbackTimeout = 5 * time.Second

// Component is the smallest runtime lifecycle unit. Stop must be idempotent and
// safe after Start was attempted, including when Start returned an error.
type Component interface {
	Name() string
	Start(context.Context) error
	Stop(context.Context) error
}

// State is the observable lifecycle state of a Kernel.
type State uint8

const (
	Created State = iota
	Starting
	Running
	Stopping
	Stopped
)

func (state State) String() string {
	switch state {
	case Created:
		return "created"
	case Starting:
		return "starting"
	case Running:
		return "running"
	case Stopping:
		return "stopping"
	case Stopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// Kernel starts components in registration order and stops them in reverse.
type Kernel struct {
	mu         sync.RWMutex
	state      State
	components []Component
	started    int
	stopErr    error
}

// New returns an empty kernel in the Created state.
func New() *Kernel {
	return &Kernel{state: Created}
}

// Register adds a component before startup.
func (kernel *Kernel) Register(component Component) error {
	if component == nil || isNilComponent(component) {
		return fmt.Errorf("component is nil")
	}
	rawName := component.Name()
	name := strings.TrimSpace(rawName)
	if name == "" {
		return fmt.Errorf("component name is empty")
	}
	if name != rawName {
		return fmt.Errorf("component name %q has surrounding whitespace", rawName)
	}

	kernel.mu.Lock()
	defer kernel.mu.Unlock()
	if kernel.state != Created {
		return fmt.Errorf("cannot register component %q while kernel is %s", name, kernel.state)
	}
	for _, existing := range kernel.components {
		if existing.Name() == name {
			return fmt.Errorf("component %q is already registered", name)
		}
	}
	kernel.components = append(kernel.components, component)
	return nil
}

func isNilComponent(component Component) bool {
	value := reflect.ValueOf(component)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Start starts every registered component. A partial start is rolled back.
func (kernel *Kernel) Start(ctx context.Context) error {
	kernel.mu.Lock()
	if kernel.state != Created {
		state := kernel.state
		kernel.mu.Unlock()
		return fmt.Errorf("cannot start kernel while it is %s", state)
	}
	kernel.state = Starting
	components := append([]Component(nil), kernel.components...)
	kernel.mu.Unlock()

	for index, component := range components {
		if err := component.Start(ctx); err != nil {
			rollbackErr := rollback(components[:index+1])
			kernel.mu.Lock()
			kernel.state = Stopped
			kernel.started = 0
			kernel.stopErr = rollbackErr
			kernel.mu.Unlock()
			return errors.Join(fmt.Errorf("start component %q: %w", component.Name(), err), rollbackErr)
		}
		kernel.mu.Lock()
		kernel.started = index + 1
		kernel.mu.Unlock()
	}

	kernel.mu.Lock()
	kernel.state = Running
	kernel.mu.Unlock()
	return nil
}

// Stop stops started components in reverse order. Sequential calls are
// idempotent; one lifecycle owner must serialize overlapping Start/Stop calls.
func (kernel *Kernel) Stop(ctx context.Context) error {
	kernel.mu.Lock()
	switch kernel.state {
	case Stopped:
		result := kernel.stopErr
		kernel.mu.Unlock()
		return result
	case Created:
		kernel.state = Stopped
		kernel.mu.Unlock()
		return nil
	case Starting:
		kernel.mu.Unlock()
		return fmt.Errorf("cannot stop kernel while it is starting")
	case Stopping:
		kernel.mu.Unlock()
		return fmt.Errorf("kernel is already stopping")
	case Running:
		kernel.state = Stopping
	}
	components := append([]Component(nil), kernel.components[:kernel.started]...)
	kernel.mu.Unlock()

	var result error
	for index := len(components) - 1; index >= 0; index-- {
		if err := components[index].Stop(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("stop component %q: %w", components[index].Name(), err))
		}
	}

	kernel.mu.Lock()
	kernel.started = 0
	kernel.state = Stopped
	kernel.stopErr = result
	kernel.mu.Unlock()
	return result
}

// Ready reports whether every component has started successfully.
func (kernel *Kernel) Ready() bool {
	kernel.mu.RLock()
	defer kernel.mu.RUnlock()
	return kernel.state == Running
}

// State returns the current lifecycle state.
func (kernel *Kernel) State() State {
	kernel.mu.RLock()
	defer kernel.mu.RUnlock()
	return kernel.state
}

func rollback(components []Component) error {
	ctx, cancel := context.WithTimeout(context.Background(), rollbackTimeout)
	defer cancel()
	var result error
	for index := len(components) - 1; index >= 0; index-- {
		if err := components[index].Stop(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("rollback component %q: %w", components[index].Name(), err))
		}
	}
	return result
}

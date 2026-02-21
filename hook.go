package gosignal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
)

var (
	ErrFunctionExists    = errors.New("function with this name is already added to hook")
	ErrNotifyExists      = errors.New("notify with this name is already added to hook")
	ErrFunctionNameEmpty = errors.New("function name is empty")
	ErrNotifyNameEmpty   = errors.New("notify name is empty")
	ErrFuncNil           = errors.New("func is nil")
	ErrChanNil           = errors.New("chan is nil")
)

type Hook interface {
	Exec(ctx context.Context) error

	GetFunction(name string) *Function
	GetNotify(name string) *Notify

	Function(function *Function) error
	Notify(notify *Notify) error
}

// impl Hook
type hook struct {
	name string
	desc string

	functions map[string]*Function
	notifies  map[string]*Notify

	doReorder     bool
	functionOrder []*Function
	notifyOrder   []*Notify

	mu sync.RWMutex
}

func (h *hook) reorder() {
	if !h.doReorder {
		return
	}

	clear(h.functionOrder)
	clear(h.notifyOrder)
	var lowest uint16 = math.MaxUint16
	var lowestName string

	// order functons
	for len(h.functions) > 0 {
		lowest = math.MaxUint16
		lowestName = ""

		for _, function := range h.functions {
			if function.Order <= lowest {
				lowest = function.Order
				lowestName = function.Name
			}
		}

		h.functionOrder = append(h.functionOrder, h.functions[lowestName])
		delete(h.functions, lowestName)
	}

	// order notifies
	for len(h.notifies) > 0 {
		lowest = math.MaxUint16
		lowestName = ""

		for _, notify := range h.notifies {
			if notify.Order < lowest {
				lowest = notify.Order
				lowestName = notify.Name
			}
		}

		h.notifyOrder = append(h.notifyOrder, h.notifies[lowestName])
		delete(h.notifies, lowestName)
	}

	// restore h.functions and h.notifies
	for _, function := range h.functionOrder {
		h.functions[function.Name] = function
	}
	for _, notify := range h.notifyOrder {
		h.notifies[notify.Name] = notify
	}
}

func (h *hook) Exec(ctx context.Context) error {
	h.mu.Lock()
	h.reorder()
	h.mu.Unlock()

	h.mu.RLock()
	defer h.mu.RUnlock()

	// functions
	for _, function := range h.functionOrder {
		if function.Concurrent {
			go function.Func(ctx)
		} else {
			function.Func(ctx)
		}
	}

	// notifies
	for _, notify := range h.notifyOrder {
		if notify.NonBlocking {
			// non-blocking channel write
			select {
			case notify.Chan <- struct{}{}:
			default:
			}
		} else {
			notify.Chan <- struct{}{}
		}

		if notify.DoneChan != nil {
			<-notify.DoneChan
		}
	}

	return nil
}

func (h *hook) GetFunction(name string) *Function {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if function, ok := h.functions[name]; ok {
		return function
	}
	return nil
}

func (h *hook) GetNotify(name string) *Notify {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if notify, ok := h.notifies[name]; ok {
		return notify
	}
	return nil
}

func (h *hook) Function(function *Function) error {
	// requirements
	if function.Name == "" {
		return ErrFunctionNameEmpty
	}
	if function.Func == nil {
		return ErrFuncNil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.functions[function.Name]; ok && !function.Overwrite {
		return fmt.Errorf("%w: \"%s\"", ErrFunctionExists, function.Name)
	}

	h.doReorder = true
	h.functions[function.Name] = function
	return nil
}

func (h *hook) Notify(notify *Notify) error {
	// requirements
	if notify.Name == "" {
		return ErrNotifyNameEmpty
	}
	if notify.Chan == nil {
		return ErrChanNil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.notifies[notify.Name]; ok && !notify.Overwrite {
		return fmt.Errorf("%w: \"%s\"", ErrNotifyExists, notify.Name)
	}

	h.doReorder = true
	h.notifies[notify.Name] = notify
	return nil
}

var _ Hook = (*hook)(nil)

func NewHook(name, desc string) Hook {
	return &hook{
		name:      name,
		desc:      desc,
		functions: make(map[string]*Function),
		notifies:  make(map[string]*Notify),
	}
}

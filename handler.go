package gosignal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
)

var (
	ErrSignalExists = errors.New("hook for signal is already added")
	ErrExitExists   = errors.New("hook for program exit is already added")
	ErrHookNil      = errors.New("hook is nil")
	ErrLoopLocked   = errors.New("loop is already running")
	ErrExitLocked   = errors.New("exit is already running")
)

type et uint8

const (
	ExitTypeSignal et = iota
	ExitTypeManual
)

type OSSignal struct {
	Signal   os.Signal
	Exit     bool
	ExitType et
	ExitCode int
}

type Handler interface {
	SetExit(Hook) error
	GetExit() Hook
	GetsExit() Hook

	Set(os.Signal, Hook) error
	Get(os.Signal) Hook
	Gets(os.Signal) Hook

	Loop()
	Exit(int)
}

// impl Handler
type handler struct {
	exitHook  Hook
	hookMap   map[os.Signal]Hook
	capturing []os.Signal

	sigCh  chan os.Signal
	exitCh chan struct{}
	mu     sync.Mutex

	loopLock bool
	exitLock bool
}

func (h *handler) setExit(hook Hook) error {
	if hook == nil {
		return ErrHookNil
	}
	if h.exitHook != nil {
		return ErrExitExists
	}

	h.exitHook = hook
	for _, sig := range []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT} {
		if !slices.Contains(h.capturing, sig) {
			signal.Notify(h.sigCh, sig)
			h.capturing = append(h.capturing, sig)
		}
	}

	return nil
}

func (h *handler) SetExit(hook Hook) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.setExit(hook)
}

func (h *handler) GetExit() Hook {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.exitHook
}

func (h *handler) GetsExit() Hook {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.exitHook == nil {
		h.setExit(NewHook("exit", "Handle program exit"))
	}
	return h.exitHook
}

func (h *handler) set(sig os.Signal, hook Hook) error {
	if hook == nil {
		return ErrHookNil
	}
	if _, ok := h.hookMap[sig]; ok {
		return fmt.Errorf("%w: %s", ErrSignalExists, sig.String())
	}

	h.hookMap[sig] = hook
	if !slices.Contains(h.capturing, sig) {
		signal.Notify(h.sigCh, sig)
		h.capturing = append(h.capturing, sig)
	}

	return nil
}

func (h *handler) Set(sig os.Signal, hook Hook) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.set(sig, hook)
}

func (h *handler) Get(sig os.Signal) Hook {
	h.mu.Lock()
	defer h.mu.Unlock()
	if hook, ok := h.hookMap[sig]; ok {
		return hook
	}
	return nil
}

func (h *handler) Gets(sig os.Signal) Hook {
	h.mu.Lock()
	defer h.mu.Unlock()
	if hook, ok := h.hookMap[sig]; ok {
		return hook
	}
	h.set(sig, NewHook(
		fmt.Sprintf("signal.%s", strings.ReplaceAll(sig.String(), " ", "_")),
		fmt.Sprintf("Handle %s signal", sig.String()),
	))
	return h.hookMap[sig]
}

func (h *handler) handleSignal(sig os.Signal) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.exitLock {
		return
	}

	s := &OSSignal{
		Signal:   sig,
		Exit:     false,
		ExitType: ExitTypeSignal,
		ExitCode: 0,
	}
	ctx := context.TODO()
	ctx = context.WithValue(ctx, "signal", s)

	switch sig {
	case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
		s.Exit = true
		s.ExitCode = 0
	}

	if hook, ok := h.hookMap[sig]; ok {
		hook.Exec(ctx)
	}
	switch sig {
	case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
		h.exit(0, ExitTypeSignal, sig)
	}
}

func (h *handler) Loop() {
	h.mu.Lock()
	if h.exitLock {
		h.mu.Unlock()
		panic(ErrExitLocked)
	}
	if h.loopLock {
		h.mu.Unlock()
		panic(ErrLoopLocked)
	}
	h.loopLock = true
	h.mu.Unlock()

handling_loop:
	for {
		select {
		case sig := <-h.sigCh:
			h.handleSignal(sig)
		case <-h.exitCh:
			break handling_loop
		}
	}
}

func (h *handler) exit(code int, t et, sig os.Signal) {
	if h.exitLock {
		return
	}
	h.exitLock = true
	select {
	case h.exitCh <- struct{}{}:
	default:
	}
	h.loopLock = false

	s := &OSSignal{
		Signal:   sig,
		Exit:     true,
		ExitType: t,
		ExitCode: code,
	}
	ctx := context.TODO()
	ctx = context.WithValue(ctx, "signal", s)

	if h.exitHook != nil {
		h.exitHook.Exec(ctx)
	}
	signal.Reset(h.capturing...)
	os.Exit(code)
}

func (h *handler) Exit(code int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.exit(code, ExitTypeManual, nil)
}

var _ Handler = (*handler)(nil)

// newHandler is private because there must be only one handler for the entire program
func newHandler() Handler {
	return &handler{
		hookMap: make(map[os.Signal]Hook),
		sigCh:   make(chan os.Signal, 1),
		exitCh:  make(chan struct{}),
	}
}

var defaultHandler Handler = nil

func GetHandler() Handler {
	if defaultHandler == nil {
		defaultHandler = newHandler()
	}
	return defaultHandler
}

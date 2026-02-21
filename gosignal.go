package gosignal

import "context"

type Function struct {
	Func func(context.Context)

	Name      string
	Desc      string
	Overwrite bool

	Order      uint16
	Concurrent bool
}

type Notify struct {
	Chan     chan struct{}
	DoneChan chan struct{}

	Name      string
	Desc      string
	Overwrite bool

	Order       uint16
	NonBlocking bool
}

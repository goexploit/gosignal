package gosignal

type Function struct {
	Func func()

	Name string
	Desc string

	Order      uint16
	Concurrent bool
}

type Notify struct {
	Chan     chan struct{}
	DoneChan chan struct{}

	Name string
	Desc string

	Order       uint16
	NonBlocking bool
}

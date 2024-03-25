package natshelper

// Kind - kind of notification
type Kind string

// describes possible kinds
const (
	// KindInfo - informational message
	KindInfo Kind = "inf"
	// KindError - error
	KindError Kind = "err"
	// KindRequest - request of some kind
	KindRequest Kind = "req"
	// KindSystem - system message
	KindSystem Kind = "sys"
	// KindEvent - some event in the system
	KindEvent Kind = "evn"
	// KindUnknown may be used in case if kind is not in the list
	KindUnknown Kind = "unknown"
	// KindAbsolut should be used if prefix should not be added
	KindAbsolut Kind = "abs"
)

type Option int

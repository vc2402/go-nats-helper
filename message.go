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
	// KindUnknown
	KindUnknown Kind = "unknown"
)

type Option int

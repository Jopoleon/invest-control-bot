package messenger

// UserIdentity identifies an end-user inside a specific messenger transport.
type UserIdentity struct {
	Kind     Kind
	ID       int64
	Username string
}

// IncomingMessage is a transport-neutral text message event.
type IncomingMessage struct {
	User   UserIdentity
	ChatID int64
	Text   string
}

// IncomingAction is a transport-neutral callback/button action event.
type IncomingAction struct {
	Ref       ActionRef
	User      UserIdentity
	ChatID    int64
	MessageID int
	Data      string
}

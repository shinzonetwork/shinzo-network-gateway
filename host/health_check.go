package host

type ConnectionChecker interface {
	CheckConnection(h Host) ConnectionStatus
}

type ConnectionStatus struct {
	Online bool
}

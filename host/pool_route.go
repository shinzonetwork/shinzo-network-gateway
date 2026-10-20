package host

// PoolRoute is the routing information the gateway keeps for one on-chain pool:
// the collection its view serves, the member hosts (by endpoint), and whether the
// pool is currently active. It lets the gateway fan a query out to exactly the
// hosts in the pool the client signed, instead of every host serving the view.
type PoolRoute struct {
	Collection string
	Members    []Host
	IsActive   bool
}

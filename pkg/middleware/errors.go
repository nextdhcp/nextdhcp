package middleware

import "errors"

var (
	// ErrDropRequest signals that the DHCPv4 request should be dropped and no
	// reply should be sent to the requesting client
	ErrDropRequest = errors.New("drop request")
)

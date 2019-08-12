package subnet

import "errors"

var (
	// ErrDropRequest signals the DHCP server handler that this request is
	// dropped on purpose
	ErrDropRequest = errors.New("drop-request")
)

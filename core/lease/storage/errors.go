package storage

import (
	"errors"
	"fmt"
	"net"
)

type (
	// ErrDuplicateIP is returned by the storage if an IP
	// address is already used in a lease
	ErrDuplicateIP struct {
		// IP holds the IP address that is used multiple times
		IP net.IP

		// ClientID may hold the ID of the client that has the IP assigned
		ClientID string
	}

	// ErrDuplicateClientID is returned if the client ID already has a
	// IP lease assigned
	ErrDuplicateClientID struct {
		// ClientID is the client ID that is used multiple times
		ClientID string

		// IP may hold the IP address used by the client
		IP net.IP
	}

	// ErrIPNotFound is returned when the IP in question is not
	// available in the lease storage
	ErrIPNotFound struct {
		IP net.IP
	}
)

var (
	// ErrAlreadyCreated is returned when the IP / clientID pair is already
	// stored
	ErrAlreadyCreated = errors.New("entry already available")

	// ErrClientMismatch is returned from Delete if the given client ID does
	// not match the one stored
	ErrClientMismatch = errors.New("expected client ID does not match")
)

func (eip *ErrDuplicateIP) Error() string {
	return fmt.Sprintf("%s already used by %q", eip.IP.String(), eip.ClientID)
}

func (ecid *ErrDuplicateClientID) Error() string {
	return fmt.Sprintf("%s already has IP %s assigned", ecid.ClientID, ecid.IP.String())
}

func (ein *ErrIPNotFound) Error() string {
	return fmt.Sprintf("%s not found", ein.IP.String())
}

// IsNotFound returns true if err is an IP or client not found
// error
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}

	_, ok := err.(*ErrIPNotFound)
	return ok
}

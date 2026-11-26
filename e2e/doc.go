// Package e2e holds end-to-end tests that run NextDHCP and a DHCP client in
// two network namespaces, connected by a veth pair. They only run on Linux,
// as root, with the ip command of iproute2, and are skipped otherwise:
//
//	sudo go test ./e2e/
package e2e

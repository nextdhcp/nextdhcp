package bootfile

import (
	"context"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBootfileServeDHCP(t *testing.T) {
	for name, tc := range map[string]struct {
		modifiers []dhcpv4.Modifier
		bootFile  string
	}{
		// clients that don't boot from the network don't send their
		// architecture, and get no boot file
		"no architecture": {},
		"BIOS": {
			modifiers: []dhcpv4.Modifier{dhcpv4.WithOption(dhcpv4.OptClientArch(iana.INTEL_X86PC))},
			bootFile:  "pxelinux.0",
		},
		"UEFI": {
			modifiers: []dhcpv4.Modifier{dhcpv4.WithOption(dhcpv4.OptClientArch(iana.EFI_X86_64))},
			bootFile:  "bootx64.efi",
		},
	} {
		t.Run(name, func(t *testing.T) {
			nextCalled := false
			p := &Plugin{
				Bootfile: map[BootMode]string{BIOS: "pxelinux.0", UEFI: "bootx64.efi"},
				L:        zap.NewNop().Sugar(),
				Next: test.HandlerFunc(func(context.Context, *dhcpv4.DHCPv4, *dhcpv4.DHCPv4) error {
					nextCalled = true
					return nil
				}),
			}

			req, err := dhcpv4.NewDiscovery(net.HardwareAddr{0, 1, 2, 3, 4, 5}, tc.modifiers...)
			require.NoError(t, err)

			res, err := dhcpv4.NewReplyFromRequest(req)
			require.NoError(t, err)

			require.NoError(t, p.ServeDHCP(context.Background(), req, res))
			assert.True(t, nextCalled, "the request must be passed to the next handler")
			assert.Equal(t, tc.bootFile, res.BootFileNameOption())
		})
	}
}

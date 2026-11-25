package bootfile

import (
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootfileSetup(t *testing.T) {
	p := new(Plugin)
	require.NoError(t, p.UnmarshalCaddyfile(caddyfile.NewTestDispenser("bootfile {\n bios pxelinux.0\n uefi bootx64.efi\n}")))
	assert.Equal(t, map[BootMode]string{BIOS: "pxelinux.0", UEFI: "bootx64.efi"}, p.Bootfile)

	p = new(Plugin)
	require.NoError(t, p.UnmarshalCaddyfile(caddyfile.NewTestDispenser("bootfile {\n legacy pxelinux.0\n}")))
	assert.Equal(t, map[BootMode]string{BIOS: "pxelinux.0"}, p.Bootfile)

	for _, input := range []string{
		// boot files are only configured in a block
		"bootfile bios pxelinux.0",
		"bootfile {\n bios\n}",
		"bootfile {\n bios pxelinux.0 other.0\n}",
		"bootfile {\n arm boot.efi\n}",
	} {
		assert.Error(t, new(Plugin).UnmarshalCaddyfile(caddyfile.NewTestDispenser(input)), input)
	}
}

package bootfile

import (
	"context"
	"errors"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/nextdhcp/nextdhcp/core/option"
)

/*** Client Arch type
	0    Intel x86PC
	1    NEC/PC98
	2    EFI Itanium
	3    DEC Alpha
	4    Arc x86
	5    Intel Lean Client
	6    EFI IA32
	7    EFI BC
	8    EFI Xscale
	9    EFI x86-64
***/

var errNoClientARCH = errors.New("no client arch")

// BootMode is the key used to associate request timestamp with a context.Context
type BootMode string

const (
	// BIOS bios boot
	BIOS BootMode = "bios"
	// UEFI uefi boot
	UEFI BootMode = "uefi"
)

// GetBootFileOpt returns option of DHCPs
func (p *Plugin) GetBootFileOpt(ctx context.Context, req, res *dhcpv4.DHCPv4) (*dhcpv4.Option, error) {
	bootFileName := p.parseBootFileName(req)
	if bootFileName == "" {
		return nil, errNoClientARCH
	}
	code, value, err := option.ParseKnown("filename", []string{bootFileName})
	if err != nil {
		return nil, err
	}
	option := dhcpv4.OptGeneric(code, value.ToBytes())
	return &option, nil
}

func (p *Plugin) parseBootFileName(req *dhcpv4.DHCPv4) string {
	archs := iana.Archs(req.ClientArch())

	var bootFile string

	switch archs[0] {
	case iana.INTEL_X86PC:
		fallthrough
	case iana.NEC_PC98:
		fallthrough
	case iana.DEC_ALPHA:
		fallthrough
	case iana.ARC_X86:
		fallthrough
	case iana.INTEL_LEAN_CLIENT:
		bootFile = p.Bootfile[BIOS]
	case iana.EFI_ITANIUM:
		fallthrough
	case iana.EFI_IA32:
		fallthrough
	case iana.EFI_BC:
		fallthrough
	case iana.EFI_XSCALE:
		fallthrough
	case iana.EFI_X86_64:
		bootFile = p.Bootfile[UEFI]
	}

	p.L.Debugf("receive client request with client_archs option: %s, dhcp server set boot-file-name as %s",
		archs.String(), bootFile)
	return bootFile
}

// Name implements the plugin.Handler interface and returns "bootfile"
func (p *Plugin) Name() string {
	return "bootfile"
}

// ServeDHCP handle dhcp request
func (p *Plugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	bootFile, err := p.GetBootFileOpt(ctx, req, res)
	if err != nil && err != errNoClientARCH {
		return err
	}
	if err == nil {
		res.UpdateOption(*bootFile)
	}
	return p.Next.ServeDHCP(ctx, req, res)
}

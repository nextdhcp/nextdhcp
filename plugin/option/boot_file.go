package option

import (
	"context"
	"regexp"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/nextdhcp/nextdhcp/core/option"
	"github.com/nextdhcp/nextdhcp/plugin/logger"
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

const (
	// TODO: will set in database and get from database
	efiBootFile    = "ipxe.efi"
	legacyBootFile = "undionly.kpxe"
)

func (p *Plugin) GetBootFileOpt(ctx context.Context, req, res *dhcpv4.DHCPv4) (*dhcpv4.Option, error) {
	bootFileName := parseBootFileName(req)
	code, value, err := option.ParseKnown("filename", []string{bootFileName})
	if err != nil {
		return nil, err
	}
	option := dhcpv4.OptGeneric(code, value.ToBytes())
	return &option, nil
}

func parseBootFileName(req *dhcpv4.DHCPv4) string {
	archs := iana.Archs(req.ClientArch())

	rex := regexp.MustCompile(`(^EFI \S+$)`)
	bootFile := legacyBootFile
	if rex.MatchString(archs.String()) {
		bootFile = efiBootFile
	}
	logger.Log.Debugf("receive client request with client_archs option :%s, dhcp server set boot-file-name as %s",
		archs.String(), bootFile)
	return bootFile
}

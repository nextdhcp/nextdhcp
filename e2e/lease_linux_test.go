//go:build linux

package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/mdlayher/packet"
	"github.com/nextdhcp/nextdhcp/dhcpmain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// the dhcp app, its plugins and the lease databases, as in the nextdhcp
	// command
	_ "github.com/nextdhcp/nextdhcp/core"
)

// The test binary runs itself as the server and as the client, in their
// network namespaces, in the role that roleEnv names.
const (
	roleEnv     = "NEXTDHCP_E2E_ROLE"     // "server" or "client"
	dhcpfileEnv = "NEXTDHCP_E2E_DHCPFILE" // the Dhcpfile of the server
)

const (
	clientInterface = "veth-c"
	serverIP        = "10.98.0.1"
	firstLease      = "10.98.0.100"
)

// dhcpfile serves the client's subnet, and a second subnet without clients,
// so that the servers of the udp transport share its socket.
const dhcpfile = `
10.98.0.1/24 {
	transport %[1]s
	database memory
	range 10.98.0.100 10.98.0.150
	lease 1h
}

10.97.0.1/24 {
	transport %[1]s
	database memory
	range 10.97.0.100 10.97.0.150
	lease 1h
}
`

func TestMain(m *testing.M) {
	switch os.Getenv(roleEnv) {
	case "server":
		os.Args = []string{os.Args[0], "-conf", os.Getenv(dhcpfileEnv)}
		dhcpmain.Run()
		os.Exit(0)
	case "client":
		os.Exit(runClient())
	}

	os.Exit(m.Run())
}

// TestLease checks that a client gets a lease and renews it with each
// transport, and where the server sends its replies.
func TestLease(t *testing.T) {
	requireNetworkNamespaces(t)

	cases := []struct {
		transport string

		// whether the server sends the OFFER and the ACK to the client's
		// hardware address and the leased address, instead of broadcasting
		// them
		unicast bool
	}{
		{transport: "packet", unicast: true},
		{transport: "udp", unicast: false},
	}

	for _, tc := range cases {
		t.Run(tc.transport, func(t *testing.T) {
			n := newNetwork(t)
			startServer(t, n, fmt.Sprintf(dhcpfile, tc.transport))

			res := runClientIn(t, n)
			assert.Equal(t, firstLease, res.Lease)
			assert.Equal(t, serverIP, res.Server)
			assert.Equal(t, firstLease, res.Renewed)

			leaseReply := reply{EthDst: "ff:ff:ff:ff:ff:ff", IPSrc: serverIP, IPDst: "255.255.255.255"}
			if tc.unicast {
				leaseReply = reply{EthDst: res.MAC, IPSrc: serverIP, IPDst: firstLease}
			}

			// The server unicasts its reply to a renewal to the client's
			// address with either transport
			renewReply := reply{EthDst: res.MAC, IPSrc: serverIP, IPDst: firstLease}

			var types []string
			for _, r := range res.Replies {
				types = append(types, r.Type+" "+r.XID)

				switch r.XID {
				case res.LeaseXID:
					assert.Equal(t, leaseReply, r.addresses(), "%s of the lease", r.Type)
				case res.RenewXID:
					assert.Equal(t, renewReply, r.addresses(), "%s of the renewal", r.Type)
				default:
					t.Errorf("unexpected reply %+v", r)
				}
			}

			assert.Contains(t, types, "OFFER "+res.LeaseXID)
			assert.Contains(t, types, "ACK "+res.LeaseXID)
			assert.Contains(t, types, "ACK "+res.RenewXID)
		})
	}
}

// requireNetworkNamespaces skips the test unless it can create network
// namespaces.
func requireNetworkNamespaces(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("end-to-end tests don't run in short mode")
	}

	if os.Geteuid() != 0 {
		t.Skip("creating network namespaces needs root")
	}

	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("creating network namespaces needs the ip command of iproute2")
	}

	name := fmt.Sprintf("nextdhcp-e2e-check-%d", os.Getpid())
	if out, err := exec.Command("ip", "netns", "add", name).CombinedOutput(); err != nil {
		t.Skipf("failed to create a network namespace: %s", bytes.TrimSpace(out))
	}

	_ = exec.Command("ip", "netns", "del", name).Run()
}

// network is a pair of network namespaces: the server's, and the client's.
type network struct {
	srv, cli string
}

var networks atomic.Int32

// newNetwork creates the network namespaces of a server and a client, which
// are removed when the test ends. The client's interface clientInterface is
// connected to the server's interface on the subnet 10.98.0.0/24. The server
// has a second interface on the subnet 10.97.0.0/24.
func newNetwork(t *testing.T) network {
	t.Helper()

	id := fmt.Sprintf("%d-%d", os.Getpid(), networks.Add(1))
	n := network{srv: "nextdhcp-e2e-srv-" + id, cli: "nextdhcp-e2e-cli-" + id}

	for _, ns := range []string{n.srv, n.cli} {
		ip(t, "netns", "add", ns)
		t.Cleanup(func() { _ = exec.Command("ip", "netns", "del", ns).Run() })

		ip(t, "-n", ns, "link", "set", "lo", "up")
	}

	for _, link := range []struct{ server, client, addr string }{
		{server: "veth-s", client: clientInterface, addr: serverIP + "/24"},
		{server: "veth-s2", client: "veth-c2", addr: "10.97.0.1/24"},
	} {
		ip(t, "link", "add", link.server, "netns", n.srv, "type", "veth", "peer", "name", link.client, "netns", n.cli)
		ip(t, "-n", n.srv, "addr", "add", link.addr, "dev", link.server)
		ip(t, "-n", n.srv, "link", "set", link.server, "up")
		ip(t, "-n", n.cli, "link", "set", link.client, "up")
	}

	return n
}

// ip runs the ip command with args.
func ip(t *testing.T, args ...string) {
	t.Helper()

	out, err := exec.Command("ip", args...).CombinedOutput()
	require.NoError(t, err, "ip %s: %s", strings.Join(args, " "), out)
}

// startServer starts NextDHCP with dhcpfile in the server's network
// namespace, and waits until it serves. It's stopped when the test ends.
func startServer(t *testing.T, n network, dhcpfile string) {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)

	dir := t.TempDir()
	conf := filepath.Join(dir, "Dhcpfile")
	require.NoError(t, os.WriteFile(conf, []byte(dhcpfile), 0o600))

	logs, err := os.Create(filepath.Join(dir, "server.log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = logs.Close() })

	cmd := exec.Command("ip", "netns", "exec", n.srv, exe)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), roleEnv+"=server", dhcpfileEnv+"="+conf)
	cmd.Stderr = logs

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		if t.Failed() {
			out, _ := os.ReadFile(logs.Name())
			t.Logf("server logs:\n%s", out)
		}
	})

	// NextDHCP prints the subnets it serves once it serves them
	ready := make(chan struct{})
	go func() {
		s := bufio.NewScanner(stdout)
		var once sync.Once
		for s.Scan() {
			if strings.HasPrefix(s.Text(), "Serving the following subnets") {
				once.Do(func() { close(ready) })
			}
		}
	}()

	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("the server didn't start serving within 10s")
	}
}

// clientResult is what the client prints.
type clientResult struct {
	MAC      string  `json:"mac"`       // the hardware address of the client
	Lease    string  `json:"lease"`     // the leased address
	Server   string  `json:"server"`    // the server identifier of the lease
	LeaseXID string  `json:"lease_xid"` // the transaction of the lease
	Renewed  string  `json:"renewed"`   // the address of the renewed lease
	RenewXID string  `json:"renew_xid"` // the transaction of the renewal
	Replies  []reply `json:"replies"`   // the server's replies, in order
}

// reply is a reply of the server that the client received.
type reply struct {
	Type   string `json:"type,omitempty"` // the DHCP message type
	XID    string `json:"xid,omitempty"`  // the transaction ID
	EthDst string `json:"eth_dst"`
	IPSrc  string `json:"ip_src"`
	IPDst  string `json:"ip_dst"`
}

// addresses returns r with only its addresses.
func (r reply) addresses() reply {
	return reply{EthDst: r.EthDst, IPSrc: r.IPSrc, IPDst: r.IPDst}
}

// runClientIn runs the client in the client's network namespace, and
// returns what it printed.
func runClientIn(t *testing.T, n network) clientResult {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", n.cli, exe)
	cmd.Env = append(os.Environ(), roleEnv+"=client")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	require.NoError(t, cmd.Run(), "the client failed: %s", stderr.String())

	var res clientResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &res), stdout.String())

	return res
}

// runClient gets a lease on clientInterface and renews it, records the
// server's replies, and prints the result as JSON. It runs in the client's
// network namespace, and returns the exit code of the process.
func runClient() int {
	res, err := leaseAndRenew()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := json.NewEncoder(os.Stdout).Encode(res); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}

func leaseAndRenew() (*clientResult, error) {
	ifi, err := net.InterfaceByName(clientInterface)
	if err != nil {
		return nil, err
	}

	replies, err := captureReplies(ifi)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Like a client without an address, send through a raw socket, which
	// broadcasts the requests and receives the replies whatever their
	// destination
	c, err := nclient4.New(clientInterface, nclient4.WithTimeout(2*time.Second))
	if err != nil {
		return nil, err
	}
	defer c.Close()

	lease, err := c.Request(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get a lease: %w", err)
	}

	// Like a client that has an address, renew through a UDP socket on the
	// address, which the server sends the reply to
	addr := lease.ACK.YourIPAddr
	if out, err := exec.Command("ip", "addr", "add", addr.String()+"/24", "dev", clientInterface).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to add the leased address: %w: %s", err, out)
	}

	rc, err := nclient4.New(clientInterface,
		nclient4.WithTimeout(2*time.Second),
		nclient4.WithUnicast(&net.UDPAddr{IP: addr, Port: dhcpv4.ClientPort}),
		nclient4.WithServerAddr(&net.UDPAddr{IP: lease.ACK.ServerIdentifier(), Port: dhcpv4.ServerPort}),
	)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	// A new transaction, to tell the replies to the renewal apart
	xid, err := dhcpv4.GenerateTransactionID()
	if err != nil {
		return nil, err
	}

	renewed, err := rc.Renew(ctx, lease, dhcpv4.WithTransactionID(xid))
	if err != nil {
		return nil, fmt.Errorf("failed to renew the lease: %w", err)
	}

	return &clientResult{
		MAC:      ifi.HardwareAddr.String(),
		Lease:    addr.String(),
		Server:   lease.ACK.ServerIdentifier().String(),
		LeaseXID: lease.ACK.TransactionID.String(),
		Renewed:  renewed.ACK.YourIPAddr.String(),
		RenewXID: xid.String(),
		Replies:  replies(),
	}, nil
}

// captureReplies records the DHCP replies that arrive on ifi until the
// function it returns is called, which returns them.
func captureReplies(ifi *net.Interface) (func() []reply, error) {
	conn, err := packet.Listen(ifi, packet.Raw, int(layers.EthernetTypeIPv4), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to capture on %s: %w", ifi.Name, err)
	}

	var (
		mu      sync.Mutex
		replies []reply
		done    = make(chan struct{})
	)

	go func() {
		defer close(done)

		buf := make([]byte, 1<<16)
		for {
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}

			if r, ok := parseReply(buf[:n]); ok {
				mu.Lock()
				replies = append(replies, r)
				mu.Unlock()
			}
		}
	}()

	return func() []reply {
		// Give the socket time to read the replies that already arrived
		time.Sleep(200 * time.Millisecond)
		_ = conn.Close()
		<-done

		mu.Lock()
		defer mu.Unlock()

		return replies
	}, nil
}

// parseReply returns the reply in frame, if it's a DHCP reply.
func parseReply(frame []byte) (reply, bool) {
	p := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)

	eth, ok := p.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
	if !ok {
		return reply{}, false
	}

	ip4, ok := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	if !ok {
		return reply{}, false
	}

	udp, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
	if !ok || udp.SrcPort != dhcpv4.ServerPort || udp.DstPort != dhcpv4.ClientPort {
		return reply{}, false
	}

	msg, err := dhcpv4.FromBytes(udp.Payload)
	if err != nil || msg.OpCode != dhcpv4.OpcodeBootReply {
		return reply{}, false
	}

	return reply{
		Type:   msg.MessageType().String(),
		XID:    msg.TransactionID.String(),
		EthDst: eth.DstMAC.String(),
		IPSrc:  ip4.SrcIP.String(),
		IPDst:  ip4.DstIP.String(),
	}, true
}

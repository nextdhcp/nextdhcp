package mqtt

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type publishedMessage struct {
	topic   string
	payload string
}

// fakeBroker is an MQTT broker that accepts all clients and records the
// messages they publish with QoS 0.
type fakeBroker struct {
	ln          net.Listener
	conns       atomic.Int32
	handlers    sync.WaitGroup
	published   chan publishedMessage
	disconnects chan struct{} // receives a value for each DISCONNECT packet

	// If release is set, the broker sends a value to connects when it gets
	// a CONNECT packet, and answers CONNECT packets only once release is
	// closed: it accepts the connections if accept is true, and closes them
	// otherwise. Clients may try to connect more than once.
	connects chan struct{}
	release  chan struct{}
	accept   bool
}

func newFakeBroker(t *testing.T) *fakeBroker {
	return startBroker(t, new(fakeBroker))
}

// newHoldingBroker returns a broker that answers CONNECT packets only once
// the test closes its release channel.
func newHoldingBroker(t *testing.T, accept bool) *fakeBroker {
	return startBroker(t, &fakeBroker{
		connects: make(chan struct{}, 1),
		release:  make(chan struct{}),
		accept:   accept,
	})
}

func startBroker(t *testing.T, b *fakeBroker) *fakeBroker {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	b.ln = ln
	b.published = make(chan publishedMessage, 16)
	b.disconnects = make(chan struct{}, 16)

	go b.serve()
	t.Cleanup(func() { _ = ln.Close() })

	return b
}

func (b *fakeBroker) url() string {
	return "tcp://" + b.ln.Addr().String()
}

func (b *fakeBroker) serve() {
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return
		}

		b.conns.Add(1)
		b.handlers.Add(1)
		go b.handle(conn)
	}
}

func (b *fakeBroker) handle(conn net.Conn) {
	defer b.handlers.Done()
	defer conn.Close()

	r := bufio.NewReader(conn)
	for {
		header, body, err := readPacket(r)
		if err != nil {
			return
		}

		switch header >> 4 {
		case 1: // CONNECT
			if b.release != nil {
				select {
				case b.connects <- struct{}{}:
				default:
				}

				<-b.release
				if !b.accept {
					return
				}
			}

			_, _ = conn.Write([]byte{0x20, 0x02, 0x00, 0x00}) // CONNACK, accepted
		case 3: // PUBLISH
			n := int(binary.BigEndian.Uint16(body))
			b.published <- publishedMessage{
				topic:   string(body[2 : 2+n]),
				payload: string(body[2+n:]),
			}
		case 12: // PINGREQ
			_, _ = conn.Write([]byte{0xd0, 0x00}) // PINGRESP
		case 14: // DISCONNECT
			b.disconnects <- struct{}{}
			return
		}
	}
}

func readPacket(r *bufio.Reader) (byte, []byte, error) {
	header, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	length, multiplier := 0, 1
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, nil, err
		}

		length += int(b&127) * multiplier
		if b&128 == 0 {
			break
		}
		multiplier *= 128
	}

	body := make([]byte, length)
	_, err = io.ReadFull(r, body)

	return header, body, err
}

// TestMqttPublish publishes messages for a configuration without payload,
// using the connection of a configuration without topic.
func TestMqttPublish(t *testing.T) {
	broker := newFakeBroker(t)

	m, err := makeMqttPlugin(t, fmt.Sprintf(`
	mqtt {
		name local
		broker %s
	}

	mqtt msgtype == 'DISCOVER' {
		use local
		topic nextdhcp/{msgtype}/{hwaddr}
	}`, broker.url()))
	require.NoError(t, err)
	m.Wrap(test.NoOpHandler)

	req, err := dhcpv4.NewDiscovery(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee})
	require.NoError(t, err)
	res, err := dhcpv4.NewReplyFromRequest(req)
	require.NoError(t, err)

	require.NoError(t, m.ServeDHCP(context.Background(), req, res))
	m.wg.Wait()

	select {
	case msg := <-broker.published:
		assert.Equal(t, "nextdhcp/DISCOVER/00:aa:bb:cc:dd:ee", msg.topic)
		assert.Equal(t, "", msg.payload)
	case <-time.After(5 * time.Second):
		t.Fatal("no message was published")
	}

	// disconnect and let the broker read everything the client sent
	require.NoError(t, m.Cleanup())
	broker.handlers.Wait()

	// the configuration without topic didn't publish anything
	assert.Empty(t, broker.published)
	assert.Equal(t, int32(1), broker.conns.Load())
}

// TestMqttNoConnectionAfterCleanup checks that a message that is still being
// published when the plugin is cleaned up doesn't open a new connection.
func TestMqttNoConnectionAfterCleanup(t *testing.T) {
	broker := newFakeBroker(t)

	m, err := makeMqttPlugin(t, fmt.Sprintf("mqtt {\n broker %s\n topic some/topic\n}", broker.url()))
	require.NoError(t, err)
	require.NoError(t, m.Cleanup())

	_, _, err = m.getClient(m.configs[0])
	assert.Error(t, err)
	assert.Equal(t, int32(0), broker.conns.Load())
}

// TestMqttCleanupDuringConnect checks that Cleanup doesn't wait for a
// connection that is still being opened, and that such a connection is
// closed if it opens after all.
func TestMqttCleanupDuringConnect(t *testing.T) {
	for _, accept := range []bool{false, true} {
		t.Run(fmt.Sprintf("accept=%v", accept), func(t *testing.T) {
			broker := newHoldingBroker(t, accept)

			m, err := makeMqttPlugin(t, fmt.Sprintf("mqtt {\n broker %s\n topic some/topic\n}", broker.url()))
			require.NoError(t, err)

			connected := make(chan error, 1)
			go func() {
				_, _, err := m.getClient(m.configs[0])
				connected <- err
			}()

			select {
			case <-broker.connects:
			case <-time.After(5 * time.Second):
				t.Fatal("the client didn't connect")
			}

			cleanedUp := make(chan error, 1)
			go func() { cleanedUp <- m.Cleanup() }()

			select {
			case err := <-cleanedUp:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("Cleanup waits for the connection being opened")
			}

			close(broker.release)

			select {
			case err := <-connected:
				if accept {
					assert.ErrorIs(t, err, errConnectionClosed)
				} else {
					assert.Error(t, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("getClient didn't return")
			}

			if accept {
				select {
				case <-broker.disconnects:
				case <-time.After(5 * time.Second):
					t.Fatal("the connection opened after Cleanup wasn't closed")
				}
			}

			conn := m.configs[0].conn
			conn.l.Lock()
			defer conn.l.Unlock()
			assert.Nil(t, conn.c)
		})
	}
}

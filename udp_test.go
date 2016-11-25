package osc

import (
	"bytes"
	"log"
	"net"
	"testing"

	"github.com/pkg/errors"
)

func TestInvalidAddress(t *testing.T) {
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server, err := ListenUDP("udp", laddr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }() // Best effort.

	if err := server.Serve(map[string]Method{
		"/[": func(msg Message) error {
			return nil
		},
	}); err != ErrInvalidAddress {
		t.Fatal("expected invalid address error")
	}
}

func TestDialUDP(t *testing.T) {
	if _, err := DialUDP("asdfiauosweif", nil, nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListenUDP(t *testing.T) {
	if _, err := ListenUDP("asdfiauosweif", nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUDPConnSend_OK(t *testing.T) {
	errChan := make(chan error)

	// Setup the server.
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server, err := ListenUDP("udp", laddr)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		errChan <- server.Serve(map[string]Method{
			"/close": func(msg Message) error {
				return server.Close()
			},
		})
	}()

	// Setup the client.
	raddr, err := net.ResolveUDPAddr("udp", server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn, err := DialUDP("udp", nil, raddr)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Send(Message{Address: "/close"}); err != nil {
		t.Fatal(err)
	}
	if err := <-errChan; err != nil {
		t.Fatal(err)
	}
}

type errConn struct {
	udpConn
}

func (e errConn) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	return 0, nil, errors.New("oops")
}

func TestUDPConnServe_ReadError(t *testing.T) {
	errChan := make(chan error)

	// Setup the server.
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverConn, err := ListenUDP("udp", laddr)
	if err != nil {
		t.Fatal(err)
	}
	server := &UDPConn{
		udpConn: errConn{udpConn: serverConn},
	}
	go func() {
		errChan <- server.Serve(map[string]Method{
			"/close": func(msg Message) error {
				return server.Close()
			},
		})
	}()

	// Setup the client.
	raddr, err := net.ResolveUDPAddr("udp", server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn, err := DialUDP("udp", nil, raddr)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Send(Message{Address: "/close"}); err != nil {
		t.Fatal(err)
	}
	if err := <-errChan; err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUDPConnServe_NilDispatcher(t *testing.T) {
	// Setup the server.
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server, err := ListenUDP("udp", laddr)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// badPacket is a Packet that returns an OSC message with typetag 'Q'
type badPacket struct{}

func (bp badPacket) Bytes() []byte {
	return bytes.Join(
		[][]byte{
			{'/', 'f', 'o', 'o', 0, 0, 0, 0},
			{TypetagPrefix, 'Q', 0, 0},
		},
		[]byte{},
	)
}

func testUDPServer(t *testing.T, dispatcher Dispatcher) (*UDPConn, chan error) {
	// Setup the server.
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server, err := ListenUDP("udp", laddr)
	if err != nil {
		t.Fatal(err)
	}
	errChan := make(chan error)
	go func() {
		errChan <- server.Serve(dispatcher)
	}()

	// Send a message with a bad address.
	raddr, err := net.ResolveUDPAddr("udp", server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn, err := DialUDP("udp", nil, raddr)
	if err != nil {
		t.Fatal(err)
	}
	return conn, errChan
}

func TestUDPConnServe_BadInboundAddr(t *testing.T) {
	// Send a message with a bad address.
	conn, errChan := testUDPServer(t, Dispatcher{
		"/foo": func(msg Message) error {
			return nil
		},
	})
	if err := conn.Send(Message{Address: "/["}); err != nil {
		t.Fatal(err)
	}
	if err := <-errChan; err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUDPConnServe_BadInboundLeadingChar(t *testing.T) {
	conn, errChan := testUDPServer(t, Dispatcher{
		"/foo": func(msg Message) error {
			return nil
		},
	})
	if err := conn.Send(Message{Address: "["}); err != nil {
		t.Fatal(err)
	}
	if err := <-errChan; err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUDPConnServe_BadInboundTypetag(t *testing.T) {
	conn, errChan := testUDPServer(t, Dispatcher{
		"/foo": func(msg Message) error {
			return nil
		},
	})
	laddr2, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	conn2, err := ListenUDP("udp", laddr2)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn2.SendTo(conn.RemoteAddr(), badPacket{}); err != nil {
		t.Fatal(err)
	}
	if err := <-errChan; err == nil {
		t.Fatal("expected error, got nil")
	}
}

func ExampleUDPConn_Send() {
	errChan := make(chan error)

	// Setup the server.
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	server, err := ListenUDP("udp", laddr)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		errChan <- server.Serve(map[string]Method{
			"/close": func(msg Message) error {
				return server.Close()
			},
		})
	}()

	// Setup the client.
	raddr, err := net.ResolveUDPAddr("udp", server.LocalAddr().String())
	if err != nil {
		log.Fatal(err)
	}
	conn, err := DialUDP("udp", nil, raddr)
	if err != nil {
		log.Fatal(err)
	}
	if err := conn.Send(Message{Address: "/close"}); err != nil {
		log.Fatal(err)
	}
	if err := <-errChan; err != nil {
		log.Fatal(err)
	}
}

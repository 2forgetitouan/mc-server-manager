package rcon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// writeRCONPacket writes a properly formatted RCON response packet to a writer.
func writeRCONPacket(w io.Writer, requestID, packetType int32, payload string) error {
	payloadBytes := []byte(payload)
	length := int32(4 + 4 + len(payloadBytes) + 2)

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, length)
	binary.Write(buf, binary.LittleEndian, requestID)
	binary.Write(buf, binary.LittleEndian, packetType)
	buf.Write(payloadBytes)
	buf.WriteByte(0)
	buf.WriteByte(0)

	_, err := w.Write(buf.Bytes())
	return err
}

// readRCONPacket reads and parses a single RCON packet from a reader.
func readRCONPacket(r io.Reader) (requestID, packetType int32, payload string, err error) {
	var length int32
	if err = binary.Read(r, binary.LittleEndian, &length); err != nil {
		return
	}

	data := make([]byte, length)
	if _, err = io.ReadFull(r, data); err != nil {
		return
	}

	reader := bytes.NewReader(data)
	binary.Read(reader, binary.LittleEndian, &requestID)
	binary.Read(reader, binary.LittleEndian, &packetType)

	payloadLen := int(length) - 4 - 4 - 2
	if payloadLen < 0 {
		payloadLen = 0
	}
	payloadBuf := make([]byte, payloadLen)
	io.ReadFull(reader, payloadBuf)
	payload = string(payloadBuf)
	return
}

// startMockRCONServer starts a TCP server that handles RCON authentication
// and one command. It returns the listener address and a cleanup function.
func startMockRCONServer(t *testing.T, password string, authSucceed bool, commandResponse string) (string, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		conn.SetDeadline(time.Now().Add(5 * time.Second))

		// Read login packet.
		reqID, pktType, receivedPw, err := readRCONPacket(conn)
		if err != nil {
			return
		}

		// Verify it's a login packet.
		if pktType != packetTypeLogin {
			return
		}

		// Send auth response.
		if authSucceed && receivedPw == password {
			writeRCONPacket(conn, reqID, packetTypeLoginResp, "")
		} else {
			writeRCONPacket(conn, packetAuthFailRequestID, packetTypeLoginResp, "")
			return
		}

		// Read command packet.
		cmdReqID, cmdType, _, err := readRCONPacket(conn)
		if err != nil {
			return
		}

		if cmdType != packetTypeCommand {
			return
		}

		// Send command response.
		writeRCONPacket(conn, cmdReqID, packetTypeCommandResp, commandResponse)
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

func TestDialAndExecute(t *testing.T) {
	addr, cleanup := startMockRCONServer(t, "testpass", true, "Hello, World!")
	defer cleanup()

	client, err := Dial(addr, "testpass", 5*time.Second)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	resp, err := client.Execute("say hello")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if resp != "Hello, World!" {
		t.Errorf("Execute response = %q, want %q", resp, "Hello, World!")
	}
}

func TestDialAuthFailure(t *testing.T) {
	addr, cleanup := startMockRCONServer(t, "rightpassword", false, "")
	defer cleanup()

	_, err := Dial(addr, "wrongpassword", 5*time.Second)
	if err == nil {
		t.Fatal("expected authentication error")
	}

	if !errors.Is(err, ErrAuth) {
		t.Errorf("error = %v, want ErrAuth", err)
	}
}

func TestDialConnectionRefused(t *testing.T) {
	// Use a port that nothing is listening on.
	_, err := Dial("127.0.0.1:1", "password", 2*time.Second)
	if err == nil {
		t.Fatal("expected connection error")
	}

	if !errors.Is(err, ErrConnection) && !errors.Is(err, ErrTimeout) {
		t.Errorf("error = %v, want ErrConnection or ErrTimeout", err)
	}
}

func TestDialTimeout(t *testing.T) {
	// Start a server that accepts but never responds.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Hold the connection open, never respond.
		defer conn.Close()
		time.Sleep(10 * time.Second)
	}()

	_, err = Dial(ln.Addr().String(), "password", 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestPacketEncoding(t *testing.T) {
	// Test that packets we send can be read back correctly.
	buf := new(bytes.Buffer)

	// Write a packet.
	err := writeRCONPacket(buf, 42, packetTypeCommand, "test payload")
	if err != nil {
		t.Fatalf("writeRCONPacket failed: %v", err)
	}

	// Read it back.
	reqID, pktType, payload, err := readRCONPacket(buf)
	if err != nil {
		t.Fatalf("readRCONPacket failed: %v", err)
	}

	if reqID != 42 {
		t.Errorf("requestID = %d, want 42", reqID)
	}
	if pktType != packetTypeCommand {
		t.Errorf("packetType = %d, want %d", pktType, packetTypeCommand)
	}
	if payload != "test payload" {
		t.Errorf("payload = %q, want %q", payload, "test payload")
	}
}

func TestPacketEncodingEmpty(t *testing.T) {
	buf := new(bytes.Buffer)
	writeRCONPacket(buf, 1, packetTypeCommandResp, "")
	reqID, pktType, payload, err := readRCONPacket(buf)
	if err != nil {
		t.Fatalf("readRCONPacket failed: %v", err)
	}
	if reqID != 1 {
		t.Errorf("requestID = %d, want 1", reqID)
	}
	if pktType != packetTypeCommandResp {
		t.Errorf("packetType = %d, want %d", pktType, packetTypeCommandResp)
	}
	if payload != "" {
		t.Errorf("payload = %q, want empty", payload)
	}
}

func TestClientClose(t *testing.T) {
	addr, cleanup := startMockRCONServer(t, "pass", true, "ok")
	defer cleanup()

	client, err := Dial(addr, "pass", 5*time.Second)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Closing again should not panic.
	// (conn is nil after first close? Actually no, conn is still set.)
	// Execute after close should fail.
	_, err = client.Execute("test")
	if err == nil {
		t.Error("expected error executing on closed client")
	}
}

func TestFullRCONSession(t *testing.T) {
	// Integration-style test: connect, authenticate, execute command.
	const password = "mc-test-password"
	const response = "There are 3 of a max of 20 players online"

	addr, cleanup := startMockRCONServer(t, password, true, response)
	defer cleanup()

	client, err := Dial(addr, password, 5*time.Second)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	got, err := client.Execute("list")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if got != response {
		t.Errorf("response = %q, want %q", got, response)
	}
}

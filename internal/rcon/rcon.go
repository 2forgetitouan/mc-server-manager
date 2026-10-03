package rcon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

var (
	ErrAuth       = errors.New("rcon: authentication failed")
	ErrConnection = errors.New("rcon: connection failed")
	ErrTimeout    = errors.New("rcon: connection timed out")
)

const (
	packetTypeCommand       int32 = 2
	packetTypeLogin         int32 = 3
	packetTypeCommandResp   int32 = 0
	packetTypeLoginResp     int32 = 2
	packetAuthFailRequestID int32 = -1
)

// Client is a Minecraft RCON client.
type Client struct {
	conn      net.Conn
	mu        sync.Mutex
	requestID int32
	timeout   time.Duration
}

// Dial connects and authenticates to a Minecraft RCON server.
func Dial(address, password string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}

	c := &Client{
		conn:      conn,
		requestID: 0,
		timeout:   timeout,
	}

	// Authenticate
	reqID, err := c.sendPacket(packetTypeLogin, password)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}

	respReqID, _, _, err := c.readPacket()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}

	if respReqID == packetAuthFailRequestID || respReqID != reqID {
		conn.Close()
		return nil, ErrAuth
	}

	return c, nil
}

// Execute sends a command and returns the response string.
func (c *Client) Execute(command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	reqID, err := c.sendPacket(packetTypeCommand, command)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrConnection, err)
	}

	respReqID, _, payload, err := c.readPacket()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrConnection, err)
	}

	if respReqID != reqID {
		return "", fmt.Errorf("%w: response ID mismatch (got %d, want %d)", ErrConnection, respReqID, reqID)
	}

	return payload, nil
}

// Close closes the RCON connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// sendPacket writes a single RCON packet and returns the request ID used.
func (c *Client) sendPacket(packetType int32, payload string) (int32, error) {
	c.requestID++
	reqID := c.requestID

	payloadBytes := []byte(payload)
	// length = 4 (requestID) + 4 (type) + len(payload) + 1 (null terminator) + 1 (padding null)
	length := int32(4 + 4 + len(payloadBytes) + 2)

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, length)
	binary.Write(buf, binary.LittleEndian, reqID)
	binary.Write(buf, binary.LittleEndian, packetType)
	buf.Write(payloadBytes)
	buf.WriteByte(0) // null terminator
	buf.WriteByte(0) // padding null

	if c.timeout > 0 {
		c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	}

	_, err := c.conn.Write(buf.Bytes())
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return 0, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return 0, err
	}

	return reqID, nil
}

// readPacket reads a single RCON packet and returns requestID, type, and payload.
func (c *Client) readPacket() (int32, int32, string, error) {
	if c.timeout > 0 {
		c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	}

	// Read the 4-byte length prefix.
	var length int32
	if err := binary.Read(c.conn, binary.LittleEndian, &length); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return 0, 0, "", fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		if errors.Is(err, io.EOF) {
			return 0, 0, "", fmt.Errorf("%w: connection closed", ErrConnection)
		}
		return 0, 0, "", err
	}

	if length < 10 {
		return 0, 0, "", fmt.Errorf("%w: packet too short (length=%d)", ErrConnection, length)
	}

	// Read the rest of the packet.
	data := make([]byte, length)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return 0, 0, "", fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, 0, "", fmt.Errorf("%w: connection closed mid-packet", ErrConnection)
		}
		return 0, 0, "", err
	}

	r := bytes.NewReader(data)

	var reqID int32
	if err := binary.Read(r, binary.LittleEndian, &reqID); err != nil {
		return 0, 0, "", fmt.Errorf("failed to read request ID: %w", err)
	}

	var packetType int32
	if err := binary.Read(r, binary.LittleEndian, &packetType); err != nil {
		return 0, 0, "", fmt.Errorf("failed to read packet type: %w", err)
	}

	// Payload is the remaining bytes minus the 2 trailing null bytes.
	payloadLen := int(length) - 4 - 4 - 2
	if payloadLen < 0 {
		payloadLen = 0
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, 0, "", fmt.Errorf("failed to read payload: %w", err)
	}

	return reqID, packetType, string(payload), nil
}

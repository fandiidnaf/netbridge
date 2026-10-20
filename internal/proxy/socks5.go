package proxy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	socksVersion5       = 0x05
	socksMethodNoAuth   = 0x00
	socksCmdConnect     = 0x01
	socksAtypIPv4       = 0x01
	socksAtypDomainName = 0x03
	socksAtypIPv6       = 0x04
	socksReplySuccess   = 0x00
	socksReplyFailed    = 0x01
)

// SOCKSServer implements an RFC 1928 compliant SOCKS5 proxy server.
type SOCKSServer struct {
	addr     string
	listener net.Listener
	metrics  *Metrics
	mu       sync.Mutex
	closed   bool
}

// NewSOCKSServer instantiates a SOCKS5 proxy server on the given address.
func NewSOCKSServer(host string, port int, metrics *Metrics) *SOCKSServer {
	if host == "" {
		host = "0.0.0.0"
	}
	if metrics == nil {
		metrics = NewMetrics()
	}
	return &SOCKSServer{
		addr:    fmt.Sprintf("%s:%d", host, port),
		metrics: metrics,
	}
}

// Start begins accepting incoming SOCKS5 connections.
func (s *SOCKSServer) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	for {
		conn, err := listener.Accept()
		if err != nil {
			s.mu.Lock()
			isClosed := s.closed
			s.mu.Unlock()
			if isClosed {
				return nil
			}
			return err
		}

		go s.handleConnection(conn)
	}
}

// Close gracefully terminates the SOCKS5 server listener.
func (s *SOCKSServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// Addr returns the resolved network address of the listener.
func (s *SOCKSServer) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

func (s *SOCKSServer) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	clientIP, _, _ := net.SplitHostPort(clientConn.RemoteAddr().String())
	s.metrics.RecordConnection(clientIP)
	defer s.metrics.ReleaseConnection()

	// 1. Negotiation handshake
	if err := s.handshake(clientConn); err != nil {
		return
	}

	// 2. Process request command
	targetAddr, err := s.readRequest(clientConn)
	if err != nil {
		_ = sendSocksReply(clientConn, socksReplyFailed, nil)
		return
	}

	// 3. Dial upstream target
	targetConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		_ = sendSocksReply(clientConn, socksReplyFailed, nil)
		return
	}
	defer targetConn.Close()

	localAddr := targetConn.LocalAddr().(*net.TCPAddr)
	if err := sendSocksReply(clientConn, socksReplySuccess, localAddr); err != nil {
		return
	}

	// 4. Bi-directional data transfer
	s.tunnel(clientConn, targetConn)
}

func (s *SOCKSServer) handshake(conn net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}

	if header[0] != socksVersion5 {
		return fmt.Errorf("unsupported socks version: %d", header[0])
	}

	numMethods := int(header[1])
	methods := make([]byte, numMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}

	supportsNoAuth := false
	for _, m := range methods {
		if m == socksMethodNoAuth {
			supportsNoAuth = true
			break
		}
	}

	if !supportsNoAuth {
		_, _ = conn.Write([]byte{socksVersion5, 0xFF})
		return errors.New("no acceptable authentication methods")
	}

	_, err := conn.Write([]byte{socksVersion5, socksMethodNoAuth})
	return err
}

func (s *SOCKSServer) readRequest(conn net.Conn) (string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}

	if header[0] != socksVersion5 {
		return "", fmt.Errorf("invalid socks version: %d", header[0])
	}
	if header[1] != socksCmdConnect {
		return "", fmt.Errorf("unsupported socks command: %d", header[1])
	}

	var host string
	switch header[3] {
	case socksAtypIPv4:
		ipBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, ipBuf); err != nil {
			return "", err
		}
		host = net.IP(ipBuf).String()
	case socksAtypDomainName:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", err
		}
		domainBuf := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(conn, domainBuf); err != nil {
			return "", err
		}
		host = string(domainBuf)
	case socksAtypIPv6:
		ipBuf := make([]byte, 16)
		if _, err := io.ReadFull(conn, ipBuf); err != nil {
			return "", err
		}
		host = net.IP(ipBuf).String()
	default:
		return "", fmt.Errorf("unsupported address type: %d", header[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBuf)

	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

func sendSocksReply(conn net.Conn, replyCode byte, localAddr *net.TCPAddr) error {
	reply := []byte{socksVersion5, replyCode, 0x00, socksAtypIPv4, 0, 0, 0, 0, 0, 0}
	if localAddr != nil {
		ip4 := localAddr.IP.To4()
		if ip4 != nil {
			copy(reply[4:8], ip4)
			binary.BigEndian.PutUint16(reply[8:10], uint16(localAddr.Port))
		}
	}
	_, err := conn.Write(reply)
	return err
}

func (s *SOCKSServer) tunnel(c1, c2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	pipe := func(dst, src net.Conn, isUpload bool) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		n, _ := io.CopyBuffer(dst, src, buf)
		if isUpload {
			s.metrics.AddBytes(uint64(n), 0)
		} else {
			s.metrics.AddBytes(0, uint64(n))
		}
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}

	go pipe(c2, c1, true)
	go pipe(c1, c2, false)

	wg.Wait()
}

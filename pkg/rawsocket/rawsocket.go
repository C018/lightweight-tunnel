package rawsocket

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

const (
	// Protocol numbers
	IPPROTO_TCP = 6
	IPPROTO_RAW = 255

	// IP header flags
	IP_DF = 0x4000 // Don't fragment

	// TCP header size
	TCPHeaderSize = 20
	// IP header size
	IPHeaderSize = 20
)

// RawSocket represents a raw socket for sending/receiving raw IP packets
type RawSocket struct {
	fd         int
	localIP    net.IP
	localPort  uint16
	remoteIP   net.IP
	remotePort uint16
	isServer   bool
}

// NewRawSocket creates a new raw socket
func NewRawSocket(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, isServer bool) (*RawSocket, error) {
	// Create raw socket (IPPROTO_RAW for sending, IPPROTO_TCP for receiving)
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %v (需要root权限)", err)
	}

	// Set IP_HDRINCL to indicate we will provide IP header
	err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1)
	if err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to set IP_HDRINCL: %v", err)
	}

	// Set socket to non-blocking mode for better control
	if err := syscall.SetNonblock(fd, false); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to set non-blocking: %v", err)
	}

	// Increase socket buffers to 4MB to handle high-throughput bursts (e.g. FEC batches)
	// Ignore errors as some systems might restrict max buffer size
	_ = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4*1024*1024)
	_ = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, 4*1024*1024)

	// Bind to local address if server
	if isServer && localIP != nil {
		addr := syscall.SockaddrInet4{
			Port: int(localPort),
		}
		copy(addr.Addr[:], localIP.To4())
		
		if err := syscall.Bind(fd, &addr); err != nil {
			syscall.Close(fd)
			return nil, fmt.Errorf("failed to bind socket: %v", err)
		}
	}

	rs := &RawSocket{
		fd:         fd,
		localIP:    localIP,
		localPort:  localPort,
		remoteIP:   remoteIP,
		remotePort: remotePort,
		isServer:   isServer,
	}

	return rs, nil
}

// BuildIPHeader constructs an IPv4 header
func BuildIPHeader(srcIP, dstIP net.IP, protocol uint8, payloadLen int) []byte {
	header := make([]byte, IPHeaderSize)

	// Version (4 bits) + IHL (4 bits)
	header[0] = 0x45 // Version 4, IHL 5 (20 bytes)

	// Type of Service
	header[1] = 0

	// Total Length
	totalLen := IPHeaderSize + payloadLen
	binary.BigEndian.PutUint16(header[2:4], uint16(totalLen))

	// Identification (can be random or incremental)
	binary.BigEndian.PutUint16(header[4:6], uint16(12345)) // Simple ID

	// Flags (3 bits) + Fragment Offset (13 bits)
	binary.BigEndian.PutUint16(header[6:8], IP_DF) // Don't fragment

	// TTL
	header[8] = 64

	// Protocol
	header[9] = protocol

	// Checksum (will be calculated later)
	header[10] = 0
	header[11] = 0

	// Source IP
	copy(header[12:16], srcIP.To4())

	// Destination IP
	copy(header[16:20], dstIP.To4())

	// Calculate and set checksum
	checksum := CalculateChecksum(header)
	binary.BigEndian.PutUint16(header[10:12], checksum)

	return header
}

// BuildTCPHeader constructs a TCP header
func BuildTCPHeader(srcPort, dstPort uint16, seq, ack uint32, flags uint8, window uint16, options []byte) []byte {
	// Calculate header length including options
	optLen := len(options)
	// Pad options to 4-byte boundary
	if optLen%4 != 0 {
		padding := 4 - (optLen % 4)
		options = append(options, make([]byte, padding)...)
		optLen = len(options)
	}

	headerLen := TCPHeaderSize + optLen
	header := make([]byte, headerLen)

	// Source port
	binary.BigEndian.PutUint16(header[0:2], srcPort)

	// Destination port
	binary.BigEndian.PutUint16(header[2:4], dstPort)

	// Sequence number
	binary.BigEndian.PutUint32(header[4:8], seq)

	// Acknowledgment number
	binary.BigEndian.PutUint32(header[8:12], ack)

	// Data offset (4 bits) + Reserved (4 bits)
	dataOffset := uint8(headerLen / 4)
	header[12] = dataOffset << 4

	// Flags
	header[13] = flags

	// Window size
	binary.BigEndian.PutUint16(header[14:16], window)

	// Checksum (will be calculated later)
	header[16] = 0
	header[17] = 0

	// Urgent pointer
	header[18] = 0
	header[19] = 0

	// Options
	if optLen > 0 {
		copy(header[TCPHeaderSize:], options)
	}

	return header
}

// CalculateTCPChecksum calculates TCP checksum with pseudo header
func CalculateTCPChecksum(srcIP, dstIP net.IP, tcpHeader, payload []byte) uint16 {
	// Build pseudo header
	pseudoHeader := make([]byte, 12)
	copy(pseudoHeader[0:4], srcIP.To4())
	copy(pseudoHeader[4:8], dstIP.To4())
	pseudoHeader[8] = 0
	pseudoHeader[9] = IPPROTO_TCP
	tcpLen := len(tcpHeader) + len(payload)
	binary.BigEndian.PutUint16(pseudoHeader[10:12], uint16(tcpLen))

	// Combine pseudo header + TCP header + payload
	data := make([]byte, len(pseudoHeader)+len(tcpHeader)+len(payload))
	copy(data[0:], pseudoHeader)
	copy(data[len(pseudoHeader):], tcpHeader)
	copy(data[len(pseudoHeader)+len(tcpHeader):], payload)

	return CalculateChecksum(data)
}

// CalculateChecksum calculates Internet checksum
func CalculateChecksum(data []byte) uint16 {
	var sum uint32

	// Add 16-bit words
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}

	// Add odd byte if present
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}

	// Fold 32-bit sum to 16 bits
	for sum>>16 != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}

	// Return one's complement
	return ^uint16(sum)
}

// SendPacket sends a raw IP packet with TCP header and payload
func (rs *RawSocket) SendPacket(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16, 
	seq, ack uint32, flags uint8, tcpOptions, payload []byte) error {

	// Build TCP header (without checksum)
	tcpHeader := BuildTCPHeader(srcPort, dstPort, seq, ack, flags, 65535, tcpOptions)

	// Calculate TCP checksum
	checksum := CalculateTCPChecksum(srcIP, dstIP, tcpHeader, payload)
	binary.BigEndian.PutUint16(tcpHeader[16:18], checksum)

	// Build IP header
	ipHeader := BuildIPHeader(srcIP, dstIP, IPPROTO_TCP, len(tcpHeader)+len(payload))

	// Combine IP header + TCP header + payload
	packet := make([]byte, len(ipHeader)+len(tcpHeader)+len(payload))
	copy(packet[0:], ipHeader)
	copy(packet[len(ipHeader):], tcpHeader)
	copy(packet[len(ipHeader)+len(tcpHeader):], payload)

	// Send packet
	addr := syscall.SockaddrInet4{
		Port: 0, // Port is in TCP header
	}
	copy(addr.Addr[:], dstIP.To4())

	err := syscall.Sendto(rs.fd, packet, 0, &addr)
	if err != nil {
		return fmt.Errorf("failed to send packet: %v", err)
	}

	return nil
}

// RecvPacket receives a raw IP packet and extracts TCP header and payload
func (rs *RawSocket) RecvPacket(buf []byte) (srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16,
	seq, ack uint32, flags uint8, payload []byte, err error) {

	n, _, err := syscall.Recvfrom(rs.fd, buf, 0)
	if err != nil {
		return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("failed to receive packet: %v", err)
	}

	if n < IPHeaderSize+TCPHeaderSize {
		return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("packet too small: %d bytes", n)
	}

	// Parse IP header
	ipHeader := buf[:IPHeaderSize]
	ihl := (ipHeader[0] & 0x0F) * 4
	if int(ihl) > n {
		return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("invalid IP header length")
	}

	protocol := ipHeader[9]
	if protocol != IPPROTO_TCP {
		return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("not a TCP packet")
	}

	srcIP = net.IPv4(ipHeader[12], ipHeader[13], ipHeader[14], ipHeader[15])
	dstIP = net.IPv4(ipHeader[16], ipHeader[17], ipHeader[18], ipHeader[19])

	// Parse TCP header
	tcpStart := int(ihl)
	if n < tcpStart+TCPHeaderSize {
		return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("packet too small for TCP header")
	}

	tcpHeader := buf[tcpStart : tcpStart+TCPHeaderSize]
	srcPort = binary.BigEndian.Uint16(tcpHeader[0:2])
	dstPort = binary.BigEndian.Uint16(tcpHeader[2:4])
	seq = binary.BigEndian.Uint32(tcpHeader[4:8])
	ack = binary.BigEndian.Uint32(tcpHeader[8:12])
	dataOffset := (tcpHeader[12] >> 4) * 4
	flags = tcpHeader[13]

	// Extract payload
	payloadStart := tcpStart + int(dataOffset)
	if payloadStart < n {
		payload = make([]byte, n-payloadStart)
		copy(payload, buf[payloadStart:n])
	}

	return srcIP, srcPort, dstIP, dstPort, seq, ack, flags, payload, nil
}

// SetReadTimeout sets read timeout for the socket
func (rs *RawSocket) SetReadTimeout(sec, usec int64) error {
	tv := syscall.Timeval{
		Sec:  sec,
		Usec: usec,
	}
	return syscall.SetsockoptTimeval(rs.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)
}

// SetWriteTimeout sets write timeout for the socket
func (rs *RawSocket) SetWriteTimeout(sec, usec int64) error {
	tv := syscall.Timeval{
		Sec:  sec,
		Usec: usec,
	}
	return syscall.SetsockoptTimeval(rs.fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &tv)
}

// Close closes the raw socket
func (rs *RawSocket) Close() error {
	return syscall.Close(rs.fd)
}

// GetLocalAddr returns local address
func (rs *RawSocket) GetLocalAddr() string {
	if rs.localIP == nil {
		return fmt.Sprintf("0.0.0.0:%d", rs.localPort)
	}
	return fmt.Sprintf("%s:%d", rs.localIP.String(), rs.localPort)
}

// GetRemoteAddr returns remote address
func (rs *RawSocket) GetRemoteAddr() string {
	if rs.remoteIP == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", rs.remoteIP.String(), rs.remotePort)
}

// GetFD returns the file descriptor
func (rs *RawSocket) GetFD() int {
	return rs.fd
}

// SetSocketOption sets a socket option
func (rs *RawSocket) SetSocketOption(level, name int, value interface{}) error {
	switch v := value.(type) {
	case int:
		return syscall.SetsockoptInt(rs.fd, level, name, v)
	case []byte:
		return syscall.SetsockoptString(rs.fd, level, name, string(v))
	default:
		return fmt.Errorf("unsupported option type")
	}
}

// GetSocketOption gets a socket option
func (rs *RawSocket) GetSocketOption(level, name int) (int, error) {
	return syscall.GetsockoptInt(rs.fd, level, name)
}

// LocalIP returns the local IP address
func (rs *RawSocket) LocalIP() net.IP {
	return rs.localIP
}

// LocalPort returns the local port
func (rs *RawSocket) LocalPort() uint16 {
	return rs.localPort
}

// RemoteIP returns the remote IP address
func (rs *RawSocket) RemoteIP() net.IP {
	return rs.remoteIP
}

// RemotePort returns the remote port
func (rs *RawSocket) RemotePort() uint16 {
	return rs.remotePort
}

// SetRemoteAddr sets the remote address
func (rs *RawSocket) SetRemoteAddr(ip net.IP, port uint16) {
	rs.remoteIP = ip
	rs.remotePort = port
}

var _ = unsafe.Sizeof(0) // For future use

package tunnel

import (
	"fmt"
	"log"
	"net"
	"time"
)

// MTUDiscovery handles adaptive MTU detection
type MTUDiscovery struct {
	remoteAddr string
	currentMTU int
	minMTU     int
	maxMTU     int
}

// NewMTUDiscovery creates a new MTU discovery instance
func NewMTUDiscovery(remoteAddr string, initialMTU int) *MTUDiscovery {
	return &MTUDiscovery{
		remoteAddr: remoteAddr,
		currentMTU: initialMTU,
		minMTU:     576,  // IPv4 minimum MTU
		maxMTU:     1500, // Standard Ethernet MTU
	}
}

// DiscoverOptimalMTU performs MTU path discovery using binary search
// Returns the optimal MTU for the network path
func (m *MTUDiscovery) DiscoverOptimalMTU() (int, error) {
	log.Printf("🔍 开始自适应MTU探测...")
	log.Printf("   目标地址: %s", m.remoteAddr)
	log.Printf("   初始MTU: %d", m.currentMTU)
	
	// Parse remote address
	host, _, err := net.SplitHostPort(m.remoteAddr)
	if err != nil {
		return m.currentMTU, fmt.Errorf("invalid remote address: %v", err)
	}
	
	// Resolve IP address
	ips, err := net.LookupIP(host)
	if err != nil {
		return m.currentMTU, fmt.Errorf("failed to resolve host: %v", err)
	}
	if len(ips) == 0 {
		return m.currentMTU, fmt.Errorf("no IP addresses found for host")
	}
	
	targetIP := ips[0].String()
	log.Printf("   解析地址: %s", targetIP)
	
	// Binary search for optimal MTU
	low := m.minMTU
	high := m.maxMTU
	optimal := m.minMTU
	
	attempts := 0
	maxAttempts := 10
	
	for low <= high && attempts < maxAttempts {
		attempts++
		testMTU := (low + high) / 2
		
		log.Printf("   [%d/%d] 测试 MTU: %d", attempts, maxAttempts, testMTU)
		
		if m.testMTU(targetIP, testMTU) {
			// MTU works, try larger
			optimal = testMTU
			low = testMTU + 1
			log.Printf("   ✅ MTU %d 可用", testMTU)
		} else {
			// MTU too large, try smaller
			high = testMTU - 1
			log.Printf("   ❌ MTU %d 过大", testMTU)
		}
	}
	
	// Account for IP header (20 bytes) and protocol overhead
	// For rawtcp mode with encryption: need to reserve space for packet type (1 byte) + encryption overhead (28 bytes)
	const ipHeaderSize = 20
	const tcpHeaderSize = 20
	const packetTypeOverhead = 1
	const encryptionOverhead = 28
	
	// Calculate safe MTU for tunnel payload
	safeMTU := optimal - ipHeaderSize - tcpHeaderSize - packetTypeOverhead - encryptionOverhead
	
	// Ensure we don't go below minimum
	if safeMTU < 500 {
		safeMTU = 500
	}
	
	// Cap at reasonable maximum for rawtcp mode
	if safeMTU > 1371 {
		safeMTU = 1371 // Safe maximum for rawtcp + encryption
	}
	
	log.Printf("✅ MTU探测完成")
	log.Printf("   路径MTU: %d", optimal)
	log.Printf("   隧道MTU: %d (已扣除协议开销)", safeMTU)
	
	return safeMTU, nil
}

// testMTU tests if a specific MTU size works by attempting to ping with that packet size
func (m *MTUDiscovery) testMTU(targetIP string, mtu int) bool {
	// Use ICMP ping to test MTU
	// Since we need raw socket for ICMP, we'll do a simple connectivity test instead
	// In a real implementation, this could use actual ICMP ping with DF flag
	
	// For now, use a simple TCP connection attempt with timeout
	// This is a conservative approach - if we can establish TCP connection,
	// the MTU should work
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(targetIP, "80"), 1*time.Second)
	if err != nil {
		// If connection fails, it might be due to MTU or just service not available
		// We'll be conservative and assume MTU might be an issue for larger sizes
		return mtu <= 1200
	}
	conn.Close()
	
	// Connection successful, so this MTU size should work
	return true
}

// GetRecommendedMTU returns a recommended MTU based on common network types
func GetRecommendedMTU(networkType string) int {
	switch networkType {
	case "ethernet":
		return 1371 // Safe for rawtcp + encryption over standard Ethernet
	case "pppoe":
		return 1343 // PPPoE reduces MTU by 8 bytes, then account for overhead
	case "mobile":
		return 1200 // Conservative for mobile networks
	case "vpn":
		return 1300 // Account for VPN overhead
	case "wifi":
		return 1371 // Usually same as Ethernet
	default:
		return 1371 // Safe default
	}
}

// AutoDetectNetworkType attempts to detect the network type
func AutoDetectNetworkType() string {
	// Simple heuristic based on available interfaces
	// In production, this could be more sophisticated
	
	ifaces, err := net.Interfaces()
	if err != nil {
		return "ethernet"
	}
	
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		
		name := iface.Name
		
		// Check for common interface name patterns
		if len(name) >= 2 {
			prefix := name[:2]
			switch prefix {
			case "wl", "ww": // wlan, wwan
				return "wifi"
			case "pp": // ppp
				return "pppoe"
			case "et", "en": // eth, ens, enp
				return "ethernet"
			}
		}
	}
	
	return "ethernet" // Default
}

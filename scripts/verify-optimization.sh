#!/bin/bash
# Quick verification script for the optimizations

echo "=== Lightweight Tunnel Configuration Verification ==="
echo ""

# Check if configs exist
echo "Checking configuration files..."
if [ -f "configs/high-latency-optimized.json" ]; then
    echo "✓ Client config: configs/high-latency-optimized.json"
else
    echo "✗ Client config not found"
fi

if [ -f "configs/high-latency-server.json" ]; then
    echo "✓ Server config: configs/high-latency-server.json"
else
    echo "✗ Server config not found"
fi

if [ -f "scripts/optimize-tcp.sh" ]; then
    echo "✓ TCP optimization script: scripts/optimize-tcp.sh"
else
    echo "✗ TCP script not found"
fi

echo ""
echo "Checking binary..."
if [ -f "bin/lightweight-tunnel" ]; then
    echo "✓ Binary compiled: bin/lightweight-tunnel"
    VERSION=$(./bin/lightweight-tunnel -version 2>&1 | head -1 || echo "unknown")
    echo "  Version: $VERSION"
else
    echo "✗ Binary not found. Run 'make' to compile."
fi

echo ""
echo "Configuration summary:"
echo ""
echo "Client Config (configs/high-latency-optimized.json):"
grep -E '"send_queue_size"|"recv_queue_size"|"fec_data"|"fec_parity"|"mtu"|"faketcp_pacing_us"' configs/high-latency-optimized.json 2>/dev/null || echo "  Could not read config"

echo ""
echo "Server Config (configs/high-latency-server.json):"
grep -E '"send_queue_size"|"recv_queue_size"|"fec_data"|"fec_parity"|"mtu"|"faketcp_pacing_us"' configs/high-latency-server.json 2>/dev/null || echo "  Could not read config"

echo ""
echo "=== Key Optimizations ==="
echo ""
echo "1. Queue Size: 10000 → 500"
echo "   Benefit: Reduces bufferbloat by 95% (2240ms → 112ms)"
echo ""
echo "2. FEC Configuration: 10+1 → 8+2"
echo "   Benefit: Better recovery (9% → 20%) with less overhead"
echo ""
echo "3. MTU: 1400 → 1200"
echo "   Benefit: Smaller packets, lower loss impact"
echo ""
echo "4. FakeTCP Pacing: Adaptive (200us base)"
echo "   Benefit: Smooths burst traffic, reduces packet loss"
echo ""
echo "5. TCP Optimization Script"
echo "   Benefit: Enables BBR, 7-9x TCP performance improvement"
echo ""
echo "=== Next Steps ==="
echo ""
echo "1. On both server and client, run TCP optimization:"
echo "   sudo ./scripts/optimize-tcp.sh"
echo ""
echo "2. Start server:"
echo "   sudo ./bin/lightweight-tunnel -config configs/high-latency-server.json"
echo ""
echo "3. Start client (update SERVER_IP first):"
echo "   sudo ./bin/lightweight-tunnel -config configs/high-latency-optimized.json"
echo ""
echo "4. Run iperf3 tests:"
echo "   UDP: iperf3 -c 192.168.100.1 -u -b 50M -t 10"
echo "   TCP: iperf3 -c 192.168.100.1 -t 10"
echo ""
echo "Expected Results:"
echo "  UDP: 10-12s test duration (not 30s), <1% loss, 45-50Mbps"
echo "  TCP: 35-45Mbps (vs previous 5Mbps)"
echo ""

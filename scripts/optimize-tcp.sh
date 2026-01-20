#!/bin/bash
# TCP Performance Optimization Script for High-Latency Networks
# This script optimizes Linux TCP settings for better performance over high-latency (170ms+) networks

set -e

echo "=== TCP Performance Optimization for High-Latency Networks ==="
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo "Please run as root (sudo)"
    exit 1
fi

# Save current settings
echo "Backing up current sysctl settings to /tmp/sysctl_backup_$(date +%Y%m%d_%H%M%S).txt"
sysctl -a | grep -E 'net.core|net.ipv4' > "/tmp/sysctl_backup_$(date +%Y%m%d_%H%M%S).txt" 2>/dev/null || true

echo ""
echo "Applying optimizations..."

# 1. Enable BBR congestion control (best for high-latency networks)
echo "1. Enabling BBR congestion control..."
modprobe tcp_bbr 2>/dev/null || echo "   Warning: BBR module not available, trying to continue..."
sysctl -w net.core.default_qdisc=fq
sysctl -w net.ipv4.tcp_congestion_control=bbr
echo "   ✓ BBR enabled (best for high-latency + lossy networks)"

# 2. Increase TCP buffer sizes for high BDP (Bandwidth-Delay Product)
# BDP = 50Mbps * 170ms = 1.06MB, so we need larger buffers
echo ""
echo "2. Optimizing TCP buffer sizes for high latency..."
# Default, Min, Max (in bytes)
sysctl -w net.ipv4.tcp_rmem="4096 131072 6291456"  # 4KB default, 128KB min, 6MB max
sysctl -w net.ipv4.tcp_wmem="4096 65536 4194304"   # 4KB default, 64KB min, 4MB max
sysctl -w net.core.rmem_max=6291456
sysctl -w net.core.wmem_max=4194304
sysctl -w net.core.rmem_default=131072
sysctl -w net.core.wmem_default=65536
echo "   ✓ Buffer sizes increased for high BDP"

# 3. Enable TCP window scaling (critical for high-latency)
echo ""
echo "3. Enabling TCP window scaling..."
sysctl -w net.ipv4.tcp_window_scaling=1
echo "   ✓ Window scaling enabled"

# 4. Optimize for high latency
echo ""
echo "4. Tuning TCP parameters for high latency..."
sysctl -w net.ipv4.tcp_slow_start_after_idle=0  # Don't reduce cwnd after idle
sysctl -w net.ipv4.tcp_no_metrics_save=1        # Don't cache metrics (better for variable paths)
sysctl -w net.ipv4.tcp_mtu_probing=1            # Enable Path MTU Discovery
echo "   ✓ High-latency tuning applied"

# 5. Reduce bufferbloat
echo ""
echo "5. Reducing bufferbloat..."
sysctl -w net.core.netdev_max_backlog=2500
sysctl -w net.ipv4.tcp_limit_output_bytes=262144
echo "   ✓ Queue sizes optimized"

# 6. Enable TCP Fast Open (reduce handshake overhead)
echo ""
echo "6. Enabling TCP Fast Open..."
sysctl -w net.ipv4.tcp_fastopen=3  # 1=client, 2=server, 3=both
echo "   ✓ TCP Fast Open enabled"

# 7. Optimize retransmission settings for lossy networks
echo ""
echo "7. Optimizing for packet loss (1% expected)..."
sysctl -w net.ipv4.tcp_retries2=8              # Increase retries (default 15 is too high)
sysctl -w net.ipv4.tcp_reordering=3            # Assume some reordering
sysctl -w net.ipv4.tcp_early_retrans=1         # Enable early retransmit
echo "   ✓ Retransmission tuning applied"

# 8. Disable TCP timestamps if causing issues (optional)
# Uncomment if you see timestamp-related problems
# sysctl -w net.ipv4.tcp_timestamps=0

# 9. Set reasonable keepalive values
echo ""
echo "8. Setting keepalive parameters..."
sysctl -w net.ipv4.tcp_keepalive_time=60       # Start after 60s idle
sysctl -w net.ipv4.tcp_keepalive_intvl=10      # Probe every 10s
sysctl -w net.ipv4.tcp_keepalive_probes=6      # 6 probes before timeout
echo "   ✓ Keepalive configured"

echo ""
echo "=== Optimization Complete ==="
echo ""
echo "Current TCP settings:"
echo "  Congestion Control: $(sysctl -n net.ipv4.tcp_congestion_control)"
echo "  Queue Discipline:   $(sysctl -n net.core.default_qdisc)"
echo "  TCP Window Scaling: $(sysctl -n net.ipv4.tcp_window_scaling)"
echo "  TCP Fast Open:      $(sysctl -n net.ipv4.tcp_fastopen)"
echo "  TCP RMem (max):     $(sysctl -n net.core.rmem_max) bytes"
echo "  TCP WMem (max):     $(sysctl -n net.core.wmem_max) bytes"
echo ""
echo "To make these changes permanent, add them to /etc/sysctl.conf or /etc/sysctl.d/99-tcp-tuning.conf"
echo ""
echo "Test with: iperf3 -c <server> -t 10 -i 1"
echo ""

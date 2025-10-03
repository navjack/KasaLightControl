#!/bin/bash

echo "🚀 KasaLightControl Performance Comparison"
echo "=========================================="
echo ""

echo "Available versions:"
echo "  colormorph-profiling  - Subprocess version with profiling"
echo "  colormorph-direct     - Direct TCP version (NEW!)"
echo ""

if [[ ! -f "colormorph-profiling" ]] || [[ ! -f "colormorph-direct" ]]; then
    echo "❌ Missing binaries. Please run:"
    echo "   go build -o colormorph-profiling colormorph.go"
    echo "   go build -o colormorph-direct colormorph.go"
    exit 1
fi

echo "🔬 Quick Performance Test Instructions:"
echo ""
echo "1. Test SUBPROCESS version (slow):"
echo "   ./colormorph-profiling"
echo "   - Watch the PERF logs for timing"
echo "   - Look for high Frame/Network times"
echo "   - Profile available at http://localhost:6060"
echo ""

echo "2. Test DIRECT TCP version (fast!):"
echo "   ./colormorph-direct"  
echo "   - Watch the PERF logs for timing"
echo "   - Should see dramatically lower Frame/Network times"
echo "   - Profile available at http://localhost:6060"
echo ""

echo "💡 Expected Performance Improvements:"
echo "   - Frame time: 50-200ms → 5-20ms (10x faster)"
echo "   - Network time: 30-100ms → 2-10ms (5-10x faster)" 
echo "   - CPU usage: High → Very low"
echo "   - Process overhead: Eliminated completely"
echo ""

echo "📊 Key metrics to compare:"
echo "   PERF: Frame XXXms | Network XXXms | Concurrent XXXms"
echo ""

read -p "🤔 Which version would you like to start? (p=profiling/subprocess, d=direct, n=none): " -n 1 -r
echo

case $REPLY in
    [Pp]* )
        echo "🐌 Starting SUBPROCESS version (with profiling)..."
        echo "Expected: Slow performance, high CPU usage"
        echo ""
        ./colormorph-profiling
        ;;
    [Dd]* )
        echo "🚀 Starting DIRECT TCP version..."
        echo "Expected: Fast performance, low CPU usage"  
        echo ""
        ./colormorph-direct
        ;;
    * )
        echo "👋 No version started. Run manually when ready!"
        ;;
esac
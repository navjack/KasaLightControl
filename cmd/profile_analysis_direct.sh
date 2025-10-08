#!/bin/bash

echo "🚀 KasaLightControl Direct TCP Performance Analysis"
echo "================================================="
echo ""

if ! pgrep -f "colormorph-direct" > /dev/null; then
    echo "❌ colormorph-direct is not running."
    echo "Start it with: ./colormorph-direct"
    exit 1
fi

echo "✅ colormorph-direct is running (Direct TCP version)"
echo ""

echo "📊 Direct TCP Profiling - What to Look For:"
echo "  ✅ Zero subprocess overhead (os/exec should be absent)"
echo "  ✅ Network I/O dominance (kasa.SetLightState, net.Dial*)"
echo "  ✅ Low CPU usage overall"
echo "  ✅ Fast frame times in PERF logs"
echo ""

echo "🔬 Profiling Endpoints:"
echo "  CPU Profile:     http://localhost:6060/debug/pprof/profile"
echo "  Heap Profile:    http://localhost:6060/debug/pprof/heap"  
echo "  Goroutines:      http://localhost:6060/debug/pprof/goroutine"
echo "  Interactive:     http://localhost:6060/debug/pprof/"
echo ""

echo "🧪 Recommended Analysis Commands:"
echo ""

echo "1. CPU hotspots (30 seconds):"
echo "   go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30"
echo "   (Look for: kasa.SetLightState, net.Dial, NO os/exec)"
echo ""

echo "2. Function breakdown:"
echo "   go tool pprof -list main.setBulbColorDirectTCP http://localhost:6060/debug/pprof/profile?seconds=30"
echo ""

echo "3. Current memory usage:"
echo "   go tool pprof http://localhost:6060/debug/pprof/heap"
echo ""

echo "4. Goroutine patterns:"
echo "   go tool pprof http://localhost:6060/debug/pprof/goroutine"
echo ""

echo "5. Live profiling in browser:"
echo "   open http://localhost:6060/debug/pprof/"
echo ""

echo "📈 Quick Profile Capture:"
echo "   curl -o direct_tcp_cpu.prof http://localhost:6060/debug/pprof/profile?seconds=30"
echo "   curl -o direct_tcp_heap.prof http://localhost:6060/debug/pprof/heap"
echo "   curl -o direct_tcp_goroutines.prof http://localhost:6060/debug/pprof/goroutine"
echo ""

echo "📊 Expected Results (Direct TCP vs Subprocess):"
echo "   CPU Profile: kasa.SetLightState ~60% (vs os/exec.(*Cmd).Start ~50%)"
echo "   Network: net.Dial* calls (vs zero network in subprocess version)"
echo "   Memory: Lower overall usage (no process overhead)"
echo "   Goroutines: Clean concurrent pattern"
echo ""

if command -v go >/dev/null 2>&1; then
    read -p "🤔 Would you like to start an interactive CPU profile now? (y/n): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "🚀 Starting 30-second CPU profile..."
        echo "Expected: kasa.SetLightState dominance, NO subprocess overhead"
        go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=30
    fi
else
    echo "⚠️  Go not found in PATH - install Go to use pprof analysis"
fi

#!/bin/bash

echo "🔬 KasaLightControl Performance Profiling Analysis"
echo "================================================"
echo ""

if ! pgrep -f "colormorph-profiling" > /dev/null; then
    echo "❌ colormorph-profiling is not running."
    echo "Start it with: ./colormorph-profiling"
    exit 1
fi

echo "✅ colormorph-profiling is running"
echo ""

echo "📊 Available profiling endpoints:"
echo "  CPU Profile:     http://localhost:6060/debug/pprof/profile"
echo "  Heap Profile:    http://localhost:6060/debug/pprof/heap"  
echo "  Goroutines:      http://localhost:6060/debug/pprof/goroutine"
echo "  Interactive:     http://localhost:6060/debug/pprof/"
echo ""

echo "🧪 Quick profiling commands:"
echo ""

echo "1. CPU Profile (30 seconds):"
echo "   go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30"
echo ""

echo "2. Current heap usage:"
echo "   go tool pprof http://localhost:6060/debug/pprof/heap"
echo ""

echo "3. Goroutine analysis:"
echo "   go tool pprof http://localhost:6060/debug/pprof/goroutine"
echo ""

echo "4. Live profiling in browser:"
echo "   open http://localhost:6060/debug/pprof/"
echo ""

echo "📈 To capture profiles automatically:"
echo "   curl -o cpu.prof http://localhost:6060/debug/pprof/profile?seconds=30"
echo "   curl -o heap.prof http://localhost:6060/debug/pprof/heap"
echo "   curl -o goroutines.prof http://localhost:6060/debug/pprof/goroutine"
echo ""

if command -v go >/dev/null 2>&1; then
    read -p "🤔 Would you like to start an interactive CPU profile now? (y/n): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "🚀 Starting 30-second CPU profile..."
        go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=30
    fi
else
    echo "⚠️  Go not found in PATH - install Go to use pprof analysis"
fi
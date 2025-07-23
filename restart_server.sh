#!/bin/bash
echo "Stopping any existing kasaserver processes..."
pkill -f kasaserver 2>/dev/null || true
sleep 2

echo "Starting Kasa Light Control Server..."
cd "$(dirname "$0")"
go run cmd/kasaserver/main.go

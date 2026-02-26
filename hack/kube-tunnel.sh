#!/usr/bin/env bash
#
# kube-tunnel.sh - Establishes SSH tunnel to Kubernetes API server
#
# Usage:
#   ./hack/kube-tunnel.sh start [options]
#   ./hack/kube-tunnel.sh stop
#   ./hack/kube-tunnel.sh status
#
# Environment variables:
#   SSH_HOST          - SSH server hostname or IP (required)
#   SSH_USER          - SSH username (required)
#   SSH_PORT          - SSH port (default: 22)
#   SSH_KEY_FILE      - Path to SSH private key (default: ~/.ssh/id_rsa)
#   K8S_API_HOST      - Kubernetes API host from SSH server's perspective (default: 127.0.0.1)
#   K8S_API_PORT      - Kubernetes API port (default: 6443)
#   LOCAL_PORT        - Local port for tunnel (default: 16443)
#   TUNNEL_PID_FILE   - PID file location (default: /tmp/kube-tunnel.pid)
#

set -euo pipefail

# Defaults
SSH_PORT="${SSH_PORT:-22}"
SSH_KEY_FILE="${SSH_KEY_FILE:-$HOME/.ssh/id_rsa}"
K8S_API_HOST="${K8S_API_HOST:-127.0.0.1}"
K8S_API_PORT="${K8S_API_PORT:-6443}"
LOCAL_PORT="${LOCAL_PORT:-16443}"
TUNNEL_PID_FILE="${TUNNEL_PID_FILE:-/tmp/kube-tunnel.pid}"

log() {
    echo "[kube-tunnel] $*" >&2
}

error() {
    echo "[kube-tunnel] ERROR: $*" >&2
    exit 1
}

check_required_vars() {
    if [[ -z "${SSH_HOST:-}" ]]; then
        error "SSH_HOST is required"
    fi
    if [[ -z "${SSH_USER:-}" ]]; then
        error "SSH_USER is required"
    fi
}

start_tunnel() {
    check_required_vars

    # Check if tunnel is already running
    if [[ -f "$TUNNEL_PID_FILE" ]]; then
        local pid
        pid=$(cat "$TUNNEL_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            log "Tunnel already running (PID: $pid)"
            return 0
        else
            rm -f "$TUNNEL_PID_FILE"
        fi
    fi

    # Check if local port is already in use
    if nc -z 127.0.0.1 "$LOCAL_PORT" 2>/dev/null; then
        error "Local port $LOCAL_PORT is already in use"
    fi

    log "Starting SSH tunnel: localhost:$LOCAL_PORT -> $K8S_API_HOST:$K8S_API_PORT via $SSH_USER@$SSH_HOST:$SSH_PORT"

    # Build SSH options
    local ssh_opts=(
        -o "StrictHostKeyChecking=no"
        -o "UserKnownHostsFile=/dev/null"
        -o "ServerAliveInterval=30"
        -o "ServerAliveCountMax=3"
        -o "ExitOnForwardFailure=yes"
        -o "LogLevel=ERROR"
        -N
        -L "$LOCAL_PORT:$K8S_API_HOST:$K8S_API_PORT"
        -p "$SSH_PORT"
    )

    if [[ -f "$SSH_KEY_FILE" ]]; then
        ssh_opts+=(-i "$SSH_KEY_FILE")
    fi

    # Start tunnel in background
    ssh "${ssh_opts[@]}" "$SSH_USER@$SSH_HOST" &
    local pid=$!

    echo "$pid" > "$TUNNEL_PID_FILE"

    # Wait for tunnel to be ready
    local max_attempts=30
    local attempt=0
    while ! nc -z 127.0.0.1 "$LOCAL_PORT" 2>/dev/null; do
        attempt=$((attempt + 1))
        if [[ $attempt -ge $max_attempts ]]; then
            kill "$pid" 2>/dev/null || true
            rm -f "$TUNNEL_PID_FILE"
            error "Tunnel failed to start after $max_attempts attempts"
        fi
        sleep 1
    done

    log "Tunnel established successfully (PID: $pid)"
    echo "$LOCAL_PORT"
}

stop_tunnel() {
    if [[ -f "$TUNNEL_PID_FILE" ]]; then
        local pid
        pid=$(cat "$TUNNEL_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            log "Stopping tunnel (PID: $pid)"
            kill "$pid" 2>/dev/null || true
            # Wait for process to terminate
            local attempts=0
            while kill -0 "$pid" 2>/dev/null && [[ $attempts -lt 10 ]]; do
                sleep 1
                attempts=$((attempts + 1))
            done
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
            fi
        fi
        rm -f "$TUNNEL_PID_FILE"
        log "Tunnel stopped"
    else
        log "No tunnel PID file found"
    fi
}

status_tunnel() {
    if [[ -f "$TUNNEL_PID_FILE" ]]; then
        local pid
        pid=$(cat "$TUNNEL_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            log "Tunnel is running (PID: $pid)"
            if nc -z 127.0.0.1 "$LOCAL_PORT" 2>/dev/null; then
                log "Port $LOCAL_PORT is open"
                return 0
            else
                log "WARNING: Port $LOCAL_PORT is not responding"
                return 1
            fi
        else
            log "Tunnel process not running (stale PID file)"
            return 1
        fi
    else
        log "Tunnel is not running"
        return 1
    fi
}

case "${1:-}" in
    start)
        start_tunnel
        ;;
    stop)
        stop_tunnel
        ;;
    status)
        status_tunnel
        ;;
    *)
        echo "Usage: $0 {start|stop|status}"
        exit 1
        ;;
esac

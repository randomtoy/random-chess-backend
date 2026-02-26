#!/usr/bin/env bash
#
# deploy.sh - Deploy random-chess-backend to Kubernetes via SSH tunnel
#
# Usage:
#   ./hack/deploy.sh [options]
#
# Options:
#   --image-tag TAG          Image tag to deploy (required)
#   --namespace NS           Kubernetes namespace (default: random-chess)
#   --release NAME           Helm release name (default: random-chess-backend)
#   --values FILE            Additional values file
#   --kubeconfig FILE        Path to kubeconfig file
#   --postgres-password PWD  Create/update the postgres K8s secret with this password
#   --dry-run                Perform dry-run only
#   --skip-tunnel            Skip SSH tunnel setup (use existing kubeconfig)
#   --help                   Show this help
#
# Environment variables (for tunnel):
#   SSH_HOST, SSH_USER, SSH_PORT, SSH_KEY_FILE
#   K8S_API_HOST, K8S_API_PORT, LOCAL_PORT
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Defaults
NAMESPACE="${HELM_NAMESPACE:-random-chess}"
RELEASE_NAME="${HELM_RELEASE_NAME:-random-chess-backend}"
IMAGE_TAG=""
VALUES_FILE=""
KUBECONFIG_FILE="${KUBECONFIG:-}"
POSTGRES_PASSWORD=""
DRY_RUN=false
SKIP_TUNNEL=false
TUNNEL_STARTED=false
LOCAL_PORT="${LOCAL_PORT:-16443}"

log() {
    echo "[deploy] $*" >&2
}

error() {
    echo "[deploy] ERROR: $*" >&2
    exit 1
}

cleanup() {
    if [[ "$TUNNEL_STARTED" == "true" ]]; then
        log "Cleaning up SSH tunnel..."
        "$SCRIPT_DIR/kube-tunnel.sh" stop || true
    fi
}

trap cleanup EXIT

show_help() {
    head -30 "$0" | grep -E "^#" | sed 's/^# \?//'
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --image-tag)
                IMAGE_TAG="$2"
                shift 2
                ;;
            --namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            --release)
                RELEASE_NAME="$2"
                shift 2
                ;;
            --values)
                VALUES_FILE="$2"
                shift 2
                ;;
            --kubeconfig)
                KUBECONFIG_FILE="$2"
                shift 2
                ;;
            --postgres-password)
                POSTGRES_PASSWORD="$2"
                shift 2
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --skip-tunnel)
                SKIP_TUNNEL=true
                shift
                ;;
            --help)
                show_help
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                ;;
        esac
    done
}

patch_kubeconfig() {
    local kubeconfig_file="$1"
    local local_port="$2"
    local temp_file

    temp_file=$(mktemp)

    # Replace the server URL with localhost tunnel
    sed -E "s|(server: https?://)[^:]+:[0-9]+|\1127.0.0.1:$local_port|g" \
        "$kubeconfig_file" > "$temp_file"

    mv "$temp_file" "$kubeconfig_file"
    log "Patched kubeconfig to use 127.0.0.1:$local_port"
}

main() {
    parse_args "$@"

    if [[ -z "$IMAGE_TAG" ]]; then
        error "--image-tag is required"
    fi

    log "Deployment configuration:"
    log "  Release: $RELEASE_NAME"
    log "  Namespace: $NAMESPACE"
    log "  Image tag: $IMAGE_TAG"
    log "  Dry run: $DRY_RUN"

    # Setup SSH tunnel if not skipped
    if [[ "$SKIP_TUNNEL" != "true" ]]; then
        log "Starting SSH tunnel..."
        LOCAL_PORT=$("$SCRIPT_DIR/kube-tunnel.sh" start)
        TUNNEL_STARTED=true
        log "Tunnel established on port $LOCAL_PORT"

        # Patch kubeconfig if provided
        if [[ -n "$KUBECONFIG_FILE" && -f "$KUBECONFIG_FILE" ]]; then
            TEMP_KUBECONFIG=$(mktemp)
            cp "$KUBECONFIG_FILE" "$TEMP_KUBECONFIG"
            patch_kubeconfig "$TEMP_KUBECONFIG" "$LOCAL_PORT"
            export KUBECONFIG="$TEMP_KUBECONFIG"
        fi
    fi

    # Verify cluster connectivity
    log "Verifying cluster connectivity..."
    if ! kubectl cluster-info --request-timeout=10s >/dev/null 2>&1; then
        error "Cannot connect to Kubernetes cluster"
    fi
    log "Cluster connection verified"

    # Create namespace if not exists
    log "Ensuring namespace '$NAMESPACE' exists..."
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

    # Create/update postgres secret if password provided (dev environment)
    if [[ -n "$POSTGRES_PASSWORD" ]]; then
        local pg_secret="${RELEASE_NAME}-postgres-dev"
        local pg_host="${RELEASE_NAME}-postgresql"
        local db_url="postgres://random_chess:${POSTGRES_PASSWORD}@${pg_host}/random_chess?sslmode=disable"
        log "Creating/updating postgres secret '$pg_secret'..."
        kubectl create secret generic "$pg_secret" \
            --namespace "$NAMESPACE" \
            --from-literal=postgres-password="$POSTGRES_PASSWORD" \
            --from-literal=password="$POSTGRES_PASSWORD" \
            --from-literal=DATABASE_URL="$db_url" \
            --dry-run=client -o yaml | kubectl apply -f -
    fi

    # Build helm arguments
    local helm_args=(
        upgrade --install "$RELEASE_NAME"
        "$REPO_ROOT/deploy/helm/random-chess-backend"
        --namespace "$NAMESPACE"
        --set "image.tag=$IMAGE_TAG"
        --dependency-update
        --atomic
        --wait
        --timeout 5m
    )

    if [[ -n "$VALUES_FILE" && -f "$VALUES_FILE" ]]; then
        helm_args+=(--values "$VALUES_FILE")
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        helm_args+=(--dry-run)
    fi

    # Lint first
    log "Linting Helm chart..."
    helm lint "$REPO_ROOT/deploy/helm/random-chess-backend"

    # Deploy
    log "Deploying with Helm..."
    helm "${helm_args[@]}"

    if [[ "$DRY_RUN" != "true" ]]; then
        log "Deployment successful!"
        log "Verifying rollout..."
        kubectl rollout status deployment/"$RELEASE_NAME" -n "$NAMESPACE" --timeout=5m
    else
        log "Dry run completed successfully"
    fi
}

main "$@"

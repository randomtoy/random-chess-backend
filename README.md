# random-chess-backend

Backend for random chess battle. Anonymous users make exactly one legal move in an assigned ongoing game, then are switched to another game. Server is authoritative.

API contract: [`contracts/openapi.yaml`](contracts/openapi.yaml)

---

## Local Development

```bash
# Clone with submodules
git clone --recurse-submodules https://github.com/randomtoy/random-chess-backend.git

# Run tests
go test -v -race ./...

# Run linter
golangci-lint run

# Run the server (requires go.mod + source)
go run ./cmd/api
```

---

## Build / Push Image

Images are published to GHCR automatically by the [Release workflow](.github/workflows/release.yml) when a semver tag is pushed:

```bash
git tag v1.2.3
git push origin v1.2.3
```

To build locally:

```bash
docker build -t ghcr.io/randomtoy/random-chess-backend:dev .
```

---

## Deploy with Helm

**One-liner (skip tunnel if kubeconfig is already configured):**

```bash
helm upgrade --install random-chess-backend deploy/helm/random-chess-backend \
  --namespace random-chess \
  --create-namespace \
  --values deploy/helm/values/prod.yaml \
  --set image.tag=1.2.3 \
  --atomic --wait --timeout 5m
```

**Via the deploy script (sets up SSH tunnel to k3s API):**

```bash
export SSH_HOST=<server-ip>
export SSH_USER=<user>
export SSH_KEY_FILE=~/.ssh/deploy_key
export KUBECONFIG=~/.kube/random-chess-kubeconfig

./hack/deploy.sh \
  --image-tag 1.2.3 \
  --values deploy/helm/values/prod.yaml
```

**Via GitHub Actions:** trigger the [Deploy workflow](.github/workflows/deploy.yml) manually from the Actions tab, providing the image tag.

### Helm chart values

| Path | Default | Description |
|------|---------|-------------|
| `image.repository` | `ghcr.io/randomtoy/random-chess-backend` | Image repository |
| `image.tag` | chart appVersion | Image tag |
| `replicaCount` | `1` | Number of replicas |
| `ingress.enabled` | `false` | Enable Ingress |
| `ingress.host` | `""` | Ingress hostname |
| `autoscaling.enabled` | `false` | Enable HPA |
| `networkPolicy.enabled` | `false` | Enable NetworkPolicy |
| `serviceAccount.create` | `false` | Create ServiceAccount |

---

## Required GitHub Secrets

The following secrets must be configured in the repository settings for the deploy workflow:

| Secret | Used by | Description |
|--------|---------|-------------|
| `SSH_HOST` | deploy | Hostname or IP of the k3s server |
| `SSH_USER` | deploy | SSH username on the server |
| `SSH_PRIVATE_KEY` | deploy | SSH private key for server access |
| `KUBECONFIG_B64` | deploy | Base64-encoded kubeconfig for the k3s cluster |

`GITHUB_TOKEN` is provided automatically by GitHub Actions and is used to publish images to GHCR — no configuration needed.

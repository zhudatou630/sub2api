#!/usr/bin/env bash
# Build a local fork image from a calm-* tag by overlaying the fork binary on the
# matching official runtime image (works with Docker's legacy builder, no BuildKit).
# Builds and tags locally only: no push, no container start, no deployment.
#
# Usage: build-image.sh <calm-tag> <app-version> <official-image@sha256:digest>
#   e.g. build-image.sh calm-v0.2.24-identity.1 0.2.24-calm.1 weishaw/sub2api:0.2.24@sha256:...
# Requires: exact Go from backend/go.mod, Node 24 + pnpm 9 on PATH, docker.
set -euo pipefail

[ $# -eq 3 ] || { sed -n '2,9p' "$0"; exit 2; }
TAG=$1 VERSION=$2 BASE=$3
ROOT=$(cd -- "$(dirname -- "$0")/../.." && pwd)
IMAGE="calm/sub2api:${VERSION}"

die() { echo "build-image: $*" >&2; exit 1; }
[[ $TAG == calm-* ]] || die "tag must be calm-*"
[[ $BASE == *@sha256:* ]] || die "base image must be pinned by digest"
[[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+-calm\.[0-9]+$ ]] || die "version must look like 0.2.24-calm.1"
! docker image inspect "$IMAGE" >/dev/null 2>&1 || die "$IMAGE already exists; versions are immutable"

COMMIT=$(git -C "$ROOT" rev-parse --verify "refs/tags/${TAG}^{commit}") || die "unknown tag $TAG"
UPSTREAM=$(git -C "$ROOT" rev-parse --verify "$(git -C "$ROOT" describe --tags --abbrev=0 --match 'v*' "$COMMIT")^{commit}")
GO_WANT=$(git -C "$ROOT" show "$COMMIT:backend/go.mod" | awk '$1=="go"{print $2}')
[ "$(go env GOVERSION)" = "go${GO_WANT}" ] || die "need go${GO_WANT}, have $(go env GOVERSION)"
[[ $(node -v) == v24.* ]] || die "need Node 24, have $(node -v)"
[[ $(pnpm -v) == 9.* ]] || die "need pnpm 9, have $(pnpm -v)"

WORK=$(mktemp -d /tmp/calm-build.XXXXXX)
trap 'rm -rf "$WORK"' EXIT
git -C "$ROOT" archive "$COMMIT" | tar -x -C "$WORK"

(cd "$WORK/frontend" && pnpm install --frozen-lockfile --ignore-scripts && pnpm run build)
DATE=$(git -C "$ROOT" show -s --format=%cI "$COMMIT")
(cd "$WORK/backend" && CGO_ENABLED=0 go build -mod=readonly -trimpath -tags embed \
  -ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${DATE} -X main.BuildType=release" \
  -o "$WORK/image/sub2api" ./cmd/server)
SHA=$(sha256sum "$WORK/image/sub2api" | cut -d' ' -f1)

docker pull "$BASE"
cat > "$WORK/image/Dockerfile" <<EOF
FROM ${BASE}
LABEL com.calmabacus.sub2api.version=${VERSION} \\
      com.calmabacus.sub2api.tag=${TAG} \\
      com.calmabacus.sub2api.fork-commit=${COMMIT} \\
      com.calmabacus.sub2api.upstream=${UPSTREAM} \\
      com.calmabacus.sub2api.base=${BASE} \\
      com.calmabacus.sub2api.app-sha256=${SHA}
COPY --chown=1000:1000 sub2api /app/sub2api
EOF
DOCKER_BUILDKIT=0 docker build -q -t "$IMAGE" "$WORK/image"

# Verify the shipped binary without starting the application.
GOT=$(docker run --rm --network none --entrypoint sha256sum "$IMAGE" /app/sub2api | cut -d' ' -f1)
[ "$GOT" = "$SHA" ] || die "binary hash mismatch in image"
echo "image=${IMAGE} id=$(docker image inspect -f '{{.Id}}' "$IMAGE") commit=${COMMIT} app-sha256=${SHA}"

#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./scripts/build-all.sh [options] <main-package> [<main-package> ...]

Build one or more Go main packages for a standard platform matrix, write a
canonical checksums.txt file, and optionally sign it.

This script is shipped in gokit as a reusable helper for downstream application
repos. It does not assume the current repository contains a ./cmd tree; pass the
main package paths you want to build explicitly.

Options:
  -o, --outdir <dir>      Output directory for built artifacts (default: ./bin)
  -t, --tag <tag>         Release tag written into checksums.txt (default: local)
  -s, --seed-file <path>  Ed25519 seed file used to sign checksums.txt
                          (default: ./ed25519_seed.bin if present)
  -h, --help              Show this help text

Examples:
  ./scripts/build-all.sh ./cmd/myapp
  ./scripts/build-all.sh -o ./dist -t v1.2.3 ./cmd/myapp ./cmd/worker
EOF
}

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUTDIR="$ROOT/bin"
TAG="local"
SEED_FILE="$ROOT/ed25519_seed.bin"

PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

PACKAGES=()

while [ "$#" -gt 0 ]; do
  case "$1" in
    -o|--outdir)
      if [ "$#" -lt 2 ]; then
        echo "missing value for $1" >&2
        usage
        exit 2
      fi
      OUTDIR="$2"
      shift 2
      ;;
    -t|--tag)
      if [ "$#" -lt 2 ]; then
        echo "missing value for $1" >&2
        usage
        exit 2
      fi
      TAG="$2"
      shift 2
      ;;
    -s|--seed-file)
      if [ "$#" -lt 2 ]; then
        echo "missing value for $1" >&2
        usage
        exit 2
      fi
      SEED_FILE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      while [ "$#" -gt 0 ]; do
        PACKAGES+=("$1")
        shift
      done
      ;;
    -*)
      echo "unknown option: $1" >&2
      usage
      exit 2
      ;;
    *)
      PACKAGES+=("$1")
      shift
      ;;
  esac
done

if [ ${#PACKAGES[@]} -eq 0 ]; then
  echo "no main packages provided" >&2
  usage
  exit 1
fi

mkdir -p "$OUTDIR"

echo "Building ${#PACKAGES[@]} package(s): ${PACKAGES[*]}"
echo "Platforms: ${PLATFORMS[*]}"
echo "Output directory: $OUTDIR"

for plat in "${PLATFORMS[@]}"; do
  GOOS=${plat%/*}
  GOARCH=${plat#*/}

  for pkg in "${PACKAGES[@]}"; do
    name=$(basename "$pkg")
    outfile="$OUTDIR/${name}-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
      outfile="${outfile}.exe"
    fi

    echo "-> Building $pkg for $GOOS/$GOARCH -> $outfile"
    env CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags "-s -w" -o "$outfile" "$pkg"
  done
done

CHECKS="$OUTDIR/checksums.txt"
printf "# release: %s\n" "$TAG" > "$CHECKS"

while IFS= read -r file; do
  base=$(basename "$file")
  case "$base" in
    checksums.txt|checksums.txt.sig)
      continue
      ;;
  esac

  if command -v sha256sum >/dev/null 2>&1; then
    hash=$(sha256sum "$file" | awk '{print $1}')
  else
    hash=$(shasum -a 256 "$file" | awk '{print $1}')
  fi
  printf "%s  %s\n" "$hash" "$base" >> "$CHECKS"
done < <(find "$OUTDIR" -maxdepth 1 -type f | sort)

if [ -f "$SEED_FILE" ]; then
  echo "Signing checksums.txt with seed file $SEED_FILE"
  go run "$ROOT/scripts/sign_checksums/sign_checksums.go" "$CHECKS" "$SEED_FILE"
else
  echo "No seed file at $SEED_FILE; skipping signing of checksums.txt"
fi

echo "Builds complete. Artifacts available under: $OUTDIR"

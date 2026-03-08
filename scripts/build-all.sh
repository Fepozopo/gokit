#!/usr/bin/env bash
set -euo pipefail

TAG=v0.6.0

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN_DIR="$ROOT/bin"
mkdir -p "$BIN_DIR"

# Platforms to build for
PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

# Collect commands found under cmd/
CMD_DIRS=()
for d in "$ROOT"/cmd/*; do
  [ -d "$d" ] || continue
  CMD_DIRS+=("$(basename "$d")")
done

if [ ${#CMD_DIRS[@]} -eq 0 ]; then
  echo "No commands found under cmd/ to build. Exiting."
  exit 1
fi

echo "Building ${#CMD_DIRS[@]} command(s): ${CMD_DIRS[*]}"

echo "Platforms: ${PLATFORMS[*]}"

for plat in "${PLATFORMS[@]}"; do
  GOOS=${plat%/*}
  GOARCH=${plat#*/}

    for cmd in "${CMD_DIRS[@]}"; do
    # Place binary directly in $BIN_DIR with filename: <cmd>-<os>-<arch>
    outfile="$BIN_DIR/${cmd}-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then outfile="${outfile}.exe"; fi

    echo "-> Building $cmd for $GOOS/$GOARCH -> $outfile"
    # Disable CGO for maximum portability, strip debug info
    env CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags "-s -w" -o "$outfile" "./cmd/$cmd"
  done
done

# After building, produce canonical checksums.txt and sign it if a private
# seed file exists at $ROOT/ed25519_seed.bin. This keeps signing logic
# colocated with builds for simple CI setups.
OUTDIR="$BIN_DIR"
CHECKS="$OUTDIR/checksums.txt"
echo "# release: ${TAG:-local}" > "$CHECKS"
for f in $(ls -1 "$OUTDIR" | sort); do
  [[ "$f" == "checksums.txt" || "$f" == "checksums.txt.sig" ]] && continue
  if command -v sha256sum >/dev/null 2>&1; then
    h=$(sha256sum "$OUTDIR/$f" | awk '{print $1}')
  else
    h=$(shasum -a 256 "$OUTDIR/$f" | awk '{print $1}')
  fi
  printf "%s  %s\n" "$h" "$f" >> "$CHECKS"
done

SEED_FILE="$ROOT/ed25519_seed.bin"
if [ -f "$SEED_FILE" ]; then
  echo "Signing checksums.txt with seed file $SEED_FILE"
  # sign_checksums.go expects a seed file path as the second argument
  go run "$ROOT/scripts/sign_checksums/sign_checksums.go" "$CHECKS" "$SEED_FILE"
else
  echo "No seed file at $SEED_FILE; skipping signing of checksums.txt"
fi

echo "Builds complete. Binaries available under: $BIN_DIR"

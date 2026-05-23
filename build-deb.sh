#!/usr/bin/env bash
set -euo pipefail

version=0.1.0
arch="${1:-amd64}"

echo "building deb for do-droplets-tui $version ($arch)"

if ! command -v dpkg-deb > /dev/null 2>&1; then
  echo "ERROR: please install dpkg (apt install dpkg)"
  exit 1
fi

case "$arch" in
  amd64)  goarch="amd64" ; goarm="" ;;
  arm64)  goarch="arm64" ; goarm="" ;;
  armhf)  goarch="arm"   ; goarm="7" ;;
  *)      echo "unsupported architecture: $arch"; exit 1 ;;
esac

project="do-droplets-tui_${version}_${arch}"
folder_name="build/$project"
echo "creating $folder_name"
mkdir -p "$folder_name"
cp -r DEBIAN/ "$folder_name"
bin_dir="$folder_name/usr/bin"
mkdir -p "$bin_dir"

build_args=(-trimpath -ldflags "-s -w")
if [ "$goarch" = "$(go env GOARCH)" ] && [ "$GOOS" = "$(go env GOOS)" ] 2>/dev/null; then
  go build "${build_args[@]}" -o do-droplets-tui ./cmd/do-droplets-tui
else
  env CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" ${goarm:+GOARM=$goarm} \
    go build "${build_args[@]}" -o do-droplets-tui ./cmd/do-droplets-tui
fi

mv do-droplets-tui "$bin_dir/"
chmod 0755 "$bin_dir/do-droplets-tui"
sed -i "s/_version_/$version/g"           "$folder_name/DEBIAN/control"
sed -i "s/^Architecture: .*/Architecture: $arch/" "$folder_name/DEBIAN/control"

cd build/ && dpkg-deb --build -Z gzip --root-owner-group "$project"
echo ">> build/${project}.deb"

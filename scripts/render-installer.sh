#!/bin/sh
# Render a release-specific installer from the committed development template.
set -eu
export LC_ALL=C

template=${1:?template path required}
checksums=${2:?SHA256SUMS path required}
output=${3:?output path required}
tag=${4:?release tag required}
repository=${5:?release repository required}
version=${tag#v}
release_tag_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'

case "$tag" in
  *[!0-9A-Za-z.+-]*)
    echo "invalid release tag" >&2
    exit 1
    ;;
esac
if ! printf '%s\n' "$tag" | grep -Eq "$release_tag_pattern"; then
  echo "invalid release tag" >&2
  exit 1
fi
case "$repository" in
  *[!0-9A-Za-z._/-]*)
    echo "invalid release repository" >&2
    exit 1
    ;;
esac
if ! printf '%s\n' "$repository" | grep -Eq '^[0-9A-Za-z][0-9A-Za-z-]*/[0-9A-Za-z._-]+$'; then
  echo "invalid release repository" >&2
  exit 1
fi

value_for() {
  file="burnerpad_${version}_$1_$2.tar.gz"
  value=$(awk -v file="$file" '$2 == file { print $1 }' "$checksums")
  case "$value" in
    ''|*[!0-9a-f]*)
      echo "missing or invalid checksum for $file" >&2
      exit 1
      ;;
  esac
  [ ${#value} -eq 64 ] || { echo "missing or invalid checksum for $file" >&2; exit 1; }
  printf '%s' "$value"
}

linux_amd64=$(value_for linux amd64)
linux_arm64=$(value_for linux arm64)
darwin_amd64=$(value_for darwin amd64)
darwin_arm64=$(value_for darwin arm64)

sed \
  -e "s|^REPO=.*|REPO=\"$repository\"|" \
  -e "s/^VERSION=.*/VERSION=\"$version\"/" \
  -e "s/^SHA256_linux_amd64=.*/SHA256_linux_amd64=\"$linux_amd64\"/" \
  -e "s/^SHA256_linux_arm64=.*/SHA256_linux_arm64=\"$linux_arm64\"/" \
  -e "s/^SHA256_darwin_amd64=.*/SHA256_darwin_amd64=\"$darwin_amd64\"/" \
  -e "s/^SHA256_darwin_arm64=.*/SHA256_darwin_arm64=\"$darwin_arm64\"/" \
  "$template" > "$output"
chmod 0755 "$output"

#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 OUTPUT_DIRECTORY" >&2
  exit 2
fi

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
inputs="$repository_root/test/provider-matrix/fixtures/core-package-inputs"
output=$1
source_epoch=1784419200
signing_time=20260719T000000
mismatch_time=20260719T000100
signing_fingerprint=8DDFCCB89FC8A63796554F956177FE96142F67AB
mismatch_fingerprint=F9E2B9F7F04D8BB33EC7FB3431DD6980551A87F1
arch_image='archlinux:latest@sha256:2b4d67033863d9f495dfd0f52ad8b451fae84adb71b4bdf63f69d10643df2403'

if [[ -e "$output" ]]; then
  echo "fixture output already exists: $output" >&2
  exit 2
fi

for command_name in docker dpkg-deb dpkg-scanpackages gpg gzip md5sum sha256sum tar zstd; do
  command -v "$command_name" >/dev/null || {
    echo "required fixture command is unavailable: $command_name" >&2
    exit 2
  }
done

fixture_tmp=$(mktemp -d)
trap 'rm -rf "$fixture_tmp"' EXIT
fixture_gnupg="$fixture_tmp/gnupg"
mkdir -m 0700 "$fixture_gnupg"
gpg --homedir "$fixture_gnupg" --batch --import \
  "$inputs/keys/signing-private.asc" "$inputs/keys/mismatch-private.asc" >/dev/null 2>&1

mkdir -p \
  "$output/apt/pool/main/r/remotr-fixture" \
  "$output/apt/dists/stable/main/binary-amd64" \
  "$output/pacman/v1" \
  "$output/pacman/v2" \
  "$output/aur/remotr-aur-fixture" \
  "$output/keys" \
  "$output/native-config/apt" \
  "$output/native-config/pacman"

cp "$inputs/keys/signing-public.asc" "$output/keys/signing-public.asc"
cp "$inputs/keys/mismatch-public.asc" "$output/keys/mismatch-public.asc"
cp "$inputs/aur/PKGBUILD" "$inputs/aur/.SRCINFO" "$inputs/aur/remotr-aur-fixture.sh" \
  "$output/aur/remotr-aur-fixture/"
cp "$inputs/native-config/apt/unrelated.sources" "$output/native-config/apt/"
cp "$inputs/native-config/pacman/unrelated.conf" "$output/native-config/pacman/"

build_deb() {
  local version=$1
  local stage="$fixture_tmp/deb-$version"
  local target="$output/apt/pool/main/r/remotr-fixture/remotr-fixture_${version}_amd64.deb"
  mkdir -p "$stage/DEBIAN" "$stage/usr/bin"
  sed "s/@VERSION@/$version/g" "$inputs/package/debian-control" >"$stage/DEBIAN/control"
  sed "s/@VERSION@/$version/g" "$inputs/package/remotr-fixture" >"$stage/usr/bin/remotr-fixture"
  chmod 0755 "$stage/usr/bin/remotr-fixture"
  find "$stage" -exec touch -h -d "@$source_epoch" {} +
  SOURCE_DATE_EPOCH=$source_epoch dpkg-deb --root-owner-group --build "$stage" "$target" >/dev/null
  touch -d "@$source_epoch" "$target"
}

build_pacman_package() {
  local version=$1
  local repository=$2
  local stage="$fixture_tmp/pacman-$version"
  local target="$output/pacman/$repository/remotr-fixture-${version}-x86_64.pkg.tar.zst"
  mkdir -p "$stage/usr/bin"
  sed "s/@VERSION@/$version/g" "$inputs/package/remotr-fixture" >"$stage/usr/bin/remotr-fixture"
  chmod 0755 "$stage/usr/bin/remotr-fixture"
  local installed_size
  installed_size=$(wc -c <"$stage/usr/bin/remotr-fixture" | tr -d ' ')
  sed -e "s/@VERSION@/$version/g" -e "s/@SIZE@/$installed_size/g" \
    "$inputs/package/pacman-pkginfo" >"$stage/.PKGINFO"
  find "$stage" -exec touch -h -d "@$source_epoch" {} +
  tar --sort=name --mtime="@$source_epoch" --owner=0 --group=0 --numeric-owner \
    --format=gnu -C "$stage" -cf - .PKGINFO usr \
    | zstd --quiet --ultra -19 --threads=1 -o "$target"
  touch -d "@$source_epoch" "$target"
  gpg --homedir "$fixture_gnupg" --batch --yes --pinentry-mode loopback --passphrase '' \
    --faked-system-time "$signing_time" --local-user "$signing_fingerprint" \
    --detach-sign --output "$target.sig" "$target"
  touch -d "@$source_epoch" "$target.sig"
}

build_deb 1.0.0-1
build_deb 2.0.0-1

(
  cd "$output/apt"
  dpkg-scanpackages --multiversion pool /dev/null >dists/stable/main/binary-amd64/Packages
)
gzip -n -9 -c "$output/apt/dists/stable/main/binary-amd64/Packages" \
  >"$output/apt/dists/stable/main/binary-amd64/Packages.gz"

release_file="$output/apt/dists/stable/Release"
{
  printf '%s\n' \
    'Origin: Remotr Test Fixtures' \
    'Label: Remotr Test Fixtures' \
    'Suite: stable' \
    'Codename: stable' \
    'Date: Sun, 19 Jul 2026 00:00:00 +0000' \
    'Architectures: amd64' \
    'Components: main' \
    'Description: Deterministic Remotr APT provider fixture' \
    'MD5Sum:'
  for relative in main/binary-amd64/Packages main/binary-amd64/Packages.gz; do
    file="$output/apt/dists/stable/$relative"
    printf ' %s %16d %s\n' "$(md5sum "$file" | cut -d' ' -f1)" "$(wc -c <"$file")" "$relative"
  done
  printf '%s\n' 'SHA256:'
  for relative in main/binary-amd64/Packages main/binary-amd64/Packages.gz; do
    file="$output/apt/dists/stable/$relative"
    printf ' %s %16d %s\n' "$(sha256sum "$file" | cut -d' ' -f1)" "$(wc -c <"$file")" "$relative"
  done
} >"$release_file"

gpg --homedir "$fixture_gnupg" --batch --yes --pinentry-mode loopback --passphrase '' \
  --faked-system-time "$signing_time" --local-user "$signing_fingerprint" \
  --armor --detach-sign --output "$release_file.gpg" "$release_file"
gpg --homedir "$fixture_gnupg" --batch --yes --pinentry-mode loopback --passphrase '' \
  --faked-system-time "$signing_time" --local-user "$signing_fingerprint" \
  --armor --clearsign --output "$output/apt/dists/stable/InRelease" "$release_file"
gpg --homedir "$fixture_gnupg" --batch --yes --pinentry-mode loopback --passphrase '' \
  --faked-system-time "$mismatch_time" --local-user "$mismatch_fingerprint" \
  --armor --detach-sign --output "$output/apt/dists/stable/Release.mismatch.gpg" "$release_file"

build_pacman_package 1.0.0-1 v1
build_pacman_package 2.0.0-1 v2

for repository in v1 v2; do
  repository_tmp="$fixture_tmp/repository-$repository"
  database_tmp="$fixture_tmp/database-$repository"
  mkdir -p "$repository_tmp" "$database_tmp"
  cp "$output/pacman/$repository"/*.pkg.tar.zst "$output/pacman/$repository"/*.pkg.tar.zst.sig "$repository_tmp/"
  docker run --rm --volume "$repository_tmp:/repo" "$arch_image" \
    sh -eu -c 'repo-add /repo/remotr-fixture.db.tar.gz /repo/*.pkg.tar.zst >/dev/null'
  tar -xzf "$repository_tmp/remotr-fixture.db.tar.gz" -C "$database_tmp"
  (
    cd "$database_tmp"
    tar --sort=name --mtime="@$source_epoch" --owner=0 --group=0 --numeric-owner \
      --format=gnu -cf - *
  ) | gzip -n -9 >"$output/pacman/$repository/remotr-fixture.db"
  cp "$output/pacman/$repository/remotr-fixture.db" \
    "$output/pacman/$repository/remotr-fixture.db.tar.gz"
  gpg --homedir "$fixture_gnupg" --batch --yes --pinentry-mode loopback --passphrase '' \
    --faked-system-time "$signing_time" --local-user "$signing_fingerprint" \
    --detach-sign --output "$output/pacman/$repository/remotr-fixture.db.sig" \
    "$output/pacman/$repository/remotr-fixture.db"
  gpg --homedir "$fixture_gnupg" --batch --yes --pinentry-mode loopback --passphrase '' \
    --faked-system-time "$mismatch_time" --local-user "$mismatch_fingerprint" \
    --detach-sign --output "$output/pacman/$repository/remotr-fixture.db.mismatch.sig" \
    "$output/pacman/$repository/remotr-fixture.db"
done

find "$output" -type f -exec touch -h -d "@$source_epoch" {} +
printf '%s\n' \
  '{' \
  '  "sourceDateEpoch": 1784419200,' \
  '  "signingFingerprint": "8DDFCCB89FC8A63796554F956177FE96142F67AB",' \
  '  "mismatchFingerprint": "F9E2B9F7F04D8BB33EC7FB3431DD6980551A87F1",' \
  '  "aptVersions": ["1.0.0-1", "2.0.0-1"],' \
  '  "pacmanVersions": ["1.0.0-1", "2.0.0-1"],' \
  '  "aurVersion": "1.0.0-1",' \
  '  "requiredArtifacts": [' \
  '    "apt/dists/stable/InRelease",' \
  '    "apt/dists/stable/Release.gpg",' \
  '    "apt/dists/stable/Release.mismatch.gpg",' \
  '    "apt/pool/main/r/remotr-fixture/remotr-fixture_1.0.0-1_amd64.deb",' \
  '    "apt/pool/main/r/remotr-fixture/remotr-fixture_2.0.0-1_amd64.deb",' \
  '    "aur/remotr-aur-fixture/PKGBUILD",' \
  '    "native-config/apt/unrelated.sources",' \
  '    "native-config/pacman/unrelated.conf",' \
  '    "pacman/v1/remotr-fixture-1.0.0-1-x86_64.pkg.tar.zst",' \
  '    "pacman/v1/remotr-fixture.db.sig",' \
  '    "pacman/v1/remotr-fixture.db.mismatch.sig",' \
  '    "pacman/v2/remotr-fixture-2.0.0-1-x86_64.pkg.tar.zst",' \
  '    "pacman/v2/remotr-fixture.db.sig",' \
  '    "pacman/v2/remotr-fixture.db.mismatch.sig"' \
  '  ]' \
  '}' >"$output/METADATA.json"
touch -d "@$source_epoch" "$output/METADATA.json"
(
  cd "$output"
  find . -type f ! -name SHA256SUMS -print0 \
    | sort -z \
    | xargs -0 sha256sum >SHA256SUMS
)

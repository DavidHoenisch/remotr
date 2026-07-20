#!/usr/bin/env bash
set -euo pipefail

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
inputs="$repository_root/test/provider-matrix/fixtures/core-package-inputs"
fixtures="$repository_root/test/provider-matrix/fixtures/core-packages"
signing_fingerprint=8DDFCCB89FC8A63796554F956177FE96142F67AB
mismatch_fingerprint=F9E2B9F7F04D8BB33EC7FB3431DD6980551A87F1
verify_reproducible=false

if [[ $# -gt 1 ]]; then
  echo "usage: $0 [--reproducible]" >&2
  exit 2
fi
if [[ $# -eq 1 ]]; then
  if [[ $1 != --reproducible ]]; then
    echo "unknown option: $1" >&2
    exit 2
  fi
  verify_reproducible=true
fi

for command_name in dpkg-deb gpg gpgv sha256sum tar; do
  command -v "$command_name" >/dev/null || {
    echo "required fixture verification command is unavailable: $command_name" >&2
    exit 2
  }
done

(
  cd "$inputs"
  sha256sum --check INPUT_SHA256SUMS
)
(
  cd "$fixtures"
  sha256sum --check SHA256SUMS
)

fixture_tmp=$(mktemp -d)
trap 'rm -rf "$fixture_tmp"' EXIT
signing_keyring="$fixture_tmp/signing-public.gpg"
mismatch_keyring="$fixture_tmp/mismatch-public.gpg"
gpg --batch --yes --dearmor --output "$signing_keyring" "$fixtures/keys/signing-public.asc"
gpg --batch --yes --dearmor --output "$mismatch_keyring" "$fixtures/keys/mismatch-public.asc"

actual_signing=$(gpg --batch --show-keys --with-colons "$fixtures/keys/signing-public.asc" | awk -F: '$1 == "fpr" {print $10; exit}')
actual_mismatch=$(gpg --batch --show-keys --with-colons "$fixtures/keys/mismatch-public.asc" | awk -F: '$1 == "fpr" {print $10; exit}')
[[ $actual_signing == "$signing_fingerprint" ]]
[[ $actual_mismatch == "$mismatch_fingerprint" ]]

release="$fixtures/apt/dists/stable/Release"
gpgv --keyring "$signing_keyring" "$release.gpg" "$release" >/dev/null 2>&1
gpgv --keyring "$signing_keyring" "$fixtures/apt/dists/stable/InRelease" >/dev/null 2>&1
if gpgv --keyring "$signing_keyring" \
  "$fixtures/apt/dists/stable/Release.mismatch.gpg" "$release" >/dev/null 2>&1; then
  echo "mismatched APT signature verified with the trusted fixture key" >&2
  exit 1
fi
gpgv --keyring "$mismatch_keyring" \
  "$fixtures/apt/dists/stable/Release.mismatch.gpg" "$release" >/dev/null 2>&1

for version in 1.0.0-1 2.0.0-1; do
  package="$fixtures/apt/pool/main/r/remotr-fixture/remotr-fixture_${version}_amd64.deb"
  [[ $(dpkg-deb --field "$package" Package) == remotr-fixture ]]
  [[ $(dpkg-deb --field "$package" Version) == "$version" ]]
  [[ $(dpkg-deb --field "$package" Architecture) == amd64 ]]
  dpkg-deb --fsys-tarfile "$package" | tar -xOf - ./usr/bin/remotr-fixture \
    | grep -Fq "'$version'"
  grep -Fq "Version: $version" "$fixtures/apt/dists/stable/main/binary-amd64/Packages"
done

for repository_version in v1:1.0.0-1 v2:2.0.0-1; do
  repository=${repository_version%%:*}
  version=${repository_version#*:}
  package="$fixtures/pacman/$repository/remotr-fixture-${version}-x86_64.pkg.tar.zst"
  package_info=$(tar --zstd -xOf "$package" .PKGINFO)
  grep -Fqx 'pkgname = remotr-fixture' <<<"$package_info"
  grep -Fqx "pkgver = $version" <<<"$package_info"
  gpgv --keyring "$signing_keyring" "$package.sig" "$package" >/dev/null 2>&1

  database="$fixtures/pacman/$repository/remotr-fixture.db"
  database_description=$(tar -xOzf "$database" --wildcards '*/desc')
  grep -Fqx "$version" <<<"$database_description"
  grep -Fqx "$(basename "$package")" <<<"$database_description"
  gpgv --keyring "$signing_keyring" "$database.sig" "$database" >/dev/null 2>&1
  if gpgv --keyring "$signing_keyring" \
    "$database.mismatch.sig" "$database" >/dev/null 2>&1; then
    echo "mismatched Pacman database signature verified with the trusted fixture key" >&2
    exit 1
  fi
  gpgv --keyring "$mismatch_keyring" \
    "$database.mismatch.sig" "$database" >/dev/null 2>&1
done

aur_source="$fixtures/aur/remotr-aur-fixture/remotr-aur-fixture.sh"
aur_checksum=$(sha256sum "$aur_source" | cut -d' ' -f1)
grep -Fq "sha256sums=('$aur_checksum')" "$fixtures/aur/remotr-aur-fixture/PKGBUILD"
grep -Fq "sha256sums = $aur_checksum" "$fixtures/aur/remotr-aur-fixture/.SRCINFO"
grep -Fq 'https://unrelated.invalid/debian' "$fixtures/native-config/apt/unrelated.sources"
grep -Fq '[unrelated]' "$fixtures/native-config/pacman/unrelated.conf"

if $verify_reproducible; then
  regenerated="$fixture_tmp/regenerated"
  bash "$repository_root/scripts/generate-package-provider-fixtures.sh" "$regenerated"
  diff -r "$fixtures" "$regenerated"
fi

echo "core package-provider fixtures verified"

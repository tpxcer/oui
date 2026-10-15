#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=../update.sh
source "$repo_root/update.sh"

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/oui-digest-test.XXXXXX")"
trap 'rm -rf "$tmpdir"' EXIT

package="$tmpdir/x-ui-linux-amd64.tar.gz"
printf 'oui package' > "$package"
package_sha="$(file_sha256 "$package")"

cat > "$tmpdir/release.json" <<EOF
{
  "name": "",
  "assets": [
    {
      "name": "x-ui-linux-386.tar.gz",
      "uploader": {
        "login": "github-actions[bot]"
      },
      "digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
      "browser_download_url": "https://example.invalid/x-ui-linux-386.tar.gz"
    },
    {
      "name": "x-ui-linux-amd64.tar.gz",
      "uploader": {
        "login": "github-actions[bot]"
      },
      "digest": "sha256:${package_sha}",
      "browser_download_url": "https://example.invalid/x-ui-linux-amd64.tar.gz"
    },
    {
      "name": "x-ui-linux-arm64.tar.gz",
      "browser_download_url": "https://example.invalid/x-ui-linux-arm64.tar.gz"
    },
    {
      "name": "x-ui-linux-armv7.tar.gz",
      "digest": "sha256:7777777777777777777777777777777777777777777777777777777777777777",
      "browser_download_url": "https://example.invalid/x-ui-linux-armv7.tar.gz"
    }
  ]
}
EOF

if [[ "$(parse_release_asset_digest x-ui-linux-amd64.tar.gz < "$tmpdir/release.json")" != "$package_sha" ]]; then
    echo "digest for the requested asset was not parsed" >&2
    exit 1
fi
if [[ -n "$(parse_release_asset_digest x-ui-linux-arm64.tar.gz < "$tmpdir/release.json")" ]]; then
    echo "an asset without a digest must not borrow the next asset's digest" >&2
    exit 1
fi
if [[ -n "$(parse_release_asset_digest x-ui-linux-s390x.tar.gz < "$tmpdir/release.json")" ]]; then
    echo "a missing asset must not return a digest" >&2
    exit 1
fi

fake_curl="$tmpdir/fake-curl"
printf '#!/usr/bin/env sh\ncat %q\n' "$tmpdir/release.json" > "$fake_curl"
chmod +x "$fake_curl"
curl_bin="$fake_curl"

verify_release_package "test-tag" "x-ui-linux-amd64.tar.gz" "$package" > /dev/null
if [[ ! -f "$package" ]]; then
    echo "a matching package must be kept" >&2
    exit 1
fi

tampered="$tmpdir/tampered.tar.gz"
printf 'tampered' > "$tampered"
if (verify_release_package "test-tag" "x-ui-linux-amd64.tar.gz" "$tampered" > /dev/null); then
    echo "a mismatching package must abort the update" >&2
    exit 1
fi
if [[ -f "$tampered" ]]; then
    echo "a mismatching package must be removed" >&2
    exit 1
fi

printf '#!/usr/bin/env sh\nexit 22\n' > "$fake_curl"
verify_release_package "test-tag" "x-ui-linux-amd64.tar.gz" "$package" > /dev/null
if [[ ! -f "$package" ]]; then
    echo "an unavailable digest must not block the update" >&2
    exit 1
fi

echo "update release digest test passed"

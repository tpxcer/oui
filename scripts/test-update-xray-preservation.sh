#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=../update.sh
source "$repo_root/update.sh"

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/oui-update-test.XXXXXX")"
trap 'rm -rf "$tmpdir"' EXIT

xui_folder="$tmpdir/x-ui"
unset XUI_BIN_FOLDER
arch() { echo "amd64"; }

for test_arch in amd64 arm64 armv5 armv6 armv7 386 s390x; do
    arch() { echo "$test_arch"; }
    case "$test_arch" in
        armv5 | armv6 | armv7) expected_name="xray-linux-arm" ;;
        *) expected_name="xray-linux-$test_arch" ;;
    esac
    if [[ "$(xray_binary_name)" != "$expected_name" ]]; then
        echo "unexpected Xray binary name for $test_arch" >&2
        exit 1
    fi
done
arch() { echo "amd64"; }

XUI_BIN_FOLDER="custom-bin"
if [[ "$(xray_binary_path)" != "$xui_folder/custom-bin/xray-linux-amd64" ]]; then
    echo "relative XUI_BIN_FOLDER was not resolved from the panel directory" >&2
    exit 1
fi
XUI_BIN_FOLDER="$tmpdir/external-bin"
if [[ "$(xray_binary_path)" != "$tmpdir/external-bin/xray-linux-amd64" ]]; then
    echo "absolute XUI_BIN_FOLDER was changed unexpectedly" >&2
    exit 1
fi
unset XUI_BIN_FOLDER

target="$xui_folder/bin/xray-linux-amd64"
mkdir -p "$(dirname "$target")"
cat > "$target" <<'EOF'
#!/usr/bin/env sh
echo "Xray test-preserved"
EOF
chmod 0751 "$target"

preserve_existing_xray
printf '#!/usr/bin/env sh\necho "Xray bundled"\n' > "$target"
chmod 0755 "$target"
restore_preserved_xray

if ! grep -q "test-preserved" "$target"; then
    echo "existing Xray core was not restored" >&2
    exit 1
fi
if [[ -n "$preserved_xray_backup" ]]; then
    echo "temporary Xray backup was not cleared" >&2
    exit 1
fi

preserve_existing_xray
printf '#!/usr/bin/env sh\necho "Xray bundled after failure"\n' > "$target"
chmod 0755 "$target"
xui_service_update_started=false
recover_xui_service_on_failure 1
if ! grep -q "test-preserved" "$target"; then
    echo "failed update did not restore the existing Xray core" >&2
    exit 1
fi

rm -f "$target"
preserve_existing_xray
printf '#!/usr/bin/env sh\necho "Xray bundled"\n' > "$target"
restore_preserved_xray
if ! grep -q "Xray bundled" "$target"; then
    echo "bundled Xray core should remain on an installation without an existing core" >&2
    exit 1
fi

echo "update Xray preservation test passed"

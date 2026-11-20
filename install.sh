#!/bin/sh
# Installs the hearth binary from a GitHub Release.
#
#   curl -fsSL https://raw.githubusercontent.com/CTHua/hearth/main/install.sh | sh
#
# Environment:
#   HEARTH_VERSION      version to install, 0.1.0 or v0.1.0 (default: latest Release)
#   HEARTH_INSTALL_DIR  install directory (default: $HOME/.local/bin)
#   HEARTH_BASE_URL     download root (default: https://github.com/CTHua/hearth/releases)
#
# Everything lives in one function called on the last line, so a partially
# downloaded script defines nothing that runs.

hearth_install() {
	set -eu

	BINARY="hearth"
	DEFAULT_BASE_URL="https://github.com/CTHua/hearth/releases"
	DEFAULT_INSTALL_SUBDIR=".local/bin"
	CHECKSUMS="SHA256SUMS"
	SUPPORTED_PLATFORMS="darwin/amd64 darwin/arm64 linux/amd64 linux/arm64"
	LOG_PREFIX="hearth install"

	tmp_dir=""
	staged_file=""

	fail() {
		printf '%s: %s\n' "$LOG_PREFIX" "$1" >&2
		exit 1
	}

	cleanup() {
		if [ -n "$staged_file" ]; then
			rm -f "$staged_file"
		fi
		if [ -n "$tmp_dir" ]; then
			rm -rf "$tmp_dir"
		fi
	}

	require_tools() {
		for tool in curl tar; do
			command -v "$tool" >/dev/null 2>&1 || fail "missing required tool: $tool"
		done
		if command -v sha256sum >/dev/null 2>&1; then
			sha_tool="sha256sum"
		elif command -v shasum >/dev/null 2>&1; then
			sha_tool="shasum"
		else
			fail "missing required tool: sha256sum or shasum"
		fi
	}

	detect_platform() {
		uname_s="$(uname -s)" || fail "detecting the platform: uname -s failed"
		uname_m="$(uname -m)" || fail "detecting the platform: uname -m failed"
		case "$uname_s" in
		Darwin) os="darwin" ;;
		Linux) os="linux" ;;
		*) os="" ;;
		esac
		case "$uname_m" in
		arm64 | aarch64) arch="arm64" ;;
		x86_64 | amd64) arch="amd64" ;;
		*) arch="" ;;
		esac
		if [ -z "$os" ] || [ -z "$arch" ]; then
			fail "detecting the platform: $uname_s/$uname_m is not supported; supported platforms: $SUPPORTED_PLATFORMS"
		fi
		archive="${BINARY}_${os}_${arch}.tar.gz"
	}

	# Same rule as hearth's version.Parse, after an optional leading v.
	is_version() {
		case "$1" in
		"" | *[!0-9.]* | .* | *. | *..* | *.*.*.*) return 1 ;;
		*.*.*) return 0 ;;
		esac
		return 1
	}

	resolve_source() {
		base_url="${HEARTH_BASE_URL:-}"
		protocols="=https,file"
		if [ -z "$base_url" ]; then
			base_url="$DEFAULT_BASE_URL"
			protocols="=https"
		fi
		base_url="${base_url%/}"
		requested="${HEARTH_VERSION:-}"
		if [ -z "$requested" ]; then
			download_url="$base_url/latest/download"
			return
		fi
		version="${requested#v}"
		is_version "$version" || fail "HEARTH_VERSION \"$requested\" must be MAJOR.MINOR.PATCH, optionally prefixed with v"
		download_url="$base_url/download/v$version"
	}

	resolve_install_dir() {
		install_dir="${HEARTH_INSTALL_DIR:-}"
		if [ -n "$install_dir" ]; then
			return
		fi
		[ -n "${HOME:-}" ] || fail "HOME is not set; set HEARTH_INSTALL_DIR"
		install_dir="$HOME/$DEFAULT_INSTALL_SUBDIR"
	}

	download() {
		curl -fsSL --proto "$protocols" --tlsv1.2 -o "$2" "$1" || fail "downloading failed: $1"
	}

	sha256_of() {
		if [ "$sha_tool" = "sha256sum" ]; then
			sha256sum "$1" | awk '{ print $1 }'
		else
			shasum -a 256 "$1" | awk '{ print $1 }'
		fi
	}

	verify_checksum() {
		expected="$(awk -v name="$archive" '$2 == name { print $1; exit }' "$tmp_dir/$CHECKSUMS")" ||
			fail "verifying the checksum: cannot read $CHECKSUMS"
		[ -n "$expected" ] || fail "verifying the checksum: $CHECKSUMS has no entry for $archive"
		actual="$(sha256_of "$tmp_dir/$archive")"
		[ -n "$actual" ] || fail "verifying the checksum: hashing $archive failed"
		[ "$expected" = "$actual" ] ||
			fail "verifying the checksum: $archive has SHA-256 $actual, $CHECKSUMS expects $expected"
	}

	extract() {
		mkdir "$tmp_dir/extract" || fail "extracting: cannot create a directory in $tmp_dir"
		tar -xzf "$tmp_dir/$archive" -C "$tmp_dir/extract" "$BINARY" || fail "extracting $BINARY from $archive failed"
		[ -f "$tmp_dir/extract/$BINARY" ] || fail "extracting: $archive has no $BINARY file"
	}

	# The binary is staged and run inside the install directory, not the temp
	# directory, because /tmp is mounted noexec on some machines; mv within one
	# file system then replaces the old binary atomically.
	install_binary() {
		mkdir -p "$install_dir" || fail "installing: cannot create $install_dir"
		staged_file="$(mktemp "$install_dir/.$BINARY.XXXXXX")" || fail "installing: cannot create a file in $install_dir"
		cp "$tmp_dir/extract/$BINARY" "$staged_file" || fail "installing: cannot copy $BINARY into $install_dir"
		chmod 755 "$staged_file" || fail "installing: cannot make $staged_file executable"
		installed_version="$("$staged_file" version)" ||
			fail "installing: the downloaded $BINARY does not run on this machine ($os/$arch); nothing was replaced"
		mv -f "$staged_file" "$install_dir/$BINARY" || fail "installing: cannot move $BINARY into $install_dir"
		staged_file=""
	}

	report() {
		printf 'installed %s %s to %s\n' "$BINARY" "$installed_version" "$install_dir/$BINARY"
		case ":${PATH:-}:" in
		*":$install_dir:"*) ;;
		*) printf '%s is not on your PATH; add it with: export PATH="%s:%s"\n' "$install_dir" "$install_dir" "\$PATH" ;;
		esac
	}

	trap cleanup EXIT
	trap 'exit 1' HUP INT TERM

	require_tools
	detect_platform
	resolve_source
	resolve_install_dir
	tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/$BINARY-install.XXXXXX")" || fail "cannot create a temporary directory"
	download "$download_url/$archive" "$tmp_dir/$archive"
	download "$download_url/$CHECKSUMS" "$tmp_dir/$CHECKSUMS"
	verify_checksum
	extract
	install_binary
	report
}

hearth_install "$@"

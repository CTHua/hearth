#!/bin/sh
# Builds one tar.gz per supported platform plus SHA256SUMS.
# Usage: scripts/build-release.sh <MAJOR.MINOR.PATCH> [<outdir>]
set -eu

MODULE="github.com/CTHua/hearth"
BINARY="hearth"
PACKAGE="./cmd/hearth"
PLATFORMS="darwin/amd64 darwin/arm64 linux/amd64 linux/arm64"
CHECKSUMS="SHA256SUMS"
DEFAULT_OUTDIR="dist"
BUNDLED_FILES="LICENSE README.md"
EXIT_USAGE=2

usage_error() {
	printf 'build-release: %s\n' "$1" >&2
	printf 'usage: scripts/build-release.sh <MAJOR.MINOR.PATCH> [<outdir>]\n' >&2
	exit "$EXIT_USAGE"
}

fail() {
	printf 'build-release: %s\n' "$1" >&2
	exit 1
}

# Same rule as version.Parse: three non-empty decimal parts, no prefix.
is_version() {
	case "$1" in
	"" | *[!0-9.]* | .* | *. | *..* | *.*.*.*) return 1 ;;
	*.*.*) return 0 ;;
	esac
	return 1
}

archive_name() {
	printf '%s_%s_%s.tar.gz' "$BINARY" "$1" "$2"
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1"
	else
		shasum -a 256 "$1"
	fi
}

build_platform() {
	os="${1%/*}"
	arch="${1#*/}"
	rm -f "$stage/$BINARY"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
		-ldflags "-s -w -X $MODULE/internal/version.Version=$version" \
		-o "$stage/$BINARY" "$PACKAGE" || fail "go build failed for $1"
	# COPYFILE_DISABLE keeps macOS tar from adding ._* members for xattrs.
	# shellcheck disable=SC2086 # BUNDLED_FILES is a fixed list of plain names
	COPYFILE_DISABLE=1 tar -czf "$outdir/$(archive_name "$os" "$arch")" \
		-C "$stage" "$BINARY" $BUNDLED_FILES || fail "packing failed for $1"
}

write_checksums() {
	unsorted="$stage/$CHECKSUMS.unsorted"
	: >"$unsorted"
	(
		cd "$outdir" || exit 1
		for platform in $PLATFORMS; do
			sha256_of "$(archive_name "${platform%/*}" "${platform#*/}")" >>"$unsorted" || exit 1
		done
	) || fail "hashing the archives failed"
	LC_ALL=C sort -k 2 "$unsorted" >"$stage/$CHECKSUMS" || fail "sorting $CHECKSUMS failed"
	mv -f "$stage/$CHECKSUMS" "$outdir/$CHECKSUMS" || fail "writing $CHECKSUMS failed"
}

main() {
	[ "$#" -ge 1 ] || usage_error "missing version"
	[ "$#" -le 2 ] || usage_error "too many arguments"
	version="$1"
	is_version "$version" || usage_error "version \"$version\" must be MAJOR.MINOR.PATCH without a v prefix"
	outdir="${2:-$DEFAULT_OUTDIR}"
	case "$outdir" in
	/*) ;;
	*) outdir="$(pwd)/$outdir" ;;
	esac

	root="$(cd "$(dirname "$0")/.." && pwd)" || fail "cannot find the repo root"
	cd "$root" || fail "cannot enter $root"
	mkdir -p "$outdir" || fail "cannot create $outdir"
	stage="$(mktemp -d "${TMPDIR:-/tmp}/build-release.XXXXXX")" || fail "cannot create a staging directory"
	trap 'rm -rf "$stage"' EXIT
	trap 'exit 1' HUP INT TERM
	for file in $BUNDLED_FILES; do
		cp "$file" "$stage/$file" || fail "cannot copy $file"
	done

	for platform in $PLATFORMS; do
		build_platform "$platform"
	done
	write_checksums
	printf 'built %s for %s into %s\n' "$version" "$PLATFORMS" "$outdir"
}

main "$@"

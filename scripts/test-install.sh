#!/bin/sh
# Offline end-to-end test of install.sh against file:// releases built by
# scripts/build-release.sh. Each check names the acceptance criterion it covers.
set -eu

BINARY="hearth"
VERSION="0.0.0"
CHECKSUMS="SHA256SUMS"
SUPPORTED_PLATFORMS="darwin/amd64 darwin/arm64 linux/amd64 linux/arm64"
PATH_HINT="is not on your PATH"
OLD_BINARY_TEXT="old hearth"

unset HEARTH_VERSION HEARTH_INSTALL_DIR HEARTH_BASE_URL

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/test-install.XXXXXX")"
trap 'rm -rf "$work"' EXIT
trap 'exit 1' HUP INT TERM

failures=0
run_count=0

fail() {
	printf 'FAIL [%s] %s\n' "$1" "$2"
	failures=$((failures + 1))
}

setup_error() {
	printf 'test-install: %s\n' "$1" >&2
	exit 1
}

host_archive() {
	case "$(uname -s)" in
	Darwin) os="darwin" ;;
	Linux) os="linux" ;;
	*) setup_error "unsupported host system $(uname -s)" ;;
	esac
	case "$(uname -m)" in
	arm64 | aarch64) arch="arm64" ;;
	x86_64 | amd64) arch="amd64" ;;
	*) setup_error "unsupported host architecture $(uname -m)" ;;
	esac
	archive="${BINARY}_${os}_${arch}.tar.gz"
}

sha256_line() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1"
	else
		shasum -a 256 "$1"
	fi
}

# Lays out a release directory the way GitHub serves it: latest/download/ and
# download/v<version>/. $1 is the root, $2 the directory with the files, $3
# the layout: latest or pinned.
publish_release() {
	case "$3" in
	latest) release_dir="$1/latest/download" ;;
	pinned) release_dir="$1/download/v$VERSION" ;;
	*) setup_error "unknown release layout $3" ;;
	esac
	mkdir -p "$release_dir"
	cp "$2"/* "$release_dir/"
}

make_old_binary() {
	mkdir -p "$1"
	printf '#!/bin/sh\necho %s\n' "'$OLD_BINARY_TEXT'" >"$1/$BINARY"
	chmod 755 "$1/$BINARY"
	cp "$1/$BINARY" "$1.orig"
}

# Runs install.sh with its own empty TMPDIR. Sets run_status, run_out, run_err
# and run_tmp. $1 install dir, $2 base URL, $3 HEARTH_VERSION ("" for unset),
# $4 extra PATH prefix ("" for none).
run_install() {
	run_count=$((run_count + 1))
	run_tmp="$work/runs/$run_count/tmp"
	run_out="$work/runs/$run_count/stdout"
	run_err="$work/runs/$run_count/stderr"
	mkdir -p "$run_tmp"
	set +e
	(
		TMPDIR="$run_tmp"
		HEARTH_INSTALL_DIR="$1"
		HEARTH_BASE_URL="$2"
		export TMPDIR HEARTH_INSTALL_DIR HEARTH_BASE_URL
		if [ -n "$3" ]; then
			HEARTH_VERSION="$3"
			export HEARTH_VERSION
		fi
		if [ -n "$4" ]; then
			PATH="$4:$PATH"
			export PATH
		fi
		exec sh "$root/install.sh"
	) >"$run_out" 2>"$run_err"
	run_status=$?
	set -e
	check_clean "$1"
}

# Criterion 13: after every run the temp dir is empty and the install dir
# holds nothing but the binary.
check_clean() {
	leftover="$(find "$run_tmp" -mindepth 1 -maxdepth 1)"
	if [ -n "$leftover" ]; then
		fail 13 "run $run_count left files in TMPDIR: $leftover"
	fi
	if [ -d "$1" ]; then
		leftover="$(find "$1" -mindepth 1 -maxdepth 1 ! -name "$BINARY")"
		if [ -n "$leftover" ]; then
			fail 13 "run $run_count left files in the install dir: $leftover"
		fi
	fi
}

expect_success() {
	if [ "$run_status" -ne 0 ]; then
		fail "$1" "expected exit 0, got $run_status: $(cat "$run_err")"
		return
	fi
	if [ ! -x "$2/$BINARY" ]; then
		fail "$1" "$2/$BINARY is missing or not executable"
		return
	fi
	got="$("$2/$BINARY" version)" || got="(version failed)"
	if [ "$got" != "$VERSION" ]; then
		fail "$1" "installed version is \"$got\", want \"$VERSION\""
	fi
	if ! grep -q "$2/$BINARY" "$run_out" || ! grep -q "$VERSION" "$run_out"; then
		fail "$1" "stdout does not name the install path and version: $(cat "$run_out")"
	fi
}

expect_failure() {
	if [ "$run_status" -ne 1 ]; then
		fail "$1" "expected exit 1, got $run_status"
	fi
}

expect_stderr() {
	if ! grep -q -- "$2" "$run_err"; then
		fail "$1" "stderr does not mention \"$2\": $(cat "$run_err")"
	fi
}

expect_absent() {
	if [ -e "$2" ]; then
		fail "$1" "$2 should not exist"
	fi
}

expect_unchanged() {
	if ! cmp -s "$2/$BINARY" "$2.orig"; then
		fail "$1" "$2/$BINARY was changed"
	fi
}

# Runs a failing install twice: into a missing dir, which must stay missing
# (criterion 12), and over an old binary, which must stay as it was.
expect_untouched_failure() {
	criterion="$1"
	label="$2"
	base="$3"
	extra_path="$4"
	missing_dir="$work/install/$label-missing"
	run_install "$missing_dir" "$base" "" "$extra_path"
	expect_failure "$criterion"
	expect_absent 12 "$missing_dir"
	existing_dir="$work/install/$label-existing"
	make_old_binary "$existing_dir"
	run_install "$existing_dir" "$base" "" "$extra_path"
	expect_failure "$criterion"
	expect_unchanged "$criterion" "$existing_dir"
}

build_releases() {
	printf 'building %s release archives\n' "$VERSION"
	# The test must not reach the network; module downloads fail instead.
	GOPROXY=off "$root/scripts/build-release.sh" "$VERSION" "$work/dist" >"$work/build.log" 2>&1 || {
		cat "$work/build.log" >&2
		printf 'test-install: build-release.sh failed\n' >&2
		exit 1
	}
	host_archive
	publish_release "$work/good" "$work/dist" latest
	publish_release "$work/pinned" "$work/dist" pinned

	mkdir -p "$work/tampered-files"
	cp "$work/dist"/* "$work/tampered-files/"
	printf 'tampered' >>"$work/tampered-files/$archive"
	publish_release "$work/tampered" "$work/tampered-files" latest

	mkdir -p "$work/noline-files"
	cp "$work/dist"/* "$work/noline-files/"
	grep -v " $archive\$" "$work/dist/$CHECKSUMS" >"$work/noline-files/$CHECKSUMS"
	publish_release "$work/noline" "$work/noline-files" latest

	mkdir -p "$work/empty"

	# A release whose binary cannot run, with a checksum that matches it.
	mkdir -p "$work/broken-stage" "$work/broken-files"
	printf '#!/bin/sh\nexit 1\n' >"$work/broken-stage/$BINARY"
	chmod 755 "$work/broken-stage/$BINARY"
	cp "$root/LICENSE" "$root/README.md" "$work/broken-stage/"
	COPYFILE_DISABLE=1 tar -czf "$work/broken-files/$archive" -C "$work/broken-stage" "$BINARY" LICENSE README.md
	(cd "$work/broken-files" && sha256_line "$archive") >"$work/broken-files/$CHECKSUMS"
	publish_release "$work/broken" "$work/broken-files" latest

	# Fake uname and a curl that only records that it was called.
	mkdir -p "$work/fakebin"
	cat >"$work/fakebin/uname" <<'EOF'
#!/bin/sh
case "$1" in
-s) echo "$FAKE_UNAME_S" ;;
-m) echo "$FAKE_UNAME_M" ;;
esac
EOF
	cat >"$work/fakebin/curl" <<'EOF'
#!/bin/sh
echo "$*" >>"$FAKE_CURL_LOG"
exit 1
EOF
	chmod 755 "$work/fakebin/uname" "$work/fakebin/curl"
}

test_latest() {
	dir="$work/install/latest"
	run_install "$dir" "file://$work/good" "" ""
	expect_success 6 "$dir"
	if [ "$run_status" -eq 0 ] && ! grep -q "$PATH_HINT" "$run_out"; then
		fail 12 "no PATH hint although $dir is not on PATH"
	fi
	# Criterion 12: install dir created by a successful install.
	if [ ! -d "$dir" ]; then
		fail 12 "successful install did not create $dir"
	fi
}

test_on_path() {
	dir="$work/install/on-path"
	run_install "$dir" "file://$work/good" "" "$dir"
	expect_success 12 "$dir"
	if grep -q "$PATH_HINT" "$run_out"; then
		fail 12 "PATH hint printed although $dir is on PATH"
	fi
}

test_pinned() {
	for requested in "$VERSION" "v$VERSION"; do
		dir="$work/install/pinned-$requested"
		run_install "$dir" "file://$work/pinned" "$requested" ""
		expect_success 7 "$dir"
	done
	dir="$work/install/pinned-invalid"
	run_install "$dir" "file://$work/pinned" "abc" ""
	expect_failure 7
	expect_absent 7 "$dir"
}

test_tampered() {
	expect_untouched_failure 8 tampered "file://$work/tampered" ""
	expect_stderr 8 checksum
}

test_missing_line() {
	expect_untouched_failure 9 noline "file://$work/noline" ""
	expect_stderr 9 "$archive"
}

test_missing_files() {
	expect_untouched_failure 10 empty "file://$work/empty" ""
	expect_stderr 10 "file://$work/empty/latest/download/$archive"
}

test_unsupported() {
	for pair in "FreeBSD x86_64" "Linux mips64"; do
		FAKE_UNAME_S="${pair% *}"
		FAKE_UNAME_M="${pair#* }"
		FAKE_CURL_LOG="$work/curl.log"
		export FAKE_UNAME_S FAKE_UNAME_M FAKE_CURL_LOG
		rm -f "$FAKE_CURL_LOG"
		expect_untouched_failure 11 "unsupported-$FAKE_UNAME_S" "file://$work/good" "$work/fakebin"
		for platform in $SUPPORTED_PLATFORMS; do
			expect_stderr 11 "$platform"
		done
		if [ -e "$FAKE_CURL_LOG" ]; then
			fail 11 "curl was called for $pair: $(cat "$FAKE_CURL_LOG")"
		fi
	done
	unset FAKE_UNAME_S FAKE_UNAME_M FAKE_CURL_LOG
}

test_broken_binary() {
	dir="$work/install/broken"
	make_old_binary "$dir"
	run_install "$dir" "file://$work/broken" "" ""
	expect_failure 21
	expect_unchanged 21 "$dir"
}

main() {
	build_releases
	test_latest
	test_on_path
	test_pinned
	test_tampered
	test_missing_line
	test_missing_files
	test_unsupported
	test_broken_binary
	if [ "$failures" -ne 0 ]; then
		printf '%s check(s) failed across %s install runs\n' "$failures" "$run_count"
		exit 1
	fi
	printf 'ok: %s install runs passed\n' "$run_count"
}

main "$@"

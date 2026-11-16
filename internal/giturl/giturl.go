// Package giturl parses the three clone URL forms that can be converted
// between ssh and https, and converts them.
package giturl

import (
	"fmt"
	"regexp"
	"strings"
)

// Protocols a URL can be converted to.
const (
	SSH   = "ssh"
	HTTPS = "https"
)

const (
	scpPrefix      = "git@"
	sshURLPrefix   = "ssh://git@"
	httpsPrefix    = "https://"
	scpSeparator   = ":"
	pathSeparator  = "/"
	gitSuffix      = ".git"
	minPathSegment = 2
	// FormsHint names the accepted forms in error messages.
	FormsHint = "git@<host>:<path>, ssh://git@<host>/<path> or https://<host>/<path>"
)

var (
	hostLabelRE   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)
	pathSegmentRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// URL is a parsed clone URL; Host and Path are kept exactly as written.
type URL struct {
	Host string
	Path string
}

// Parse accepts only the three convertible forms; anything else, including a
// port, another user, http:// or a local path, is an error.
func Parse(raw string) (URL, error) {
	host, path, err := split(raw)
	if err != nil {
		return URL{}, err
	}
	if err := checkHost(host); err != nil {
		return URL{}, err
	}
	if err := checkPath(path); err != nil {
		return URL{}, err
	}
	return URL{Host: host, Path: path}, nil
}

func split(raw string) (string, string, error) {
	switch {
	case strings.HasPrefix(raw, sshURLPrefix):
		return splitHostPath(strings.TrimPrefix(raw, sshURLPrefix), pathSeparator)
	case strings.HasPrefix(raw, httpsPrefix):
		return splitHostPath(strings.TrimPrefix(raw, httpsPrefix), pathSeparator)
	case strings.HasPrefix(raw, scpPrefix):
		return splitHostPath(strings.TrimPrefix(raw, scpPrefix), scpSeparator)
	}
	return "", "", fmt.Errorf("must be %s", FormsHint)
}

func splitHostPath(rest string, separator string) (string, string, error) {
	host, path, found := strings.Cut(rest, separator)
	if !found {
		return "", "", fmt.Errorf("must be %s", FormsHint)
	}
	return host, path, nil
}

func checkHost(host string) error {
	if host == "" {
		return fmt.Errorf("host must not be empty")
	}
	for _, label := range strings.Split(host, ".") {
		if !hostLabelRE.MatchString(label) {
			return fmt.Errorf("host %q must be dot-separated labels of letters, digits and '-' (no port, user or IPv6)", host)
		}
	}
	return nil
}

func checkPath(path string) error {
	segments := strings.Split(path, pathSeparator)
	if len(segments) < minPathSegment {
		return fmt.Errorf("path %q must have at least %d segments", path, minPathSegment)
	}
	for _, segment := range segments {
		if segment == "." || segment == ".." || !pathSegmentRE.MatchString(segment) {
			return fmt.Errorf("path segment %q must be letters, digits, '.', '_' or '-' and not '.' or '..'", segment)
		}
	}
	return nil
}

// Format writes u in the canonical form for protocol, which must be SSH or HTTPS.
func (u URL) Format(protocol string) string {
	if protocol == SSH {
		return scpPrefix + u.Host + scpSeparator + u.Path
	}
	return httpsPrefix + u.Host + pathSeparator + u.Path
}

// Expected returns the URL to clone raw with: raw itself when protocol is
// empty, otherwise raw converted to protocol.
func Expected(raw string, protocol string) (string, error) {
	if protocol == "" {
		return raw, nil
	}
	u, err := Parse(raw)
	if err != nil {
		return "", err
	}
	return u.Format(protocol), nil
}

// OnlyProtocolDiffers reports whether origin differs from expected but both
// parse to the same host and path, so switching protocol makes them equal.
func OnlyProtocolDiffers(origin string, expected string) bool {
	if origin == expected {
		return false
	}
	a, err := Parse(origin)
	if err != nil {
		return false
	}
	b, err := Parse(expected)
	if err != nil {
		return false
	}
	return a == b
}

// DefaultName is the last path segment of u without a trailing ".git".
func (u URL) DefaultName() string {
	segments := strings.Split(u.Path, pathSeparator)
	return strings.TrimSuffix(segments[len(segments)-1], gitSuffix)
}

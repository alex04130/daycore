package adapters

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// validateBaseURL rejects addresses an adapter has no business being at.
//
// # What this is and is not
//
// It is not an SSRF defence — the real one is that base_url is boot-layer and
// only reachable by somebody with shell access. This is the second layer: an
// operator who pastes the wrong thing, and a build flag or a mirror that
// substitutes a URL. It catches the mistake, not the attacker; an attacker with
// write access to providers.yaml already has the machine.
//
// # Why loopback and link-local are refused anyway
//
//   - 169.254.169.254 and friends are cloud metadata endpoints. An adapter
//     pointed there returns credentials with no authentication at all, and the
//     response body travels days[].text → the model → the user's chat window.
//     That is a complete read-and-exfiltrate path built out of two config lines,
//     and nothing legitimate lives at that address.
//   - Private ranges and LOOPBACK are allowed. Both are legitimate and common:
//     an adapter on the same intranet, or on the same host — which is the normal
//     shape for `exec` and a perfectly ordinary one for `http`.
//
// ⚠️ Loopback was banned in the first version of this file, on the grounds that
// an adapter at 127.0.0.1 is indistinguishable from this server's own admin
// surface. That reasoning does not survive contact with the threat model:
// anybody who can edit providers.yaml already has shell access and can simply
// read ADMIN_TOKEN out of .env. The ban bought nothing and broke the case that
// matters most — running the reference adapter, and every same-host deployment.
// The end-to-end test found it on its first run, which is the kind of thing an
// httptest server inside this package could never have told us.
//
// The check is on the literal host in the config. Resolution-time rebinding is
// not covered here and cannot be — the honest answer is that the redirect
// refusal in the client is what closes the dynamic half.
// ValidateBaseURL is the exported gate for a console-supplied address.
//
// Exported on 2026-08-09 when base_url became editable. The point is that the
// SAME check runs whether the value came from the file or from a web form:
// "who set it" was never the defence — refusing link-local addresses and
// refusing to follow redirects is, and neither cares where the string came from.
func ValidateBaseURL(raw string) error { return validateBaseURL(raw) }

func validateBaseURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("format http needs base_url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("base_url is not a URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base_url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("base_url has no host")
	}
	// A token in the URL would be logged by every proxy between here and there,
	// and would land in an admin response the moment base_url is displayed.
	if u.User != nil {
		return errors.New("base_url must not carry credentials; use token_env")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		switch {
		case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
			return errors.New("base_url must not be link-local: that range holds cloud metadata endpoints, and an adapter's response reaches the model and then the user")
		case ip.IsUnspecified():
			return errors.New("base_url must name a real address")
		}
	}
	return nil
}

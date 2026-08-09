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
//     pointed there returns credentials, and the response body travels
//     days[].text → the model → the user's chat window. That is a complete read
//     and exfiltration path built out of two config lines.
//   - A private-range address is legitimate and common (an adapter on the same
//     intranet), so it is ALLOWED. Refusing it would break the deployments this
//     project cares most about. Loopback is the exception: an adapter on
//     127.0.0.1 is indistinguishable from the backend's own admin surface.
//
// The check is on the literal host in the config. Resolution-time rebinding is
// not covered here and cannot be — the honest answer is that the redirect
// refusal in the client is what closes the dynamic half.
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
		case ip.IsLoopback():
			return errors.New("base_url must not be loopback: an adapter there is indistinguishable from this server's own admin surface")
		case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
			return errors.New("base_url must not be link-local: that range holds cloud metadata endpoints, and an adapter's response reaches the model and then the user")
		case ip.IsUnspecified():
			return errors.New("base_url must name a real address")
		}
	} else if host == "localhost" {
		return errors.New("base_url must not be localhost: an adapter there is indistinguishable from this server's own admin surface")
	}
	return nil
}

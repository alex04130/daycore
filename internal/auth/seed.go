package auth

import _ "embed"

// The OAuth provider seed.
//
// Unlike the model catalog, a missing oauth.yaml is not fatal —
// LoadOAuthProviders returns an empty manager and the deployment simply has no
// social login. So this file is not written to make the process start; it is
// written to make the feature DISCOVERABLE. An operator holding one binary has
// no way to learn that Google and GitHub are presets needing only two fields,
// and a capability nobody can find is the same as one that does not exist.
//
// The stub entries are inert: LoadOAuthProviders skips any provider with an
// empty client_id, so the extracted file adds no login buttons until somebody
// fills it in. That is the property that makes writing it safe — a placeholder
// that half-registered a provider would give users a login button that fails.
//
//go:embed seed/oauth.yaml
var oauthSeed []byte

// OAuthSeed returns the starter OAuth configuration written by `daycore install`.
func OAuthSeed() []byte { return oauthSeed }

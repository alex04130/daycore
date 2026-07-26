package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// OAuthProviderConfig is one entry in config/oauth.yaml. For the built-in
// "google"/"github" presets only client_id/client_secret are required; the URLs
// and field mappings are filled in automatically. Any other provider can be
// added by supplying all fields — no code change, just config.
type OAuthProviderConfig struct {
	Name               string   `yaml:"name"`
	ClientID           string   `yaml:"client_id"`
	ClientSecret       string   `yaml:"client_secret"`
	AuthURL            string   `yaml:"auth_url"`
	TokenURL           string   `yaml:"token_url"`
	UserInfoURL        string   `yaml:"userinfo_url"`
	Scopes             []string `yaml:"scopes"`
	IDField            string   `yaml:"id_field"`
	EmailField         string   `yaml:"email_field"`
	EmailVerifiedField string   `yaml:"email_verified_field"`
	NameField          string   `yaml:"name_field"`
	AvatarField        string   `yaml:"avatar_field"`
}

type oauthFile struct {
	Providers []OAuthProviderConfig `yaml:"providers"`
}

// OAuthUser is the normalized identity returned after a successful exchange.
type OAuthUser struct {
	ProviderUserID string
	Email          string
	EmailVerified  bool // true only when the provider asserts the email is verified
	Name           string
	AvatarURL      string
}

// OAuthManager performs the authorization-code flow for all configured providers.
type OAuthManager struct {
	providers map[string]OAuthProviderConfig
	baseURL   string
	http      *http.Client
}

// LoadOAuthProviders reads the YAML file (missing file is fine → no providers),
// applies presets, and returns a manager. publicBaseURL is used to build the
// redirect URIs (publicBaseURL + /api/auth/oauth/<name>/callback).
func LoadOAuthProviders(path, publicBaseURL string) (*OAuthManager, error) {
	m := &OAuthManager{
		providers: map[string]OAuthProviderConfig{},
		baseURL:   strings.TrimRight(publicBaseURL, "/"),
		http:      &http.Client{Timeout: 15 * time.Second},
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	var f oauthFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse oauth config: %w", err)
	}
	for _, p := range f.Providers {
		if p.Name == "" || p.ClientID == "" {
			continue // skip incomplete/placeholder entries
		}
		applyPreset(&p)
		m.providers[p.Name] = p
	}
	return m, nil
}

// Providers lists the names of all enabled providers.
func (m *OAuthManager) Providers() []string {
	out := make([]string, 0, len(m.providers))
	for name := range m.providers {
		out = append(out, name)
	}
	return out
}

// Enabled reports whether a provider is configured.
func (m *OAuthManager) Enabled(name string) bool {
	_, ok := m.providers[name]
	return ok
}

// AuthCodeURL builds the provider's authorize URL for the given anti-CSRF state.
func (m *OAuthManager) AuthCodeURL(name, state string) (string, error) {
	p, ok := m.providers[name]
	if !ok {
		return "", domainUnknownProvider(name)
	}
	q := url.Values{}
	q.Set("client_id", p.ClientID)
	q.Set("redirect_uri", m.redirectURI(name))
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(p.Scopes, " "))
	q.Set("state", state)
	return p.AuthURL + "?" + q.Encode(), nil
}

// Exchange swaps an authorization code for the normalized OAuth user identity.
func (m *OAuthManager) Exchange(ctx context.Context, name, code string) (*OAuthUser, error) {
	p, ok := m.providers[name]
	if !ok {
		return nil, domainUnknownProvider(name)
	}

	token, err := m.exchangeToken(ctx, p, code)
	if err != nil {
		return nil, err
	}
	fields, err := m.fetchUserInfo(ctx, p, token)
	if err != nil {
		return nil, err
	}
	return &OAuthUser{
		ProviderUserID: stringField(fields, p.IDField),
		Email:          stringField(fields, p.EmailField),
		EmailVerified:  boolField(fields, p.EmailVerifiedField),
		Name:           stringField(fields, p.NameField),
		AvatarURL:      stringField(fields, p.AvatarField),
	}, nil
}

func (m *OAuthManager) exchangeToken(ctx context.Context, p OAuthProviderConfig, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", m.redirectURI(p.Name))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json") // GitHub returns form-encoded without this
	resp, err := m.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("oauth token exchange failed: %s", resp.Status)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", errors.New("oauth token exchange returned no access_token")
	}
	return tr.AccessToken, nil
}

func (m *OAuthManager) fetchUserInfo(ctx context.Context, p OAuthProviderConfig, token string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("oauth userinfo failed: %s", resp.Status)
	}
	var fields map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&fields); err != nil {
		return nil, err
	}
	return fields, nil
}

func (m *OAuthManager) redirectURI(name string) string {
	return m.baseURL + "/api/auth/oauth/" + name + "/callback"
}

// NewState returns a random anti-CSRF state token.
func NewState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func applyPreset(p *OAuthProviderConfig) {
	switch p.Name {
	case "google":
		setDefault(&p.AuthURL, "https://accounts.google.com/o/oauth2/v2/auth")
		setDefault(&p.TokenURL, "https://oauth2.googleapis.com/token")
		setDefault(&p.UserInfoURL, "https://openidconnect.googleapis.com/v1/userinfo")
		setDefaultSlice(&p.Scopes, []string{"openid", "email", "profile"})
		setDefault(&p.IDField, "sub")
		setDefault(&p.EmailField, "email")
		setDefault(&p.EmailVerifiedField, "email_verified")
		setDefault(&p.NameField, "name")
		setDefault(&p.AvatarField, "picture")
	case "github":
		setDefault(&p.AuthURL, "https://github.com/login/oauth/authorize")
		setDefault(&p.TokenURL, "https://github.com/login/oauth/access_token")
		setDefault(&p.UserInfoURL, "https://api.github.com/user")
		setDefaultSlice(&p.Scopes, []string{"read:user", "user:email"})
		setDefault(&p.IDField, "id")
		setDefault(&p.EmailField, "email")
		setDefault(&p.NameField, "name")
		setDefault(&p.AvatarField, "avatar_url")
	}
}

func setDefault(field *string, def string) {
	if strings.TrimSpace(*field) == "" {
		*field = def
	}
}

func setDefaultSlice(field *[]string, def []string) {
	if len(*field) == 0 {
		*field = def
	}
}

// stringField extracts a field from a decoded JSON object and stringifies it,
// handling the common numeric-id case (e.g. GitHub's integer user id).
func stringField(m map[string]any, key string) string {
	if key == "" {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// boolField extracts a boolean claim, tolerating the common string encodings
// ("true"/"1") some providers use. Missing/other types are treated as false —
// so an unverified or absent email_verified never grants account linking.
func boolField(m map[string]any, key string) bool {
	if key == "" {
		return false
	}
	switch t := m[key].(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	default:
		return false
	}
}

func domainUnknownProvider(name string) error {
	return fmt.Errorf("unknown oauth provider: %q", name)
}

package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"

	"github.com/google/uuid"
)

func init() {
	registerRoutes("admin (pairings)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/pairings", s.handleAdminPairingList)
		mux.HandleFunc("POST /api/admin/pairings", s.handleAdminPairingCreate)
		mux.HandleFunc("PUT /api/admin/pairings/{id}/roles", s.handleAdminPairingRoles)
		mux.HandleFunc("PUT /api/admin/pairings/{id}/full", s.handleAdminPairingFull)
		mux.HandleFunc("DELETE /api/admin/pairings/{id}", s.handleAdminPairingDelete)
	})
}

// Attaching an external console — a cluster manager, or somebody else's.
//
// The design and the reason the backend issues the key are in
// domain/pairing.go. This file is the credential's HTTP side: how it is
// presented, how it is verified, and the one moment it is ever shown.
//
// # The key is shown exactly once, and that is not a nicety
//
// It is generated here, hashed, and the hash is what is stored. There is no
// endpoint that reveals it again because there is nothing left to reveal —
// which is what makes "this key leaked" a question with one answer (revoke and
// re-pair) instead of two (…or did it leak from us?).
//
// ⚠️ Anything that made it retrievable would also make it stealable from the
// console screen, from a backup, and from the database browser. The browser's
// catalogue redacts secret_hash for the same reason.
//
// # A leaked key is recognisable on sight
//
// The prefix (`dcp_`) exists so a key pasted into a chat, a log or a screenshot
// is identifiable as a credential without knowing what it belongs to. Secret
// scanners key off exactly this.

const pairingHeader = "X-Pairing-Key"

var (
	keyPairingBadName = i18n.Reg("admin.pairings.bad_name", i18n.Text{
		"zh-CN": "配对名不能为空，且不超过 64 个字符",
		"en-US": "A pairing needs a name of 64 characters or fewer",
	})
	keyPairingUnknownRole = i18n.Reg("admin.pairings.unknown_role", i18n.Text{
		"zh-CN": "没有这个组：",
		"en-US": "No such group: ",
	})
	keyPairingNotFound = i18n.Reg("admin.pairings.not_found", i18n.Text{
		"zh-CN": "没有这个配对，可能已经被撤销了",
		"en-US": "No such pairing — it may already have been revoked",
	})
	keyPairingDegraded = i18n.Reg("admin.pairings.degraded", i18n.Text{
		"zh-CN": "数据库不可用，配对读不到也发不出 —— 降级模式下只有 ADMIN_TOKEN 能用，这是有意的",
		"en-US": "The database is unavailable, so pairings can be neither read nor issued — in degraded mode only ADMIN_TOKEN works, by design",
	})
	keyPairingInternal = i18n.Reg("admin.pairings.internal", i18n.Text{
		"zh-CN": "配对读写失败",
		"en-US": "Could not read or write the pairing",
	})
	keyPairingBadRequest = i18n.Reg("admin.pairings.bad_request", i18n.Text{
		"zh-CN": "请求格式错误",
		"en-US": "Malformed request",
	})
	keyPairingShownOnce = i18n.Reg("admin.pairings.shown_once", i18n.Text{
		"zh-CN": "这把钥匙只显示这一次。现在就把它搬到那个控制台去 —— 关掉这个框之后，服务端只留一个哈希，谁也拿不回来了。",
		"en-US": "This key is shown once and never again. Carry it to that console now — after this box closes the server holds only a hash, and nobody can get it back.",
	})
)

const maxPairingName = 64

// newPairingKey mints an id and a secret, and returns the credential a person
// carries away plus the hash to store.
//
// 32 bytes from crypto/rand: 256 bits, so the stored hash does not need to be
// slow. See domain.Pairing.SecretHash for why that is a statement about entropy
// rather than an excuse.
func newPairingKey() (id, presented, hash string, err error) {
	id = uuid.NewString()
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(buf)
	return id, domain.PairingKeyPrefix + "_" + id + "_" + secret, hashPairingSecret(secret), nil
}

func hashPairingSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// verifyPairing checks a presented key and returns whose it is.
//
// # The id travels in the clear, on purpose
//
// It is what turns verification into ONE indexed read. The alternative — a bare
// secret — means hashing it against every pairing on every request, so
// attaching a hundred consoles costs a hundred hashes per request. The id is
// not secret; the half after it is.
//
// ⚠️ The comparison is constant-time even though both sides are hashes of
// high-entropy input. Not because a timing attack on a 256-bit secret is
// plausible, but because `==` on a credential is the line somebody copies into
// the next check, where the input might be a password.
func (s *Server) verifyPairing(r *http.Request, presented string) (id string, full, ok bool) {
	if s.store == nil {
		// Degraded: pairings live in the database. Only the root credential
		// works there, which is the arrangement degraded boot was built around.
		return "", false, false
	}
	parts := strings.SplitN(presented, "_", 3)
	if len(parts) != 3 || parts[0] != domain.PairingKeyPrefix || parts[1] == "" || parts[2] == "" {
		return "", false, false
	}
	id, secret := parts[1], parts[2]
	ctx := r.Context()
	p, err := s.store.Pairings().Get(ctx, id)
	if err != nil || p == nil {
		return "", false, false
	}
	if subtle.ConstantTimeCompare([]byte(hashPairingSecret(secret)), []byte(p.SecretHash)) != 1 {
		return "", false, false
	}
	// Best-effort, throttled inside the store: "is this still in use" is the
	// only question anybody asks before revoking one, and it must not cost a
	// write per request to answer. A failure here is not a reason to refuse a
	// credential that verified.
	if _, terr := s.store.Pairings().TouchLastSeen(ctx, id, time.Now()); terr != nil {
		s.log.Debug("could not record pairing last-seen", "pairing", id, "err", terr)
	}
	return id, p.Full, true
}

type pairingView struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Roles       []string `json:"roles"`
	// Full is root-equivalence. Rendered as its own thing rather than folded
	// into the permission list, because "everything, including what no
	// permission reaches" is not a longer list — it is a different claim.
	Full bool `json:"full"`
	// Permissions is what those roles actually add up to. Shown because a list
	// of group names is not something anybody can judge a credential by — the
	// same reason the role editor prints damage lines rather than ids.
	Permissions []string `json:"permissions"`
	LastSeenAt  string   `json:"lastSeenAt,omitempty"`
	CreatedAt   string   `json:"createdAt,omitempty"`
}

func (s *Server) pairingView(ctx context.Context, p domain.Pairing) pairingView {
	v := pairingView{ID: p.ID, Name: p.Name, Description: p.Description, Roles: p.Roles, Full: p.Full, Permissions: []string{}}
	if perms := s.permissionsOfRoles(ctx, p.Roles); len(perms) > 0 {
		v.Permissions = perms
	}
	if !p.LastSeenAt.IsZero() {
		v.LastSeenAt = p.LastSeenAt.UTC().Format(time.RFC3339)
	}
	if !p.CreatedAt.IsZero() {
		v.CreatedAt = p.CreatedAt.UTC().Format(time.RFC3339)
	}
	return v
}

// GET /api/admin/pairings — what is attached to this deployment.
func (s *Server) handleAdminPairingList(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyPairingDegraded, locale))
		return
	}
	ps, err := s.store.Pairings().List(r.Context())
	if err != nil {
		s.log.Error("pairing list", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyPairingInternal, locale))
		return
	}
	out := make([]pairingView, 0, len(ps))
	for _, p := range ps {
		out = append(out, s.pairingView(r.Context(), p))
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"pairings": out,
		// The header the other console must send. Returned rather than
		// documented-only, because the person carrying the key is usually not
		// the person who read the spec.
		"header": pairingHeader,
	})
}

// POST /api/admin/pairings — issue a key.
//
// ⚠️ THE ONLY MOMENT THE KEY EXISTS OUTSIDE THE OTHER CONSOLE. The response
// carries it once; the server keeps a hash.
func (s *Server) handleAdminPairingCreate(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyPairingDegraded, locale))
		return
	}
	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Roles       []string `json:"roles"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", i18n.T(keyPairingBadRequest, locale))
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len([]rune(name)) > maxPairingName {
		s.writeErr(w, http.StatusBadRequest, "bad_name", i18n.T(keyPairingBadName, locale))
		return
	}
	roles, ok := s.checkRoles(w, r, body.Roles)
	if !ok {
		return
	}

	id, presented, hash, err := newPairingKey()
	if err != nil {
		s.log.Error("pairing key generation", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyPairingInternal, locale))
		return
	}
	p := &domain.Pairing{ID: id, Name: name, Description: body.Description, SecretHash: hash, Roles: roles}
	if err := s.store.Pairings().Create(r.Context(), p); err != nil {
		s.log.Error("pairing create", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyPairingInternal, locale))
		return
	}
	s.log.Warn("a pairing key was issued", "pairing", id, "name", name, "roles", roles)
	s.writeJSON(w, http.StatusOK, map[string]any{
		"pairing": s.pairingView(r.Context(), *p),
		"key":     presented,
		"header":  pairingHeader,
		"notice":  i18n.T(keyPairingShownOnce, locale),
	})
}

// PUT /api/admin/pairings/{id}/roles — change what an attached console may do.
func (s *Server) handleAdminPairingRoles(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyPairingDegraded, locale))
		return
	}
	var body struct {
		Roles []string `json:"roles"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", i18n.T(keyPairingBadRequest, locale))
		return
	}
	roles, ok := s.checkRoles(w, r, body.Roles)
	if !ok {
		return
	}
	err := s.store.Pairings().SetRoles(r.Context(), r.PathValue("id"), roles)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "not_found", i18n.T(keyPairingNotFound, locale))
		return
	}
	if err != nil {
		s.log.Error("pairing roles", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyPairingInternal, locale))
		return
	}
	s.log.Warn("a pairing's groups changed", "pairing", r.PathValue("id"), "roles", roles)
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "roles": roles})
}

// DELETE /api/admin/pairings/{id} — detach a console.
//
// Deleting is the revocation: there is no disabled state, because a credential
// that is "off" is a credential somebody can turn back on without deciding to.
func (s *Server) handleAdminPairingDelete(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyPairingDegraded, locale))
		return
	}
	id := r.PathValue("id")
	if err := s.store.Pairings().Delete(r.Context(), id); err != nil {
		s.log.Error("pairing delete", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyPairingInternal, locale))
		return
	}
	s.log.Warn("a pairing was revoked", "pairing", id)
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// checkRoles validates group names against what exists, and enforces the same
// escalation boundary that applies to people.
//
// ⚠️ Attaching a console to a group that carries permissions is granting those
// permissions to a program on another machine. It needs roles.edit for exactly
// the reason putting a PERSON into an admin group does: without it,
// pairings.manage alone would be a way to hand out anything that already
// exists.
func (s *Server) checkRoles(w http.ResponseWriter, r *http.Request, want []string) ([]string, bool) {
	locale := s.requestLocale(r)
	defined, err := s.store.Roles().ListRoles(r.Context())
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyPairingInternal, locale))
		return nil, false
	}
	byName := map[string]domain.Role{}
	for _, role := range defined {
		byName[role.Name] = role
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(want))
	admin := false
	for _, n := range want {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		role, ok := byName[n]
		if !ok {
			s.writeErr(w, http.StatusNotFound, "unknown_role", i18n.T(keyPairingUnknownRole, locale)+n)
			return nil, false
		}
		seen[n] = true
		out = append(out, n)
		if role.IsAdminRole() {
			admin = true
		}
	}
	sort.Strings(out)
	if admin && !s.authorize(r, PermRolesEdit) {
		s.writeErr(w, http.StatusForbidden, "needs_roles_edit", i18n.T(keyAdminRoleNeedsEdit, locale))
		return nil, false
	}
	return out, true
}

// PUT /api/admin/pairings/{id}/full — make an attached console root-equivalent.
//
// ⚠️ ROOT CREDENTIAL ONLY (permRoot), like the owner mark on a user, and for
// the same reason: delegating root has to be an act somebody performed WITH the
// root credential, or the set of things that can do everything grows without
// anybody deciding it should.
//
// This is what makes a cluster console usable — the author's point that a
// console which cannot reach the root-only routes manages nothing, and that an
// operator blocked here will just put ADMIN_TOKEN on the other machine instead,
// which is strictly worse. The full argument is on domain.Pairing.Full.
func (s *Server) handleAdminPairingFull(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyPairingDegraded, locale))
		return
	}
	var body struct {
		Full *bool `json:"full"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Full == nil {
		// A missing field is refused rather than read as false. "Grant root" and
		// "send an empty body" must not be the same request.
		s.writeErr(w, http.StatusBadRequest, "bad_request", i18n.T(keyPairingBadRequest, locale))
		return
	}
	err := s.store.Pairings().SetFull(r.Context(), r.PathValue("id"), *body.Full)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "not_found", i18n.T(keyPairingNotFound, locale))
		return
	}
	if err != nil {
		s.log.Error("pairing full", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyPairingInternal, locale))
		return
	}
	s.log.Warn("a pairing's root-equivalence changed", "pairing", r.PathValue("id"), "full", *body.Full)
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "full": *body.Full})
}

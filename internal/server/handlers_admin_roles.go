package server

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"daycore/internal/domain"
	"daycore/internal/i18n"
)

func init() {
	registerRoutes("admin (roles)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/permissions", s.handleAdminPermissions)
		mux.HandleFunc("GET /api/admin/roles", s.handleAdminRoleList)
		mux.HandleFunc("PUT /api/admin/roles/{name}", s.handleAdminRolePut)
		mux.HandleFunc("DELETE /api/admin/roles/{name}", s.handleAdminRoleDelete)
		mux.HandleFunc("PUT /api/admin/users/{id}/roles", s.handleAdminUserRolesPut)
		mux.HandleFunc("PUT /api/admin/users/{id}/owner", s.handleAdminUserOwnerPut)
	})
}

// Granting: who is in which group, and what each group means.
//
// # The escalation boundary is the split between two routes here
//
//	PUT /api/admin/roles/{name}       roles.edit     what a group MEANS
//	PUT /api/admin/users/{id}/roles   users.assign   who is IN a group
//
// Holding both is equivalent to holding everything — make a group all-powerful,
// then join it. Split, the commercial operation (moving a customer between
// tiers) can be delegated to support staff and can never leave them with more
// than they started with.
//
// That guarantee is not free: assignment has to refuse groups that carry
// permissions, or "assign only" quietly becomes "grant anything that already
// exists". So handleAdminUserRolesPut ADDITIONALLY requires roles.edit when any
// role being joined or left carries a permission — evaluated against the role
// as it is at that moment, never against a stored flag that could be stale.
//
// # The owner mark is set from outside the model it governs
//
// PUT /api/admin/users/{id}/owner is root-credential-only. Not "owner-only",
// not "roles.edit" — the ADMIN_TOKEN in the environment and nothing else.
//
// Three reasons, and the third is the one that matters:
//
//  1. **The first one has to come from somewhere.** The author ruled out
//     "whoever registers first becomes admin", which is the usual answer and is
//     a race with the internet on a fresh deployment. So the bootstrap is: the
//     operator, holding the token they configured, marks a person.
//  2. **It is the recovery path.** Every other route can be locked out — the
//     last owner deleted, the role rows wrong, somebody's account gone. This
//     one is reachable as long as the process has its environment.
//  3. **It cannot be delegated by accident.** An owner can still hand out
//     roles.edit, which is owner-equivalent in effect — that is stated plainly
//     in permissions.go. What they cannot do is create another break-glass
//     holder, so the set of people who can rescue a deployment stays a decision
//     somebody made at the console with the token in hand.
//
// ⚠️ There is deliberately no "cannot remove the last owner" rule. The author's
// decision on break-glass was "可授可撤" — it grants and it revokes — and a
// guard against removing the last owner would only be protecting against a
// state the root credential can undo in one request. What the console does
// instead is say how many owners are left.
var (
	keyAdminRoleName = i18n.Reg("admin.roles.bad_name", i18n.Text{
		"zh-CN": "组名不能为空，且不超过 64 个字符",
		"en-US": "A group name is required and must be 64 characters or fewer",
	})
	keyAdminRoleUnknownPerm = i18n.Reg("admin.roles.unknown_permission", i18n.Text{
		"zh-CN": "这个部署里没有这一项权限：",
		"en-US": "This deployment has no such permission: ",
	})
	keyAdminRoleNotFound = i18n.Reg("admin.roles.not_found", i18n.Text{
		"zh-CN": "没有这个组",
		"en-US": "No such group",
	})
	keyAdminRoleNeedsEdit = i18n.Reg("admin.roles.needs_edit", i18n.Text{
		"zh-CN": "这个组带管理员权限，动它的成员还需要「改组权限」这一项",
		"en-US": "This group carries admin permissions; changing its members also needs the roles.edit permission",
	})
	keyAdminRoleUserNotFound = i18n.Reg("admin.roles.user_not_found", i18n.Text{
		"zh-CN": "没有这个用户",
		"en-US": "No such user",
	})
	keyAdminRoleDegraded = i18n.Reg("admin.roles.degraded", i18n.Text{
		"zh-CN": "数据库不可用，组和成员改不了",
		"en-US": "The database is unavailable, so groups and membership cannot be changed",
	})
	keyAdminRoleBadRequest = i18n.Reg("admin.roles.bad_request", i18n.Text{
		"zh-CN": "请求格式错误",
		"en-US": "Malformed request",
	})
	keyAdminRoleInternal = i18n.Reg("admin.roles.internal", i18n.Text{
		"zh-CN": "组信息读写失败",
		"en-US": "Could not read or write the group",
	})
)

// maxRoleName is a storage bound, not a style rule: the name is a primary key
// on three SQL engines and MySQL's utf8mb4 index limit is what decides it.
const maxRoleName = 64

// GET /api/admin/permissions — the registry, with the sentence to grant from.
func (s *Server) handleAdminPermissions(w http.ResponseWriter, r *http.Request) {
	all := Permissions()
	out := make([]map[string]string, 0, len(all))
	for _, p := range all {
		out = append(out, map[string]string{"id": p.ID, "damage": p.Damage})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"permissions": out})
}

type adminRoleView struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	// Members is who is in it. Returned with the list rather than behind a
	// second call because the question a role list is read to answer is "who can
	// do this", and a screen that needs N+1 requests to answer it will be built
	// to not answer it.
	Members []string `json:"members"`
	// Admin is Role.IsAdminRole — derived here too, so the console does not have
	// to re-derive the escalation rule and get it slightly different.
	Admin bool `json:"admin"`
}

// GET /api/admin/roles — every group, what it grants, and who is in it.
func (s *Server) handleAdminRoleList(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyAdminRoleDegraded, locale))
		return
	}
	ctx := r.Context()
	roles, err := s.store.Roles().ListRoles(ctx)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminRoleInternal, locale))
		return
	}
	out := make([]adminRoleView, 0, len(roles))
	for _, role := range roles {
		members, err := s.store.Roles().MembersOf(ctx, role.Name)
		if err != nil {
			members = []string{}
		}
		out = append(out, adminRoleView{
			Name:        role.Name,
			Description: role.Description,
			Permissions: role.Permissions,
			Members:     members,
			Admin:       role.IsAdminRole(),
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"roles": out})
}

// PUT /api/admin/roles/{name} — create or replace what a group grants.
//
// Replace, not merge. A merge endpoint cannot express "take this away", and the
// console's editor is a set of switches whose whole content is the answer — so
// the body IS the new permission set.
func (s *Server) handleAdminRolePut(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyAdminRoleDegraded, locale))
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || len([]rune(name)) > maxRoleName {
		s.writeErr(w, http.StatusBadRequest, "bad_name", i18n.T(keyAdminRoleName, locale))
		return
	}
	var body struct {
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", i18n.T(keyAdminRoleBadRequest, locale))
		return
	}
	// An unknown permission id is refused rather than stored. A stored id that
	// nothing matches is a role that grants LESS than its own definition says —
	// and the definition is what the person granting it reads.
	perms := make([]string, 0, len(body.Permissions))
	seen := map[string]bool{}
	for _, p := range body.Permissions {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if !PermissionExists(p) {
			s.writeErr(w, http.StatusBadRequest, "unknown_permission", i18n.T(keyAdminRoleUnknownPerm, locale)+p)
			return
		}
		seen[p] = true
		perms = append(perms, p)
	}
	sort.Strings(perms)
	if err := s.store.Roles().UpsertRole(r.Context(), domain.Role{
		Name:        name,
		Description: body.Description,
		Permissions: perms,
	}); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminRoleInternal, locale))
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "permissions": perms})
}

// DELETE /api/admin/roles/{name} — remove a group and everybody's membership in it.
func (s *Server) handleAdminRoleDelete(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyAdminRoleDegraded, locale))
		return
	}
	if err := s.store.Roles().DeleteRole(r.Context(), r.PathValue("name")); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminRoleInternal, locale))
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// PUT /api/admin/users/{id}/roles — set which groups one person is in.
//
// The whole set, for the same reason as the role editor: a console showing
// checkboxes has the answer already, and an add/remove pair invites two
// requests that can half-apply.
func (s *Server) handleAdminUserRolesPut(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyAdminRoleDegraded, locale))
		return
	}
	ctx := r.Context()
	userID := r.PathValue("id")
	user, err := s.store.Users().GetByID(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) || user == nil {
		s.writeErr(w, http.StatusNotFound, "user_not_found", i18n.T(keyAdminRoleUserNotFound, locale))
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminRoleInternal, locale))
		return
	}
	var body struct {
		Roles []string `json:"roles"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", i18n.T(keyAdminRoleBadRequest, locale))
		return
	}

	defined, err := s.store.Roles().ListRoles(ctx)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminRoleInternal, locale))
		return
	}
	byName := map[string]domain.Role{}
	for _, role := range defined {
		byName[role.Name] = role
	}

	want := map[string]bool{}
	for _, n := range body.Roles {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := byName[n]; !ok {
			s.writeErr(w, http.StatusNotFound, "role_not_found", i18n.T(keyAdminRoleNotFound, locale))
			return
		}
		want[n] = true
	}
	current, err := s.store.Roles().RolesOf(ctx, userID)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminRoleInternal, locale))
		return
	}
	has := map[string]bool{}
	for _, n := range current {
		has[n] = true
	}

	// The escalation boundary, enforced on the DIFFERENCE rather than on the
	// whole set. Leaving somebody in an admin group they are already in is not
	// an escalation and must not require the higher permission — otherwise
	// support staff cannot change a customer's tier once an administrator has
	// ever been put in an admin group, which is the ordinary case.
	//
	// Both directions count: removing somebody from an admin group is a change
	// to who can do things, and "assign only" must not be able to make it.
	touchesAdmin := false
	for n := range want {
		if !has[n] && byName[n].IsAdminRole() {
			touchesAdmin = true
		}
	}
	for n := range has {
		if !want[n] {
			// A membership row whose role definition is gone cannot escalate
			// anything, so removing it is always allowed.
			if role, ok := byName[n]; ok && role.IsAdminRole() {
				touchesAdmin = true
			}
		}
	}
	if touchesAdmin && !s.authorize(r, PermRolesEdit) {
		s.writeErr(w, http.StatusForbidden, "needs_roles_edit", i18n.T(keyAdminRoleNeedsEdit, locale))
		return
	}

	for n := range want {
		if !has[n] {
			if err := s.store.Roles().AddMember(ctx, n, userID); err != nil {
				s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminRoleInternal, locale))
				return
			}
		}
	}
	for n := range has {
		if !want[n] {
			if err := s.store.Roles().RemoveMember(ctx, n, userID); err != nil {
				s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminRoleInternal, locale))
				return
			}
		}
	}
	out := make([]string, 0, len(want))
	for n := range want {
		out = append(out, n)
	}
	sort.Strings(out)
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "userId": userID, "roles": out})
}

// PUT /api/admin/users/{id}/owner — mark or unmark a super-administrator.
//
// Root credential only; see the file comment for the three reasons. The gate
// already refused everybody else, so there is no credential check here — which
// is exactly the property admin_gate.go exists to provide, and it is worth
// noticing that the most dangerous route in the console has no auth code in it.
func (s *Server) handleAdminUserOwnerPut(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyAdminRoleDegraded, locale))
		return
	}
	ctx := r.Context()
	userID := r.PathValue("id")
	user, err := s.store.Users().GetByID(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) || user == nil {
		s.writeErr(w, http.StatusNotFound, "user_not_found", i18n.T(keyAdminRoleUserNotFound, locale))
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminRoleInternal, locale))
		return
	}
	var body struct {
		Owner *bool `json:"owner"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Owner == nil {
		// A missing field is refused rather than read as false. "Set owner" and
		// "send an empty body" must not be the same request when the difference
		// is somebody losing the last break-glass mark.
		s.writeErr(w, http.StatusBadRequest, "bad_request", i18n.T(keyAdminRoleBadRequest, locale))
		return
	}
	if err := s.store.Users().SetOwner(ctx, userID, *body.Owner); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminRoleInternal, locale))
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "userId": userID, "owner": *body.Owner})
}

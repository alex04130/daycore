package server

import (
	"fmt"
	"sort"
	"sync"
)

// The console permission list.
//
// # Named by consequence, not by section and not by HTTP verb
//
// The unit is "a set of endpoints whose consequences are the same", and the
// only test for whether two things are one permission or two is: **is there a
// real person you would give A and not B?** If there is not, they are one.
//
// Both obvious alternatives were tried against the real routes and both fail:
//
//   - **One per console section.** `GET /api/admin/db/export` (the whole
//     database, in one file) and `GET /api/admin/db/table/{name}` (one page of
//     one table) sit in the same section and differ by an order of magnitude.
//     Nobody grants those together.
//   - **Read/write per section.** The verb and the consequence point opposite
//     ways in two places: `GET /api/admin/db/export` is a read and a total
//     disclosure, while `POST /api/admin/models/{id}/test` is a write that only
//     spends a few tokens. Splitting by verb hands the most dangerous capability
//     to whoever was given "read only".
//
// # The two that are split because splitting them IS the escalation boundary
//
// PermRolesEdit ("change what a role can do") and PermUsersAssign ("put a user
// in a group that carries no admin permission") are separate on purpose:
// holding both is equivalent to holding everything, because you can make a
// group all-powerful and then join it. Split, support staff can move a customer
// between tiers and that action can never leave them with more than they had.
//
// # PermRolesEdit is effectively owner
//
// Say it plainly, because it reads like an ordinary line in the list: whoever
// can edit a role's permissions can give themselves anything. That is true of
// every RBAC and it is not a flaw here — but granting it is the same decision
// as making somebody an owner, and the console has to say so next to the
// switch.
const (
	// Overview. Deliberately its own permission and deliberately NOT the one
	// that reveals a driver error — that string can carry a DSN password, so it
	// is gated on the root credential rather than on any permission at all.
	PermOverview = "overview.read"

	PermConfigRead  = "config.read"
	PermConfigWrite = "config.write"

	PermProvidersRead  = "providers.read"
	PermProvidersWrite = "providers.write"

	PermModelsRead = "models.read"
	// Separate from models.read because it spends money on somebody else's key
	// and reaches out of the network. Reading the catalog does neither.
	PermModelsTest = "models.test"

	PermOAuthRead = "oauth.read"

	PermPromptsRead = "prompts.read"
	// Editing a prompt changes what the assistant says to every user of this
	// deployment. It is the highest-consequence write that is not a data
	// operation, which is why it is not folded into any "write" bucket.
	PermPromptsWrite = "prompts.write"

	// AI call logs. Metadata only today (model, usage, timing) — if user content
	// ever lands in that table this permission has to be re-examined against the
	// content permissions below.
	PermAILogsRead = "ailogs.read"

	PermUsersRead = "users.read"
	// Deleting a user is irreversible and cascades. Nothing else in the console
	// destroys a person's data.
	PermUsersDelete = "users.delete"
	// Assigning a user to a group that grants no admin permission. This is the
	// commercial operation — moving a customer between tiers — and it is
	// deliberately reachable without any power to change what those tiers mean.
	PermUsersAssign = "users.assign"

	// Changing what a role can do. See the note above: this is owner-equivalent.
	PermRolesEdit = "roles.edit"

	// The database browser, split three ways because the author's decision that
	// user content is *assignable* only means something if it can be assigned
	// separately.
	//
	// PermDBOperational is the tables an operator debugs with: sessions,
	// operation_logs, job_runs. PermDBUserContent is chat_messages,
	// mood_checkins, memory_facts — the data docs/STRATEGY.md puts at the same
	// level as the companion boundaries. There is very obviously a real person
	// you would give the first and not the second.
	PermDBOperational = "db.operational"
	PermDBUserContent = "db.user_content"
	// One row, bypassing every business rule. Its own permission because it is
	// the only write in the browser.
	PermDBDeleteRow = "db.delete_row"
	// Whole-database in and out. Export is a total disclosure of everything the
	// two browse permissions cover; import can overwrite it.
	PermDBExport = "db.export"
	PermDBImport = "db.import"
)

// Permission is one entry in the list, with the sentence an operator reads
// before granting it.
type Permission struct {
	ID string
	// Damage says what somebody holding this can do, in terms of consequence
	// rather than endpoints. It is required and checked for length: "manage
	// config" is not a sentence anybody can make a decision from.
	Damage string
}

var (
	permMu   sync.RWMutex
	permList = map[string]Permission{}
)

// registerPerm adds a permission to the list. Called from init() next to the
// routes that use it, the same placement registerRevert uses.
func registerPerm(id, damage string) string {
	permMu.Lock()
	defer permMu.Unlock()
	if _, dup := permList[id]; dup {
		panic("server: duplicate permission " + id)
	}
	permList[id] = Permission{ID: id, Damage: damage}
	return id
}

// Permissions returns the full list, sorted. The console renders it; the gate
// checks it.
func Permissions() []Permission {
	permMu.RLock()
	defer permMu.RUnlock()
	out := make([]Permission, 0, len(permList))
	for _, p := range permList {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// PermissionExists reports whether an id is a real permission, so a grant of a
// misspelled one can be refused rather than stored and silently never matched.
func PermissionExists(id string) bool {
	permMu.RLock()
	defer permMu.RUnlock()
	_, ok := permList[id]
	return ok
}

func init() {
	registerPerm(PermOverview,
		"看这个部署的运行状态：版本、启动时间、数据库是否可达。⚠️ 带密码的驱动错误不在其中 —— 那个只给 ADMIN_TOKEN")
	registerPerm(PermConfigRead,
		"看全部配置项的当前值（密钥只显示配没配，不显示值）")
	registerPerm(PermConfigWrite,
		"改运行时配置。改错一个阈值会影响每一次 AI 调用；密钥与启动期项改不了")
	registerPerm(PermProvidersRead,
		"看天气与搜索的源、它们的健康状态、以及适配层自报的信息")
	registerPerm(PermProvidersWrite,
		"启停源、改适配层地址、改并批准给模型的描述。⚠️ 批准过的描述会进每一次对话的系统提示词")
	registerPerm(PermModelsRead,
		"看模型目录：接口地址、上游模型名、key 配没配（值不显示）")
	registerPerm(PermModelsTest,
		"真的调一次模型。会花这个部署的钱，也会向外发一次网络请求")
	registerPerm(PermOAuthRead,
		"看第三方登录配了哪些、以及要在厂商后台填的回调地址（client_secret 不显示）")
	registerPerm(PermPromptsRead,
		"看全部提示词模板，包括运维改写过的覆盖。提示词是助手行为的定义，读它等于读这个部署的全部规则")
	registerPerm(PermPromptsWrite,
		"改提示词。这直接改变助手对**每一个用户**说的话，是控制台里影响面最大的非数据写操作")
	registerPerm(PermAILogsRead,
		"看 AI 调用记录：模型、用量、耗时、成败。今天不含对话正文")
	registerPerm(PermUsersRead,
		"看用户列表：邮箱、注册时间、所在的组。不含任何人写下的内容")
	registerPerm(PermUsersDelete,
		"删除用户。不可撤销，且会连带删掉那个人的数据")
	// ⚠️ users.assign / roles.edit / db.user_content 有意暂不注册。
	//
	// 它们的端点还没写，而这个文件的反向闸门要求「每个注册过的权限至少被一条
	// 路由引用」—— 那条闸门防的正是「控制台上一个什么都不授予的开关」，而一个
	// 开关如果读起来像保护、实际什么都不管，比没有它更糟。
	//
	// 所以常量先定义好（形状已经裁决完，见 docs/AUTH.md），注册与路由同批。
	registerPerm(PermDBOperational,
		"浏览运维类的表：会话、操作日志、任务场次。不含任何用户写下的内容")
	registerPerm(PermDBDeleteRow,
		"直接删一行，绕过所有业务规则。不走撤销日志，删掉就没了")
	registerPerm(PermDBExport,
		"导出整个数据库。⚠️ 一次点击拿走上面两条浏览权限覆盖的全部内容")
	registerPerm(PermDBImport,
		"导入数据库。可能覆盖现有数据")
}

// routePermissions maps a route pattern to the permission it requires.
//
// # Forgetting to mark a route means DENY (author's decision)
//
// An unmarked /api/admin/ route is refused for everybody except the root
// credential. The alternative — unmarked means open — turns "somebody added an
// endpoint and forgot" into a silent hole, and this repository has measured
// exactly that failure once already: thirteen admin routes sat in a test
// exemption list, and deleting an authorisation check from one of them left the
// whole suite green.
//
// The gate in permissions_test.go requires every registered /api/admin/ route
// to appear here, so "forgot" is a red test rather than a quiet 403 discovered
// in production.
var routePermissions = map[string]string{
	// Session: the credential exchange itself. It cannot require a permission,
	// because it is what somebody uses before they have any.
	"POST /api/admin/session":   "",
	"DELETE /api/admin/session": "",

	"GET /api/admin/health": PermOverview,
	"GET /api/admin/stats":  PermOverview,

	"GET /api/admin/config": PermConfigRead,
	"PUT /api/admin/config": PermConfigWrite,

	"GET /api/admin/providers": PermProvidersRead,
	"PUT /api/admin/providers": PermProvidersWrite,

	"GET /api/admin/models":            PermModelsRead,
	"POST /api/admin/models/{id}/test": PermModelsTest,
	"GET /api/admin/oauth":             PermOAuthRead,

	"GET /api/admin/prompts":       PermPromptsRead,
	"GET /api/admin/prompts/{key}": PermPromptsRead,
	"PUT /api/admin/prompts/{key}": PermPromptsWrite,

	"GET /api/admin/ailogs": PermAILogsRead,

	"GET /api/admin/users":         PermUsersRead,
	"DELETE /api/admin/users/{id}": PermUsersDelete,

	// The database browser. Which of the two browse permissions applies is
	// decided per table INSIDE the handler, because the route pattern cannot
	// tell chat_messages from operation_logs. The route-level requirement is
	// the weaker of the two; the handler tightens it.
	"GET /api/admin/db/tables":               PermDBOperational,
	"GET /api/admin/db/table/{name}":         PermDBOperational,
	"DELETE /api/admin/db/table/{name}/{id}": PermDBDeleteRow,
	"GET /api/admin/db/export":               PermDBExport,
	"GET /api/admin/db/backup":               PermDBExport,
	"POST /api/admin/db/import":              PermDBImport,
}

// PermissionFor returns the permission a route requires, and whether the route
// was declared at all. An undeclared route is denied — see the comment above.
func PermissionFor(pattern string) (perm string, declared bool) {
	p, ok := routePermissions[pattern]
	return p, ok
}

func mustPermission(pattern string) string {
	p, ok := routePermissions[pattern]
	if !ok {
		panic(fmt.Sprintf("server: %s has no permission declared; unmarked admin routes are denied", pattern))
	}
	return p
}

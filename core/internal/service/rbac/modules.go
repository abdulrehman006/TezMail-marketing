package rbac

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// ModuleAction is the fixed action used for per-module (page-level) permissions.
// The RBAC model in this product is module/feature based rather than per-CRUD,
// so every seeded permission uses this single action.
const ModuleAction = "access"

// Module is a logical, user-facing feature area gated by RBAC. Each module
// corresponds to one or more API path prefixes and to a menu section in the
// frontend. This slice is the single source of truth shared by the permission
// seeder, the enforcement middleware and the management API.
type Module struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Modules lists every gated feature area, in the order shown in the role editor.
var Modules = []Module{
	{"dashboard", "Dashboard", "View the dashboard and overview statistics"},
	{"campaigns", "Campaigns", "Create, send and manage email campaigns"},
	{"templates", "Email Templates", "Create and manage email templates"},
	{"contacts", "Contacts & Lists", "Manage contacts, subscriber lists and tags"},
	{"mailboxes", "Mailboxes", "Create and manage mailboxes"},
	{"domains", "Domains & SSL", "Manage sending domains and SSL certificates"},
	{"mail_services", "Mail Services", "Manage relay, forwarding, BCC and the mail queue"},
	{"ai", "AI Assistant", "Use the AI assistant and template generator"},
	{"settings", "Settings", "Change system settings and configuration"},
	{"logs", "Operation Logs", "View operation and audit logs"},
	{"system", "Access Control", "Manage accounts, roles and permissions"},
}

// segmentToModule maps the first path segment after /api/ to a logical module key.
// Any API segment not listed here is treated as ungated (allowed for any
// authenticated user) so that utility endpoints can never lock users out.
var segmentToModule = map[string]string{
	"overview":           "dashboard",
	"batch_mail":         "campaigns",
	"subscription":       "campaigns",
	"email_template":     "templates",
	"contact":            "contacts",
	"tags":               "contacts",
	"subscribe":          "contacts",
	"abnormal_recipient": "contacts",
	"mailbox":            "mailboxes",
	"domains":            "domains",
	"ssl":                "domains",
	"multi_ip_domain":    "domains",
	"domain_blocklist":   "domains",
	"services":           "mail_services",
	"mail_forward":       "mail_services",
	"mail_bcc":           "mail_services",
	"postfix_queue":      "mail_services",
	"relay":              "mail_services",
	"askai":              "ai",
	"docker_api":         "settings",
	"settings":           "settings",
	"file":               "settings",
	"operation_log":      "logs",
	"account":            "system",
	"role":               "system",
	"permission":         "system",
}

// selfServiceSegments are API paths every authenticated user may call
// regardless of role: the auth lifecycle plus pre-login helpers. They are
// never gated by module permissions.
var selfServiceSegments = map[string]struct{}{
	"login":             {},
	"logout":            {},
	"refresh-token":     {},
	"current-user":      {},
	"get_validate_code": {},
	"languages":         {},
}

// selfServicePaths are exact API paths any authenticated user may call even
// though their segment maps to a gated module. /account/password lives under
// the "system" module for management, but a user must always be able to change
// their OWN password — the controller enforces caller==target for non-admins.
var selfServicePaths = map[string]struct{}{
	"/api/account/password": {},
}

// firstAPISegment returns the <seg> in /api/<seg>/... (or "" if not an API path).
func firstAPISegment(path string) string {
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 3)
	if len(parts) < 2 || parts[0] != "api" {
		return ""
	}
	return parts[1]
}

// ModuleForPath resolves an API request path to the logical module that gates
// it. selfServe is true for auth/public paths that must always be allowed;
// when selfServe is false and module is "", the path is not gated (allow).
func ModuleForPath(path string) (module string, selfServe bool) {
	if _, ok := selfServicePaths[path]; ok {
		return "", true
	}
	seg := firstAPISegment(path)
	if seg == "" {
		// Not an /api/ path — never reached by the API middleware, but be safe.
		return "", true
	}
	if _, ok := selfServiceSegments[seg]; ok {
		return "", true
	}
	return segmentToModule[seg], false
}

// SeedModulePermissions inserts one permission row per logical module if it is
// not already present, and keeps the name/description current otherwise.
// Idempotent — safe to call on every startup.
func SeedModulePermissions(ctx context.Context) error {
	now := time.Now().Unix()
	for _, m := range Modules {
		exists, err := g.DB().Model("permission").
			Where("module = ?", m.Key).
			Where("action = ?", ModuleAction).
			Where("resource = ?", m.Key).
			Exist()
		if err != nil {
			return err
		}
		if exists {
			_, err = g.DB().Model("permission").
				Data(g.Map{
					"permission_name": m.Name,
					"description":     m.Description,
					"status":          1,
					"update_time":     now,
				}).
				Where("module = ?", m.Key).
				Where("action = ?", ModuleAction).
				Where("resource = ?", m.Key).
				Update()
			if err != nil {
				return err
			}
			continue
		}
		_, err = g.DB().Model("permission").Insert(g.Map{
			"permission_name": m.Name,
			"description":     m.Description,
			"module":          m.Key,
			"action":          ModuleAction,
			"resource":        m.Key,
			"status":          1,
			"create_time":     now,
			"update_time":     now,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ModulePermissionIds returns the permission_id of every seeded module
// permission — used to grant the full set to the admin role.
func ModulePermissionIds(ctx context.Context) ([]int64, error) {
	vals, err := g.DB().Model("permission").
		Where("action = ?", ModuleAction).
		Array("permission_id")
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(vals))
	for _, v := range vals {
		ids = append(ids, v.Int64())
	}
	return ids, nil
}

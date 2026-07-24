package middlewares

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/gconv"

	"billionmail-core/internal/service/rbac"
)

// RBACMiddleware handles per-module permission verification for API requests.
//
// The model is module/feature based (see rbac.Modules): every request path is
// resolved to a logical module, and the caller must hold at least one active
// permission in that module through one of their roles. The built-in "admin"
// role bypasses all checks. Auth-lifecycle and utility endpoints are never
// gated (see rbac.ModuleForPath).
type RBACMiddleware struct {
	PermissionService rbac.IPermission
}

// NewRBACMiddleware creates a new RBACMiddleware
func NewRBACMiddleware() *RBACMiddleware {
	return &RBACMiddleware{
		PermissionService: rbac.Permission(),
	}
}

// PermissionCheck enforces module access for the current request. It runs after
// the JWT middleware, which populates ctx "accountId" and "roles" ([]string).
func (m *RBACMiddleware) PermissionCheck(r *ghttp.Request) {
	module, selfServe := rbac.ModuleForPath(r.URL.Path)

	// Auth lifecycle + pre-login helpers are always allowed.
	if selfServe {
		r.Middleware.Next()
		return
	}

	// If the JWT middleware did not set an account id, this is a public route it
	// deliberately skipped (unsubscribe, subscribe/submit, the send API, the
	// language switch, etc.). JWT already 401s protected routes that lack a
	// valid token before this middleware runs, so reaching here with no account
	// id means the route is public — let it through untouched. (Returning 401
	// here would break every public endpoint.)
	accountIdVar := r.GetCtxVar("accountId")
	if accountIdVar == nil {
		r.Middleware.Next()
		return
	}
	accountId := gconv.Int64(accountIdVar)

	// The admin role has unrestricted access.
	roles := r.GetCtxVar("roles", []string{}).Strings()
	for _, role := range roles {
		if role == "admin" {
			r.Middleware.Next()
			return
		}
	}

	// Utility endpoints not mapped to any module are open to any authenticated
	// user — never fail-closed on an unknown path, that would lock people out.
	if module == "" {
		r.Middleware.Next()
		return
	}

	hasPermission, err := m.PermissionService.CheckModule(r.GetCtx(), accountId, module)
	if err != nil {
		g.Log().Error(r.GetCtx(), "[RBAC] permission check error:", err)
		r.Response.WriteJson(g.Map{
			"code": 500,
			"msg":  "Error checking permissions",
		})
		r.Exit()
		return
	}

	if !hasPermission {
		g.Log().Warning(r.GetCtx(),
			fmt.Sprintf("[RBAC] deny account=%d module=%q path=%q", accountId, module, r.URL.Path))
		r.Response.WriteJson(g.Map{
			"code": 403,
			"msg":  "Insufficient permissions",
		})
		r.Exit()
		return
	}

	r.Middleware.Next()
}

// HasModule reports whether the current account may access the given logical
// module. Admins always may. Usable from controllers for fine-grained checks.
func HasModule(ctx context.Context, module string) bool {
	for _, role := range rbac.GetCurrentRoles(ctx) {
		if role == "admin" {
			return true
		}
	}
	accountId := rbac.GetCurrentAccountId(ctx)
	if accountId == 0 {
		return false
	}
	ok, err := rbac.Permission().CheckModule(ctx, accountId, module)
	if err != nil {
		g.Log().Error(ctx, "[RBAC] HasModule check error:", err)
		return false
	}
	return ok
}

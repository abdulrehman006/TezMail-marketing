package rbac

import (
	"billionmail-core/internal/service/public"
	"strings"
	"sync"
)

// ============================================================================
// RBAC FEATURE FLAG
// ============================================================================
//
// Role-Based Access Control is an OPTIONAL product feature. It is gated by the
// RBAC_ENABLED environment variable (read from the docker .env file) and
// defaults to OFF, so the product behaves EXACTLY as it did before RBAC existed
// unless an operator explicitly turns it on.
//
// This is the single toggle for the whole feature. Everything RBAC-specific
// checks IsEnabled() at a well-marked "RBAC FEATURE FLAG" comment:
//
//   • cmd.go .......................... registers the enforcement middleware
//                                        only when enabled.
//   • database_initialization/rbac.go .. seeds the 11 module permissions and
//                                        grants them to the admin role only
//                                        when enabled.
//   • controller/rbac (login, current-user) .. returns rbacEnabled to the
//                                        frontend so the UI can react.
//   • frontend store/Sidebar ........... hides the Accounts/Roles UI and turns
//                                        off menu gating when disabled.
//
// WHEN OFF (default):
//   - no enforcement middleware is registered (zero per-request cost),
//   - module permissions are not seeded,
//   - the frontend hides Accounts/Roles and applies no menu gating.
//   => Adding or removing the feature has NO effect on the rest of the product
//      (SES, mailboxes, campaigns, domains, settings, etc.).
//
// WHEN ON:
//   - the per-module enforcement middleware runs,
//   - module permissions are seeded and granted to admin,
//   - the Accounts/Roles management UI appears and menus are gated by role.
//
// TO TOGGLE:
//   set RBAC_ENABLED=true (or false) in the .env file, then restart the core
//   container. The value is read once at startup and cached.
//
// NOTE: the account/role/permission database TABLES always exist regardless of
// this flag — they are the authentication foundation the login system uses.
// The flag only controls the ENFORCEMENT + management feature layered on top.
// ============================================================================

var (
	rbacEnabledOnce sync.Once
	rbacEnabledVal  bool
)

// IsEnabled reports whether the RBAC feature is turned on via RBAC_ENABLED in
// the .env file. Missing/empty/unrecognised values mean OFF. The result is read
// once and cached, so changing the env var requires a core restart to take
// effect.
func IsEnabled() bool {
	rbacEnabledOnce.Do(func() {
		v, err := public.DockerEnv("RBAC_ENABLED")
		if err != nil {
			rbacEnabledVal = false
			return
		}
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			rbacEnabledVal = true
		default:
			rbacEnabledVal = false
		}
	})
	return rbacEnabledVal
}

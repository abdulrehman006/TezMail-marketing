package rbac

import (
	v1 "billionmail-core/api/rbac/v1"
	"billionmail-core/internal/model"
	service "billionmail-core/internal/service/rbac"
	"context"

	"github.com/gogf/gf/v2/frame/g"
)

// currentAccountId returns the authenticated account id from the request
// context. It reads via the GoFrame request (SetCtxVar/GetCtxVar), which is the
// only reliable way — a plain ctx.Value lookup does not see GoFrame ctx vars.
func currentAccountId(ctx context.Context) int64 {
	r := g.RequestFromCtx(ctx)
	if r == nil {
		return 0
	}
	return r.GetCtxVar("accountId").Int64()
}

// currentRoles returns the authenticated account's role names from context.
func currentRoles(ctx context.Context) []string {
	r := g.RequestFromCtx(ctx)
	if r == nil {
		return nil
	}
	return r.GetCtxVar("roles", []string{}).Strings()
}

// isAdminCtx reports whether the caller holds the admin role.
func isAdminCtx(ctx context.Context) bool {
	for _, role := range currentRoles(ctx) {
		if role == "admin" {
			return true
		}
	}
	return false
}

// adminRoleId returns the role_id of the built-in admin role (0 if missing).
func adminRoleId(ctx context.Context) int64 {
	v, err := g.DB().Model("role").Where("role_name", "admin").Value("role_id")
	if err != nil || v == nil {
		return 0
	}
	return v.Int64()
}

// accountHasAdminRole reports whether the given account currently holds admin.
func accountHasAdminRole(ctx context.Context, accountId int64) bool {
	roles, err := service.Account().GetAccountRoles(ctx, accountId)
	if err != nil {
		return false
	}
	for _, r := range roles {
		if r.RoleName == "admin" {
			return true
		}
	}
	return false
}

// roleIdsIncludeAdmin reports whether the admin role is in the given id list.
func roleIdsIncludeAdmin(ctx context.Context, roleIds []int64) bool {
	aid := adminRoleId(ctx)
	if aid == 0 {
		return false
	}
	for _, id := range roleIds {
		if id == aid {
			return true
		}
	}
	return false
}

func toAccountItem(a model.Account) v1.AccountInfoItem {
	return v1.AccountInfoItem{
		Id:         a.AccountId,
		Username:   a.Username,
		Email:      a.Email,
		Status:     a.Status,
		Language:   a.Language,
		CreateTime: a.CreateTime,
	}
}

func toRoleItem(r model.Role) v1.RoleInfoItem {
	return v1.RoleInfoItem{
		Id:          r.RoleId,
		Name:        r.RoleName,
		Description: r.Description,
		Status:      r.Status,
		CreateTime:  r.CreateTime,
	}
}

func toPermissionItem(p model.Permission) v1.PermissionInfoItem {
	return v1.PermissionInfoItem{
		Id:          p.PermissionId,
		Name:        p.PermissionName,
		Description: p.Description,
		Module:      p.Module,
		Action:      p.Action,
		Resource:    p.Resource,
		Status:      p.Status,
		CreateTime:  p.CreateTime,
		UpdateTime:  p.UpdateTime,
	}
}

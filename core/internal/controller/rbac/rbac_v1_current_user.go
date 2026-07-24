package rbac

import (
	v1 "billionmail-core/api/rbac/v1"
	service "billionmail-core/internal/service/rbac"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

// CurrentUser returns the authenticated account, its role names and the list of
// logical module keys it may access. The frontend uses Permissions to gate menu
// items; admins receive every module key.
func (c *ControllerV1) CurrentUser(ctx context.Context, req *v1.CurrentUserReq) (res *v1.CurrentUserRes, err error) {
	res = &v1.CurrentUserRes{}

	accountId := currentAccountId(ctx)
	if accountId == 0 {
		res.SetError(gerror.New("Unauthorized"))
		return res, nil
	}

	acc, err := service.Account().GetById(ctx, accountId)
	if err != nil || acc == nil || acc.AccountId == 0 {
		res.SetError(gerror.New("Failed to get account details"))
		return res, nil
	}

	res.Data.Account.Id = acc.AccountId
	res.Data.Account.Username = acc.Username
	res.Data.Account.Email = acc.Email
	res.Data.Account.Status = acc.Status
	res.Data.Account.Lang = acc.Language

	roles, _ := service.Account().GetAccountRoles(ctx, accountId)
	isAdmin := false
	res.Data.Roles = make([]string, 0, len(roles))
	for _, r := range roles {
		res.Data.Roles = append(res.Data.Roles, r.RoleName)
		if r.RoleName == "admin" {
			isAdmin = true
		}
	}

	res.Data.Permissions = make([]string, 0)
	if isAdmin {
		// Admins can access every feature area.
		for _, m := range service.Modules {
			res.Data.Permissions = append(res.Data.Permissions, m.Key)
		}
	} else {
		perms, _ := service.Account().GetAccountPermissions(ctx, accountId)
		seen := make(map[string]bool)
		for _, p := range perms {
			if p.Module != "" && !seen[p.Module] {
				seen[p.Module] = true
				res.Data.Permissions = append(res.Data.Permissions, p.Module)
			}
		}
	}

	res.SetSuccess("Success")
	return
}

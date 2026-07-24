package rbac

import (
	v1 "billionmail-core/api/rbac/v1"
	"billionmail-core/internal/model"
	service "billionmail-core/internal/service/rbac"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

// AccountList returns a paginated list of accounts.
func (c *ControllerV1) AccountList(ctx context.Context, req *v1.AccountListReq) (res *v1.AccountListRes, err error) {
	res = &v1.AccountListRes{}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}

	list, total, err := service.Account().GetList(ctx, page, pageSize, req.Username, req.Email, req.Status)
	if err != nil {
		res.SetError(err)
		return res, nil
	}

	for _, a := range list {
		res.Data.List = append(res.Data.List, toAccountItem(a))
	}
	res.Data.Total = total
	res.Data.Page = page
	res.SetSuccess("Success")
	return
}

// AccountDetail returns one account with its roles and the full role list.
func (c *ControllerV1) AccountDetail(ctx context.Context, req *v1.AccountDetailReq) (res *v1.AccountDetailRes, err error) {
	res = &v1.AccountDetailRes{}

	acc, err := service.Account().GetById(ctx, req.AccountId)
	if err != nil || acc == nil || acc.AccountId == 0 {
		res.SetError(gerror.New("Account not found"))
		return res, nil
	}
	res.Data.Account = toAccountItem(*acc)

	roles, _ := service.Account().GetAccountRoles(ctx, req.AccountId)
	for _, r := range roles {
		res.Data.Roles = append(res.Data.Roles, toRoleItem(r))
	}

	allRoles, _ := service.Role().GetAll(ctx)
	for _, r := range allRoles {
		res.Data.AllRoles = append(res.Data.AllRoles, toRoleItem(r))
	}

	res.SetSuccess("Success")
	return
}

// AccountCreate creates a new account and assigns roles.
func (c *ControllerV1) AccountCreate(ctx context.Context, req *v1.AccountCreateReq) (res *v1.AccountCreateRes, err error) {
	res = &v1.AccountCreateRes{}

	exists, _ := service.Account().UsernameExists(ctx, req.Username)
	if exists {
		res.SetError(gerror.New("Username already exists"))
		return res, nil
	}
	if req.Email != "" {
		if emailUsed, _ := service.Account().EmailExists(ctx, req.Email); emailUsed {
			res.SetError(gerror.New("Email already exists"))
			return res, nil
		}
	}

	lang := req.Lang
	if lang == "" {
		lang = "en"
	}

	id, err := service.Account().Create(ctx, &model.Account{
		Username: req.Username,
		Password: req.Password,
		Email:    req.Email,
		Status:   req.Status,
		Language: lang,
	})
	if err != nil {
		res.SetError(err)
		return res, nil
	}

	if len(req.RoleIds) > 0 {
		if err := service.Account().BindRoles(ctx, id, req.RoleIds); err != nil {
			res.SetError(err)
			return res, nil
		}
	}

	res.Data.AccountId = id
	res.SetSuccess("Account created successfully")
	return
}

// AccountUpdate updates account details and role assignments.
func (c *ControllerV1) AccountUpdate(ctx context.Context, req *v1.AccountUpdateReq) (res *v1.AccountUpdateRes, err error) {
	res = &v1.AccountUpdateRes{}

	acc, err := service.Account().GetById(ctx, req.AccountId)
	if err != nil || acc == nil || acc.AccountId == 0 {
		res.SetError(gerror.New("Account not found"))
		return res, nil
	}

	// Never let the last administrator lose the admin role.
	if accountHasAdminRole(ctx, req.AccountId) && !roleIdsIncludeAdmin(ctx, req.RoleIds) {
		if n, _ := service.Account().CountAdmins(ctx); n <= 1 {
			res.SetError(gerror.New("Cannot remove the administrator role from the last administrator"))
			return res, nil
		}
	}

	username := req.Username
	if username == "" {
		username = acc.Username
	}
	email := req.Email
	if email == "" {
		email = acc.Email
	}
	lang := req.Lang
	if lang == "" {
		lang = acc.Language
	}

	// Reject duplicate username/email up front with a clear message instead of
	// letting the DB unique constraint surface a raw driver error.
	if username != acc.Username {
		if exists, _ := service.Account().UsernameExists(ctx, username); exists {
			res.SetError(gerror.New("Username already exists"))
			return res, nil
		}
	}
	if email != acc.Email && email != "" {
		if exists, _ := service.Account().EmailExists(ctx, email); exists {
			res.SetError(gerror.New("Email already exists"))
			return res, nil
		}
	}

	if err := service.Account().Update(ctx, &model.Account{
		AccountId: req.AccountId,
		Username:  username,
		Email:     email,
		Status:    req.Status,
		Language:  lang,
	}); err != nil {
		res.SetError(err)
		return res, nil
	}

	if err := service.Account().BindRoles(ctx, req.AccountId, req.RoleIds); err != nil {
		res.SetError(err)
		return res, nil
	}

	res.SetSuccess("Account updated successfully")
	return
}

// AccountPassword changes an account password. Admins may reset anyone; a
// non-admin may only change their own password and must supply the old one.
func (c *ControllerV1) AccountPassword(ctx context.Context, req *v1.AccountPasswordReq) (res *v1.AccountPasswordRes, err error) {
	res = &v1.AccountPasswordRes{}

	callerId := currentAccountId(ctx)
	admin := isAdminCtx(ctx)

	if !admin && callerId != req.AccountId {
		res.SetError(gerror.New("You can only change your own password"))
		return res, nil
	}

	if !admin {
		acc, err := service.Account().GetById(ctx, req.AccountId)
		if err != nil || acc == nil || acc.AccountId == 0 {
			res.SetError(gerror.New("Account not found"))
			return res, nil
		}
		if !service.Account().VerifyPassword(acc.Password, req.OldPassword) {
			res.SetError(gerror.New("Old password is incorrect"))
			return res, nil
		}
	}

	if err := service.Account().UpdatePassword(ctx, req.AccountId, req.NewPassword); err != nil {
		res.SetError(err)
		return res, nil
	}

	res.SetSuccess("Password updated successfully")
	return
}

// AccountDelete removes an account. Guards against self-deletion and against
// deleting the last administrator.
func (c *ControllerV1) AccountDelete(ctx context.Context, req *v1.AccountDeleteReq) (res *v1.AccountDeleteRes, err error) {
	res = &v1.AccountDeleteRes{}

	if currentAccountId(ctx) == req.AccountId {
		res.SetError(gerror.New("You cannot delete your own account"))
		return res, nil
	}

	if accountHasAdminRole(ctx, req.AccountId) {
		if n, _ := service.Account().CountAdmins(ctx); n <= 1 {
			res.SetError(gerror.New("Cannot delete the last administrator"))
			return res, nil
		}
	}

	_ = service.Account().ClearRoles(ctx, req.AccountId)
	if err := service.Account().Delete(ctx, req.AccountId); err != nil {
		res.SetError(err)
		return res, nil
	}

	res.SetSuccess("Account deleted successfully")
	return
}

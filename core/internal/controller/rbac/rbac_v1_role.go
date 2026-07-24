package rbac

import (
	v1 "billionmail-core/api/rbac/v1"
	service "billionmail-core/internal/service/rbac"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

// RoleList returns a paginated list of roles.
func (c *ControllerV1) RoleList(ctx context.Context, req *v1.RoleListReq) (res *v1.RoleListRes, err error) {
	res = &v1.RoleListRes{}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}

	list, total, err := service.Role().GetList(ctx, page, pageSize, req.Name, req.Status)
	if err != nil {
		res.SetError(err)
		return res, nil
	}

	for _, r := range list {
		res.Data.List = append(res.Data.List, toRoleItem(r))
	}
	res.Data.Total = total
	res.Data.Page = page
	res.SetSuccess("Success")
	return
}

// RoleDetail returns one role with its assigned permissions and the full
// permission catalogue (the assignable feature modules).
func (c *ControllerV1) RoleDetail(ctx context.Context, req *v1.RoleDetailReq) (res *v1.RoleDetailRes, err error) {
	res = &v1.RoleDetailRes{}

	role, err := service.Role().GetById(ctx, req.RoleId)
	if err != nil || role == nil || role.RoleId == 0 {
		res.SetError(gerror.New("Role not found"))
		return res, nil
	}
	res.Data.Role = toRoleItem(*role)

	perms, _ := service.Role().GetPermissions(ctx, req.RoleId)
	for _, p := range perms {
		res.Data.Permissions = append(res.Data.Permissions, toPermissionItem(p))
	}

	allPerms, _ := service.Permission().GetAll(ctx)
	for _, p := range allPerms {
		res.Data.AllPermissions = append(res.Data.AllPermissions, toPermissionItem(p))
	}

	res.SetSuccess("Success")
	return
}

// RoleCreate creates a role and binds its module permissions.
func (c *ControllerV1) RoleCreate(ctx context.Context, req *v1.RoleCreateReq) (res *v1.RoleCreateRes, err error) {
	res = &v1.RoleCreateRes{}

	if req.Name == "admin" {
		res.SetError(gerror.New("\"admin\" is a reserved role name"))
		return res, nil
	}
	if exists, _ := service.Role().NameExists(ctx, req.Name); exists {
		res.SetError(gerror.New("Role name already exists"))
		return res, nil
	}

	id, err := service.Role().Create(ctx, req.Name, req.Description, req.Status)
	if err != nil {
		res.SetError(err)
		return res, nil
	}

	if len(req.PermissionIds) > 0 {
		if err := service.Role().BindPermissions(ctx, id, req.PermissionIds); err != nil {
			res.SetError(err)
			return res, nil
		}
	}

	res.Data.RoleId = id
	res.SetSuccess("Role created successfully")
	return
}

// RoleUpdate updates a role and replaces its module permissions. The built-in
// admin role is protected from modification.
func (c *ControllerV1) RoleUpdate(ctx context.Context, req *v1.RoleUpdateReq) (res *v1.RoleUpdateRes, err error) {
	res = &v1.RoleUpdateRes{}

	role, err := service.Role().GetById(ctx, req.RoleId)
	if err != nil || role == nil || role.RoleId == 0 {
		res.SetError(gerror.New("Role not found"))
		return res, nil
	}
	if role.RoleName == "admin" {
		res.SetError(gerror.New("The administrator role cannot be modified"))
		return res, nil
	}

	name := req.Name
	if name == "" {
		name = role.RoleName
	}
	if name != role.RoleName {
		if name == "admin" {
			res.SetError(gerror.New("\"admin\" is a reserved role name"))
			return res, nil
		}
		if exists, _ := service.Role().NameExists(ctx, name); exists {
			res.SetError(gerror.New("Role name already exists"))
			return res, nil
		}
	}

	if err := service.Role().Update(ctx, req.RoleId, name, req.Description, req.Status); err != nil {
		res.SetError(err)
		return res, nil
	}

	if err := service.Role().BindPermissions(ctx, req.RoleId, req.PermissionIds); err != nil {
		res.SetError(err)
		return res, nil
	}

	res.SetSuccess("Role updated successfully")
	return
}

// RoleDelete removes a role. The admin role is protected, and a role still
// assigned to accounts cannot be deleted.
func (c *ControllerV1) RoleDelete(ctx context.Context, req *v1.RoleDeleteReq) (res *v1.RoleDeleteRes, err error) {
	res = &v1.RoleDeleteRes{}

	role, err := service.Role().GetById(ctx, req.RoleId)
	if err != nil || role == nil || role.RoleId == 0 {
		res.SetError(gerror.New("Role not found"))
		return res, nil
	}
	if role.RoleName == "admin" {
		res.SetError(gerror.New("The administrator role cannot be deleted"))
		return res, nil
	}

	if hasAccounts, _ := service.Role().HasAccounts(ctx, req.RoleId); hasAccounts {
		res.SetError(gerror.New("This role is still assigned to one or more accounts"))
		return res, nil
	}

	_ = service.Role().ClearPermissions(ctx, req.RoleId)
	if err := service.Role().Delete(ctx, req.RoleId); err != nil {
		res.SetError(err)
		return res, nil
	}

	res.SetSuccess("Role deleted successfully")
	return
}

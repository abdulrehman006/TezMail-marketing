package rbac

import (
	v1 "billionmail-core/api/rbac/v1"
	service "billionmail-core/internal/service/rbac"
	"context"
)

// PermissionList returns the catalogue of module permissions. In the
// per-module model these are the seeded feature areas the role editor offers.
func (c *ControllerV1) PermissionList(ctx context.Context, req *v1.PermissionListReq) (res *v1.PermissionListRes, err error) {
	res = &v1.PermissionListRes{}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 1000
	}

	list, total, err := service.Permission().GetList(ctx, page, pageSize, req.Module, req.Action, req.Status)
	if err != nil {
		res.SetError(err)
		return res, nil
	}

	for _, p := range list {
		res.Data.List = append(res.Data.List, toPermissionItem(p))
	}
	res.Data.Total = total
	res.Data.Page = page
	res.SetSuccess("Success")
	return
}

package v1

import (
	"billionmail-core/utility/types/api_v1"

	"github.com/gogf/gf/v2/frame/g"
)

// PermissionInfoItem defines the permission information structure
type PermissionInfoItem struct {
	Id          int64  `json:"id" dc:"Permission ID"`
	Name        string `json:"name" dc:"Permission name"`
	Description string `json:"description" dc:"Permission description"`
	Module      string `json:"module" dc:"Module name"`
	Action      string `json:"action" dc:"Action name (create/read/update/delete)"`
	Resource    string `json:"resource" dc:"Resource name"`
	Status      int    `json:"status" dc:"Status (0:disabled, 1:enabled)"`
	CreateTime  int64  `json:"create_time" dc:"Creation time"`
	UpdateTime  int64  `json:"update_time" dc:"Update time"`
}

// PermissionListReq defines the request for getting permission list.
// In the per-module model this returns the seeded module permissions, which the
// role editor renders as the set of assignable feature areas.
type PermissionListReq struct {
	g.Meta        `path:"/permission/list" method:"get" tags:"RBAC" summary:"Get permission list" sm:"Get permission list" in:"query"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Page          int    `p:"page" d:"1" v:"min:1#Page number must be greater than 0" dc:"Page number"`
	PageSize      int    `p:"pageSize" d:"1000" v:"min:1#Items per page must be greater than 0" dc:"Items per page"`
	Module        string `p:"module" dc:"Module name filter"`
	Action        string `p:"action" dc:"Action name filter"`
	Status        int    `p:"status" dc:"Status filter"`
}

// PermissionListRes defines the response for getting permission list
type PermissionListRes struct {
	api_v1.StandardRes
	Data struct {
		List  []PermissionInfoItem `json:"list" dc:"Permission list"`
		Total int                  `json:"total" dc:"Total count"`
		Page  int                  `json:"page" dc:"Current page number"`
	} `json:"data"`
}

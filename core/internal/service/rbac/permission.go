package rbac

import (
	"billionmail-core/internal/model"
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type permissionService struct{}

func newPermissionService() *permissionService {
	return &permissionService{}
}

// GetList gets permission list with pagination
func (s *permissionService) GetList(ctx context.Context, page, pageSize int, module, action string, status int) ([]model.Permission, int, error) {
	var permissions []model.Permission
	var total int

	query := g.DB().Model("permission")
	if module != "" {
		query = query.Where("module LIKE ?", "%"+module+"%")
	}
	if action != "" {
		query = query.Where("action LIKE ?", "%"+action+"%")
	}
	if status > 0 {
		query = query.Where("status = ?", status)
	}

	// Get total count
	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}
	total = count

	// Get paginated data
	err = query.Page(page, pageSize).Order("permission_id ASC").Scan(&permissions)
	if err != nil {
		return nil, 0, err
	}

	return permissions, total, nil
}

// GetAll gets all permissions
func (s *permissionService) GetAll(ctx context.Context) ([]model.Permission, error) {
	var permissions []model.Permission
	err := g.DB().Model("permission").Order("permission_id ASC").Scan(&permissions)
	return permissions, err
}

// GetById gets permission details by ID
func (s *permissionService) GetById(ctx context.Context, permissionId int64) (*model.Permission, error) {
	var permission model.Permission
	err := g.DB().Model("permission").Where("permission_id = ?", permissionId).Scan(&permission)
	if err != nil {
		return nil, err
	}
	return &permission, nil
}

// Create creates a new permission
func (s *permissionService) Create(ctx context.Context, name, description, module, action, resource string, status int) (int64, error) {
	now := time.Now().Unix()
	result, err := g.DB().Model("permission").Data(g.Map{
		"permission_name": name,
		"description":     description,
		"module":          module,
		"action":          action,
		"resource":        resource,
		"status":          status,
		"create_time":     now,
		"update_time":     now,
	}).Insert()
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update updates permission information
func (s *permissionService) Update(ctx context.Context, permissionId int64, name, description, module, action, resource string, status int) error {
	_, err := g.DB().Model("permission").Data(g.Map{
		"permission_name": name,
		"description":     description,
		"module":          module,
		"action":          action,
		"resource":        resource,
		"status":          status,
		"update_time":     time.Now().Unix(),
	}).Where("permission_id = ?", permissionId).Update()
	return err
}

// Delete deletes a permission
func (s *permissionService) Delete(ctx context.Context, permissionId int64) error {
	_, err := g.DB().Model("permission").Where("permission_id = ?", permissionId).Delete()
	return err
}

// Check checks whether an account has a specific (module, action, resource)
// permission. Only active permissions granted through an active role count.
func (s *permissionService) Check(ctx context.Context, accountId int64, module, action, resource string) (bool, error) {
	count, err := g.DB().Model("permission").
		LeftJoin("role_permission", "permission.permission_id = role_permission.permission_id").
		LeftJoin("account_role", "role_permission.role_id = account_role.role_id").
		LeftJoin("role", "account_role.role_id = role.role_id").
		Where("account_role.account_id = ?", accountId).
		Where("permission.module = ?", module).
		Where("permission.action = ?", action).
		Where("permission.resource = ?", resource).
		Where("permission.status = ?", 1).
		Where("role.status = ?", 1).
		Count()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CheckModule checks whether an account has access to a logical module. This is
// the primary enforcement query for the per-module RBAC model: it returns true
// if any ACTIVE role assigned to the account grants any active permission in the
// module. Disabling a role or a permission therefore revokes access immediately.
func (s *permissionService) CheckModule(ctx context.Context, accountId int64, module string) (bool, error) {
	count, err := g.DB().Model("permission").
		LeftJoin("role_permission", "permission.permission_id = role_permission.permission_id").
		LeftJoin("account_role", "role_permission.role_id = account_role.role_id").
		LeftJoin("role", "account_role.role_id = role.role_id").
		Where("account_role.account_id = ?", accountId).
		Where("permission.module = ?", module).
		Where("permission.status = ?", 1).
		Where("role.status = ?", 1).
		Count()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

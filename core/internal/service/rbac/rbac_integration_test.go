//go:build integration

// Package-level integration tests for the RBAC service against a real
// PostgreSQL instance. These validate that the service-layer SQL (column names,
// joins, epoch timestamps) actually runs against the real schema — the class of
// bug that static review alone cannot catch.
//
// Run with a throwaway Postgres:
//
//	docker network create tz && \
//	docker run -d --name tzpg --network tz -e POSTGRES_USER=billionmail \
//	  -e POSTGRES_PASSWORD=test -e POSTGRES_DB=billionmail postgres:15-alpine && \
//	docker run --rm --network tz -e PGTEST_HOST=tzpg -v "$PWD:/app" -w /app \
//	  golang:1.23-alpine go test -tags integration ./internal/service/rbac/... -v
package rbac

import (
	"billionmail-core/internal/model"
	"context"
	"errors"
	"os"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

const schemaDDL = `
DROP TABLE IF EXISTS role_permission CASCADE;
DROP TABLE IF EXISTS account_role CASCADE;
DROP TABLE IF EXISTS permission CASCADE;
DROP TABLE IF EXISTS role CASCADE;
DROP TABLE IF EXISTS account CASCADE;
CREATE TABLE account (
	account_id SERIAL PRIMARY KEY,
	username VARCHAR(64) NOT NULL UNIQUE,
	password VARCHAR(255) NOT NULL,
	email VARCHAR(255) NOT NULL,
	status INT NOT NULL DEFAULT 1,
	language VARCHAR(50) NOT NULL DEFAULT 'en',
	last_login_time INT NOT NULL DEFAULT 0,
	create_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
	update_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
);
CREATE TABLE role (
	role_id SERIAL PRIMARY KEY,
	role_name VARCHAR(64) NOT NULL UNIQUE,
	description TEXT,
	status INT NOT NULL DEFAULT 1,
	create_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
	update_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
);
CREATE TABLE permission (
	permission_id SERIAL PRIMARY KEY,
	permission_name VARCHAR(64) NOT NULL UNIQUE,
	description TEXT,
	module VARCHAR(64) NOT NULL,
	action VARCHAR(64) NOT NULL,
	resource VARCHAR(64) NOT NULL,
	status INT NOT NULL DEFAULT 1,
	create_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
	update_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
	UNIQUE(module, action, resource)
);
CREATE TABLE account_role (
	id SERIAL PRIMARY KEY,
	account_id INT NOT NULL,
	role_id INT NOT NULL,
	create_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
	UNIQUE(account_id, role_id),
	FOREIGN KEY (account_id) REFERENCES account(account_id) ON DELETE CASCADE,
	FOREIGN KEY (role_id) REFERENCES role(role_id) ON DELETE CASCADE
);
CREATE TABLE role_permission (
	id SERIAL PRIMARY KEY,
	role_id INT NOT NULL,
	permission_id INT NOT NULL,
	create_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
	UNIQUE(role_id, permission_id),
	FOREIGN KEY (role_id) REFERENCES role(role_id) ON DELETE CASCADE,
	FOREIGN KEY (permission_id) REFERENCES permission(permission_id) ON DELETE CASCADE
);
`

func TestMain(m *testing.M) {
	if err := gdb.SetConfig(gdb.Config{
		"default": gdb.ConfigGroup{
			gdb.ConfigNode{
				Host: env("PGTEST_HOST", "127.0.0.1"),
				Port: env("PGTEST_PORT", "5432"),
				User: env("PGTEST_USER", "billionmail"),
				Pass: env("PGTEST_PASS", "test"),
				Name: env("PGTEST_DB", "billionmail"),
				Type: "pgsql",
				Role: "master",
			},
		},
	}); err != nil {
		panic(err)
	}
	if _, err := g.DB().Exec(context.Background(), schemaDDL); err != nil {
		panic("failed to create schema: " + err.Error())
	}
	os.Exit(m.Run())
}

// reset truncates all RBAC tables and re-seeds the module permissions so each
// test starts from a known state.
func reset(t *testing.T) {
	ctx := context.Background()
	if _, err := g.DB().Exec(ctx,
		"TRUNCATE role_permission, account_role, permission, role, account RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate failed: %v", err)
	}
	if err := SeedModulePermissions(ctx); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
}

func mailboxesPermId(t *testing.T) int64 {
	v, err := g.DB().Model("permission").Where("module", "mailboxes").Value("permission_id")
	if err != nil || v == nil {
		t.Fatalf("could not read mailboxes permission id: %v", err)
	}
	return v.Int64()
}

// --- Seeding -------------------------------------------------------------

func TestIntegration_SeedIdempotent(t *testing.T) {
	reset(t)
	ctx := context.Background()

	// Seeding again must not create duplicates.
	if err := SeedModulePermissions(ctx); err != nil {
		t.Fatalf("re-seed failed: %v", err)
	}
	count, err := g.DB().Model("permission").Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != len(Modules) {
		t.Fatalf("permission count = %d, want %d", count, len(Modules))
	}

	ids, err := ModulePermissionIds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != len(Modules) {
		t.Fatalf("ModulePermissionIds len = %d, want %d", len(ids), len(Modules))
	}
}

// --- CheckModule (the primary enforcement query) -------------------------

func TestIntegration_CheckModule_GrantAndDeny(t *testing.T) {
	reset(t)
	ctx := context.Background()

	roleId, err := Role().Create(ctx, "staff", "Mailbox staff", 1)
	if err != nil {
		t.Fatalf("role create: %v", err)
	}
	accId, err := Account().Create(ctx, &model.Account{
		Username: "staff1", Password: "secret123", Email: "s1@example.com", Status: 1, Language: "en",
	})
	if err != nil {
		t.Fatalf("account create: %v", err)
	}
	if err := Account().BindRoles(ctx, accId, []int64{roleId}); err != nil {
		t.Fatalf("bind roles: %v", err)
	}
	if err := Role().BindPermissions(ctx, roleId, []int64{mailboxesPermId(t)}); err != nil {
		t.Fatalf("bind permissions: %v", err)
	}

	ok, err := Permission().CheckModule(ctx, accId, "mailboxes")
	if err != nil {
		t.Fatalf("CheckModule(mailboxes): %v", err)
	}
	if !ok {
		t.Error("expected access to granted module 'mailboxes', got denied")
	}

	ok, err = Permission().CheckModule(ctx, accId, "domains")
	if err != nil {
		t.Fatalf("CheckModule(domains): %v", err)
	}
	if ok {
		t.Error("expected DENY for ungranted module 'domains', got allowed")
	}

	// Triple-form Check must agree for the seeded (module, access, module) row.
	ok, err = Permission().Check(ctx, accId, "mailboxes", ModuleAction, "mailboxes")
	if err != nil {
		t.Fatalf("Check triple: %v", err)
	}
	if !ok {
		t.Error("Check(mailboxes, access, mailboxes) expected true")
	}
}

// --- Account <-> role/permission joins -----------------------------------

func TestIntegration_AccountRolesAndPermissions(t *testing.T) {
	reset(t)
	ctx := context.Background()

	roleId, _ := Role().Create(ctx, "editor", "", 1)
	accId, _ := Account().Create(ctx, &model.Account{
		Username: "ed", Password: "secret123", Email: "ed@example.com", Status: 1, Language: "en",
	})
	_ = Account().BindRoles(ctx, accId, []int64{roleId})
	_ = Role().BindPermissions(ctx, roleId, []int64{mailboxesPermId(t)})

	roles, err := Account().GetAccountRoles(ctx, accId)
	if err != nil {
		t.Fatalf("GetAccountRoles: %v", err)
	}
	if len(roles) != 1 || roles[0].RoleName != "editor" {
		t.Fatalf("GetAccountRoles = %+v, want one 'editor'", roles)
	}

	// GetRoles is the second (previously broken) join — must also work now.
	roles2, err := Account().GetRoles(ctx, accId)
	if err != nil {
		t.Fatalf("GetRoles: %v", err)
	}
	if len(roles2) != 1 {
		t.Fatalf("GetRoles len = %d, want 1", len(roles2))
	}

	perms, err := Account().GetAccountPermissions(ctx, accId)
	if err != nil {
		t.Fatalf("GetAccountPermissions: %v", err)
	}
	if len(perms) != 1 || perms[0].Module != "mailboxes" {
		t.Fatalf("GetAccountPermissions = %+v, want one 'mailboxes'", perms)
	}
}

// --- Role service column correctness -------------------------------------

func TestIntegration_RoleCRUDColumns(t *testing.T) {
	reset(t)
	ctx := context.Background()

	roleId, err := Role().Create(ctx, "temp", "first", 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := Role().GetById(ctx, roleId)
	if err != nil || got == nil {
		t.Fatalf("GetById: %v", err)
	}
	if got.RoleName != "temp" || got.RoleId != roleId {
		t.Fatalf("GetById = %+v, want role_id=%d name=temp", got, roleId)
	}

	if err := Role().Update(ctx, roleId, "temp2", "second", 0); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = Role().GetById(ctx, roleId)
	if got.RoleName != "temp2" || got.Status != 0 {
		t.Fatalf("after update = %+v, want name=temp2 status=0", got)
	}

	if exists, _ := Role().NameExists(ctx, "temp2"); !exists {
		t.Error("NameExists(temp2) expected true")
	}
	if exists, _ := Role().NameExists(ctx, "nope"); exists {
		t.Error("NameExists(nope) expected false")
	}

	// Bind then confirm HasAccounts toggles correctly.
	if has, _ := Role().HasAccounts(ctx, roleId); has {
		t.Error("HasAccounts expected false before assignment")
	}
	accId, _ := Account().Create(ctx, &model.Account{
		Username: "u", Password: "secret123", Email: "u@example.com", Status: 1, Language: "en",
	})
	_ = Account().BindRoles(ctx, accId, []int64{roleId})
	if has, _ := Role().HasAccounts(ctx, roleId); !has {
		t.Error("HasAccounts expected true after assignment")
	}

	if err := Role().Delete(ctx, roleId); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ = Role().GetById(ctx, roleId)
	if got != nil && got.RoleId != 0 {
		t.Errorf("role still present after delete: %+v", got)
	}
}

// --- BindPermissions replaces the set ------------------------------------

func TestIntegration_BindPermissionsReplaces(t *testing.T) {
	reset(t)
	ctx := context.Background()

	roleId, _ := Role().Create(ctx, "r", "", 1)
	all, _ := ModulePermissionIds(ctx)
	if len(all) < 2 {
		t.Fatal("need at least 2 seeded permissions")
	}

	_ = Role().BindPermissions(ctx, roleId, []int64{all[0], all[1]})
	perms, _ := Role().GetPermissions(ctx, roleId)
	if len(perms) != 2 {
		t.Fatalf("after first bind = %d perms, want 2", len(perms))
	}

	// Rebinding to a single id must replace, not append.
	_ = Role().BindPermissions(ctx, roleId, []int64{all[0]})
	perms, _ = Role().GetPermissions(ctx, roleId)
	if len(perms) != 1 {
		t.Fatalf("after rebind = %d perms, want 1 (replace not append)", len(perms))
	}
}

// --- Admin counting (last-admin guard depends on this) -------------------

func TestIntegration_CountAdmins(t *testing.T) {
	reset(t)
	ctx := context.Background()

	if n, _ := Account().CountAdmins(ctx); n != 0 {
		t.Fatalf("CountAdmins with no admins = %d, want 0", n)
	}

	adminRole, _ := Role().Create(ctx, "admin", "", 1)
	a1, _ := Account().Create(ctx, &model.Account{
		Username: "admin1", Password: "secret123", Email: "a1@example.com", Status: 1, Language: "en",
	})
	_ = Account().BindRoles(ctx, a1, []int64{adminRole})

	if n, _ := Account().CountAdmins(ctx); n != 1 {
		t.Fatalf("CountAdmins with one admin = %d, want 1", n)
	}

	if ok, _ := Account().IsAdmin(ctx, a1); !ok {
		t.Error("IsAdmin(admin1) expected true")
	}
}

// --- Status enforcement --------------------------------------------------

func TestIntegration_DisabledRoleRevokesAccess(t *testing.T) {
	reset(t)
	ctx := context.Background()

	roleId, _ := Role().Create(ctx, "staff", "", 1)
	accId, _ := Account().Create(ctx, &model.Account{
		Username: "s", Password: "secret123", Email: "s@example.com", Status: 1, Language: "en",
	})
	_ = Account().BindRoles(ctx, accId, []int64{roleId})
	_ = Role().BindPermissions(ctx, roleId, []int64{mailboxesPermId(t)})

	if ok, _ := Permission().CheckModule(ctx, accId, "mailboxes"); !ok {
		t.Fatal("active role should grant access")
	}

	// Disabling the role must immediately revoke access.
	if err := Role().Update(ctx, roleId, "staff", "", 0); err != nil {
		t.Fatalf("disable role: %v", err)
	}
	if ok, _ := Permission().CheckModule(ctx, accId, "mailboxes"); ok {
		t.Error("disabled role must NOT grant access")
	}
}

func TestIntegration_DisabledAccountCannotLogin(t *testing.T) {
	reset(t)
	ctx := context.Background()

	accId, err := Account().Create(ctx, &model.Account{
		Username: "u", Password: "secret123", Email: "u@example.com", Status: 1, Language: "en",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := Account().Login(ctx, "u", "secret123"); err != nil {
		t.Fatalf("active account should log in: %v", err)
	}

	// Disable the account.
	if err := Account().Update(ctx, &model.Account{
		AccountId: accId, Username: "u", Email: "u@example.com", Status: 0, Language: "en",
	}); err != nil {
		t.Fatalf("disable: %v", err)
	}

	_, err = Account().Login(ctx, "u", "secret123")
	if !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("disabled account login: got err=%v, want ErrAccountDisabled", err)
	}
}

//go:build integration

// End-to-end HTTP tests for the RBAC surface. These start a real GoFrame server
// with the production middleware chain (JWT auth -> RBAC enforcement -> response
// handler), bind the real controllers, and drive them over HTTP against a real
// Postgres. This is the layer the service/unit tests cannot reach: route
// registration, middleware ordering, the admin bypass, per-module allow/deny,
// self-service exceptions, and the response envelope.
//
// Run (from core/):
//	docker network create tz && \
//	docker run -d --name tzpg --network tz -e POSTGRES_USER=billionmail \
//	  -e POSTGRES_PASSWORD=test -e POSTGRES_DB=billionmail postgres:15-alpine && \
//	docker run --rm --network tz -e PGTEST_HOST=tzpg -v "$PWD:/app" -w /app \
//	  golang:1.23-alpine go test -tags integration ./internal/controller/rbac/... -v
package rbac

import (
	"billionmail-core/internal/model"
	"billionmail-core/internal/service/middlewares"
	"billionmail-core/internal/service/public"
	service "billionmail-core/internal/service/rbac"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gcfg"
)

const e2eAddr = "127.0.0.1:18099"
const e2eBase = "http://" + e2eAddr

var (
	e2eAdminId    int64
	e2eMailboxPid int64
)

const e2eSchema = `
DROP TABLE IF EXISTS role_permission CASCADE;
DROP TABLE IF EXISTS account_role CASCADE;
DROP TABLE IF EXISTS permission CASCADE;
DROP TABLE IF EXISTS role CASCADE;
DROP TABLE IF EXISTS account CASCADE;
CREATE TABLE account (account_id SERIAL PRIMARY KEY, username VARCHAR(64) NOT NULL UNIQUE,
  password VARCHAR(255) NOT NULL, email VARCHAR(255) NOT NULL, status INT NOT NULL DEFAULT 1,
  language VARCHAR(50) NOT NULL DEFAULT 'en', last_login_time INT NOT NULL DEFAULT 0,
  create_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()), update_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()));
CREATE TABLE role (role_id SERIAL PRIMARY KEY, role_name VARCHAR(64) NOT NULL UNIQUE, description TEXT,
  status INT NOT NULL DEFAULT 1, create_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()), update_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()));
CREATE TABLE permission (permission_id SERIAL PRIMARY KEY, permission_name VARCHAR(64) NOT NULL UNIQUE, description TEXT,
  module VARCHAR(64) NOT NULL, action VARCHAR(64) NOT NULL, resource VARCHAR(64) NOT NULL, status INT NOT NULL DEFAULT 1,
  create_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()), update_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()), UNIQUE(module, action, resource));
CREATE TABLE account_role (id SERIAL PRIMARY KEY, account_id INT NOT NULL, role_id INT NOT NULL,
  create_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()), UNIQUE(account_id, role_id),
  FOREIGN KEY (account_id) REFERENCES account(account_id) ON DELETE CASCADE, FOREIGN KEY (role_id) REFERENCES role(role_id) ON DELETE CASCADE);
CREATE TABLE role_permission (id SERIAL PRIMARY KEY, role_id INT NOT NULL, permission_id INT NOT NULL,
  create_time INT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()), UNIQUE(role_id, permission_id),
  FOREIGN KEY (role_id) REFERENCES role(role_id) ON DELETE CASCADE, FOREIGN KEY (permission_id) REFERENCES permission(permission_id) ON DELETE CASCADE);
`

func e2eEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func TestMain(m *testing.M) {
	// GoFrame's g.Cfg() panics when no config file exists at all (the real app
	// ships manifest/config/config.yaml). Give the test a minimal hermetic
	// config providing the JWT secret and the Redis endpoint (g.Redis(), used by
	// the JWT blacklist check, reads its config from g.Cfg()).
	redisAddr := e2eEnv("REDISTEST_HOST", "127.0.0.1") + ":6379"
	_ = os.MkdirAll("/tmp/e2e/cfg", 0o755)
	_ = os.WriteFile("/tmp/e2e/cfg/config.yaml", []byte(
		"jwt:\n  secret: e2e-test-secret\n  accessExpiry: 86400\n  refreshExpiry: 604800\n"+
			"redis:\n  default:\n    address: "+redisAddr+"\n    db: 0\n"), 0o644)
	if a, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile); ok {
		_ = a.SetPath("/tmp/e2e/cfg")
	}

	// DockerEnv reads ../.env relative to ROOT_PATH; point it at a temp dir too
	// so any incidental lookups do not touch the repo.
	public.ROOT_PATH = "/tmp/e2e/app"
	_ = os.MkdirAll(public.ROOT_PATH, 0o755)
	_ = os.WriteFile("/tmp/e2e/.env", []byte("DBPASS=e2esecret\nREDISPASS=e2esecret\n"), 0o644)

	ctx := context.Background()
	if err := gdb.SetConfig(gdb.Config{"default": gdb.ConfigGroup{gdb.ConfigNode{
		Host: e2eEnv("PGTEST_HOST", "127.0.0.1"), Port: e2eEnv("PGTEST_PORT", "5432"),
		User: e2eEnv("PGTEST_USER", "billionmail"), Pass: e2eEnv("PGTEST_PASS", "test"),
		Name: e2eEnv("PGTEST_DB", "billionmail"), Type: "pgsql", Role: "master",
	}}}); err != nil {
		panic(err)
	}
	if _, err := g.DB().Exec(ctx, e2eSchema); err != nil {
		panic("schema: " + err.Error())
	}
	if err := service.SeedModulePermissions(ctx); err != nil {
		panic("seed: " + err.Error())
	}

	// Seed admin role + account, mirroring the real bootstrap.
	adminRole, _ := service.Role().Create(ctx, "admin", "admin", 1)
	if ids, _ := service.ModulePermissionIds(ctx); len(ids) > 0 {
		_ = service.Role().BindPermissions(ctx, adminRole, ids)
	}
	e2eAdminId, _ = service.Account().Create(ctx, &model.Account{
		Username: "admin", Password: "admin123", Email: "admin@example.com", Status: 1, Language: "en",
	})
	_ = service.Account().BindRoles(ctx, e2eAdminId, []int64{adminRole})
	if v, _ := g.DB().Model("permission").Where("module", "mailboxes").Value("permission_id"); v != nil {
		e2eMailboxPid = v.Int64()
	}

	// Start the real middleware chain.
	s := g.Server("rbac-e2e")
	s.SetAddr(e2eAddr)
	s.SetDumpRouterMap(false)
	s.SetAccessLogEnabled(false)
	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(service.JWT().JWTAuthMiddleware)
		group.Middleware(middlewares.NewRBACMiddleware().PermissionCheck)
		group.Middleware(middlewares.HandleApiResponse)
		group.Bind(NewV1())
		// Stand-in handlers for two gated non-RBAC modules, to prove the
		// middleware gates them by module before the handler runs.
		group.GET("/mailbox/ping", func(r *ghttp.Request) { r.Response.WriteJson(g.Map{"success": true, "code": 0, "msg": "pong"}) })
		group.GET("/domains/ping", func(r *ghttp.Request) { r.Response.WriteJson(g.Map{"success": true, "code": 0, "msg": "pong"}) })
	})
	if err := s.Start(); err != nil {
		panic("server start: " + err.Error())
	}
	waitReady()

	os.Exit(m.Run())
}

func waitReady() {
	for i := 0; i < 100; i++ {
		if c, err := net.Dial("tcp", e2eAddr); err == nil {
			_ = c.Close()
			time.Sleep(100 * time.Millisecond)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func doJSON(method, path, token string, body interface{}) (int, map[string]interface{}) {
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e2eBase+path, buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var mp map[string]interface{}
	_ = json.Unmarshal(data, &mp)
	return resp.StatusCode, mp
}

func login(t *testing.T, username, password string) string {
	_, m := doJSON("POST", "/api/login", "", g.Map{"username": username, "password": password})
	if m == nil || m["success"] != true {
		t.Fatalf("login %q failed: %v", username, m)
	}
	return m["data"].(map[string]interface{})["token"].(string)
}

func code(m map[string]interface{}) int {
	if m == nil {
		return -1
	}
	if c, ok := m["code"].(float64); ok {
		return int(c)
	}
	return -1
}

func TestE2E_RBACEnforcement(t *testing.T) {
	adminTok := login(t, "admin", "admin123")

	// Admin can list accounts (system module) via the admin bypass.
	if _, m := doJSON("GET", "/api/account/list", adminTok, nil); m["success"] != true {
		t.Fatalf("admin account/list should succeed, got %v", m)
	}

	// Admin creates a limited role granting only the mailboxes module.
	_, m := doJSON("POST", "/api/role/create", adminTok, g.Map{
		"name": "mailonly", "description": "mailbox staff",
		"permissionIds": []int64{e2eMailboxPid}, "status": 1,
	})
	if m["success"] != true {
		t.Fatalf("role/create failed: %v", m)
	}
	roleId := int64(m["data"].(map[string]interface{})["roleId"].(float64))

	// Admin creates a staff account with that role.
	if _, m := doJSON("POST", "/api/account/create", adminTok, g.Map{
		"username": "staff", "password": "staff123", "email": "staff@example.com",
		"roleIds": []int64{roleId}, "status": 1,
	}); m["success"] != true {
		t.Fatalf("account/create failed: %v", m)
	}

	staffTok := login(t, "staff", "staff123")

	// current-user reflects exactly the granted access.
	_, m = doJSON("GET", "/api/current-user", staffTok, nil)
	if m["success"] != true {
		t.Fatalf("current-user failed: %v", m)
	}
	cu := m["data"].(map[string]interface{})
	staffId := int64(cu["account"].(map[string]interface{})["id"].(float64))
	perms := toStrs(cu["permissions"])
	if len(perms) != 1 || perms[0] != "mailboxes" {
		t.Errorf("staff permissions = %v, want [mailboxes]", perms)
	}

	// Staff MAY reach the granted module.
	if _, m := doJSON("GET", "/api/mailbox/ping", staffTok, nil); m["success"] != true {
		t.Errorf("staff should access mailboxes, got %v", m)
	}
	// Staff is DENIED an ungranted module.
	if _, m := doJSON("GET", "/api/domains/ping", staffTok, nil); code(m) != 403 {
		t.Errorf("staff domains should be 403, got %v", m)
	}
	// Staff is DENIED the system module (account management).
	if _, m := doJSON("GET", "/api/account/list", staffTok, nil); code(m) != 403 {
		t.Errorf("staff account/list should be 403, got %v", m)
	}
	// Admin bypass reaches everything.
	if _, m := doJSON("GET", "/api/domains/ping", adminTok, nil); m["success"] != true {
		t.Errorf("admin should bypass to domains, got %v", m)
	}
	// No token -> JWT rejects with 401 before RBAC runs.
	if _, m := doJSON("GET", "/api/mailbox/ping", "", nil); code(m) != 401 {
		t.Errorf("no-token should be 401, got %v", m)
	}

	// Self-service: staff changes their OWN password (system module, but the
	// exact path is exempt; controller enforces caller==target).
	if _, m := doJSON("POST", "/api/account/password", staffTok, g.Map{
		"accountId": staffId, "newPassword": "newpass123", "oldPassword": "staff123",
	}); m["success"] != true {
		t.Errorf("staff self password change should succeed, got %v", m)
	}
	// Staff may NOT change someone else's password.
	if _, m := doJSON("POST", "/api/account/password", staffTok, g.Map{
		"accountId": e2eAdminId, "newPassword": "hacked123",
	}); m["success"] == true {
		t.Errorf("staff changing admin password should be refused")
	}

	// Disable staff, then confirm login is blocked with the clear message.
	if _, m := doJSON("POST", "/api/account/update", adminTok, g.Map{
		"accountId": staffId, "username": "staff", "email": "staff@example.com",
		"roleIds": []int64{roleId}, "status": 0,
	}); m["success"] != true {
		t.Fatalf("disable staff failed: %v", m)
	}
	if _, m := doJSON("POST", "/api/login", "", g.Map{"username": "staff", "password": "newpass123"}); m["success"] == true {
		t.Errorf("disabled staff should not be able to log in")
	}

	// Guard: admin cannot delete their own account.
	if _, m := doJSON("POST", "/api/account/delete", adminTok, g.Map{"accountId": e2eAdminId}); m["success"] == true {
		t.Errorf("admin deleting own account should be refused")
	}
}

func toStrs(v interface{}) []string {
	arr, _ := v.([]interface{})
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

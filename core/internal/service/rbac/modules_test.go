package rbac

import "testing"

func TestModuleForPath(t *testing.T) {
	cases := []struct {
		path       string
		wantModule string
		wantSelf   bool
	}{
		// Gated feature paths resolve to their logical module.
		{"/api/mailbox/list", "mailboxes", false},
		{"/api/mailbox/create", "mailboxes", false},
		{"/api/batch_mail/api/send", "campaigns", false},
		{"/api/subscription/x", "campaigns", false},
		{"/api/email_template/list", "templates", false},
		{"/api/contact/list", "contacts", false},
		{"/api/tags/list", "contacts", false},
		{"/api/domains/list", "domains", false},
		{"/api/ssl/list", "domains", false},
		{"/api/services/list", "mail_services", false},
		{"/api/relay/list", "mail_services", false},
		{"/api/askai/chat", "ai", false},
		{"/api/settings/get", "settings", false},
		{"/api/operation_log/list", "logs", false},
		{"/api/account/list", "system", false},
		{"/api/role/list", "system", false},
		{"/api/permission/list", "system", false},

		// Changing your OWN password must be reachable by any authenticated user,
		// even though /account/* otherwise maps to the gated "system" module.
		{"/api/account/password", "", true},

		// Self-service / pre-login endpoints are never gated.
		{"/api/login", "", true},
		{"/api/logout", "", true},
		{"/api/refresh-token", "", true},
		{"/api/current-user", "", true},
		{"/api/get_validate_code", "", true},
		{"/api/languages/get", "", true},
		{"/api/languages/set", "", true},

		// Unknown API segments are ungated (allowed for any authenticated user),
		// never fail-closed.
		{"/api/something_new/x", "", false},

		// Non-API paths never reach the API middleware; treat as self-serve.
		{"/favicon.ico", "", true},
	}

	for _, tc := range cases {
		gotModule, gotSelf := ModuleForPath(tc.path)
		if gotModule != tc.wantModule || gotSelf != tc.wantSelf {
			t.Errorf("ModuleForPath(%q) = (%q, %v); want (%q, %v)",
				tc.path, gotModule, gotSelf, tc.wantModule, tc.wantSelf)
		}
	}
}

// TestModuleRegistryConsistency guards against a segment mapping to a module
// key that does not exist in Modules — which would seed no permission and
// silently deny access to that feature for every non-admin.
func TestModuleRegistryConsistency(t *testing.T) {
	valid := make(map[string]bool, len(Modules))
	for _, m := range Modules {
		if valid[m.Key] {
			t.Errorf("duplicate module key %q in Modules", m.Key)
		}
		valid[m.Key] = true
		if m.Name == "" {
			t.Errorf("module %q has an empty display name", m.Key)
		}
	}

	for segment, moduleKey := range segmentToModule {
		if !valid[moduleKey] {
			t.Errorf("segment %q maps to unknown module key %q (not in Modules)", segment, moduleKey)
		}
	}

	// A segment must not be both self-service and module-gated.
	for segment := range selfServiceSegments {
		if _, ok := segmentToModule[segment]; ok {
			t.Errorf("segment %q is both self-service and module-gated", segment)
		}
	}
}

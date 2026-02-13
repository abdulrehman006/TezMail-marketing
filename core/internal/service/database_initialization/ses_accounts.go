package database_initialization

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
)

func init() {
	registerHandler(func() {
		ctx := context.Background()

		sqlList := []string{
			// SES Accounts table - stores AWS SES API credentials
			`CREATE TABLE IF NOT EXISTS bm_ses_accounts (
				id BIGSERIAL PRIMARY KEY,
				name VARCHAR(100) NOT NULL,
				description VARCHAR(255),
				region VARCHAR(50) NOT NULL,
				access_key VARCHAR(255) NOT NULL,
				secret_key VARCHAR(255) NOT NULL,
				enabled SMALLINT NOT NULL DEFAULT 1,
				status VARCHAR(20) NOT NULL DEFAULT 'pending',
				status_message VARCHAR(500),
				last_verified TIMESTAMP,
				verified_domains JSONB DEFAULT '[]',
				send_quota JSONB,
				check_interval_minutes INT NOT NULL DEFAULT 30,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(name)
			)`,

			// SES Domain Mapping table - maps sender domains to SES accounts
			`CREATE TABLE IF NOT EXISTS bm_ses_domain_mapping (
				id BIGSERIAL PRIMARY KEY,
				account_id BIGINT NOT NULL REFERENCES bm_ses_accounts(id) ON DELETE CASCADE,
				domain VARCHAR(255) NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(domain)
			)`,

			// Indexes for better performance
			`CREATE INDEX IF NOT EXISTS idx_ses_accounts_enabled ON bm_ses_accounts(enabled)`,
			`CREATE INDEX IF NOT EXISTS idx_ses_accounts_status ON bm_ses_accounts(status)`,
			`CREATE INDEX IF NOT EXISTS idx_ses_domain_mapping_domain ON bm_ses_domain_mapping(domain)`,
			`CREATE INDEX IF NOT EXISTS idx_ses_domain_mapping_account_id ON bm_ses_domain_mapping(account_id)`,
		}

		for _, sql := range sqlList {
			_, err := g.DB().Exec(ctx, sql)
			if err != nil {
				g.Log().Error(ctx, "Failed to create SES accounts tables:", err)
				return
			}
		}

		g.Log().Info(ctx, "SES accounts tables initialized successfully")
	})
}

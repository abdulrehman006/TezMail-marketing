package ses_api

import (
	"billionmail-core/api/settings/v1"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/gogf/gf/v2/frame/g"
)

func GetAllAccountsFromDB(ctx context.Context) ([]v1.SESAccount, error) {
	var accounts []v1.SESAccount
	result, err := g.DB().Model("bm_ses_accounts").All()
	if err != nil {
		return nil, err
	}

	for _, record := range result {
		account := v1.SESAccount{
			Id:                   record["id"].Int64(),
			Name:                 record["name"].String(),
			Description:          record["description"].String(),
			Region:               record["region"].String(),
			AccessKey:            record["access_key"].String(),
			SecretKey:            maskSecretKey(record["secret_key"].String()),
			Enabled:              record["enabled"].Int() == 1,
			Status:               record["status"].String(),
			StatusMessage:        record["status_message"].String(),
			CheckIntervalMinutes: record["check_interval_minutes"].Int(),
		}

		if !record["last_verified"].IsNil() && !record["last_verified"].IsEmpty() {
			account.LastVerified = record["last_verified"].String()
		}
		if !record["verified_domains"].IsNil() && !record["verified_domains"].IsEmpty() {
			var domains []string
			if err := json.Unmarshal(record["verified_domains"].Bytes(), &domains); err == nil {
				account.VerifiedDomains = domains
			}
		}
		if !record["send_quota"].IsNil() && !record["send_quota"].IsEmpty() {
			var quota v1.SESQuota
			if err := json.Unmarshal(record["send_quota"].Bytes(), &quota); err == nil {
				account.SendQuota = &quota
			}
		}
		if !record["created_at"].IsNil() {
			account.CreatedAt = record["created_at"].String()
		}
		if !record["updated_at"].IsNil() {
			account.UpdatedAt = record["updated_at"].String()
		}

		domains, _ := getAccountDomains(ctx, account.Id)
		account.Domains = domains
		accounts = append(accounts, account)
	}

	return accounts, nil
}

func SaveAccountToDB(ctx context.Context, account *v1.SESAccount) (*v1.SESAccount, error) {
	now := time.Now()
	data := g.Map{
		"name":                   account.Name,
		"description":           account.Description,
		"region":                account.Region,
		"access_key":            account.AccessKey,
		"enabled":               boolToInt(account.Enabled),
		"check_interval_minutes": account.CheckIntervalMinutes,
		"updated_at":            now,
	}

	if account.SecretKey != "" && !isMaskedKey(account.SecretKey) {
		data["secret_key"] = account.SecretKey
	}

	var savedId int64

	if account.Id == 0 {
		data["created_at"] = now
		data["status"] = StatusPending
		result, err := g.DB().Model("bm_ses_accounts").Data(data).Insert()
		if err != nil {
			return nil, err
		}
		savedId, _ = result.LastInsertId()
	} else {
		_, err := g.DB().Model("bm_ses_accounts").Where("id", account.Id).Data(data).Update()
		if err != nil {
			return nil, err
		}
		savedId = account.Id
	}

	if err := updateAccountDomains(ctx, savedId, account.Domains); err != nil {
		g.Log().Error(ctx, "Failed to update domain mappings:", err)
	}

	// Credentials, region, enabled flag and domain mappings may all have
	// changed, so drop the caches rather than waiting out the TTL.
	InvalidateAccountCache()

	return GetAccountByIDFromDB(ctx, savedId)
}

func GetAccountByIDFromDB(ctx context.Context, id int64) (*v1.SESAccount, error) {
	record, err := g.DB().Model("bm_ses_accounts").Where("id", id).One()
	if err != nil {
		return nil, err
	}
	if record.IsEmpty() {
		return nil, nil
	}

	account := &v1.SESAccount{
		Id:                   record["id"].Int64(),
		Name:                 record["name"].String(),
		Description:          record["description"].String(),
		Region:               record["region"].String(),
		AccessKey:            record["access_key"].String(),
		SecretKey:            maskSecretKey(record["secret_key"].String()),
		Enabled:              record["enabled"].Int() == 1,
		Status:               record["status"].String(),
		StatusMessage:        record["status_message"].String(),
		CheckIntervalMinutes: record["check_interval_minutes"].Int(),
	}

	if !record["last_verified"].IsNil() && !record["last_verified"].IsEmpty() {
		account.LastVerified = record["last_verified"].String()
	}
	if !record["verified_domains"].IsNil() && !record["verified_domains"].IsEmpty() {
		var domains []string
		if err := json.Unmarshal(record["verified_domains"].Bytes(), &domains); err == nil {
			account.VerifiedDomains = domains
		}
	}
	if !record["send_quota"].IsNil() && !record["send_quota"].IsEmpty() {
		var quota v1.SESQuota
		if err := json.Unmarshal(record["send_quota"].Bytes(), &quota); err == nil {
			account.SendQuota = &quota
		}
	}
	if !record["created_at"].IsNil() {
		account.CreatedAt = record["created_at"].String()
	}
	if !record["updated_at"].IsNil() {
		account.UpdatedAt = record["updated_at"].String()
	}

	domains, _ := getAccountDomains(ctx, account.Id)
	account.Domains = domains

	return account, nil
}

func DeleteAccountFromDB(ctx context.Context, id int64) error {
	_, err := g.DB().Model("bm_ses_accounts").Where("id", id).Delete()
	if err == nil {
		// Mappings cascade with the account, so any cached resolution to it is
		// now wrong.
		InvalidateAccountCache()
	}
	return err
}

func GetAccountForDomainFromDB(ctx context.Context, senderEmail string) (*AccountConfig, error) {
	domain := extractDomain(senderEmail)
	sesTracef(ctx, "resolving SES account for %s (domain=%s)", senderEmail, domain)
	if domain == "" {
		sesTracef(ctx, "no domain could be extracted from %q", senderEmail)
		return nil, nil
	}

	// extractDomain already lowercases, so compare case-insensitively: a domain saved as
	// "Example.com" through the UI would otherwise never match sender "user@example.com".
	// ORDER BY id so that if legacy mixed-case duplicate rows exist (from before
	// write-side normalisation), the same one is chosen every time rather than
	// whatever the DB returns first.
	mapping, err := g.DB().Ctx(ctx).Model("bm_ses_domain_mapping").
		Where("LOWER(TRIM(domain)) = ?", domain).Order("id ASC").One()
	if err != nil {
		sesErrorf(ctx, "domain mapping query failed for %q: %v", domain, err)
		return nil, err
	}

	var accountId int64
	if mapping.IsEmpty() {
		sesTracef(ctx, "no exact mapping for %q, trying wildcard", domain)
		mapping, err = g.DB().Ctx(ctx).Model("bm_ses_domain_mapping").Where("TRIM(domain) = ?", "*").One()
		if err != nil {
			sesErrorf(ctx, "wildcard mapping query failed: %v", err)
			return nil, err
		}
		if mapping.IsEmpty() {
			sesTracef(ctx, "no mapping and no wildcard for %q; this domain uses SMTP", domain)
			return nil, nil
		}
	}
	accountId = mapping["account_id"].Int64()
	sesTracef(ctx, "domain %q maps to SES account id=%d", domain, accountId)

	record, err := g.DB().Ctx(ctx).Model("bm_ses_accounts").Where("id", accountId).Where("enabled", 1).One()
	if err != nil {
		sesErrorf(ctx, "account query failed for id=%d: %v", accountId, err)
		return nil, err
	}
	if record.IsEmpty() {
		sesWarnf(ctx, "domain %q maps to SES account id=%d, but it is missing or disabled; falling back to SMTP", domain, accountId)
		return nil, nil
	}

	status := record["status"].String()
	sesTracef(ctx, "resolved to SES account %q in region %s (status %s)",
		record["name"].String(), record["region"].String(), status)
	if status != StatusConnected && status != StatusPending {
		sesWarnf(ctx, "domain %q maps to SES account %q with status %q (need connected or pending); falling back to SMTP", domain, record["name"].String(), status)
		return nil, nil
	}

	account := &AccountConfig{
		Name:        record["name"].String(),
		Description: record["description"].String(),
		Region:      record["region"].String(),
		AccessKey:   record["access_key"].String(),
		SecretKey:   record["secret_key"].String(),
		Status:      status,
	}

	if !record["verified_domains"].IsNil() && !record["verified_domains"].IsEmpty() {
		var domains []string
		if err := json.Unmarshal(record["verified_domains"].Bytes(), &domains); err == nil {
			account.VerifiedDomains = domains
		}
	}
	if !record["send_quota"].IsNil() && !record["send_quota"].IsEmpty() {
		var quota SendQuota
		if err := json.Unmarshal(record["send_quota"].Bytes(), &quota); err == nil {
			account.SendQuota = &quota
		}
	}

	return account, nil
}

func UpdateAccountStatusInDB(ctx context.Context, id int64, status string, message string, verifiedDomains []string, quota *SendQuota) error {
	data := g.Map{
		"status":         status,
		"status_message": message,
		"last_verified":  time.Now(),
		"updated_at":     time.Now(),
	}

	if verifiedDomains != nil {
		domainsJSON, _ := json.Marshal(verifiedDomains)
		data["verified_domains"] = string(domainsJSON)
	}

	if quota != nil {
		quotaJSON, _ := json.Marshal(quota)
		data["send_quota"] = string(quotaJSON)
	}

	_, err := g.DB().Model("bm_ses_accounts").Where("id", id).Data(data).Update()
	if err == nil {
		// GetAccountForDomainFromDB only returns accounts in connected or
		// pending state, so a status transition changes routing and any cached
		// resolution has to go.
		InvalidateAccountCache()
	}
	return err
}

// MarkDBAccountFailedByName flips a DB-backed account to StatusFailed when a
// send reveals its credentials are broken.
//
// Keyed by name because the send path only knows the account name, not its id.
// The `status != failed` guard makes concurrent callers idempotent: during a
// campaign many workers can hit the same credential error at once, but only the
// first write lands and only it invalidates the cache -- the rest match no rows.
//
// A no-op (0 rows) for file-config accounts, which are not in bm_ses_accounts;
// those are handled by UpdateAccountStatus.
func MarkDBAccountFailedByName(ctx context.Context, name, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	result, err := g.DB().Model("bm_ses_accounts").
		Where("name", name).
		Where("status != ?", StatusFailed).
		Data(g.Map{
			"status":         StatusFailed,
			"status_message": message,
			"updated_at":     time.Now(),
		}).
		Update()
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		InvalidateAccountCache()
		g.Log().Warningf(ctx, "[SES] account %q marked failed after a credential error; it will be re-verified automatically once fixed: %s", name, message)
	}
	return nil
}

// VerifyAllAccountsInDB re-verifies every enabled DB-backed account against AWS.
//
// This is what keeps a DB account's stored status honest after creation. Without
// it, a revoked key leaves the account "connected" forever. Skips accounts
// verified within the last two minutes so it does not repeat work a recent save
// already did, and skips those mid-verification.
//
// Each VerifyAccountByID makes a live GetAccount call, which AWS throttles to 1
// request/second per account, so this deliberately runs serially on a periodic
// tick rather than in parallel.
func VerifyAllAccountsInDB(ctx context.Context) {
	var rows []struct {
		Id           int64  `orm:"id"`
		Status       string `orm:"status"`
		LastVerified string `orm:"last_verified"`
	}
	if err := g.DB().Model("bm_ses_accounts").
		Where("enabled", 1).
		Fields("id, status, last_verified").
		Scan(&rows); err != nil {
		sesErrorf(ctx, "could not list DB accounts for re-verification: %v", err)
		return
	}

	for _, r := range rows {
		if r.Status == StatusChecking {
			continue // a verification is already in flight for this account
		}
		if r.Status == StatusConnected && recentlyVerified(r.LastVerified) {
			continue // fresh enough; do not burn a GetAccount call
		}
		if err := VerifyAccountByID(ctx, r.Id); err != nil {
			sesTracef(ctx, "re-verification of account id=%d failed (will retry next tick): %v", r.Id, err)
		}
	}
}

// recentlyVerified reports whether a stored last_verified timestamp is within
// the last two minutes.
func recentlyVerified(lastVerified string) bool {
	if lastVerified == "" {
		return false
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, lastVerified); err == nil {
			return time.Since(t) < 2*time.Minute
		}
	}
	return false
}

func getAccountDomains(ctx context.Context, accountId int64) ([]string, error) {
	var domains []string
	result, err := g.DB().Model("bm_ses_domain_mapping").Where("account_id", accountId).All()
	if err != nil {
		return nil, err
	}
	for _, record := range result {
		domains = append(domains, record["domain"].String())
	}
	return domains, nil
}

func updateAccountDomains(ctx context.Context, accountId int64, domains []string) error {
	_, err := g.DB().Model("bm_ses_domain_mapping").Where("account_id", accountId).Delete()
	if err != nil {
		return err
	}

	for _, domain := range domains {
		// Store normalised (lower-cased, trimmed). The read side matches with
		// LOWER(TRIM(domain)), and extractDomain lower-cases the sender, so
		// storing as-typed let "Example.com" and "example.com" coexist as
		// separate rows on different accounts -- and the read then picked one
		// nondeterministically. Normalising on write makes UNIQUE(domain) do
		// its job and keeps routing deterministic.
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			continue
		}
		_, err := g.DB().Model("bm_ses_domain_mapping").Data(g.Map{
			"account_id": accountId,
			"domain":     domain,
			"created_at": time.Now(),
		}).Insert()
		if err != nil {
			return err
		}
	}

	return nil
}

func maskSecretKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "********"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func isMaskedKey(key string) bool {
	if len(key) < 8 {
		return false
	}
	return key[4:8] == "****"
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestSESCredentials(ctx context.Context, region, accessKey, secretKey string) (string, []string, *SendQuota, error) {
	cfg := aws.Config{
		Region: region,
		Credentials: credentials.NewStaticCredentialsProvider(
			accessKey,
			secretKey,
			"",
		),
	}

	client := sesv2.NewFromConfig(cfg)

	accountDetails, err := client.GetAccount(ctx, &sesv2.GetAccountInput{})
	if err != nil {
		return StatusFailed, nil, nil, err
	}

	var quota *SendQuota
	if accountDetails.SendQuota != nil {
		quota = &SendQuota{
			Max24HourSend:   accountDetails.SendQuota.Max24HourSend,
			MaxSendRate:     accountDetails.SendQuota.MaxSendRate,
			SentLast24Hours: accountDetails.SendQuota.SentLast24Hours,
		}
	}

	verifiedDomains, err := getVerifiedDomains(ctx, client)
	if err != nil {
		g.Log().Warning(ctx, "Failed to get verified domains:", err)
		verifiedDomains = []string{}
	}

	status := StatusConnected
	if !accountDetails.SendingEnabled {
		status = StatusFailed
	}

	return status, verifiedDomains, quota, nil
}

func VerifyAccountByID(ctx context.Context, id int64) error {
	record, err := g.DB().Model("bm_ses_accounts").Where("id", id).One()
	if err != nil {
		return err
	}
	if record.IsEmpty() {
		return nil
	}

	region := record["region"].String()
	accessKey := record["access_key"].String()
	secretKey := record["secret_key"].String()

	if region == "" || accessKey == "" || secretKey == "" {
		return nil
	}

	_ = UpdateAccountStatusInDB(ctx, id, StatusChecking, "Verification in progress...", nil, nil)

	status, verifiedDomains, quota, err := TestSESCredentials(ctx, region, accessKey, secretKey)
	if err != nil {
		_ = UpdateAccountStatusInDB(ctx, id, StatusFailed, err.Error(), nil, nil)
		return err
	}

	statusMsg := "Connected successfully"
	if len(verifiedDomains) > 0 {
		statusMsg = fmt.Sprintf("Verified OK - %d domains available", len(verifiedDomains))
	}

	return UpdateAccountStatusInDB(ctx, id, status, statusMsg, verifiedDomains, quota)
}

package ses_api

import (
	"billionmail-core/api/settings/v1"
	"context"
	"encoding/json"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/gogf/gf/v2/frame/g"
)

// GetAllAccountsFromDB retrieves all SES accounts from the database
func GetAllAccountsFromDB(ctx context.Context) ([]v1.SESAccount, error) {
	var accounts []v1.SESAccount

	// Query accounts
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

		// Parse last_verified
		if !record["last_verified"].IsNil() && !record["last_verified"].IsEmpty() {
			account.LastVerified = record["last_verified"].String()
		}

		// Parse verified_domains JSON
		if !record["verified_domains"].IsNil() && !record["verified_domains"].IsEmpty() {
			var domains []string
			if err := json.Unmarshal(record["verified_domains"].Bytes(), &domains); err == nil {
				account.VerifiedDomains = domains
			}
		}

		// Parse send_quota JSON
		if !record["send_quota"].IsNil() && !record["send_quota"].IsEmpty() {
			var quota v1.SESQuota
			if err := json.Unmarshal(record["send_quota"].Bytes(), &quota); err == nil {
				account.SendQuota = &quota
			}
		}

		// Parse timestamps
		if !record["created_at"].IsNil() {
			account.CreatedAt = record["created_at"].String()
		}
		if !record["updated_at"].IsNil() {
			account.UpdatedAt = record["updated_at"].String()
		}

		// Get mapped domains
		domains, _ := getAccountDomains(ctx, account.Id)
		account.Domains = domains

		accounts = append(accounts, account)
	}

	return accounts, nil
}

// SaveAccountToDB saves or updates an SES account in the database
func SaveAccountToDB(ctx context.Context, account *v1.SESAccount) (*v1.SESAccount, error) {
	now := time.Now()

	// Prepare data
	data := g.Map{
		"name":                   account.Name,
		"description":           account.Description,
		"region":                account.Region,
		"access_key":            account.AccessKey,
		"enabled":               boolToInt(account.Enabled),
		"check_interval_minutes": account.CheckIntervalMinutes,
		"updated_at":            now,
	}

	// Only update secret key if provided (not masked)
	if account.SecretKey != "" && !isMaskedKey(account.SecretKey) {
		data["secret_key"] = account.SecretKey
	}

	var savedId int64

	if account.Id == 0 {
		// New account
		data["created_at"] = now
		data["status"] = StatusPending

		result, err := g.DB().Model("bm_ses_accounts").Data(data).Insert()
		if err != nil {
			return nil, err
		}
		savedId, _ = result.LastInsertId()
	} else {
		// Update existing account
		_, err := g.DB().Model("bm_ses_accounts").Where("id", account.Id).Data(data).Update()
		if err != nil {
			return nil, err
		}
		savedId = account.Id
	}

	// Update domain mappings
	if err := updateAccountDomains(ctx, savedId, account.Domains); err != nil {
		g.Log().Error(ctx, "Failed to update domain mappings:", err)
	}

	// Return saved account
	return GetAccountByIDFromDB(ctx, savedId)
}

// GetAccountByIDFromDB retrieves an SES account by ID
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

// DeleteAccountFromDB deletes an SES account from the database
func DeleteAccountFromDB(ctx context.Context, id int64) error {
	// Domain mappings will be deleted automatically via CASCADE
	_, err := g.DB().Model("bm_ses_accounts").Where("id", id).Delete()
	return err
}

// GetAccountForDomainFromDB retrieves the SES account configured for a specific domain
func GetAccountForDomainFromDB(ctx context.Context, senderEmail string) (*AccountConfig, error) {
	domain := extractDomain(senderEmail)
	if domain == "" {
		return nil, nil
	}

	// Look up domain mapping
	mapping, err := g.DB().Model("bm_ses_domain_mapping").Where("domain", domain).One()
	if err != nil {
		return nil, err
	}

	var accountId int64
	if mapping.IsEmpty() {
		// Try wildcard mapping
		mapping, err = g.DB().Model("bm_ses_domain_mapping").Where("domain", "*").One()
		if err != nil {
			return nil, err
		}
		if mapping.IsEmpty() {
			return nil, nil
		}
	}
	accountId = mapping["account_id"].Int64()

	// Get the account
	record, err := g.DB().Model("bm_ses_accounts").Where("id", accountId).Where("enabled", 1).One()
	if err != nil {
		return nil, err
	}
	if record.IsEmpty() {
		return nil, nil
	}

	// Only return if status is connected or pending
	status := record["status"].String()
	if status != StatusConnected && status != StatusPending {
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

	// Parse verified domains
	if !record["verified_domains"].IsNil() && !record["verified_domains"].IsEmpty() {
		var domains []string
		if err := json.Unmarshal(record["verified_domains"].Bytes(), &domains); err == nil {
			account.VerifiedDomains = domains
		}
	}

	// Parse send quota
	if !record["send_quota"].IsNil() && !record["send_quota"].IsEmpty() {
		var quota SendQuota
		if err := json.Unmarshal(record["send_quota"].Bytes(), &quota); err == nil {
			account.SendQuota = &quota
		}
	}

	return account, nil
}

// UpdateAccountStatusInDB updates the status of an account in the database
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
	return err
}

// Helper functions

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
	// Delete existing mappings
	_, err := g.DB().Model("bm_ses_domain_mapping").Where("account_id", accountId).Delete()
	if err != nil {
		return err
	}

	// Insert new mappings
	for _, domain := range domains {
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

// TestSESCredentials tests the SES connection with provided credentials and returns full details
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

	// Get account details
	accountDetails, err := client.GetAccount(ctx, &sesv2.GetAccountInput{})
	if err != nil {
		return StatusFailed, nil, nil, err
	}

	// Get send quota
	var quota *SendQuota
	if accountDetails.SendQuota != nil {
		quota = &SendQuota{
			Max24HourSend:   accountDetails.SendQuota.Max24HourSend,
			MaxSendRate:     accountDetails.SendQuota.MaxSendRate,
			SentLast24Hours: accountDetails.SendQuota.SentLast24Hours,
		}
	}

	// Get verified domains
	verifiedDomains, err := getVerifiedDomains(ctx, client)
	if err != nil {
		g.Log().Warning(ctx, "Failed to get verified domains:", err)
		verifiedDomains = []string{}
	}

	// Determine status based on sending enabled
	status := StatusConnected
	if !accountDetails.SendingEnabled {
		status = StatusFailed
	}

	return status, verifiedDomains, quota, nil
}

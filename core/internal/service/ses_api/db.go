package ses_api

import (
	"billionmail-core/api/settings/v1"
	"context"
	"encoding/json"
	"fmt"
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
	return err
}

func GetAccountForDomainFromDB(ctx context.Context, senderEmail string) (*AccountConfig, error) {
	domain := extractDomain(senderEmail)
	g.Log().Infof(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: email=%s, domain=%s", senderEmail, domain)
	if domain == "" {
		g.Log().Warning(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: empty domain extracted")
		return nil, nil
	}

	mapping, err := g.DB().Model("bm_ses_domain_mapping").Where("domain", domain).One()
	if err != nil {
		g.Log().Warningf(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: domain mapping query error: %v", err)
		return nil, err
	}

	var accountId int64
	if mapping.IsEmpty() {
		g.Log().Infof(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: no exact domain match for '%s', trying wildcard", domain)
		mapping, err = g.DB().Model("bm_ses_domain_mapping").Where("domain", "*").One()
		if err != nil {
			g.Log().Warningf(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: wildcard query error: %v", err)
			return nil, err
		}
		if mapping.IsEmpty() {
			g.Log().Info(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: no wildcard mapping either, returning nil")
			return nil, nil
		}
	}
	accountId = mapping["account_id"].Int64()
	g.Log().Infof(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: found mapping, account_id=%d", accountId)

	record, err := g.DB().Model("bm_ses_accounts").Where("id", accountId).Where("enabled", 1).One()
	if err != nil {
		g.Log().Warningf(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: account query error: %v", err)
		return nil, err
	}
	if record.IsEmpty() {
		g.Log().Infof(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: no enabled account found for id=%d", accountId)
		return nil, nil
	}

	status := record["status"].String()
	g.Log().Infof(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: account found - name=%s, status=%s, region=%s",
		record["name"].String(), status, record["region"].String())
	if status != StatusConnected && status != StatusPending {
		g.Log().Infof(ctx, "[SES-DEBUG] GetAccountForDomainFromDB: account status '%s' not valid (need connected/pending)", status)
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
	return err
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

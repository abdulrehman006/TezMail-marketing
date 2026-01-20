package ses_api

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/gogf/gf/v2/frame/g"
)

// VerifyAllAccounts verifies all accounts in the configuration
func VerifyAllAccounts(ctx context.Context) {
	config := GetConfig()
	if config == nil {
		g.Log().Debug(ctx, "SES config not loaded, skipping verification")
		return
	}

	if !config.Enabled {
		g.Log().Debug(ctx, "SES API is disabled, skipping verification")
		return
	}

	g.Log().Info(ctx, "Starting SES account verification...")

	for accountName, account := range config.Accounts {
		// Skip if credentials are placeholders
		if !HasValidCredentials(account) {
			g.Log().Debug(ctx, "Skipping account", accountName, "- placeholder credentials")
			_ = UpdateAccountStatus(accountName, StatusPending, "Credentials not configured yet", nil, nil)
			continue
		}

		// Verify this account
		VerifyAccount(ctx, accountName, account)
	}

	g.Log().Info(ctx, "SES account verification completed")
}

// VerifyAccount verifies a single AWS SES account
func VerifyAccount(ctx context.Context, accountName string, account *AccountConfig) {
	g.Log().Info(ctx, "Verifying SES account:", accountName)

	// Update status to checking
	_ = UpdateAccountStatus(accountName, StatusChecking, "Verification in progress...", nil, nil)

	// Create AWS config with static credentials
	cfg := aws.Config{
		Region: account.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			account.AccessKey,
			account.SecretKey,
			"",
		),
	}

	client := sesv2.NewFromConfig(cfg)

	// Step 1: Get account details to verify credentials
	accountDetails, err := client.GetAccount(ctx, &sesv2.GetAccountInput{})
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to connect: %v", err)
		g.Log().Error(ctx, "SES account verification failed for", accountName, ":", err)
		_ = UpdateAccountStatus(accountName, StatusFailed, errorMsg, nil, nil)
		return
	}

	// Step 2: Get send quota
	var quota *SendQuota
	if accountDetails.SendQuota != nil {
		quota = &SendQuota{
			Max24HourSend:   accountDetails.SendQuota.Max24HourSend,
			MaxSendRate:     accountDetails.SendQuota.MaxSendRate,
			SentLast24Hours: accountDetails.SendQuota.SentLast24Hours,
		}
	}

	// Step 3: Get verified email identities (domains)
	verifiedDomains, err := getVerifiedDomains(ctx, client)
	if err != nil {
		g.Log().Warning(ctx, "Failed to get verified domains for", accountName, ":", err)
		verifiedDomains = []string{}
	}

	// Step 4: Check if sending is enabled
	sendingEnabled := accountDetails.SendingEnabled

	// Build status message
	var statusMsg string
	if !sendingEnabled {
		statusMsg = fmt.Sprintf("Warning: Sending is DISABLED - %d domains verified", len(verifiedDomains))
		// Check enforcement status using string comparison for compatibility
		enforcementStatus := string(accountDetails.EnforcementStatus)
		if enforcementStatus == "SHUTDOWN" {
			statusMsg = "Account is in SHUTDOWN status - contact AWS support"
		} else if enforcementStatus == "PROBATION" {
			statusMsg = fmt.Sprintf("Account is on PROBATION - %d domains verified", len(verifiedDomains))
		}
	} else {
		statusMsg = fmt.Sprintf("Verified OK - %d domains available", len(verifiedDomains))

		// Add production/sandbox info
		if accountDetails.ProductionAccessEnabled {
			statusMsg += " (Production)"
		} else {
			statusMsg += " (Sandbox mode)"
		}
	}

	// Update account status
	status := StatusConnected
	if !sendingEnabled {
		status = StatusFailed
	}

	err = UpdateAccountStatus(accountName, status, statusMsg, verifiedDomains, quota)
	if err != nil {
		g.Log().Error(ctx, "Failed to update account status:", err)
	}

	g.Log().Info(ctx, "SES account", accountName, "verification result:", statusMsg)
}

// getVerifiedDomains retrieves list of verified email identities from SES
func getVerifiedDomains(ctx context.Context, client *sesv2.Client) ([]string, error) {
	var verifiedDomains []string
	var nextToken *string

	for {
		input := &sesv2.ListEmailIdentitiesInput{
			PageSize: aws.Int32(100),
		}
		if nextToken != nil {
			input.NextToken = nextToken
		}

		result, err := client.ListEmailIdentities(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, identity := range result.EmailIdentities {
			// Only include verified identities
			if identity.SendingEnabled {
				verifiedDomains = append(verifiedDomains, aws.ToString(identity.IdentityName))
			}
		}

		if result.NextToken == nil {
			break
		}
		nextToken = result.NextToken
	}

	return verifiedDomains, nil
}

// StartPeriodicVerification starts a goroutine that periodically verifies all accounts
func StartPeriodicVerification(ctx context.Context) {
	config := GetConfig()
	if config == nil {
		return
	}

	interval := config.CheckIntervalMinutes
	if interval <= 0 {
		interval = 5 // Default to 5 minutes
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()

	g.Log().Info(ctx, "SES periodic verification started, interval:", interval, "minutes")

	for {
		select {
		case <-ticker.C:
			VerifyAllAccounts(ctx)
		case <-ctx.Done():
			g.Log().Info(ctx, "SES periodic verification stopped")
			return
		}
	}
}

// TestConnection tests if SES credentials are valid by calling GetAccount API
func TestConnection(ctx context.Context, region, accessKey, secretKey string) (bool, string, error) {
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
		return false, fmt.Sprintf("Connection failed: %v", err), err
	}

	// Build success message
	msg := "Connected successfully"
	if accountDetails.SendingEnabled {
		msg += " - Sending enabled"
	} else {
		msg += " - Warning: Sending disabled"
	}

	if accountDetails.ProductionAccessEnabled {
		msg += " (Production)"
	} else {
		msg += " (Sandbox)"
	}

	return true, msg, nil
}

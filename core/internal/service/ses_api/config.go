package ses_api

import (
	"billionmail-core/internal/service/public"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gogf/gf/v2/frame/g"
)

// SESConfig represents the main configuration structure
type SESConfig struct {
	Comment              string                    `json:"_comment,omitempty"`
	Enabled              bool                      `json:"enabled"`
	CheckIntervalMinutes int                       `json:"check_interval_minutes"`
	LastChecked          string                    `json:"last_checked"`
	Accounts             map[string]*AccountConfig `json:"accounts"`
	DomainMapping        map[string]string         `json:"domain_mapping"`
}

// AccountConfig represents a single AWS SES account configuration
type AccountConfig struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Region      string `json:"region"`
	AccessKey   string `json:"access_key"`
	SecretKey   string `json:"secret_key"`

	// Status fields - auto-updated by program
	StatusSection   string      `json:"_status_section,omitempty"`
	Status          string      `json:"status"`
	StatusMessage   string      `json:"status_message"`
	LastVerified    string      `json:"last_verified"`
	VerifiedDomains []string    `json:"verified_domains"`
	SendQuota       *SendQuota  `json:"send_quota"`
}

// SendQuota represents SES sending quota information
type SendQuota struct {
	Max24HourSend   float64 `json:"max_24_hour_send"`
	MaxSendRate     float64 `json:"max_send_rate"`
	SentLast24Hours float64 `json:"sent_last_24_hours"`
}

// Status constants
const (
	StatusPending   = "pending"
	StatusChecking  = "checking"
	StatusConnected = "connected"
	StatusFailed    = "failed"
	StatusDisabled  = "disabled"
)

var (
	configPath     = ""
	currentConfig  *SESConfig
	configMutex    sync.RWMutex
	configWatcher  *fsnotify.Watcher
	watcherStarted bool
)

// GetConfigPath returns the path to ses_config.json
func GetConfigPath() string {
	if configPath == "" {
		configPath = public.AbsPath("data/ses_config.json")
	}
	return configPath
}

// LoadConfig loads the SES configuration from file
func LoadConfig() (*SESConfig, error) {
	configMutex.Lock()
	defer configMutex.Unlock()

	path := GetConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config SESConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	currentConfig = &config
	return currentConfig, nil
}

// GetConfig returns the current configuration (thread-safe)
func GetConfig() *SESConfig {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return currentConfig
}

// SaveConfig saves the configuration back to file
func SaveConfig(config *SESConfig) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	path := GetConfigPath()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}

	currentConfig = config
	return nil
}

// UpdateAccountStatus updates the status of a specific account and saves to file
func UpdateAccountStatus(accountName string, status string, message string, verifiedDomains []string, quota *SendQuota) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	if currentConfig == nil {
		return nil
	}

	account, exists := currentConfig.Accounts[accountName]
	if !exists {
		return nil
	}

	account.Status = status
	account.StatusMessage = message
	account.LastVerified = time.Now().Format("2006-01-02 15:04:05")

	if verifiedDomains != nil {
		account.VerifiedDomains = verifiedDomains
	}

	if quota != nil {
		account.SendQuota = quota
	}

	currentConfig.LastChecked = time.Now().Format("2006-01-02 15:04:05")

	// Save to file
	path := GetConfigPath()
	data, err := json.MarshalIndent(currentConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// GetAccountForDomain returns the account configuration for a given sender domain
func GetAccountForDomain(senderEmail string) *AccountConfig {
	configMutex.RLock()
	defer configMutex.RUnlock()

	if currentConfig == nil || !currentConfig.Enabled {
		return nil
	}

	// Extract domain from email
	domain := extractDomain(senderEmail)
	if domain == "" {
		return nil
	}

	// Look up in domain mapping
	accountName := ""

	// Try exact match first
	if name, ok := currentConfig.DomainMapping[domain]; ok {
		accountName = name
	} else if name, ok := currentConfig.DomainMapping["*"]; ok {
		// Fall back to wildcard
		accountName = name
	}

	if accountName == "" {
		return nil
	}

	// Get account config
	account, exists := currentConfig.Accounts[accountName]
	if !exists {
		return nil
	}

	// Only return if account is connected or pending (allow pending for first use)
	if account.Status != StatusConnected && account.Status != StatusPending {
		return nil
	}

	return account
}

// IsEnabled returns whether SES API is enabled
func IsEnabled() bool {
	configMutex.RLock()
	defer configMutex.RUnlock()

	if currentConfig == nil {
		return false
	}
	return currentConfig.Enabled
}

// HasValidCredentials checks if an account has non-placeholder credentials
func HasValidCredentials(account *AccountConfig) bool {
	if account == nil {
		return false
	}

	// Check for placeholder values
	if account.AccessKey == "" || account.SecretKey == "" {
		return false
	}
	if strings.Contains(account.AccessKey, "YOUR_") || strings.Contains(account.SecretKey, "YOUR_") {
		return false
	}
	if account.AccessKey == "AKIAXXXXXXXXXXXXXXXX" {
		return false
	}

	return true
}

// StartConfigWatcher starts watching the config file for changes
func StartConfigWatcher(ctx context.Context) error {
	if watcherStarted {
		return nil
	}

	var err error
	configWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	go func() {
		defer configWatcher.Close()

		for {
			select {
			case event, ok := <-configWatcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					g.Log().Info(ctx, "SES config file changed, reloading...")
					if _, err := LoadConfig(); err != nil {
						g.Log().Error(ctx, "Failed to reload SES config:", err)
					} else {
						g.Log().Info(ctx, "SES config reloaded successfully")
						// Trigger verification for any pending accounts
						go VerifyAllAccounts(ctx)
					}
				}
			case err, ok := <-configWatcher.Errors:
				if !ok {
					return
				}
				g.Log().Error(ctx, "SES config watcher error:", err)
			case <-ctx.Done():
				return
			}
		}
	}()

	if err := configWatcher.Add(GetConfigPath()); err != nil {
		return err
	}

	watcherStarted = true
	g.Log().Info(ctx, "SES config file watcher started")
	return nil
}

// extractDomain extracts domain from email address
func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}

// Initialize loads config and starts watcher on package init
func Initialize(ctx context.Context) error {
	// Load initial config
	if _, err := LoadConfig(); err != nil {
		// Config file might not exist yet, that's OK
		g.Log().Warning(ctx, "SES config not loaded:", err)
		return nil
	}

	// Start file watcher
	if err := StartConfigWatcher(ctx); err != nil {
		g.Log().Warning(ctx, "Failed to start SES config watcher:", err)
	}

	// Verify all accounts on startup
	go VerifyAllAccounts(ctx)

	return nil
}

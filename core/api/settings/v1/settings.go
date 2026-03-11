package v1

import (
	"billionmail-core/utility/types/api_v1"

	"github.com/gogf/gf/v2/frame/g"
)

// SSLConfig
type SSLConfig struct {
	CertPath          string   `json:"cert_path" dc:"cert path"`
	KeyPath           string   `json:"key_path" dc:"key path"`
	CertData          string   `json:"certPem" dc:"cert data"`
	KeyData           string   `json:"privateKey" dc:"key data"`
	ValidUntil        string   `json:"notAfter" dc:"cert not after"`
	ValidFrom         string   `json:"notBefore" dc:"cert not before"`
	Issuer            string   `json:"issuer" dc:"cert issuer"`
	Subject           string   `json:"subject" dc:"cert subject"`
	DNSNames          []string `json:"dns_names" dc:"dns names"`
	SerialNumber      string   `json:"serial_number" dc:"serial number"`
	IsCA              bool     `json:"is_ca" dc:"is ca certificate"`
	IssuerCommonName  string   `json:"issuer_common_name" dc:"issuer common name"`
	SubjectCommonName string   `json:"subject_common_name" dc:"subject common name"`
	Version           int      `json:"version" dc:"version"`
	Status            bool     `json:"status" dc:"ssl certificate status"`
}

// SystemConfig
type SystemConfig struct {
	ServerIP string `json:"server_ip" dc:"server ip"`
	// Basic configuration
	AdminUsername string `json:"admin_username" dc:"admin username"`
	AdminPassword string `json:"admin_password" dc:"admin password"`
	SafePath      string `json:"safe_path" dc:"safe path"`

	// Domain name configuration
	Hostname string `json:"billionmail_hostname" dc:"billionmail hostname"`

	// Database configuration
	DBName string `json:"db_name" dc:"db name"`
	DBUser string `json:"db_user" dc:"db user"`
	DBPass string `json:"db_pass" dc:"db password"`

	// Redis configuration
	RedisPass string `json:"redis_pass" dc:"redis password"`
	RedisPort string `json:"redis_port" dc:"redis port"`

	// Port configuration
	MailPorts struct {
		SMTP       int `json:"smtp" dc:"smtp port"`
		SMTPS      int `json:"smtps" dc:"smtps port"`
		Submission int `json:"submission" dc:"submission port"`
		IMAP       int `json:"imap" dc:"imap port"`
		IMAPS      int `json:"imaps" dc:"imaps port"`
		POP        int `json:"pop" dc:"pop port"`
		POPS       int `json:"pops" dc:"pops port"`
	} `json:"mail_ports" dc:"mail ports configuration"`

	// Management port configuration
	ManagePorts struct {
		HTTP     int    `json:"http" dc:"http port"`
		HTTPS    int    `json:"https" dc:"https port"`
		Command1 string `json:"command_https" dc:"command https"`
		Command2 string `json:"command_http" dc:"command http"`
	} `json:"manage_ports" dc:"management ports configuration"`

	// Time zone configuration
	ManageTimeZone struct {
		TimeZone string `json:"timezone" dc:"time zone"`
		Command  string `json:"command" dc:"command"`
	} `json:"manage_timezone" dc:"time zone configuration"`

	// IPv4 network configuration
	IPv4Network string `json:"ipv4_network" dc:"ipv4 network"`
	Fail2ban    bool   `json:"fail2ban" dc:"fail2ban"`

	// SSL certificate configuration
	SSL SSLConfig `json:"ssl" dc:"ssl certificate configuration"`

	// IP whitelist configuration
	IPWhitelist        []g.Map `json:"ip_whitelist" dc:"ip whitelist"`
	IPWhitelistEnabled bool    `json:"ip_whitelist_enable" dc:"ip whitelist enabled"`
	// Reverse proxy domain configuration

	ReverseProxyDomain struct {
		CurrentUrl   string `json:"current_url" dc:"current url"`
		ReverseProxy string `json:"reverse_proxy" dc:"reverse proxy"`
	} `json:"reverse_proxy_domain" dc:"reverse proxy domain"`

	// API configuration
	APIDocSwagger struct {
		APIDocURL     string `json:"api_doc_url" dc:"api doc url"`
		SwaggerURL    string `json:"swagger_url" dc:"swagger url"`
		APIDocEnabled bool   `json:"api_doc_enabled" dc:"enable API documentation and Swagger UI"`
		APIToken      string `json:"api_token" dc:"API access token"`
	} `json:"api_doc_swagger" dc:"API doc and Swagger UI configuration"`

	// Local SMTP enabled
	LocalSMTPEnabled bool `json:"local_smtp_enabled" dc:"local SMTP sending enabled"`

	// blacklist config
	BlacklistConfig BlacklistConfig `json:"blacklist_config" dc:"blacklist configuration"`
	// Data retention days
	RetentionDays int `json:"retention_days" dc:"Number of days to keep log backup"`
}

type BlacklistConfig struct {
	AutoScanEnabled bool                    `json:"auto_scan_enabled" dc:"Automatic scanning switch"`
	AlertEnabled    bool                    `json:"alert_enabled" dc:"Alarm switch"`
	AlertSettings   *BlacklistAlertSettings `json:"alert_settings" dc:"Alarm settings"`
}

type BlacklistAlertSettings struct {
	Name          string   `json:"name" dc:"Alarm sender's name"`
	SenderEmail   string   `json:"sender_email" dc:"sender email" v:"required|email"`
	SMTPPassword  string   `json:"smtp_password" dc:"SMTP password" v:"required"`
	SMTPServer    string   `json:"smtp_server" dc:"SMTP server" v:"required"`
	SMTPPort      int      `json:"smtp_port" dc:"SMTP port" v:"required|between:1,65535" d:"587"`
	RecipientList []string `json:"recipient_list" dc:"recipient list" v:"required"`
}

type GetVersionReq struct {
	g.Meta        `path:"/settings/get_version" tags:"Version" method:"get" summary:"Get current version information" in:"query"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

type GetVersionRes struct {
	api_v1.StandardRes
	Data struct {
		Version       string `json:"version" dc:"Version"`
		LatestVersion string `json:"latest_version" dc:"Latest version"`
		ReleaseDate   string `json:"release_date" dc:"Release date"`
		CanUpdate     bool   `json:"can_update" dc:"CanUpdate"`
	} `json:"data" dc:"Data"`
}

// Get system configuration request
type GetSystemConfigReq struct {
	g.Meta        `path:"/settings/get_system_config" tags:"Settings" method:"get" summary:"Get system configuration"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

// Get system configuration response
type GetSystemConfigRes struct {
	api_v1.StandardRes
	Data *SystemConfig `json:"data" dc:"system configuration data"`
}

// Set system configuration request
type SetSystemConfigReq struct {
	g.Meta        `path:"/settings/set_system_config" tags:"Settings" method:"post" summary:"Set system configuration"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	SystemConfig
}

// Set system configuration response
type SetSystemConfigRes struct {
	api_v1.StandardRes
}

// Set system configuration request
type SetSystemConfigKeyReq struct {
	g.Meta        `path:"/settings/set_system_config_key" tags:"Settings" method:"post" summary:"Set system configuration"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Key           string `json:"key" dc:"configuration key"`
	Value         string `json:"value" dc:"configuration value"`
}

type SetSystemConfigKeyRes struct {
	api_v1.StandardRes
}

// Set SSL certificate request
type SetSSLConfigReq struct {
	g.Meta        `path:"/settings/set_ssl_config" tags:"Settings" method:"post" summary:"Set SSL certificate configuration"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	CertData      string `json:"certPem" dc:"certificate data"`
	KeyData       string `json:"privateKey" dc:"private key data"`
}

type SetSSLConfigRes struct {
	api_v1.StandardRes
}

type GetTimeZoneListReq struct {
	g.Meta        `path:"/settings/get_timezone_list" tags:"Settings" method:"get" summary:"Get available time zones"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}
type GetTimeZoneListRes struct {
	api_v1.StandardRes
	Data map[string][]string `json:"data" dc:"available time zones"`
}

type SetIPWhitelistReq struct {
	g.Meta        `path:"/settings/set_ip_whitelist" tags:"Settings" method:"post" summary:"Set IP whitelist"`
	Authorization string   `json:"authorization" dc:"Authorization" in:"header"`
	IPList        []string `json:"ip_list" dc:"IP list"`
}

type SetIPWhitelistRes struct {
	api_v1.StandardRes
}

type DeleteIPWhitelistReq struct {
	g.Meta        `path:"/settings/delete_ip_whitelist" tags:"Settings" method:"post" summary:"Delete IP whitelist"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	ID            int    `json:"id" dc:"ID"`
}

type DeleteIPWhitelistRes struct {
	api_v1.StandardRes
}

type AddIPWhitelistReq struct {
	g.Meta        `path:"/settings/add_ip_whitelist" tags:"Settings" method:"post" summary:"Add IP whitelist"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	IP            string `json:"ip" dc:"IP"`
}

type AddIPWhitelistRes struct {
	api_v1.StandardRes
}

type SetReverseProxyDomainReq struct {
	g.Meta        `path:"/settings/set_reverse_proxy_domain" tags:"Settings" method:"post" summary:"Set reverse proxy domain"`
	Authorization string `json:"authorization" in:"header" dc:"Authorization" v:"required"`
	Domain        string `json:"domain" dc:"Domain" v:"required"`
}

type SetReverseProxyDomainRes struct {
	api_v1.StandardRes
}

type DeleteReverseProxyDomainReq struct {
	g.Meta        `path:"/settings/delete_reverse_proxy_domain" tags:"Settings" method:"post" summary:"Delete reverse proxy domain"`
	Authorization string `json:"authorization" in:"header" dc:"Authorization" v:"required"`
}
type DeleteReverseProxyDomainRes struct {
	api_v1.StandardRes
}

// Regenerate API Token request
type RegenerateAPITokenReq struct {
	g.Meta        `path:"/settings/regenerate_api_token" tags:"Settings" method:"post" summary:"Regenerate API Token"`
	Authorization string `json:"authorization" in:"header" dc:"Authorization" v:"required"`
}

// Regenerate API Token response
type RegenerateAPITokenRes struct {
	api_v1.StandardRes
	Data struct {
		APIToken string `json:"api_token" dc:"New API Token"`
	} `json:"data" dc:"Generated API Token"`
}

// Set the status of the API documentation and Swagger
type SetAPIDocSwaggerReq struct {
	g.Meta        `path:"/settings/set_api_doc_swagger" tags:"Settings" method:"post" summary:"Set API Doc and Swagger UI configuration"`
	Authorization string `json:"authorization" in:"header" dc:"Authorization" v:"required"`
	APIDocEnabled bool   `json:"api_doc_enabled" dc:"Enable API documentation and Swagger UI"`
}
type SetAPIDocSwaggerRes struct {
	api_v1.StandardRes
}

type SetBlacklistAutoScanReq struct {
	g.Meta        `path:"/settings/set_blacklist_auto_scan" tags:"Settings" method:"post" summary:"Set blacklist auto scan switch"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Enabled       bool   `json:"enabled" dc:"Automatic scanning switch" v:"required"`
}

type SetBlacklistAutoScanRes struct {
	api_v1.StandardRes
}

type SetBlacklistAlertReq struct {
	g.Meta        `path:"/settings/set_blacklist_alert" tags:"Settings" method:"post" summary:"Set blacklist alert switch"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Enabled       bool   `json:"enabled" dc:"Blacklist alarm switch" v:"required"`
}

type SetBlacklistAlertRes struct {
	api_v1.StandardRes
}

type SetBlacklistAlertSettingsReq struct {
	g.Meta        `path:"/settings/set_blacklist_alert_settings" tags:"Settings" method:"post" summary:"Set blacklist alert settings"`
	Authorization string   `json:"authorization" dc:"Authorization" in:"header"`
	Name          string   `json:"name" dc:"Alarm sender's name" v:"required"`
	SenderEmail   string   `json:"sender_email" dc:"sender email" v:"required|email" v:"required"`
	SMTPPassword  string   `json:"smtp_password" dc:"SMTP password" v:"required" v:"required"`
	SMTPServer    string   `json:"smtp_server" dc:"SMTP server" v:"required" v:"required"`
	SMTPPort      int      `json:"smtp_port" dc:"SMTP port" v:"required|between:1,65535" d:"587" v:"required"`
	RecipientList []string `json:"recipient_list" dc:"recipient list" v:"required"`
}

type SetBlacklistAlertSettingsRes struct {
	api_v1.StandardRes
}

// ============== Local SMTP Settings ==============

type SetLocalSMTPReq struct {
	g.Meta        `path:"/settings/set_local_smtp" tags:"Settings" method:"post" summary:"Enable or disable local SMTP sending"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Enabled       bool   `json:"enabled" dc:"Enable local SMTP sending"`
}

type SetLocalSMTPRes struct {
	api_v1.StandardRes
}

// ============== SES Settings API ==============

// SESAccount represents an AWS SES account configuration
type SESAccount struct {
	Id                   int64    `json:"id" dc:"Account ID"`
	Name                 string   `json:"name" dc:"Account name" v:"required|min-length:1|max-length:100"`
	Description          string   `json:"description" dc:"Account description"`
	Region               string   `json:"region" dc:"AWS region" v:"required"`
	AccessKey            string   `json:"access_key" dc:"AWS access key" v:"required"`
	SecretKey            string   `json:"secret_key" dc:"AWS secret key"`
	Enabled              bool     `json:"enabled" dc:"Account enabled status"`
	Status               string   `json:"status" dc:"Connection status"`
	StatusMessage        string   `json:"status_message" dc:"Status message"`
	LastVerified         string   `json:"last_verified" dc:"Last verification time"`
	VerifiedDomains      []string `json:"verified_domains" dc:"Verified domains"`
	SendQuota            *SESQuota `json:"send_quota" dc:"Send quota"`
	CheckIntervalMinutes int      `json:"check_interval_minutes" dc:"Check interval in minutes"`
	Domains              []string `json:"domains" dc:"Mapped domains"`
	CreatedAt            string   `json:"created_at" dc:"Created time"`
	UpdatedAt            string   `json:"updated_at" dc:"Updated time"`
}

type SESQuota struct {
	Max24HourSend   float64 `json:"max_24_hour_send" dc:"Max 24 hour send"`
	MaxSendRate     float64 `json:"max_send_rate" dc:"Max send rate"`
	SentLast24Hours float64 `json:"sent_last_24_hours" dc:"Sent last 24 hours"`
}

// Get SES configuration request
type GetSESConfigReq struct {
	g.Meta        `path:"/settings/get_ses_config" tags:"Settings" method:"get" summary:"Get SES configuration"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

type GetSESConfigRes struct {
	api_v1.StandardRes
	Data struct {
		Accounts []SESAccount `json:"accounts" dc:"SES accounts"`
	} `json:"data" dc:"SES configuration data"`
}

// Save SES account request
type SaveSESAccountReq struct {
	g.Meta               `path:"/settings/save_ses_account" tags:"Settings" method:"post" summary:"Save SES account"`
	Authorization        string   `json:"authorization" dc:"Authorization" in:"header"`
	Id                   int64    `json:"id" dc:"Account ID (0 for new)"`
	Name                 string   `json:"name" dc:"Account name" v:"required|min-length:1|max-length:100"`
	Description          string   `json:"description" dc:"Account description"`
	Region               string   `json:"region" dc:"AWS region" v:"required"`
	AccessKey            string   `json:"access_key" dc:"AWS access key" v:"required"`
	SecretKey            string   `json:"secret_key" dc:"AWS secret key"`
	Enabled              bool     `json:"enabled" dc:"Account enabled status"`
	CheckIntervalMinutes int      `json:"check_interval_minutes" dc:"Check interval in minutes"`
	Domains              []string `json:"domains" dc:"Mapped domains"`
}

type SaveSESAccountRes struct {
	api_v1.StandardRes
	Data struct {
		Account SESAccount `json:"account" dc:"Saved SES account"`
	} `json:"data" dc:"Saved account data"`
}

// Delete SES account request
type DeleteSESAccountReq struct {
	g.Meta        `path:"/settings/delete_ses_account" tags:"Settings" method:"post" summary:"Delete SES account"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Id            int64  `json:"id" dc:"Account ID" v:"required|min:1"`
}

type DeleteSESAccountRes struct {
	api_v1.StandardRes
}

// Test SES connection request
type TestSESConnectionReq struct {
	g.Meta        `path:"/settings/test_ses_connection" tags:"Settings" method:"post" summary:"Test SES connection"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Region        string `json:"region" dc:"AWS region" v:"required"`
	AccessKey     string `json:"access_key" dc:"AWS access key" v:"required"`
	SecretKey     string `json:"secret_key" dc:"AWS secret key" v:"required"`
}

type TestSESConnectionRes struct {
	api_v1.StandardRes
	Data struct {
		Status          string   `json:"status" dc:"Connection status"`
		VerifiedDomains []string `json:"verified_domains" dc:"Verified domains"`
		SendQuota       *SESQuota `json:"send_quota" dc:"Send quota"`
	} `json:"data" dc:"Test result"`
}

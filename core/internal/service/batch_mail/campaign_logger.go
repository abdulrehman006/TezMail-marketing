package batch_mail

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// CampaignLogger provides detailed logging for each campaign
type CampaignLogger struct {
	taskId      int
	taskName    string
	logFile     *os.File
	mutex       sync.Mutex
	startTime   time.Time
	emailCount  int
	warmupDelay int
}

// Global map to store campaign loggers
var (
	campaignLoggers = make(map[int]*CampaignLogger)
	loggersMutex    sync.RWMutex
)

// GetCampaignLogger gets or creates a campaign logger for a task
func GetCampaignLogger(ctx context.Context, taskId int, taskName string, warmupDelay int) (*CampaignLogger, error) {
	loggersMutex.Lock()
	defer loggersMutex.Unlock()

	if logger, exists := campaignLoggers[taskId]; exists {
		return logger, nil
	}

	logger, err := newCampaignLogger(ctx, taskId, taskName, warmupDelay)
	if err != nil {
		return nil, err
	}

	campaignLoggers[taskId] = logger
	return logger, nil
}

// CloseCampaignLogger closes and removes the logger for a task
func CloseCampaignLogger(taskId int) {
	loggersMutex.Lock()
	defer loggersMutex.Unlock()

	if logger, exists := campaignLoggers[taskId]; exists {
		logger.Close()
		delete(campaignLoggers, taskId)
	}
}

// newCampaignLogger creates a new campaign logger
func newCampaignLogger(ctx context.Context, taskId int, taskName string, warmupDelay int) (*CampaignLogger, error) {
	// Create logs directory if it doesn't exist
	logDir := "logs/campaigns"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create log file with task ID and name
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	sanitizedName := sanitizeFileName(taskName)
	fileName := fmt.Sprintf("campaign_%d_%s_%s.log", taskId, sanitizedName, timestamp)
	filePath := filepath.Join(logDir, fileName)

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	logger := &CampaignLogger{
		taskId:      taskId,
		taskName:    taskName,
		logFile:     file,
		startTime:   time.Now(),
		emailCount:  0,
		warmupDelay: warmupDelay,
	}

	// Write header
	logger.writeHeader()

	g.Log().Infof(ctx, "Campaign logger created for task %d (%s) at: %s", taskId, taskName, filePath)
	return logger, nil
}

// sanitizeFileName removes or replaces characters that are not safe for file names
func sanitizeFileName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else if c == ' ' {
			result = append(result, '_')
		}
	}
	if len(result) > 50 {
		result = result[:50]
	}
	return string(result)
}

// writeHeader writes the log file header
func (l *CampaignLogger) writeHeader() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	header := fmt.Sprintf(`================================================================================
CAMPAIGN LOG
================================================================================
Task ID:        %d
Task Name:      %s
Warmup Delay:   %d seconds
Start Time:     %s
================================================================================

%-6s | %-25s | %-40s | %-12s | %-10s | %-15s | %s
%s
`,
		l.taskId,
		l.taskName,
		l.warmupDelay,
		l.startTime.Format("2006-01-02 15:04:05 MST"),
		"#", "TIMESTAMP", "RECIPIENT", "PROVIDER", "DELAY(s)", "STATUS", "DETAILS",
		"-------|---------------------------|------------------------------------------|--------------|------------|-----------------|------------------",
	)

	l.logFile.WriteString(header)
}

// LogEmailSending logs when an email is about to be sent (before delay)
func (l *CampaignLogger) LogEmailSending(recipientId int, recipient string, providerGroup string, delaySeconds int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.emailCount++
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	elapsed := time.Since(l.startTime).Seconds()

	line := fmt.Sprintf("%-6d | %-25s | %-40s | %-12s | %-10d | %-15s | Elapsed: %.1fs\n",
		l.emailCount,
		timestamp,
		truncateString(recipient, 40),
		providerGroup,
		delaySeconds,
		"QUEUED",
		elapsed,
	)

	l.logFile.WriteString(line)
}

// LogEmailSent logs when an email has been sent successfully
func (l *CampaignLogger) LogEmailSent(recipientId int, recipient string, providerGroup string, success bool, messageId string, errorMsg string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	elapsed := time.Since(l.startTime).Seconds()

	status := "SENT"
	details := fmt.Sprintf("MsgID: %s", truncateString(messageId, 30))
	if !success {
		status = "FAILED"
		details = fmt.Sprintf("Error: %s", truncateString(errorMsg, 50))
	}

	line := fmt.Sprintf("%-6s | %-25s | %-40s | %-12s | %-10s | %-15s | %s (%.1fs)\n",
		"",
		timestamp,
		truncateString(recipient, 40),
		providerGroup,
		"",
		status,
		details,
		elapsed,
	)

	l.logFile.WriteString(line)
}

// LogWarmupDelay logs when warmup delay is being applied
func (l *CampaignLogger) LogWarmupDelay(recipientId int, recipient string, delaySeconds int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	elapsed := time.Since(l.startTime).Seconds()

	line := fmt.Sprintf("%-6s | %-25s | %-40s | %-12s | %-10d | %-15s | Applying delay... (%.1fs)\n",
		"",
		timestamp,
		truncateString(recipient, 40),
		"",
		delaySeconds,
		"DELAY",
		elapsed,
	)

	l.logFile.WriteString(line)
}

// LogWarmupWaiting logs when waiting for warmup rate limit token
func (l *CampaignLogger) LogWarmupWaiting(recipientId int, recipient string, providerGroup string, waitSeconds int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	elapsed := time.Since(l.startTime).Seconds()

	line := fmt.Sprintf("%-6s | %-25s | %-40s | %-12s | %-10d | %-15s | Waiting for token (%.1fs)\n",
		"",
		timestamp,
		truncateString(recipient, 40),
		providerGroup,
		waitSeconds,
		"RATE_WAIT",
		elapsed,
	)

	l.logFile.WriteString(line)
}

// LogSummary logs the campaign summary at the end
func (l *CampaignLogger) LogSummary(totalSent int, totalFailed int, totalRecipients int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	duration := time.Since(l.startTime)
	avgPerEmail := float64(0)
	if totalSent > 0 {
		avgPerEmail = duration.Seconds() / float64(totalSent)
	}

	summary := fmt.Sprintf(`
================================================================================
CAMPAIGN SUMMARY
================================================================================
End Time:           %s
Total Duration:     %s
Total Recipients:   %d
Emails Sent:        %d
Emails Failed:      %d
Success Rate:       %.2f%%
Avg Time/Email:     %.2fs
Warmup Delay:       %d seconds
================================================================================
`,
		time.Now().Format("2006-01-02 15:04:05 MST"),
		formatDuration(duration),
		totalRecipients,
		totalSent,
		totalFailed,
		float64(totalSent)/float64(max(totalSent+totalFailed, 1))*100,
		avgPerEmail,
		l.warmupDelay,
	)

	l.logFile.WriteString(summary)
}

// Close closes the log file
func (l *CampaignLogger) Close() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.logFile != nil {
		l.logFile.Close()
		l.logFile = nil
	}
}

// Helper functions
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

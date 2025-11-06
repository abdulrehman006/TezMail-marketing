package settings

import (
	"billionmail-core/api/settings/v1"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/relay"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
)

func (c *ControllerV1) SetBlacklistAlertSettings(ctx context.Context, req *v1.SetBlacklistAlertSettingsReq) (res *v1.SetBlacklistAlertSettingsRes, err error) {
	res = &v1.SetBlacklistAlertSettingsRes{}

	if err := validateAlertSettings(&req.Settings); err != nil {
		res.SetError(gerror.Newf(public.LangCtx(ctx, "Parameter validation failed: {}", err.Error())))
		return res, nil
	}

	//g.Log().Info(ctx, "Testing SMTP connection...")
	if err := testSMTPConnectionWithRelay(ctx, &req.Settings); err != nil {
		res.SetError(gerror.Newf(public.LangCtx(ctx, "SMTP connection test failed: {}", err.Error())))
		return res, nil
	}

	//g.Log().Info(ctx, "Sending test email...")
	if err := sendTestEmailWithMailService(ctx, &req.Settings); err != nil {
		res.SetError(gerror.Newf(public.LangCtx(ctx, "Failed to send test email: {}", err.Error())))
		return res, nil
	}

	alertSettingsFile := public.AbsPath("../core/data/blacklist_alert_settings.json")

	jsonData, err := json.MarshalIndent(req.Settings, "", "  ")
	if err != nil {
		res.SetError(gerror.Newf(public.LangCtx(ctx, "Failed to encode alert settings: {}", err.Error())))
		return res, nil
	}

	err = gfile.PutContents(alertSettingsFile, string(jsonData))
	if err != nil {
		res.SetError(gerror.Newf(public.LangCtx(ctx, "Failed to save alert settings: {}", err.Error())))
		return res, nil
	}

	res.SetSuccess(public.LangCtx(ctx, "Alert settings saved successfully and test email sent"))
	return res, nil
}

func validateAlertSettings(settings *v1.BlacklistAlertSettings) error {
	if settings.SenderEmail == "" {
		return fmt.Errorf("sender email is required")
	}

	if !strings.Contains(settings.SenderEmail, "@") {
		return fmt.Errorf("invalid sender email format")
	}

	if settings.SMTPServer == "" {
		return fmt.Errorf("SMTP server is required")
	}

	if settings.SMTPPort < 1 || settings.SMTPPort > 65535 {
		return fmt.Errorf("SMTP port must be between 1 and 65535")
	}

	if settings.SMTPPassword == "" {
		return fmt.Errorf("SMTP password is required")
	}

	if len(settings.RecipientList) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}

	for _, recipient := range settings.RecipientList {
		if !strings.Contains(recipient, "@") {
			return fmt.Errorf("invalid recipient email format: %s", recipient)
		}
	}

	return nil
}

func testSMTPConnectionWithRelay(ctx context.Context, settings *v1.BlacklistAlertSettings) error {
	//g.Log().Infof(ctx, "Testing SMTP connection to %s:%d", settings.SMTPServer, settings.SMTPPort)

	result := relay.TestSmtpConnection(
		settings.SMTPServer,
		fmt.Sprintf("%d", settings.SMTPPort),
		settings.SenderEmail,
		settings.SMTPPassword,
	)

	if !result.Success {
		return fmt.Errorf("%s", result.Message)
	}

	//g.Log().Infof(ctx, "SMTP connection test successful: %s", result.Message)
	return nil
}

func sendTestEmailWithMailService(ctx context.Context, settings *v1.BlacklistAlertSettings) error {
	//g.Log().Infof(ctx, "Sending test email to %v", settings.RecipientList)

	subject := "Blacklist Alert Test - BillionMail"
	body := buildTestEmailHTML(settings)

	sender := mail_service.NewEmailSender()
	sender.Host = settings.SMTPServer
	sender.Port = fmt.Sprintf("%d", settings.SMTPPort)
	sender.Email = settings.SenderEmail
	sender.UserName = settings.SenderEmail
	sender.Password = settings.SMTPPassword

	err := sender.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %v", err)
	}
	defer sender.Close()

	var failedRecipients []string
	for _, recipient := range settings.RecipientList {

		message := mail_service.NewMessage(subject, body)
		message.SetRealName(settings.Name)
		messageId := sender.GenerateMessageID()
		message.SetMessageID(messageId)

		err = sender.Send(message, []string{recipient})
		if err != nil {
			g.Log().Errorf(ctx, "Failed to send test email to %s: %v", recipient, err)
			failedRecipients = append(failedRecipients, recipient)
		} else {
			g.Log().Infof(ctx, "Test email sent successfully to %s", recipient)
		}
	}

	if len(failedRecipients) > 0 {
		return fmt.Errorf("failed to send test email to %d recipient(s): %v", len(failedRecipients), failedRecipients)
	}

	g.Log().Infof(ctx, "Test email sent successfully to all %d recipients", len(settings.RecipientList))
	return nil
}

func buildTestEmailHTML(settings *v1.BlacklistAlertSettings) string {
	return fmt.Sprintf(`
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #4CAF50; color: white; padding: 10px 20px; border-radius: 5px; }
        .content { padding: 20px; background: #f9f9f9; border-radius: 5px; margin-top: 20px; }
        .footer { margin-top: 20px; padding-top: 20px; border-top: 1px solid #ddd; font-size: 12px; color: #777; }
        .success { color: #4CAF50; font-weight: bold; }
        ul { list-style: none; padding-left: 0; }
        ul li { padding: 5px 0; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>✅ Blacklist Alert Configuration Test</h2>
        </div>
        <div class="content">
            <p>Hello,</p>
            <p>This is a test email from BillionMail blacklist monitoring system.</p>
            <p class="success">✓ Your alert settings have been configured successfully!</p>
            <p><strong>Configuration Details:</strong></p>
            <ul>
                <li><strong>Configuration Name:</strong> %s</li>
                <li><strong>Sender Email:</strong> %s</li>
                <li><strong>SMTP Server:</strong> %s:%d</li>
                <li><strong>Recipients:</strong> %d email(s)</li>
            </ul>
            <p>When a domain is detected on a blacklist, you will receive an alert email similar to this one.</p>
            <p><strong>Test Time:</strong> %s</p>
        </div>
        <div class="footer">
            <p>This is an automated message from BillionMail Blacklist Monitoring System.</p>
            <p>If you did not configure this alert, please contact your system administrator.</p>
        </div>
    </div>
</body>
</html>
`, settings.Name, settings.SenderEmail, settings.SMTPServer, settings.SMTPPort,
		len(settings.RecipientList), time.Now().Format("2006-01-02 15:04:05"))
}

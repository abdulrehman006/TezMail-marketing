package batch_mail

import (
	"billionmail-core/internal/consts"
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/domains"
	"billionmail-core/internal/service/mail_service"
	"billionmail-core/internal/service/ses_api"
	"database/sql"
	"strings"

	"billionmail-core/internal/service/public"
	"context"
	"fmt"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"billionmail-core/api/batch_mail/v1"
)

func (c *ControllerV1) SendTestEmail(ctx context.Context, req *v1.SendTestEmailReq) (res *v1.SendTestEmailRes, err error) {

	res = &v1.SendTestEmailRes{}

	var template entity.EmailTemplate

	err = g.DB().Model("email_templates").
		Where("id", req.TemplateId).
		Scan(&template)

	if err != nil && err != sql.ErrNoRows {
		res.Code = 500
		res.SetError(gerror.New(public.LangCtx(ctx, "failed to get template {}", err)))
		return
	}

	// Scan leaves the struct zeroed when the id matches nothing; without this the test
	// send would happily deliver a blank email.
	if template.Id == 0 {
		res.Code = 400
		res.SetError(gerror.New(public.LangCtx(ctx, "template not found")))
		return
	}

	if !strings.Contains(template.Content, "{{ .UnsubscribeURL }}") {
		template.Content = public.AddUnsubscribeButton(template.Content)
	}

	content := strings.ReplaceAll(template.Content, "__UNSUBSCRIBE_URL__", "{{ .UnsubscribeURL }}")

	jwtToken, _ := batch_mail.GenerateUnsubscribeJWT(req.Recipient, req.TemplateId, 0, 0)
	domain := domains.GetBaseURL()
	unsubscribeURL := fmt.Sprintf("%s/api/unsubscribe", domain)
	groupURL := fmt.Sprintf("%s/api/unsubscribe/user_group", domain)
	unsubscribeJumpURL := fmt.Sprintf("%s/unsubscribe.html?jwt=%s&email=%s&url_type=%s&url_unsubscribe=%s",
		domain, jwtToken, req.Recipient, groupURL, unsubscribeURL)

	var contact entity.Contact
	subject := req.Subject
	err = g.DB().Model("bm_contacts").Where("email", req.Recipient).Scan(&contact)
	if err != nil && err != sql.ErrNoRows {
		g.Log().Error(ctx, "Failed to get contact: %v", err)
	}

	// Test recipients are usually not in bm_contacts. Rendering only for known contacts
	// meant the common case delivered raw "{{ .UnsubscribeURL }}" markup, so fall back to
	// a stand-in contact carrying just the recipient address and always render.
	if contact.Id == 0 {
		contact = entity.Contact{Email: req.Recipient, Active: 1, Status: 1}
	}

	// Passing a nil task rendered every {{ .Task.* }} variable as empty, so a template
	// using them looked broken in the test but fine in the real campaign. Stand in with
	// the values this test send is actually using.
	testTask := &entity.EmailTask{
		Addresser:  req.Addresser,
		FullName:   req.FullName,
		Subject:    req.Subject,
		TemplateId: req.TemplateId,
	}

	engine := batch_mail.GetTemplateEngine()
	content, err = engine.RenderEmailTemplate(ctx, content, &contact, testTask, unsubscribeJumpURL)
	if err != nil {
		res.Code = 500
		res.SetError(gerror.New(public.LangCtx(ctx, "failed to render email content: {}", err)))
		return
	}

	subject, err = engine.RenderEmailTemplate(ctx, subject, &contact, testTask, unsubscribeJumpURL)
	if err != nil {
		res.Code = 500
		res.SetError(gerror.New(public.LangCtx(ctx, "failed to render email subject: {}", err)))
		return
	}

	// Build unsubscribe headers
	headers := make(map[string]string)
	if unsubscribeJumpURL != "" {
		headers["List-Unsubscribe"] = fmt.Sprintf("<%s>", unsubscribeJumpURL)
		headers["List-Unsubscribe-Post"] = "List-Unsubscribe=One-Click"
	}

	// Stamp a Message-ID up front so the SES and SMTP paths produce equivalent messages.
	messageID := mail_service.NewMessageID(req.Addresser)

	// Try SES API first, fall back to SMTP
	sentViaSES, sesErr := ses_api.TrySendEmail(ctx, req.Addresser, []string{req.Recipient}, subject, content, "", req.FullName, messageID, headers)

	// A non-nil error means SES *is* configured for this sender domain but the send failed.
	// Silently falling back to Postfix here would report "sent successfully" for a test whose
	// entire purpose is to prove the SES path works, so surface the real AWS error instead.
	if !sentViaSES && sesErr != nil {
		g.Log().Error(ctx, "[SES-DEBUG] SendTestEmail: SES configured but failed for", req.Addresser, "error:", sesErr)
		res.Code = 500
		res.SetError(gerror.New(public.LangCtx(ctx, "SES API send failed: {}", sesErr.Error())))
		return
	}

	if sentViaSES {
		g.Log().Info(ctx, "[LOCAL-SMTP-GUARD] SendTestEmail: sent via SES for", req.Addresser, "→", req.Recipient)
		_ = public.WriteLog(ctx, public.LogParams{
			Type: consts.LOGTYPE.Task,
			Log:  "Send test email (SES API) :" + subject + " to " + req.Recipient + " successfully",
		})
		res.SetSuccess(public.LangCtx(ctx, "send email successfully"))
		return
	}

	// Fall back to SMTP if enabled
	if !mail_service.IsLocalSMTPEnabled() {
		g.Log().Warning(ctx, "[LOCAL-SMTP-GUARD] SendTestEmail: BLOCKED - local SMTP disabled, no SES for", req.Addresser)
		res.SetError(gerror.New(public.LangCtx(ctx, "Local SMTP is disabled and no SES configured for this domain")))
		return
	}
	g.Log().Info(ctx, "[LOCAL-SMTP-GUARD] SendTestEmail: ALLOWED - falling back to SMTP for", req.Addresser, "→", req.Recipient)
	sender, err := mail_service.NewEmailSenderWithLocal(req.Addresser)
	if err != nil {
		res.Code = 400
		res.SetError(gerror.New(public.LangCtx(ctx, "create email sender failed: : {}", err.Error())))
		return
	}
	defer sender.Close()

	message := mail_service.NewMessage(subject, content)
	message.SetMessageID(messageID)

	// Mirror the campaign path: sender display name and unsubscribe headers were only
	// ever applied to the SES branch, so SMTP test mails arrived as a bare address.
	if req.FullName != "" {
		message.SetRealName(req.FullName)
	}
	if len(headers) > 0 {
		message.SetHeaders(headers)
	}

	// send email
	err = sender.Send(message, []string{req.Recipient})
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "send email to {} failed: {}", req.Recipient, err)))
		return
	}

	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Task,
		Log:  "Send test email :" + subject + " to " + req.Recipient + " successfully",
	})

	res.SetSuccess(public.LangCtx(ctx, "send email successfully"))
	return
}

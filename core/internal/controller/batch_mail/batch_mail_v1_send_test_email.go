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

	// Load the template first — this does not depend on the delivery method,
	// so we resolve the sender (SES vs local) only after the content is ready.
	var template entity.EmailTemplate

	err = g.DB().Model("email_templates").
		Where("id", req.TemplateId).
		Scan(&template)

	if err != nil {
		res.Code = 500
		res.SetError(gerror.New(public.LangCtx(ctx, "failed to get template {}", err)))
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
	// Always render the template so {{ .UnsubscribeURL }} (and any merge tags)
	// resolve — even when the test recipient is NOT a stored contact, which is
	// the common case for test sends to externally-hosted addresses. The engine
	// tolerates a nil contact (subscriber vars resolve to empty / are cleaned)
	// and always substitutes UnsubscribeURL from the URL passed in. Previously
	// this ran only for known contacts, so a test to a non-contact delivered the
	// literal "{{ .UnsubscribeURL }}" text in the email body.
	var contactPtr *entity.Contact
	if contact.Id != 0 {
		contactPtr = &contact
	}
	engine := batch_mail.GetTemplateEngine()
	content, err = engine.RenderEmailTemplate(ctx, content, contactPtr, nil, unsubscribeJumpURL)
	if err != nil {
		res.Code = 500
		res.SetError(gerror.New(public.LangCtx(ctx, "failed to render email content: {}", err)))
		return
	}
	subject, err = engine.RenderEmailTemplate(ctx, subject, contactPtr, nil, unsubscribeJumpURL)
	if err != nil {
		res.Code = 500
		res.SetError(gerror.New(public.LangCtx(ctx, "failed to render email subject: {}", err)))
		return
	}

	// The contact lookup above tolerates sql.ErrNoRows (a test address need not
	// be a known contact). Clear any leftover tolerated error so it cannot leak
	// out of an early SES return below.
	err = nil

	// ----------------------------------------------------------------------
	// Delivery routing (mirrors the campaign path in task_executor.go):
	//   - If an SES account is ACTIVE for the sender's domain, send the test
	//     through the SES API. This makes the test match how the real campaign
	//     is delivered and lets it reach recipients on externally-hosted
	//     domains that this server does NOT host locally (which previously
	//     produced a "550 User unknown in virtual mailbox table" from Postfix).
	//   - Otherwise, fall back to the local Postfix path (original behaviour,
	//     byte-for-byte unchanged).
	// ----------------------------------------------------------------------
	if ses_api.GetAccountForDomain(req.Addresser) != nil {
		sent, sesErr := sendTestEmailViaSES(ctx, req.Addresser, req.Recipient, subject, content)
		if sent {
			if sesErr != nil {
				// Keep the upstream/technical detail in the server logs only; show the
				// client a generic, white-labelled message (never name the mail provider).
				g.Log().Warning(ctx, "test email send failed for", req.Recipient, ":", sesErr)
				res.SetError(gerror.New(public.LangCtx(ctx, "failed to send test email to {}, please verify the address and try again", req.Recipient)))
				err = sesErr
				return
			}
			_ = public.WriteLog(ctx, public.LogParams{
				Type: consts.LOGTYPE.Task,
				Log:  "Send test email :" + subject + " to " + req.Recipient + " successfully",
			})
			res.SetSuccess(public.LangCtx(ctx, "send email successfully"))
			return
		}
		// sent == false: a SES sender could not be created for this domain.
		// Fall through to local SMTP, exactly like the campaign executor does.
		g.Log().Warning(ctx, "SES configured for", req.Addresser,
			"but SES sender unavailable; falling back to local SMTP for test email")
	}

	// ---- Local Postfix path (original behaviour) --------------------------
	sender, err := mail_service.NewEmailSenderWithLocal(req.Addresser)
	if err != nil {
		res.Code = 400
		res.SetError(gerror.New(public.LangCtx(ctx, "create email sender failed: : {}", err.Error())))
		return
	}
	defer sender.Close()

	message := mail_service.NewMessage(subject, content)
	message.SetMessageID(sender.GenerateMessageID())

	// send email
	err = sender.Send(message, []string{req.Recipient})
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "send email to {} failed: {}", req.Recipient, err)))
		return
	}

	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Task,
		Log:  "Send test email :" + subject + "to" + req.Recipient + " successfully",
	})

	res.SetSuccess(public.LangCtx(ctx, "send email successfully"))
	return
}

// sendTestEmailViaSES sends the test email through the SES API for the sender's
// domain. It returns sent=false (with a nil error) ONLY when a SES sender could
// not be created — signalling the caller to fall back to local SMTP, the same
// contract the campaign executor (task_executor.go) uses. When sent=true, err
// reflects the actual SES send result (nil on success).
func sendTestEmailViaSES(ctx context.Context, addresser, recipient, subject, content string) (sent bool, err error) {
	sesSender, accountName, cerr := ses_api.GetSenderForEmail(ctx, addresser)
	if cerr != nil {
		g.Log().Warning(ctx, "Failed to create SES sender for test email", addresser, ":", cerr)
		return false, nil
	}

	result := sesSender.SendEmail(ctx, &ses_api.SendEmailInput{
		From:     addresser,
		To:       []string{recipient},
		Subject:  subject,
		HtmlBody: content,
	})
	if result == nil {
		return true, gerror.New("no response from email service")
	}
	if !result.Success {
		return true, result.Error
	}

	g.Log().Debug(ctx, "Test email sent via SES API (account:", accountName, ") to:", recipient)
	return true, nil
}

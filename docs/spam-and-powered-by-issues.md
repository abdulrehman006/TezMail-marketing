# Spam and "Powered By" Issues - Fix Documentation

**Date:** January 2026
**Issue:** Emails going to Gmail spam folder + "Powered by" branding removal

---

## Table of Contents

1. [Problem Summary](#problem-summary)
2. [Root Causes Identified](#root-causes-identified)
3. [Fixes Applied](#fixes-applied)
4. [Files Modified](#files-modified)
5. [Technical Details](#technical-details)
6. [Post-Fix Actions Required](#post-fix-actions-required)

---

## Problem Summary

### Issue 1: Emails Going to Spam
Emails sent from TezMail were landing in Gmail's spam folder due to:
- Broken unsubscribe link showing raw template syntax in email body
- Missing `List-Unsubscribe` header (RFC 2369)
- DKIM signing issues with missing DNS record

### Issue 2: Branding Removal
Request to remove all "Powered by TezMail/BillionMail" branding for white-label capability.

---

## Root Causes Identified

### 1. Unsubscribe Link Template Syntax Error

**Problem:** The template used function call syntax instead of data access syntax.

```go
// WRONG - Function call syntax (Go template)
{{ UnsubscribeURL . }}

// CORRECT - Data access syntax (gview template)
{{ .UnsubscribeURL }}
```

**Why this matters:**
- gview (GoFrame's template engine) uses Go's `text/template` package
- `{{ UnsubscribeURL . }}` tries to call a function named `UnsubscribeURL` with `.` (current context) as argument
- `{{ .UnsubscribeURL }}` accesses the `UnsubscribeURL` field from the template data map
- The function `UnsubscribeURL` was registered but not receiving proper data, causing it to return empty string
- Result: Email body showed literal `{{ UnsubscribeURL . }}` text instead of the actual URL

### 2. Missing List-Unsubscribe Header

**Problem:** No `List-Unsubscribe` header was being added to emails.

**Why this matters:**
- Gmail and other providers look for this header (RFC 2369)
- It enables "one-click unsubscribe" in email clients
- Missing header is a spam signal for email providers

### 3. DKIM Selector Missing

**Problem:** The `short` DKIM selector was configured in rspamd but DNS record was missing.

**Email header showed:**
```
dkim=permerror (bad message/signature format)
```

### 4. X-Mailer Header

**Problem:** X-Mailer header showed "BillionMail" instead of "TezMail".

---

## Fixes Applied

### Fix 1: Template Syntax Correction

Changed all occurrences from:
```
{{ UnsubscribeURL . }}
```
To:
```
{{ .UnsubscribeURL }}
```

### Fix 2: Added List-Unsubscribe Header

Added RFC 2369 compliant headers in `task_executor.go`:
```go
if unsubscribeURL != "" {
    message.SetHeader("List-Unsubscribe", fmt.Sprintf("<%s>", unsubscribeURL))
    message.SetHeader("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
}
```

### Fix 3: Modified personalizeEmail Function

Changed function signature to return unsubscribe URL:
```go
// Before
func (e *TaskExecutor) personalizeEmail(...) (string, string)

// After
func (e *TaskExecutor) personalizeEmail(...) (string, string, string)
// Returns: renderedContent, renderedSubject, unsubscribeURL
```

### Fix 4: Updated Template Render Patterns

Added `.UnsubscribeURL` to known variable patterns in `cleanUndefinedVariables`:
```go
knownVariablePatterns := []string{
    `\{\{\s*\.\s*Subscriber\s*\.\s*[^\s{}]+[^}]*}}`,
    `\{\{\s*\.\s*Task\s*\.\s*[^\s{}]+[^}]*}}`,
    `\{\{\s*\.\s*API\s*\.\s*[^\s{}]+[^}]*}}`,
    `\{\{\s*\.\s*UnsubscribeURL\s*}}`,  // Added
}
```

### Fix 5: X-Mailer Header Change

In `sending.go`:
```go
// Before
"X-Mailer: BillionMail\r\n"

// After
"X-Mailer: TezMail\r\n"
```

### Fix 6: Removed "Powered by" Branding

Removed all branding from:
- Email editor config
- Default email templates
- HTML pages (subscribe, unsubscribe, etc.)
- Go backend email templates

---

## Files Modified

### Backend Go Files

| File | Changes |
|------|---------|
| `core/internal/service/batch_mail/task_executor.go` | - Fixed template syntax check<br>- Modified `personalizeEmail` to return 3 values<br>- Added List-Unsubscribe headers<br>- Updated all call sites |
| `core/internal/service/batch_mail/template_render.go` | Added `.UnsubscribeURL` pattern to `cleanUndefinedVariables` |
| `core/internal/service/batch_mail/api_mail_send.go` | Fixed template syntax for API emails |
| `core/internal/service/public/common.go` | Fixed `AddUnsubscribeButton` function |
| `core/internal/service/batch_mail/batch_mail_v1_send_test_email.go` | Fixed test email template syntax |
| `core/internal/service/askai/prompts.go` | Fixed AI prompt template syntax |
| `core/internal/service/mail_service/sending.go` | Changed X-Mailer header to TezMail |
| `core/internal/service/domains/blacklist.go` | Removed "Powered by" from blacklist alert email |
| `core/internal/controller/settings/settings_v1_set_blacklist_alert_settings.go` | Removed "Powered by" from settings email |
| `core/internal/controller/campaign/campaign_v1_form.go` | Removed "Powered by" from campaign form email |

### Frontend TypeScript/Vue Files

| File | Changes |
|------|---------|
| `core/frontend/src/features/EmailEditor/config/config.ts` | Set `copyrightVNode = null` |
| `core/frontend/src/features/EmailEditor/config/addData.ts` | Removed "Powered by" row from default HTML |
| `core/frontend/src/features/EmailEditor/hooks/useHtml.ts` | Added null check for `copyrightVNode` |
| `core/frontend/src/features/EmailEditor/components/elements/Copyright.vue` | Added `v-if` check for null |
| `core/frontend/src/features/EmailEditor/components/elements/Link.vue` | Fixed unsubscribe link syntax |

### HTML Template Files

| File | Changes |
|------|---------|
| `core/public/html/unsubscribe_success.html` | Removed footer div and CSS |
| `core/public/html/unsubscribe_new.html` | Removed footer div and CSS |
| `core/public/html/subscribe_success.html` | Removed footer div and CSS |
| `core/public/html/subscribe_confirm.html` | Removed footer div and CSS |
| `core/public/html/already_subscribed.html` | Removed footer div and CSS |

### Email Template Files

| File | Changes |
|------|---------|
| `core/template/default_confirm_email/confirm_email.html` | Removed footer div and CSS |
| `core/template/default_welcome_email/welcome_email.html` | Removed footer div and CSS |
| `core/template/default_unsubscribe_email/unsubscribe_email.html` | Removed footer div and CSS |
| `core/template/subscription_form.html` | Removed footer element |
| `core/template/subscription_success.html` | Removed footer element |

---

## Technical Details

### Template Syntax Explanation

**Go Template Engine (gview) Syntax:**

```go
// Data Access - accessing a field from template data
{{ .FieldName }}           // Access FieldName from current context
{{ .Subscriber.Email }}    // Nested access

// Function Call - calling a registered function
{{ FunctionName arg1 arg2 }}  // Call function with arguments
{{ FunctionName . }}          // Call function with current context as argument
```

**Template Data Structure Used:**
```go
templateData := g.Map{
    "Subscriber":     subscriberData,    // map with contact info
    "Task":           taskData,          // map with task info
    "UnsubscribeURL": unsubscribeURL,    // string - the URL
    "API":            apiData,           // map with API attributes
}
```

Since `UnsubscribeURL` is a string value in the template data (not a function), we access it with `{{ .UnsubscribeURL }}`.

### List-Unsubscribe Header Format

```
List-Unsubscribe: <https://domain.com/unsubscribe?token=xxx>
List-Unsubscribe-Post: List-Unsubscribe=One-Click
```

- `List-Unsubscribe`: URL for unsubscribe action (RFC 2369)
- `List-Unsubscribe-Post`: Enables one-click unsubscribe without user confirmation (RFC 8058)

### DKIM Configuration

The system uses rspamd with two DKIM selectors:
- `default` - Primary selector
- `short` - Secondary selector

If using `short` selector, add DNS TXT record:
```
short._domainkey.yourdomain.com  TXT  "v=DKIM1; k=rsa; p=<public_key>"
```

---

## Post-Fix Actions Required

### 1. Rebuild Frontend
```bash
cd core/frontend
npm run build
```

### 2. Add DKIM DNS Record (if using short selector)
Add TXT record for `short._domainkey.yourdomain.com` with the public key from:
```bash
cat /opt/BillionMail/rspamd-data/dkim/short.pub
```

### 3. Verify Changes
Send a test email and check:
- [ ] Unsubscribe link works (not showing raw template syntax)
- [ ] List-Unsubscribe header present in email headers
- [ ] X-Mailer shows "TezMail"
- [ ] No "Powered by" text in emails
- [ ] DKIM passes (check email headers)

### 4. Monitor
- Check spam folder placement after fixes
- Monitor bounce rates
- Check DKIM/SPF/DMARC alignment in email headers

---

## Related Files Reference

### Where Unsubscribe URL is Generated
- `task_executor.go` line ~1121: `unsubscribeJumpURL` is created
- Passed to template engine via `templateData["UnsubscribeURL"]`

### Where Templates are Rendered
- `template_render.go`: `RenderEmailTemplateWithAPI` function
- Uses gview's `ParseContent` method

### Where Emails are Sent
- `task_executor.go`: `sendEmail` function
- Headers added before `sender.Send(message, recipients)`

---

## Troubleshooting

### Unsubscribe Link Still Shows Template Syntax
1. Check if template has `{{ .UnsubscribeURL }}` (not `{{ UnsubscribeURL . }}`)
2. Verify `cleanUndefinedVariables` has the pattern for `.UnsubscribeURL`
3. Rebuild frontend if using email editor

### List-Unsubscribe Header Missing
1. Check `task.Unsubscribe == 1` (unsubscribe enabled for task)
2. Verify `unsubscribeURL` is not empty
3. Check `sendEmail` function in `task_executor.go`

### DKIM Still Failing
1. Verify DNS record propagation: `dig TXT short._domainkey.yourdomain.com`
2. Check rspamd logs: `docker logs billionmail-rspamd-billionmail-1`
3. Verify selector in rspamd config matches DNS

---

**Document Version:** 1.0
**Last Updated:** January 2026

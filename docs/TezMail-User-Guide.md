# TezMail User Guide

## Complete Tutorial for Email Marketing Success

---

## Table of Contents

1. [Getting Started](#1-getting-started)
2. [Dashboard Overview](#2-dashboard-overview)
3. [Setting Up Your Domain](#3-setting-up-your-domain)
4. [Configuring SMTP Servers](#4-configuring-smtp-servers)
5. [Managing Contacts](#5-managing-contacts)
6. [Creating Email Templates](#6-creating-email-templates)
7. [Launching Email Campaigns](#7-launching-email-campaigns)
8. [Using AI Assistant](#8-using-ai-assistant)
9. [Understanding Analytics](#9-understanding-analytics)
10. [IP Warmup Guide](#10-ip-warmup-guide)
11. [Best Practices](#11-best-practices)
12. [Troubleshooting](#12-troubleshooting)

---

## 1. Getting Started

### 1.1 Logging In

1. Open your browser and go to your TezMail URL
2. Enter your **username** and **password**
3. Click **Login**

> **First Time?** Your administrator will provide your login credentials.

### 1.2 First Steps After Login

After logging in for the first time, follow this order:

1. **Add your sending domain** - Required for sending emails
2. **Configure SMTP server** - Your email sending infrastructure
3. **Import contacts** - Your subscriber list
4. **Create a template** - Design your email
5. **Launch campaign** - Start sending!

---

## 2. Dashboard Overview

### 2.1 Main Navigation

| Menu Item | Description |
|-----------|-------------|
| **Dashboard** | Overview of your email statistics |
| **Campaigns** | Create and manage email campaigns |
| **Contacts** | Manage your subscriber lists |
| **Templates** | Design and save email templates |
| **Domains** | Configure sending domains |
| **SMTP** | Set up email servers |
| **Analytics** | View detailed reports |
| **AI Assistant** | Generate content with AI |
| **Settings** | System configuration |

### 2.2 Dashboard Statistics

Your dashboard shows:

- **Total Emails Sent** - All-time sending count
- **Today's Sends** - Emails sent today
- **Open Rate** - Percentage of opened emails
- **Click Rate** - Percentage of link clicks
- **Bounce Rate** - Failed deliveries
- **Recent Campaigns** - Latest campaign status

---

## 3. Setting Up Your Domain

### 3.1 Why Domain Setup is Important

Before sending emails, you must verify your domain. This:
- Proves you own the domain
- Improves email deliverability
- Prevents your emails from going to spam

### 3.2 Adding a New Domain

1. Go to **Domains** in the menu
2. Click **Add Domain**
3. Enter your domain name (e.g., `yourbusiness.com`)
4. Click **Save**

### 3.3 DNS Configuration

After adding your domain, you'll see DNS records to add:

#### SPF Record
```
Type: TXT
Host: @
Value: v=spf1 include:yourmailserver.com ~all
```

#### DKIM Record
```
Type: TXT
Host: default._domainkey
Value: [Provided by system]
```

#### DMARC Record
```
Type: TXT
Host: _dmarc
Value: v=DMARC1; p=none; rua=mailto:dmarc@yourdomain.com
```

### 3.4 How to Add DNS Records

**For cPanel:**
1. Log in to cPanel
2. Go to **Zone Editor** or **DNS Zone Editor**
3. Click **Add Record**
4. Select **TXT** type
5. Enter the host and value
6. Save

**For Cloudflare:**
1. Log in to Cloudflare
2. Select your domain
3. Go to **DNS**
4. Click **Add Record**
5. Enter the details
6. Save

### 3.5 Verifying Your Domain

1. After adding DNS records, wait 5-10 minutes
2. Go back to **Domains** in TezMail
3. Click **Verify** next to your domain
4. Status should change to **Verified** (green)

> **Note:** DNS changes can take up to 24-48 hours to propagate globally.

---

## 4. Configuring SMTP Servers

### 4.1 What is SMTP?

SMTP (Simple Mail Transfer Protocol) is how emails are sent. You need to configure your SMTP server to send emails through TezMail.

TezMail works with any SMTP provider, but we **recommend Amazon SES** for:
- High deliverability rates
- Cost-effective pricing ($0.10 per 1,000 emails)
- Excellent reputation
- Scalable infrastructure

---

## Amazon SES Setup (Recommended)

### 4.2 What is Amazon SES?

Amazon Simple Email Service (SES) is a cloud-based email sending service by Amazon Web Services (AWS). It's one of the most reliable and cost-effective ways to send marketing emails.

**Pricing:**
- **Free Tier**: 3,000 emails/month (for first 12 months)
- **After Free Tier**: $0.10 per 1,000 emails
- No monthly fees, pay only for what you send

### 4.3 Creating an AWS Account

If you don't have an AWS account:

1. Go to [aws.amazon.com](https://aws.amazon.com)
2. Click **Create an AWS Account**
3. Enter your email and password
4. Choose **Personal** or **Business** account
5. Enter payment information (credit card required)
6. Verify your phone number
7. Select **Basic Support** (free)
8. Complete registration

### 4.4 Accessing Amazon SES

1. Log in to [AWS Console](https://console.aws.amazon.com)
2. In the search bar, type **SES**
3. Click **Amazon Simple Email Service**
4. Select your preferred **Region** (top right)

**Recommended Regions:**
| Region | Best For |
|--------|----------|
| US East (N. Virginia) | North America |
| EU (Ireland) | Europe |
| Asia Pacific (Mumbai) | Asia |
| EU (Frankfurt) | Europe/Middle East |

> **Important:** Remember your region! You'll need it for SMTP settings.

### 4.5 Verifying Your Domain in Amazon SES

Before sending emails, you must verify your domain:

**Step 1: Add Domain**
1. In SES Console, go to **Verified identities**
2. Click **Create identity**
3. Select **Domain**
4. Enter your domain (e.g., `yourbusiness.com`)
5. Check **Use a custom MAIL FROM domain** (optional but recommended)
6. Click **Create identity**

**Step 2: Add DNS Records**

Amazon SES will show you DNS records to add:

**DKIM Records (3 CNAME records):**
```
Type: CNAME
Name: xxxxxxxx._domainkey.yourdomain.com
Value: xxxxxxxx.dkim.amazonses.com
```
(You'll get 3 similar records - add all 3)

**SPF Record for Custom MAIL FROM:**
```
Type: TXT
Name: mail.yourdomain.com (or your MAIL FROM subdomain)
Value: v=spf1 include:amazonses.com ~all
```

**MX Record for Custom MAIL FROM:**
```
Type: MX
Name: mail.yourdomain.com
Value: 10 feedback-smtp.us-east-1.amazonses.com
```
(Replace `us-east-1` with your region)

**Step 3: Verify**
1. Add all DNS records to your domain
2. Wait 5-10 minutes (can take up to 72 hours)
3. Status will change to **Verified** (green checkmark)

### 4.6 Moving Out of SES Sandbox

**Important:** New SES accounts are in "Sandbox Mode" which limits sending:
- Can only send to verified email addresses
- Maximum 200 emails per day
- Maximum 1 email per second

**To request production access:**

1. In SES Console, click **Account dashboard**
2. Find **Sending enabled** status
3. Click **Request production access**
4. Fill out the form:

| Field | What to Enter |
|-------|---------------|
| **Mail type** | Marketing (for newsletters) |
| **Website URL** | Your business website |
| **Use case description** | Describe your email sending (see example below) |
| **Additional contacts** | Your email for notifications |
| **Preferred contact language** | English |

**Example Use Case Description:**
```
We are an email marketing company that helps businesses
send newsletters and promotional emails to their opt-in
subscribers.

Our email list consists of users who have voluntarily
subscribed through our website signup forms. We implement
double opt-in verification and honor all unsubscribe
requests immediately.

We follow email best practices including:
- Only sending to opt-in subscribers
- Including unsubscribe links in every email
- Maintaining list hygiene by removing bounces
- Monitoring complaint rates

Expected sending volume: 10,000-50,000 emails per month
```

5. Click **Submit request**
6. Wait 24-48 hours for approval (sometimes faster)

### 4.7 Creating SMTP Credentials

Once approved, create SMTP credentials:

1. In SES Console, go to **SMTP settings**
2. Click **Create SMTP credentials**
3. Enter IAM User Name (e.g., `ses-smtp-user`)
4. Click **Create**
5. **IMPORTANT:** Download or copy credentials immediately!

You'll receive:
- **SMTP Username**: `AKIAXXXXXXXXXXXXXXXX`
- **SMTP Password**: `XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX`

> **Warning:** You can only see the password ONCE. Save it securely!

### 4.8 SES SMTP Server Settings

**SMTP Endpoints by Region:**

| Region | SMTP Endpoint |
|--------|---------------|
| US East (N. Virginia) | `email-smtp.us-east-1.amazonaws.com` |
| US West (Oregon) | `email-smtp.us-west-2.amazonaws.com` |
| EU (Ireland) | `email-smtp.eu-west-1.amazonaws.com` |
| EU (Frankfurt) | `email-smtp.eu-central-1.amazonaws.com` |
| Asia Pacific (Mumbai) | `email-smtp.ap-south-1.amazonaws.com` |
| Asia Pacific (Singapore) | `email-smtp.ap-southeast-1.amazonaws.com` |
| Asia Pacific (Sydney) | `email-smtp.ap-southeast-2.amazonaws.com` |
| Asia Pacific (Tokyo) | `email-smtp.ap-northeast-1.amazonaws.com` |

**Ports:**
| Port | Encryption | Recommended |
|------|------------|-------------|
| 587 | STARTTLS | Yes (recommended) |
| 465 | TLS Wrapper | Yes |
| 25 | STARTTLS | No (often blocked) |
| 2587 | STARTTLS | Alternative |

### 4.9 Adding Amazon SES to TezMail

Now configure TezMail with your SES credentials:

1. Go to **SMTP** in TezMail menu
2. Click **Add Server**
3. Enter these details:

| Field | Value |
|-------|-------|
| **Name** | Amazon SES (US East) |
| **Host** | `email-smtp.us-east-1.amazonaws.com` |
| **Port** | `587` |
| **Username** | Your SMTP username (starts with AKIA...) |
| **Password** | Your SMTP password |
| **Encryption** | TLS |
| **From Email** | `noreply@yourdomain.com` |
| **From Name** | Your Company Name |

4. Click **Test Connection**
5. If successful, click **Save**

### 4.10 SES Sending Limits

After production access, your limits increase gradually:

| Stage | Daily Limit | Per Second |
|-------|-------------|------------|
| Initial | 50,000 | 14 |
| After good reputation | 100,000+ | 50+ |
| Request increase | Millions | Hundreds |

**To request higher limits:**
1. Go to SES Console > **Account dashboard**
2. Click **Request a sending limit increase**
3. Explain your needs and email practices

### 4.11 Monitoring SES Reputation

Keep your SES account healthy:

**In SES Console > Reputation metrics:**
- **Bounce Rate**: Keep under 5% (ideally under 2%)
- **Complaint Rate**: Keep under 0.1%

**If rates are high:**
- Clean your email list
- Remove invalid addresses
- Improve email content
- Check for spam complaints

### 4.12 SES Configuration Sets (Advanced)

For better tracking, create a Configuration Set:

1. Go to SES Console > **Configuration sets**
2. Click **Create set**
3. Name it (e.g., `tezmail-tracking`)
4. Add **Event destinations** for:
   - Bounces
   - Complaints
   - Deliveries
5. Save

---

## Other SMTP Providers

### 4.13 Generic SMTP Setup

If not using Amazon SES, use any SMTP provider:

| Field | Description | Example |
|-------|-------------|---------|
| **Name** | Friendly name | "Main Server" |
| **Host** | SMTP server address | smtp.yourserver.com |
| **Port** | SMTP port | 587 or 465 |
| **Username** | SMTP username | user@yourdomain.com |
| **Password** | SMTP password | ******** |
| **Encryption** | TLS or SSL | TLS |
| **From Email** | Sender email | noreply@yourdomain.com |
| **From Name** | Sender name | "Your Company" |

### 4.14 Popular SMTP Providers

| Provider | SMTP Host | Port | Notes |
|----------|-----------|------|-------|
| **Amazon SES** | email-smtp.region.amazonaws.com | 587 | Best value |
| **SendGrid** | smtp.sendgrid.net | 587 | Good deliverability |
| **Mailgun** | smtp.mailgun.org | 587 | Developer friendly |
| **SMTP2GO** | mail.smtp2go.com | 587 | Easy setup |
| **Postmark** | smtp.postmarkapp.com | 587 | Transactional focus |

### 4.15 Multiple SMTP Servers

You can add multiple SMTP servers for:
- **Load balancing** - Distribute sending across servers
- **Backup** - If one server fails, use another
- **Different domains** - Each domain uses its own server
- **Regional sending** - Use closest server to recipients

### 4.16 SMTP Priority & Rotation

Set priority for each server:
- **Priority 1** - Primary server (used first)
- **Priority 2** - Secondary server (used if primary fails)
- And so on...

**Rotation Options:**
- **Round Robin** - Alternates between servers
- **Priority Based** - Uses highest priority first
- **Random** - Randomly selects server

---

## 5. Managing Contacts

### 5.1 Contact Groups

Organize your contacts into groups for targeted campaigns.

**Creating a Group:**
1. Go to **Contacts** > **Groups**
2. Click **Create Group**
3. Enter group name (e.g., "Newsletter Subscribers")
4. Click **Save**

### 5.2 Importing Contacts

**From CSV/Excel:**

1. Go to **Contacts** > **Import**
2. Click **Upload File**
3. Select your CSV or Excel file
4. Map the columns:
   - Email (required)
   - First Name
   - Last Name
   - Company
   - Custom fields
5. Select the **Group** to add contacts to
6. Click **Import**

**CSV Format Example:**
```
email,first_name,last_name,company
john@example.com,John,Smith,ABC Corp
jane@example.com,Jane,Doe,XYZ Inc
```

### 5.3 Adding Single Contact

1. Go to **Contacts**
2. Click **Add Contact**
3. Fill in the details:
   - Email address
   - First name
   - Last name
   - Select group
4. Click **Save**

### 5.4 Managing Subscriptions

- **Active** - Contact will receive emails
- **Unsubscribed** - Contact opted out
- **Bounced** - Email address is invalid

> **Important:** Never send to unsubscribed or bounced contacts. This harms your sender reputation.

### 5.5 Exporting Contacts

1. Go to **Contacts**
2. Select contacts or group
3. Click **Export**
4. Choose format (CSV)
5. Download file

---

## 6. Creating Email Templates

### 6.1 Template Types

- **Rich Text** - Easy visual editor
- **HTML** - Full code control

### 6.2 Creating a Template

1. Go to **Templates**
2. Click **Create Template**
3. Enter template name
4. Choose editor type
5. Design your email
6. Click **Save**

### 6.3 Using the Visual Editor

The visual editor provides:

- **Text formatting** - Bold, italic, underline
- **Headings** - H1, H2, H3
- **Lists** - Bullet and numbered
- **Images** - Upload or link
- **Links** - Add clickable URLs
- **Buttons** - Call-to-action buttons
- **Dividers** - Section separators

### 6.4 Personalization Tags

Use these tags to personalize emails:

| Tag | Description | Example Output |
|-----|-------------|----------------|
| `{{first_name}}` | Contact's first name | John |
| `{{last_name}}` | Contact's last name | Smith |
| `{{email}}` | Contact's email | john@example.com |
| `{{company}}` | Contact's company | ABC Corp |
| `{{unsubscribe_link}}` | Unsubscribe URL | [Link] |

**Example Usage:**
```
Hello {{first_name}},

Thank you for being a valued customer at {{company}}.

Best regards,
Your Team
```

### 6.5 Template Best Practices

1. **Keep it simple** - Clean design works best
2. **Mobile-friendly** - Most emails are read on phones
3. **Clear CTA** - One main call-to-action
4. **Include unsubscribe** - Required by law
5. **Test before sending** - Preview on different devices

---

## 7. Launching Email Campaigns

### 7.1 Creating a Campaign

1. Go to **Campaigns**
2. Click **Create Campaign**
3. Fill in campaign details:

| Field | Description |
|-------|-------------|
| **Campaign Name** | Internal reference name |
| **Subject Line** | Email subject (recipients see this) |
| **From Name** | Sender name |
| **From Email** | Sender email address |
| **Reply-To** | Where replies go |

### 7.2 Selecting Recipients

1. Choose **Contact Group(s)** to send to
2. Or select **Individual Contacts**
3. Review recipient count

### 7.3 Choosing Template

1. Select from saved templates
2. Or create new content
3. Preview the email

### 7.4 Scheduling Options

**Send Immediately:**
- Campaign starts right away

**Schedule for Later:**
1. Select date and time
2. Choose timezone
3. Campaign will auto-start

### 7.5 Campaign Settings

| Setting | Description |
|---------|-------------|
| **Track Opens** | Monitor who opens emails |
| **Track Clicks** | Monitor link clicks |
| **Enable Warmup** | Use IP warmup (recommended for new IPs) |

### 7.6 Launching

1. Review all settings
2. Click **Preview** to see final email
3. Click **Send Test** to send yourself a test
4. Click **Launch Campaign**
5. Confirm the action

### 7.7 Campaign Status

| Status | Meaning |
|--------|---------|
| **Draft** | Not yet launched |
| **Scheduled** | Waiting to start |
| **Running** | Currently sending |
| **Paused** | Temporarily stopped |
| **Completed** | All emails sent |
| **Failed** | Error occurred |

### 7.8 Managing Active Campaigns

- **Pause** - Temporarily stop sending
- **Resume** - Continue paused campaign
- **Cancel** - Stop and cannot resume

---

## 8. Using AI Assistant

### 8.1 What Can AI Do?

TezMail's AI assistant helps you:

- Generate email subject lines
- Write email content
- Improve existing text
- Translate to other languages
- Adjust tone (formal/casual)

### 8.2 Accessing AI Assistant

1. Go to **AI Assistant** in the menu
2. Or click **AI** button while editing templates

### 8.3 Generating Subject Lines

1. Open AI Assistant
2. Select **Subject Line Generator**
3. Describe your email topic
4. Click **Generate**
5. Choose from suggestions
6. Copy to your campaign

**Example:**
```
Topic: Summer sale 50% off
Generated:
- "Summer Savings: 50% Off Everything!"
- "Don't Miss Our Biggest Summer Sale"
- "Half Price Summer - Limited Time!"
```

### 8.4 Writing Email Content

1. Select **Email Content**
2. Enter details:
   - Topic/purpose
   - Key points to include
   - Tone (professional, friendly, urgent)
3. Click **Generate**
4. Edit as needed
5. Copy to template

### 8.5 Improving Existing Text

1. Paste your text
2. Select **Improve**
3. Choose improvement type:
   - Make it shorter
   - Make it more engaging
   - Fix grammar
   - Change tone
4. Review and use

---

## 9. Understanding Analytics

### 9.1 Campaign Analytics

After a campaign, view detailed stats:

| Metric | Description |
|--------|-------------|
| **Sent** | Total emails sent |
| **Delivered** | Successfully delivered |
| **Opens** | Times email was opened |
| **Unique Opens** | Individual people who opened |
| **Clicks** | Total link clicks |
| **Unique Clicks** | Individual people who clicked |
| **Bounces** | Failed deliveries |
| **Unsubscribes** | People who opted out |

### 9.2 Understanding Open Rates

**Good open rates by industry:**
- E-commerce: 15-20%
- SaaS: 20-25%
- Media: 20-25%
- Non-profit: 25-30%

**Improve open rates:**
- Write compelling subject lines
- Send at optimal times
- Keep your list clean
- Personalize sender name

### 9.3 Understanding Click Rates

**Good click rates:**
- Average: 2-5%
- Good: 5-10%
- Excellent: 10%+

**Improve click rates:**
- Clear call-to-action
- Relevant content
- Mobile-friendly design
- Limit number of links

### 9.4 Bounce Types

| Type | Description | Action |
|------|-------------|--------|
| **Hard Bounce** | Email doesn't exist | Remove from list |
| **Soft Bounce** | Temporary issue | Retry later |
| **Blocked** | Server rejected | Check reputation |

### 9.5 Exporting Reports

1. Go to **Analytics**
2. Select campaign
3. Click **Export Report**
4. Choose format (CSV, PDF)
5. Download

---

## 10. IP Warmup Guide

### 10.1 What is IP Warmup?

When you start sending from a new IP address, email providers (Gmail, Outlook, Yahoo) don't trust you yet. Warmup gradually builds your sender reputation.

### 10.2 Why Warmup Matters

Without warmup:
- Emails go to spam
- IP gets blocked
- Poor deliverability

With warmup:
- Emails reach inbox
- Good reputation builds
- Better long-term results

### 10.3 Enabling Warmup

1. When creating a campaign, enable **Warmup Mode**
2. System automatically controls:
   - Daily sending limits
   - Hourly limits
   - Spacing between emails

### 10.4 Warmup Schedule

TezMail's automatic warmup:

| Week | Daily Limit | Notes |
|------|-------------|-------|
| Week 1 | 50-100 | Start slow |
| Week 2 | 100-250 | Gradual increase |
| Week 3 | 250-500 | Building trust |
| Week 4 | 500-1000 | Growing volume |
| Week 5+ | 1000+ | Full capacity |

> **Note:** Limits vary by email provider (Gmail, Outlook, Yahoo have different rates)

### 10.5 Warmup Best Practices

1. **Start with engaged users** - Send to people who open
2. **Clean your list** - Remove bounces immediately
3. **Be patient** - Don't rush the process
4. **Monitor metrics** - Watch for issues
5. **Maintain consistency** - Send regularly

---

## 11. Best Practices

### 11.1 Email List Hygiene

**Do:**
- Remove bounced emails immediately
- Honor unsubscribe requests
- Verify emails before importing
- Segment your lists

**Don't:**
- Buy email lists
- Send to old, unused lists
- Ignore bounce rates
- Add people without permission

### 11.2 Content Best Practices

**Subject Lines:**
- Keep under 50 characters
- Avoid spam words (FREE, URGENT, ACT NOW)
- Use personalization
- Create curiosity

**Email Body:**
- Get to the point quickly
- Use short paragraphs
- Include one main CTA
- Balance text and images

### 11.3 Sending Best Practices

**Timing:**
- Tuesday-Thursday best for B2B
- Weekends can work for B2C
- Test your audience
- Avoid holidays

**Frequency:**
- Don't overwhelm subscribers
- 1-4 emails per month typical
- Be consistent
- Let subscribers choose frequency

### 11.4 Deliverability Tips

1. **Authenticate your domain** - SPF, DKIM, DMARC
2. **Monitor reputation** - Check blacklists
3. **Use warmup** - For new IPs
4. **Clean lists** - Remove inactives
5. **Avoid spam triggers** - In content and subject

---

## 12. Troubleshooting

### 12.1 Emails Going to Spam

**Check:**
- Domain DNS records verified?
- SPF/DKIM/DMARC configured?
- Spam words in content?
- IP warmup completed?

**Fix:**
1. Verify all DNS records
2. Run warmup campaign
3. Review content for spam triggers
4. Check sender reputation

### 12.2 Low Open Rates

**Causes:**
- Poor subject lines
- Wrong send time
- List quality issues
- Deliverability problems

**Solutions:**
1. A/B test subject lines
2. Try different send times
3. Clean your list
4. Check inbox placement

### 12.3 High Bounce Rates

**Acceptable:** Under 2%
**Concerning:** 2-5%
**Critical:** Over 5%

**Fix:**
1. Verify emails before import
2. Remove bounced addresses
3. Use double opt-in
4. Clean old lists

### 12.4 Campaign Not Sending

**Check:**
1. SMTP server connected?
2. Campaign status is "Running"?
3. Check error logs
4. Verify sending limits

### 12.5 Cannot Verify Domain

**Check:**
1. DNS records added correctly?
2. Wait 24-48 hours for propagation
3. Check for typos in records
4. Verify with DNS checker tool

### 12.6 Getting Help

If you encounter issues:

1. Check this guide first
2. Review error messages
3. Contact support:
   - 📧 abdul@gmail.com
   - 📞 005332367866

---

## Quick Reference Card

### Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl + S` | Save |
| `Ctrl + N` | New |
| `Ctrl + P` | Preview |
| `Esc` | Close dialog |

### Status Icons

| Icon | Meaning |
|------|---------|
| 🟢 Green | Active/Success |
| 🟡 Yellow | Pending/Warning |
| 🔴 Red | Error/Failed |
| ⚪ Gray | Inactive/Draft |

### Common Tasks Quick Guide

| Task | Steps |
|------|-------|
| Send Campaign | Campaigns > Create > Select Recipients > Choose Template > Launch |
| Import Contacts | Contacts > Import > Upload File > Map Fields > Import |
| Create Template | Templates > Create > Design > Save |
| Add Domain | Domains > Add > Enter Domain > Add DNS > Verify |

---

## Glossary

| Term | Definition |
|------|------------|
| **Bounce** | Email that couldn't be delivered |
| **CTA** | Call-to-Action (button/link you want clicked) |
| **DKIM** | Email authentication method |
| **DMARC** | Email policy for authentication |
| **Open Rate** | Percentage of emails opened |
| **SMTP** | Protocol for sending emails |
| **SPF** | Email authentication record |
| **Warmup** | Gradual increase in sending volume |

---

**TezMail User Guide v1.0**

📧 Support: abdul@gmail.com
🌐 Website: tezhost.com
📞 Phone: 005332367866

*Last Updated: January 2026*

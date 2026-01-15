# TezMail Quick Start Guide

## Get Your First Campaign Running in 30 Minutes

---

## Step 1: Login (2 minutes)

1. Open your TezMail URL in browser
2. Enter username and password
3. Click **Login**

---

## Step 2: Add Your Domain (5 minutes)

1. Go to **Domains** menu
2. Click **Add Domain**
3. Enter your domain (e.g., `yourcompany.com`)
4. Click **Save**

**Add these DNS records to your domain:**

| Type | Host | Value |
|------|------|-------|
| TXT | @ | Copy SPF value from TezMail |
| TXT | default._domainkey | Copy DKIM value from TezMail |
| TXT | _dmarc | `v=DMARC1; p=none` |

5. Click **Verify** (wait 5-10 minutes after adding DNS)

---

## Step 3: Configure SMTP (5 minutes)

1. Go to **SMTP** menu
2. Click **Add Server**
3. Enter your SMTP details:

```
Host: smtp.yourserver.com
Port: 587
Username: your-email@domain.com
Password: your-password
Encryption: TLS
From Email: noreply@yourdomain.com
From Name: Your Company
```

4. Click **Test Connection**
5. Click **Save**

---

## Step 4: Import Contacts (5 minutes)

1. Go to **Contacts** > **Groups**
2. Click **Create Group** → name it "My First List"
3. Go to **Contacts** > **Import**
4. Upload your CSV file with contacts
5. Map the email column
6. Select your group
7. Click **Import**

**CSV Format:**
```
email,first_name,last_name
john@example.com,John,Smith
jane@example.com,Jane,Doe
```

---

## Step 5: Create Template (5 minutes)

1. Go to **Templates**
2. Click **Create Template**
3. Name it "Welcome Email"
4. Write your email:

```
Subject: Welcome to Our Newsletter!

Hello {{first_name}},

Thank you for subscribing!

We're excited to have you.

Best regards,
Your Team

{{unsubscribe_link}}
```

5. Click **Save**

---

## Step 6: Launch Campaign (5 minutes)

1. Go to **Campaigns**
2. Click **Create Campaign**
3. Fill in:
   - Campaign Name: "Welcome Campaign"
   - Subject: "Welcome to Our Newsletter!"
   - From Name: "Your Company"
   - From Email: noreply@yourdomain.com
4. Select your contact group
5. Choose your template
6. Enable **Track Opens** and **Track Clicks**
7. Click **Preview** to check
8. Click **Launch Campaign**

---

## You're Done!

Monitor your campaign in **Analytics** to see:
- How many emails were sent
- Who opened your email
- Who clicked links

---

## Next Steps

- Read the full [User Guide](TezMail-User-Guide.md)
- Try the **AI Assistant** for better content
- Enable **Warmup** for new sending IPs
- Create more contact segments

---

## Need Help?

📧 abdul@gmail.com
📞 005332367866
🌐 tezhost.com

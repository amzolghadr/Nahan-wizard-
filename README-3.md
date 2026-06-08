# Nahan Wizard — Go Edition

A clean Go-based wizard for deploying [Nahan Edge Gateway](https://github.com/itsyebekhe/nahan) on Cloudflare Workers.

**No Wrangler. No Node.js. Pure Cloudflare API. Works on Android/Termux.**

---

## Features

- Install Nahan on one or multiple Cloudflare accounts simultaneously
- Auto-generated random 32-character worker names
- Update any deployed worker
- View status of all deployed workers
- Uninstall any worker with confirmation
- Exports deployed URLs as `.txt` and `.json`

---

## Quick Install

### Linux / macOS / Android (Termux)

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/amzolghadr/Nahan-wizard-/main/install.sh)
```

Then run:

```bash
nahan-wizard
```

### Windows

Download the latest `.exe` from [Releases](https://github.com/amzolghadr/Nahan-wizard-/releases/latest) and run it.

---

## Multi-Account Deployment

Nahan Wizard supports deploying to multiple Cloudflare accounts at once using a single API token.

### Step 1 — Create an API Token with access to all accounts

1. Go to [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens)
2. Click **Create Token**
3. Use the **Edit Cloudflare Workers** template
4. Add permission: **D1 — Edit**
5. Under **Account Resources**, set to **All accounts**
6. Click **Continue to summary**, then **Create Token**
7. Copy the token

### Step 2 — Run the wizard

```bash
nahan-wizard
```

Select **1) Install**, paste your token.

The wizard will list all your accounts:

```
[+] Found 3 account(s):

   1) My Main Account   (abc12345...)
   2) Business Account  (def67890...)
   3) Dev Account       (ghi11121...)

[?] Enter numbers to deploy to (e.g: 1,3) or 'all':
 > all
```

### Step 3 — Get your URLs

After deployment, URLs are printed and saved to:
- `nahan-urls.txt` — one URL per line
- `nahan-urls.json` — JSON with account, worker, and URL

---

## Requirements

- A [Cloudflare](https://cloudflare.com) account (free tier works)
- An API Token with Workers and D1 permissions

---

## Build from source

```bash
git clone https://github.com/amzolghadr/Nahan-wizard-
cd Nahan-wizard-
go build -o nahan-wizard .
./nahan-wizard
```

---

---

# ویزارد نهان — نسخه Go

ابزاری برای نصب و مدیریت [Nahan Edge Gateway](https://github.com/itsyebekhe/nahan) روی Cloudflare Workers.

**بدون Wrangler. بدون Node.js. فقط Cloudflare API. روی اندروید/Termux هم کار می‌کنه.**

---

## امکانات

- نصب نهان روی یک یا چند اکانت Cloudflare به صورت همزمان
- نام Worker کاملاً رندوم و ۳۲ حرفی
- آپدیت هر Worker به آخرین نسخه
- مشاهده وضعیت تمام Worker های نصب‌شده
- حذف هر Worker با تأیید دوگانه
- ذخیره خروجی URL ها به صورت `.txt` و `.json`

---

## نصب سریع

### لینوکس / مک / اندروید (Termux)

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/amzolghadr/Nahan-wizard-/main/install.sh)
```

بعد اجرا کن:

```bash
nahan-wizard
```

---

## نصب روی چند اکانت به صورت همزمان

### مرحله ۱ — ساخت API Token با دسترسی به همه اکانت‌ها

1. برو به [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens)
2. روی **Create Token** کلیک کن
3. تمپلت **Edit Cloudflare Workers** رو انتخاب کن
4. دسترسی **D1 — Edit** رو اضافه کن
5. در بخش **Account Resources** گزینه **All accounts** رو انتخاب کن
6. روی **Continue to summary** و بعد **Create Token** کلیک کن
7. توکن رو کپی کن

### مرحله ۲ — اجرای ویزارد

```bash
nahan-wizard
```

گزینه **1) Install** رو انتخاب کن و توکن رو paste کن.

ویزارد لیست اکانت‌ها رو نشون می‌ده:

```
[+] Found 3 account(s):

   1) اکانت اصلی      (abc12345...)
   2) اکانت تجاری     (def67890...)
   3) اکانت توسعه     (ghi11121...)

[?] Enter numbers to deploy to (e.g: 1,3) or 'all':
 > all
```

### مرحله ۳ — دریافت URL ها

بعد از نصب، URL ها نمایش داده می‌شن و توی دو فایل ذخیره می‌شن:
- `nahan-urls.txt` — یک URL در هر خط
- `nahan-urls.json` — فرمت JSON با اطلاعات کامل

---

## نیازمندی‌ها

- اکانت [Cloudflare](https://cloudflare.com) (پلن رایگان کافیه)
- API Token با دسترسی Workers و D1

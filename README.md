# 💦 Nahan Wizard

A clean Go-based wizard for installing and managing [Nahan Edge Gateway](https://github.com/itsyebekhe/nahan) on Cloudflare Workers — no Node.js required to run the wizard itself.

## Features

| Action | Description |
|---|---|
| 🚀 Install | Login to Cloudflare, create D1 database, deploy Worker |
| 🔄 Update | Download latest `_worker.js` and redeploy |
| 📊 Status | View live Worker and database status |
| 💀 Uninstall | Remove Worker and D1 database from Cloudflare |

## Quick Install

### Linux / macOS / Termux (Android)

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/YOUR_USERNAME/nahan-wizard/main/install.sh)
```

Then run:
```bash
nahan-wizard
```

### Windows

Download the latest `.exe` from [Releases](https://github.com/YOUR_USERNAME/nahan-wizard/releases/latest) and run it.

## Build from source

```bash
git clone https://github.com/YOUR_USERNAME/nahan-wizard
cd nahan-wizard
go build -o nahan-wizard .
./nahan-wizard
```

## Requirements

- **Wrangler** (installed automatically if missing)
- **Node.js + npm** (required by Wrangler)
- A **Cloudflare** account (free tier works)

## Notes

- Config is saved in `.nahan-wizard.json` in the current directory
- `wrangler.toml` and `_worker.js` are created in the current directory during install
- Run the wizard from the same directory each time for Update/Uninstall to work correctly

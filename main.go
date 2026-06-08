package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const VERSION = "v1.1.0"

// httpClient با DNS مستقیم روی 1.1.1.1
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{}
					return d.DialContext(ctx, "udp", "1.1.1.1:53")
				},
			}
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			addrs, err := resolver.LookupHost(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, a := range addrs {
				if net.ParseIP(a).To4() != nil {
					d := net.Dialer{}
					return d.DialContext(ctx, "tcp", net.JoinHostPort(a, port))
				}
			}
			return nil, fmt.Errorf("no IPv4 address for %s", host)
		},
	},
}

const (
	NC      = "\033[0m"
	RED     = "\033[1;31m"
	GREEN   = "\033[1;32m"
	YELLOW  = "\033[1;33m"
	CYAN    = "\033[1;36m"
	MAGENTA = "\033[1;35m"
	BLUE    = "\033[1;34m"
	BOLD    = "\033[1m"
	DIM     = "\033[2m"
)

var (
	OK   = GREEN + "[+]" + NC
	ERR  = RED + "[-]" + NC
	WARN = YELLOW + "[!]" + NC
	INFO = CYAN + "[i]" + NC
	ASK  = MAGENTA + "[?]" + NC
)

type Config struct {
	AccountID  string `json:"account_id"`
	APIToken   string `json:"api_token"`
	WorkerName string `json:"worker_name"`
	DBName     string `json:"db_name"`
	DBID       string `json:"db_id"`
	WorkerURL  string `json:"worker_url"`
}

const configFile = ".nahan.json"
const workerJSURL = "https://raw.githubusercontent.com/itsyebekhe/nahan/main/_worker.js"
const cfAPI = "https://api.cloudflare.com/client/v4"
const charset = "abcdefghijklmnopqrstuvwxyz0123456789"

var reader = bufio.NewReader(os.Stdin)

// ─── Random name generator ────────────────────────────────────────────────────
func randomName(length int) string {
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────
func clearScreen() {
	if runtime.GOOS == "windows" {
		fmt.Print("\033[H\033[2J")
	} else {
		fmt.Print("\033[H\033[2J")
	}
}

func prompt(label string) string {
	fmt.Printf(" %s %s\n > ", ASK, label)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func promptDefault(label, def string) string {
	fmt.Printf(" %s %s [%s%s%s]:\n > ", ASK, label, CYAN, def, NC)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		return def
	}
	return text
}

func pressEnter(msg string) {
	fmt.Printf("\n %s %s\n", INFO, msg)
	reader.ReadString('\n')
}

func stripColor(s string) string {
	for _, c := range []string{NC, RED, GREEN, YELLOW, CYAN, MAGENTA, BLUE, BOLD, DIM} {
		s = strings.ReplaceAll(s, c, "")
	}
	return s
}

func box(title string, lines []string) {
	w := 70
	fmt.Println(CYAN + "+" + strings.Repeat("-", w) + "+" + NC)
	plain := stripColor(title)
	pad := w - len([]rune(plain)) - 1
	if pad < 0 {
		pad = 0
	}
	fmt.Printf(CYAN+"|"+NC+" %s%s "+CYAN+"|"+NC+"\n", title, strings.Repeat(" ", pad))
	fmt.Println(CYAN + "|" + strings.Repeat("-", w) + "|" + NC)
	for _, l := range lines {
		plain2 := stripColor(l)
		pad2 := w - len([]rune(plain2)) - 1
		if pad2 < 0 {
			pad2 = 0
		}
		fmt.Printf(CYAN+"|"+NC+" %s%s "+CYAN+"|"+NC+"\n", l, strings.Repeat(" ", pad2))
	}
	fmt.Println(CYAN + "+" + strings.Repeat("-", w) + "+" + NC)
}

func spinner(done chan bool, msg string) {
	frames := []string{"-", "\\", "|", "/"}
	i := 0
	for {
		select {
		case <-done:
			fmt.Print("\r" + strings.Repeat(" ", len(msg)+10) + "\r")
			return
		default:
			fmt.Printf("\r %s%s%s %s", CYAN, frames[i%len(frames)], NC, msg)
			i++
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func saveConfig(cfg Config) {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configFile, data, 0600)
}

func loadConfig() (Config, bool) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return Config{}, false
	}
	var cfg Config
	json.Unmarshal(data, &cfg)
	return cfg, cfg.APIToken != ""
}

// ─── Cloudflare API ───────────────────────────────────────────────────────────
func cfRequest(method, path, token string, body interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, cfAPI+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func cfUploadWorker(accountID, workerName, token, scriptContent, dbID string) error {
	boundary := "NahanWizardBoundary"
	metadata := map[string]interface{}{
		"main_module":        "_worker.js",
		"compatibility_date": "2023-10-30",
		"bindings": []map[string]interface{}{
			{"type": "d1", "name": "IOT_DB", "database_id": dbID},
		},
	}
	metaJSON, _ := json.Marshal(metadata)
	var buf bytes.Buffer
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Disposition: form-data; name=\"metadata\"\r\n")
	buf.WriteString("Content-Type: application/json\r\n\r\n")
	buf.Write(metaJSON)
	buf.WriteString("\r\n")
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Disposition: form-data; name=\"_worker.js\"; filename=\"_worker.js\"\r\n")
	buf.WriteString("Content-Type: application/javascript+module\r\n\r\n")
	buf.WriteString(scriptContent)
	buf.WriteString("\r\n--" + boundary + "--\r\n")
	url := fmt.Sprintf("%s/accounts/%s/workers/scripts/%s", cfAPI, accountID, workerName)
	req, err := http.NewRequest("PUT", url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if success, ok := result["success"].(bool); !ok || !success {
		errs, _ := json.Marshal(result["errors"])
		return fmt.Errorf("upload failed: %s", string(errs))
	}
	return nil
}

// getAccounts برمی‌گردونه لیست همه اکانت‌ها
func getAccounts(token string) ([]map[string]interface{}, error) {
	done := make(chan bool)
	go spinner(done, "Fetching Cloudflare accounts...")
	result, err := cfRequest("GET", "/accounts?per_page=50", token, nil)
	done <- true
	if err != nil {
		return nil, err
	}
	if success, _ := result["success"].(bool); !success {
		return nil, fmt.Errorf("invalid API token")
	}
	raw, _ := result["result"].([]interface{})
	var accounts []map[string]interface{}
	for _, a := range raw {
		if acc, ok := a.(map[string]interface{}); ok {
			accounts = append(accounts, acc)
		}
	}
	return accounts, nil
}

func createD1DB(accountID, dbName, token string) (string, error) {
	done := make(chan bool)
	go spinner(done, "Creating D1 database '"+dbName+"'...")
	result, err := cfRequest("POST",
		fmt.Sprintf("/accounts/%s/d1/database", accountID),
		token,
		map[string]string{"name": dbName},
	)
	done <- true
	if err != nil {
		return "", err
	}
	if success, _ := result["success"].(bool); success {
		res, _ := result["result"].(map[string]interface{})
		id, _ := res["uuid"].(string)
		if id == "" {
			id, _ = res["database_id"].(string)
		}
		return id, nil
	}
	// شاید قبلاً ساخته شده — لیست بگیر
	done2 := make(chan bool)
	go spinner(done2, "Checking existing databases...")
	listResult, err := cfRequest("GET",
		fmt.Sprintf("/accounts/%s/d1/database?per_page=100", accountID),
		token, nil,
	)
	done2 <- true
	if err != nil {
		return "", err
	}
	if dbs, ok := listResult["result"].([]interface{}); ok {
		for _, d := range dbs {
			db := d.(map[string]interface{})
			if db["name"] == dbName {
				id, _ := db["uuid"].(string)
				if id == "" {
					id, _ = db["database_id"].(string)
				}
				fmt.Printf("\n %s Found existing database: %s%s%s\n", OK, CYAN, dbName, NC)
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("could not create or find database")
}

func downloadWorkerJS() (string, error) {
	done := make(chan bool)
	go spinner(done, "Downloading latest nahan worker.js...")
	resp, err := httpClient.Get(workerJSURL)
	done <- true
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func enableWorkerSubdomain(accountID, workerName, token string) string {
	cfRequest("POST",
		fmt.Sprintf("/accounts/%s/workers/scripts/%s/subdomain", accountID, workerName),
		token,
		map[string]bool{"enabled": true},
	)
	result, err := cfRequest("GET",
		fmt.Sprintf("/accounts/%s/workers/subdomain", accountID),
		token, nil,
	)
	if err != nil {
		return ""
	}
	if res, ok := result["result"].(map[string]interface{}); ok {
		sub, _ := res["subdomain"].(string)
		if sub != "" {
			return fmt.Sprintf("https://%s.%s.workers.dev", workerName, sub)
		}
	}
	return ""
}

// ─── Header ───────────────────────────────────────────────────────────────────
func showHeader() {
	fmt.Println(CYAN + BOLD)
	fmt.Println(`███╗   ██╗ █████╗ ██╗  ██╗ █████╗ ███╗   ██╗`)
	fmt.Println(`████╗  ██║██╔══██╗██║  ██║██╔══██╗████╗  ██║`)
	fmt.Println(`██╔██╗ ██║███████║███████║███████║██╔██╗ ██║`)
	fmt.Println(`██║╚██╗██║██╔══██║██╔══██║██╔══██║██║╚██╗██║`)
	fmt.Println(`██║ ╚████║██║  ██║██║  ██║██║  ██║██║ ╚████║`)
	fmt.Println(`╚═╝  ╚═══╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝`)
	fmt.Println(NC)
	fmt.Println(CYAN + "+----------------------------------------------------+" + NC)
	fmt.Printf(CYAN+"|"+NC+"   "+BOLD+"Nahan Edge Gateway Wizard  --  Go Edition"+NC+"   "+CYAN+"|"+NC+"\n")
	fmt.Printf(CYAN+"|"+NC+"   "+DIM+"No Wrangler. Pure API. Works on Android."+NC+"    "+CYAN+"|"+NC+"\n")
	fmt.Printf(CYAN+"|"+NC+"   "+DIM+"Version: %-38s"+NC+CYAN+"|"+NC+"\n", VERSION)
	fmt.Println(CYAN + "+----------------------------------------------------+" + NC)
}

// ─── deployToAccount یه اکانت رو deploy می‌کنه ───────────────────────────────
func deployToAccount(accountID, accountName, token, scriptContent string) {
	fmt.Printf("\n%s Deploying to account: %s%s%s\n", INFO, CYAN, accountName, NC)

	// نام رندوم ۳۲ حرفی برای worker
	workerName := randomName(32)
	// نام رندوم ۱۶ حرفی برای DB
	dbName := "nahan-" + randomName(16)

	fmt.Printf(" %s Worker name : %s%s%s\n", INFO, CYAN, workerName, NC)
	fmt.Printf(" %s DB name     : %s%s%s\n", INFO, CYAN, dbName, NC)

	dbID, err := createD1DB(accountID, dbName, token)
	if err != nil {
		fmt.Printf(" %s DB creation failed: %s\n", ERR, err.Error())
		return
	}
	fmt.Printf(" %s Database ID : %s%s%s\n", OK, DIM, dbID, NC)

	done := make(chan bool)
	go spinner(done, "Deploying Worker...")
	err = cfUploadWorker(accountID, workerName, token, scriptContent, dbID)
	done <- true
	if err != nil {
		fmt.Printf("\n %s Deploy failed: %s\n", ERR, err.Error())
		return
	}

	workerURL := enableWorkerSubdomain(accountID, workerName, token)
	if workerURL == "" {
		workerURL = fmt.Sprintf("https://%s.YOUR-SUBDOMAIN.workers.dev", workerName)
	}

	// ذخیره config برای این اکانت
	cfg := Config{
		AccountID:  accountID,
		APIToken:   token,
		WorkerName: workerName,
		DBName:     dbName,
		DBID:       dbID,
		WorkerURL:  workerURL,
	}
	cfgFile := fmt.Sprintf(".nahan-%s.json", accountID[:8])
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(cfgFile, data, 0600)

	fmt.Printf(" %s Worker deployed!\n", OK)
	fmt.Printf(" %s Dashboard: %s%s/sync/dash%s\n", OK, CYAN, workerURL, NC)
	fmt.Printf(" %s Config saved to: %s%s%s\n", OK, DIM, cfgFile, NC)
}

// ─── INSTALL ──────────────────────────────────────────────────────────────────
func installNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ PHASE 1 ] CLOUDFLARE API TOKEN --%s\n\n", MAGENTA+BOLD, NC)

	box(INFO+" How to get your API Token", []string{
		"1. Go to: dash.cloudflare.com/profile/api-tokens",
		"2. Click 'Create Token'",
		"3. Use template: 'Edit Cloudflare Workers'",
		"4. Also add permission: D1 - Edit",
		"5. Set Account access to 'All accounts' for multi-account",
		"6. Copy the token and paste it below",
	})

	fmt.Println()
	token := prompt("Paste your Cloudflare API Token:")
	if token == "" {
		fmt.Printf(" %s Token cannot be empty.\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	// لیست همه اکانت‌ها
	accounts, err := getAccounts(token)
	if err != nil {
		fmt.Printf("\n %s %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}

	if len(accounts) == 0 {
		fmt.Printf("\n %s No accounts found.\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	// نمایش اکانت‌ها
	fmt.Printf("\n %s Found %s%d%s account(s):\n\n", OK, CYAN, len(accounts), NC)
	for i, acc := range accounts {
		name, _ := acc["name"].(string)
		id, _ := acc["id"].(string)
		fmt.Printf("   %s%d)%s %s %s(%s)%s\n", GREEN, i+1, NC, name, DIM, id[:8]+"...", NC)
	}

	var selectedAccounts []map[string]interface{}

	if len(accounts) == 1 {
		selectedAccounts = accounts
		fmt.Printf("\n %s Only one account found, using it.\n", INFO)
	} else {
		fmt.Printf("\n %s Enter account numbers to deploy to (e.g: 1,3) or 'all':\n > ", ASK)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if strings.ToLower(input) == "all" {
			selectedAccounts = accounts
		} else {
			for _, part := range strings.Split(input, ",") {
				part = strings.TrimSpace(part)
				idx := 0
				fmt.Sscanf(part, "%d", &idx)
				if idx >= 1 && idx <= len(accounts) {
					selectedAccounts = append(selectedAccounts, accounts[idx-1])
				}
			}
		}
	}

	if len(selectedAccounts) == 0 {
		fmt.Printf(" %s No valid accounts selected.\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	// دانلود worker.js یه بار
	scriptContent, err := downloadWorkerJS()
	if err != nil {
		fmt.Printf("\n %s Failed to download worker.js: %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	fmt.Printf(" %s worker.js downloaded (%d KB)\n\n", OK, len(scriptContent)/1024)

	// deploy به همه اکانت‌های انتخابی
	fmt.Println(CYAN + strings.Repeat("-", 50) + NC)
	for _, acc := range selectedAccounts {
		id, _ := acc["id"].(string)
		name, _ := acc["name"].(string)
		deployToAccount(id, name, token, scriptContent)
		fmt.Println(CYAN + strings.Repeat("-", 50) + NC)
	}

	fmt.Printf("\n %s All deployments complete!\n", OK)
	pressEnter("Press Enter to return to menu...")
}

// ─── UPDATE ───────────────────────────────────────────────────────────────────
func updateNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ UPDATE ] UPGRADE TO LATEST VERSION --%s\n\n", CYAN+BOLD, NC)

	cfg, ok := loadConfig()
	if !ok {
		fmt.Printf(" %s No saved config found. Run Install first.\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	fmt.Printf(" %s Worker: %s%s%s\n", INFO, CYAN, cfg.WorkerName, NC)
	fmt.Println()

	scriptContent, err := downloadWorkerJS()
	if err != nil {
		fmt.Printf("\n %s Download failed: %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	fmt.Printf(" %s worker.js downloaded (%d KB)\n", OK, len(scriptContent)/1024)

	done := make(chan bool)
	go spinner(done, "Redeploying Worker...")
	err = cfUploadWorker(cfg.AccountID, cfg.WorkerName, cfg.APIToken, scriptContent, cfg.DBID)
	done <- true

	if err != nil {
		fmt.Printf("\n %s Update failed: %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}

	box(GREEN+"UPDATE COMPLETE"+NC, []string{
		OK + " Latest worker.js downloaded",
		OK + " Worker '" + CYAN + cfg.WorkerName + NC + "' redeployed",
	})
	pressEnter("Press Enter to return...")
}

// ─── STATUS ───────────────────────────────────────────────────────────────────
func statusNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ STATUS ] DEPLOYMENT INFO --%s\n\n", CYAN+BOLD, NC)

	cfg, ok := loadConfig()
	if !ok {
		fmt.Printf(" %s No saved config. Run Install first.\n", WARN)
		pressEnter("Press Enter to return...")
		return
	}

	done := make(chan bool)
	go spinner(done, "Checking Worker status...")
	result, err := cfRequest("GET",
		fmt.Sprintf("/accounts/%s/workers/scripts/%s", cfg.AccountID, cfg.WorkerName),
		cfg.APIToken, nil,
	)
	done <- true

	workerStatus := YELLOW + "UNKNOWN" + NC
	if err == nil {
		if success, _ := result["success"].(bool); success {
			workerStatus = GREEN + BOLD + "ACTIVE" + NC
		}
	}

	box(CYAN+"NAHAN STATUS"+NC, []string{
		BOLD + "Worker:      " + NC + CYAN + cfg.WorkerName + NC,
		BOLD + "Database:    " + NC + CYAN + cfg.DBName + NC,
		BOLD + "URL:         " + NC + CYAN + cfg.WorkerURL + NC,
		BOLD + "Dashboard:   " + NC + CYAN + cfg.WorkerURL + "/sync/dash" + NC,
		"",
		BOLD + "Live Status: " + NC + workerStatus,
	})
	pressEnter("Press Enter to return...")
}

// ─── UNINSTALL ────────────────────────────────────────────────────────────────
func uninstallNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ UNINSTALL ] REMOVE FROM CLOUDFLARE --%s\n\n", RED+BOLD, NC)

	box(RED+"[!] DANGER: PERMANENT DELETION"+NC, []string{
		"This will delete your Worker and D1 Database.",
		RED + BOLD + "THIS CANNOT BE UNDONE." + NC,
	})
	fmt.Println()

	ans := prompt("Are you sure? (y/N)")
	if strings.ToLower(ans) != "y" {
		fmt.Printf(" %s Cancelled.\n", OK)
		time.Sleep(time.Second)
		return
	}
	ans2 := prompt("Type DESTROY to confirm:")
	if ans2 != "DESTROY" {
		fmt.Printf(" %s Confirmation failed.\n", ERR)
		time.Sleep(time.Second)
		return
	}

	cfg, ok := loadConfig()
	if !ok {
		fmt.Printf(" %s No saved config found.\n", WARN)
		pressEnter("Press Enter to return...")
		return
	}

	fmt.Println()
	done := make(chan bool)
	go spinner(done, "Deleting Worker...")
	result, err := cfRequest("DELETE",
		fmt.Sprintf("/accounts/%s/workers/scripts/%s", cfg.AccountID, cfg.WorkerName),
		cfg.APIToken, nil,
	)
	done <- true
	workerDeleted := err == nil
	if r, ok2 := result["success"].(bool); ok2 {
		workerDeleted = r
	}
	if workerDeleted {
		fmt.Printf(" %s Worker deleted\n", OK)
	} else {
		fmt.Printf(" %s Worker not found\n", WARN)
	}

	done2 := make(chan bool)
	go spinner(done2, "Deleting D1 database...")
	result2, err2 := cfRequest("DELETE",
		fmt.Sprintf("/accounts/%s/d1/database/%s", cfg.AccountID, cfg.DBID),
		cfg.APIToken, nil,
	)
	done2 <- true
	dbDeleted := err2 == nil
	if r, ok2 := result2["success"].(bool); ok2 {
		dbDeleted = r
	}
	if dbDeleted {
		fmt.Printf(" %s Database deleted\n", OK)
	} else {
		fmt.Printf(" %s Database not found\n", WARN)
	}

	os.Remove(configFile)
	fmt.Printf(" %s Local config removed\n", OK)
	fmt.Println()

	wStatus := RED + "DELETED" + NC
	if !workerDeleted {
		wStatus = YELLOW + "NOT FOUND" + NC
	}
	dStatus := RED + "DELETED" + NC
	if !dbDeleted {
		dStatus = YELLOW + "NOT FOUND" + NC
	}

	box(RED+"UNINSTALL COMPLETE"+NC, []string{
		"Worker:   " + cfg.WorkerName + " -- " + wStatus,
		"Database: " + cfg.DBName + " -- " + dStatus,
	})
	pressEnter("Press Enter to return...")
}

// ─── MAIN MENU ────────────────────────────────────────────────────────────────
func mainMenu() {
	for {
		clearScreen()
		showHeader()
		fmt.Printf("\n%s-- [ MAIN MENU ] --%s\n\n", CYAN+BOLD, NC)

		box(MAGENTA+"SELECT ACTION"+NC, []string{
			"",
			" " + GREEN + "1)" + NC + "  Install Nahan to Cloudflare",
			" " + YELLOW + "2)" + NC + "  Update existing Worker",
			" " + CYAN + "3)" + NC + "  View deployment status",
			" " + RED + "4)" + NC + "  Uninstall from Cloudflare",
			" " + BOLD + "5)" + NC + "  Exit",
			"",
		})

		fmt.Printf("\n %s Enter choice [1-5]:\n > ", ASK)
		choice, _

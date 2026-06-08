package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// httpClient forces IPv4 — fixes Termux/Android IPv6 DNS issue
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: \&http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (\&net.Dialer{}).DialContext(ctx, "tcp4", addr)
		},
	},
}

// ─── Colors ───────────────────────────────────────────────────────────────────
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

// ─── Config ───────────────────────────────────────────────────────────────────
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

var reader = bufio.NewReader(os.Stdin)

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

func box(title string, lines []string) {
	w := 70
	fmt.Println(CYAN + "┌" + strings.Repeat("─", w) + "┐" + NC)
	plain := strings.ReplaceAll(strings.ReplaceAll(title, CYAN, ""), NC, "")
	plain = strings.ReplaceAll(plain, GREEN, "")
	plain = strings.ReplaceAll(plain, RED, "")
	plain = strings.ReplaceAll(plain, YELLOW, "")
	plain = strings.ReplaceAll(plain, BOLD, "")
	plain = strings.ReplaceAll(plain, MAGENTA, "")
	pad := w - len([]rune(plain)) - 1
	if pad < 0 { pad = 0 }
	fmt.Printf(CYAN+"│"+NC+" %s%s "+CYAN+"│"+NC+"\n", title, strings.Repeat(" ", pad))
	fmt.Println(CYAN + "├" + strings.Repeat("─", w) + "┤" + NC)
	for _, l := range lines {
		p := strings.ReplaceAll(l, CYAN, "")
		p = strings.ReplaceAll(p, NC, "")
		p = strings.ReplaceAll(p, GREEN, "")
		p = strings.ReplaceAll(p, RED, "")
		p = strings.ReplaceAll(p, YELLOW, "")
		p = strings.ReplaceAll(p, BOLD, "")
		p = strings.ReplaceAll(p, MAGENTA, "")
		p = strings.ReplaceAll(p, BLUE, "")
		p = strings.ReplaceAll(p, DIM, "")
		pad2 := w - len([]rune(p)) - 1
		if pad2 < 0 { pad2 = 0 }
		fmt.Printf(CYAN+"│"+NC+" %s%s "+CYAN+"│"+NC+"\n", l, strings.Repeat(" ", pad2))
	}
	fmt.Println(CYAN + "└" + strings.Repeat("─", w) + "┘" + NC)
}

func spinner(done chan bool, msg string) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	for {
		select {
		case <-done:
			fmt.Print("\r" + strings.Repeat(" ", len(msg)+10) + "\r")
			return
		default:
			fmt.Printf("\r %s%s%s %s", CYAN, frames[i%len(frames)], NC, msg)
			i++
			time.Sleep(80 * time.Millisecond)
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

	client := httpClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func cfUploadWorker(accountID, workerName, token, scriptContent string, dbName, dbID string) error {
	// Build multipart form with metadata + script
	boundary := "----NahanWizardBoundary"

	metadata := map[string]interface{}{
		"main_module": "_worker.js",
		"compatibility_date": "2023-10-30",
		"bindings": []map[string]interface{}{
			{
				"type":          "d1",
				"name":          "IOT_DB",
				"database_id":   dbID,
			},
		},
	}
	metaJSON, _ := json.Marshal(metadata)

	var buf bytes.Buffer
	// metadata part
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString(`Content-Disposition: form-data; name="metadata"` + "\r\n")
	buf.WriteString("Content-Type: application/json\r\n\r\n")
	buf.Write(metaJSON)
	buf.WriteString("\r\n")
	// script part
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString(`Content-Disposition: form-data; name="_worker.js"; filename="_worker.js"` + "\r\n")
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

	client := httpClient
	resp, err := client.Do(req)
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

func getAccountID(token string) (string, string, error) {
	done := make(chan bool)
	go spinner(done, "Fetching Cloudflare account info...")
	result, err := cfRequest("GET", "/accounts?per_page=1", token, nil)
	done <- true
	if err != nil {
		return "", "", err
	}
	success, _ := result["success"].(bool)
	if !success {
		return "", "", fmt.Errorf("invalid API token or no accounts found")
	}
	accounts, _ := result["result"].([]interface{})
	if len(accounts) == 0 {
		return "", "", fmt.Errorf("no Cloudflare accounts found")
	}
	acc := accounts[0].(map[string]interface{})
	id, _ := acc["id"].(string)
	name, _ := acc["name"].(string)
	return id, name, nil
}

func createD1DB(accountID, dbName, token string) (string, error) {
	done := make(chan bool)
	go spinner(done, "Creating D1 database...")
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

	// Maybe already exists — try listing
	done2 := make(chan bool)
	go spinner(done2, "Database may exist, fetching list...")
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
				fmt.Printf(" %s Found existing database: %s%s%s\n", OK, CYAN, dbName, NC)
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

func enableWorkerSubdomain(accountID, workerName, token string) (string, error) {
	// Enable workers.dev subdomain
	cfRequest("POST",
		fmt.Sprintf("/accounts/%s/workers/scripts/%s/subdomain", accountID, workerName),
		token,
		map[string]bool{"enabled": true},
	)

	// Get subdomain
	result, err := cfRequest("GET",
		fmt.Sprintf("/accounts/%s/workers/subdomain", accountID),
		token, nil,
	)
	if err != nil {
		return "", err
	}
	if res, ok := result["result"].(map[string]interface{}); ok {
		sub, _ := res["subdomain"].(string)
		if sub != "" {
			return fmt.Sprintf("https://%s.%s.workers.dev", workerName, sub), nil
		}
	}
	return "", nil
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
	fmt.Println(CYAN + "┌──────────────────────────────────────────────────────┐" + NC)
	fmt.Println(CYAN + "│" + NC + "    " + BOLD + "Nahan Edge Gateway Wizard  —  Go Edition" + NC + "      " + CYAN + "│" + NC)
	fmt.Println(CYAN + "│" + NC + "    " + DIM + "No Wrangler. Pure API. Works on Android." + NC + "      " + CYAN + "│" + NC)
	fmt.Println(CYAN + "└──────────────────────────────────────────────────────┘" + NC)
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
		"5. Click 'Continue to summary' then 'Create Token'",
		"6. Copy the token and paste it below",
	})

	fmt.Println()
	token := prompt("Paste your Cloudflare API Token:")
	if token == "" {
		fmt.Printf(" %s Token cannot be empty.\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	// Verify token + get account
	accountID, accountName, err := getAccountID(token)
	if err != nil {
		fmt.Printf("\n %s %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	fmt.Printf("\n %s Logged in as: %s%s%s\n", OK, CYAN, accountName, NC)
	fmt.Printf(" %s Account ID:   %s%s%s\n", OK, DIM, accountID, NC)

	pressEnter("Phase 1 complete! Press Enter to create D1 database...")

	// Phase 2: D1
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ PHASE 2 ] D1 DATABASE --%s\n\n", MAGENTA+BOLD, NC)

	dbName := promptDefault("D1 Database name", "nahan-db")
	fmt.Println()

	dbID, err := createD1DB(accountID, dbName, token)
	if err != nil {
		fmt.Printf("\n %s %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	fmt.Printf("\n %s Database ready: %s%s%s\n", OK, CYAN, dbName, NC)
	fmt.Printf(" %s Database ID:    %s%s%s\n", OK, DIM, dbID, NC)

	pressEnter("Phase 2 complete! Press Enter to deploy Worker...")

	// Phase 3: Deploy
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ PHASE 3 ] DEPLOY WORKER --%s\n\n", MAGENTA+BOLD, NC)

	workerName := promptDefault("Worker name", "nahan-core")
	fmt.Println()

	// Download worker.js
	scriptContent, err := downloadWorkerJS()
	if err != nil {
		fmt.Printf("\n %s Failed to download worker.js: %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	fmt.Printf(" %s worker.js downloaded (%d KB)\n", OK, len(scriptContent)/1024)

	// Upload worker
	done := make(chan bool)
	go spinner(done, "Deploying Worker to Cloudflare Edge...")
	err = cfUploadWorker(accountID, workerName, token, scriptContent, dbName, dbID)
	done <- true

	if err != nil {
		fmt.Printf("\n %s Deploy failed: %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	fmt.Printf("\n %s Worker deployed: %s%s%s\n", OK, CYAN, workerName, NC)

	// Enable subdomain
	workerURL, _ := enableWorkerSubdomain(accountID, workerName, token)
	if workerURL == "" {
		workerURL = fmt.Sprintf("https://%s.YOUR-SUBDOMAIN.workers.dev", workerName)
	}

	// Save config
	cfg := Config{
		AccountID:  accountID,
		APIToken:   token,
		WorkerName: workerName,
		DBName:     dbName,
		DBID:       dbID,
		WorkerURL:  workerURL,
	}
	saveConfig(cfg)

	// Success
	clearScreen()
	fmt.Println(GREEN + BOLD)
	fmt.Println(`  ██████╗  ██████╗ ███╗   ██╗███████╗`)
	fmt.Println(`  ██╔══██╗██╔═══██╗████╗  ██║██╔════╝`)
	fmt.Println(`  ██║  ██║██║   ██║██╔██╗ ██║█████╗  `)
	fmt.Println(`  ██║  ██║██║   ██║██║╚██╗██║██╔══╝  `)
	fmt.Println(`  ██████╔╝╚██████╔╝██║ ╚████║███████╗`)
	fmt.Println(`  ╚═════╝  ╚═════╝ ╚═╝  ╚═══╝╚══════╝`)
	fmt.Println(NC)

	box(GREEN+"NAHAN IS ONLINE!"+NC, []string{
		OK + " API Token verified",
		OK + " D1 Database: " + CYAN + dbName + NC,
		OK + " Worker deployed: " + CYAN + workerName + NC,
		"",
		BOLD + "Dashboard URL:" + NC,
		"  >> " + CYAN + workerURL + "/sync/dash" + NC,
		"",
		YELLOW + "[!] Default password: " + RED + BOLD + "admin" + NC,
		YELLOW + "    Change it in System settings!" + NC,
	})

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
	fmt.Printf(" %s Account: %s%s%s\n", INFO, DIM, cfg.AccountID, NC)
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
	err = cfUploadWorker(cfg.AccountID, cfg.WorkerName, cfg.APIToken, scriptContent, cfg.DBName, cfg.DBID)
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

	// Live check
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
		BOLD + "Worker:    " + NC + CYAN + cfg.WorkerName + NC,
		BOLD + "Database:  " + NC + CYAN + cfg.DBName + NC,
		BOLD + "DB ID:     " + NC + DIM + cfg.DBID + NC,
		BOLD + "URL:       " + NC + CYAN + cfg.WorkerURL + NC,
		BOLD + "Dashboard: " + NC + CYAN + cfg.WorkerURL + "/sync/dash" + NC,
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

	// Delete worker
	done := make(chan bool)
	go spinner(done, "Deleting Worker '"+cfg.WorkerName+"'...")
	result, err := cfRequest("DELETE",
		fmt.Sprintf("/accounts/%s/workers/scripts/%s", cfg.AccountID, cfg.WorkerName),
		cfg.APIToken, nil,
	)
	done <- true
	workerDeleted := err == nil
	if r, ok := result["success"].(bool); ok {
		workerDeleted = r
	}
	if workerDeleted {
		fmt.Printf(" %s Worker deleted\n", OK)
	} else {
		fmt.Printf(" %s Worker not found or already deleted\n", WARN)
	}

	// Delete D1
	done2 := make(chan bool)
	go spinner(done2, "Deleting D1 database '"+cfg.DBName+"'...")
	result2, err2 := cfRequest("DELETE",
		fmt.Sprintf("/accounts/%s/d1/database/%s", cfg.AccountID, cfg.DBID),
		cfg.APIToken, nil,
	)
	done2 <- true
	dbDeleted := err2 == nil
	if r, ok := result2["success"].(bool); ok {
		dbDeleted = r
	}
	if dbDeleted {
		fmt.Printf(" %s Database deleted\n", OK)
	} else {
		fmt.Printf(" %s Database not found or already deleted\n", WARN)
	}

	os.Remove(configFile)
	fmt.Printf(" %s Local config removed\n", OK)

	fmt.Println()
	box(RED+"UNINSTALL COMPLETE"+NC, []string{
		"Worker:   " + cfg.WorkerName + " — " + func() string { if workerDeleted { return RED + "DELETED" + NC }; return YELLOW + "NOT FOUND" + NC }(),
		"Database: " + cfg.DBName + " — " + func() string { if dbDeleted { return RED + "DELETED" + NC }; return YELLOW + "NOT FOUND" + NC }(),
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
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			installNahan()
		case "2":
			updateNahan()
		case "3":
			statusNahan()
		case "4":
			uninstallNahan()
		case "5":
			fmt.Printf("\n %s Goodbye!\n\n", OK)
			os.Exit(0)
		default:
			fmt.Printf("\n %s Invalid. Use 1-5.\n", ERR)
			time.Sleep(time.Second)
		}
	}
}

func main() {
	mainMenu()
}

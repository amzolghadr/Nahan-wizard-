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

const VERSION = "v1.1.11"

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

type WorkerEntry struct {
	AccountID   string
	AccountName string
	WorkerName  string
	WorkerURL   string
	DBID        string
	DBName      string
	Tagged      bool
}

// session token — در طول اجرا نگه داشته می‌شه
var sessionToken = ""

const configFile = ".nahan.json"
const tokenFile = ".nahan-token"
const workerJSURL = "https://raw.githubusercontent.com/itsyebekhe/nahan/main/_worker.js"
const cfAPI = "https://api.cloudflare.com/client/v4"
const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
const wizardTag  = "nahan-wizard"

var reader = bufio.NewReader(os.Stdin)

func randomName(length int) string {
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}

func clearScreen() {
	if runtime.GOOS == "windows" {
		fmt.Print("\033[H\033[2J")
	} else {
		fmt.Print("\033[H\033[2J")
	}
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

func saveToken(token string) {
	os.WriteFile(tokenFile, []byte(token), 0600)
}

func loadToken() string {
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
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


func addWizardTag(accountID, workerName, token string) {
	// tag رو با multipart/form-data ست کن
	boundary := "WizardTagBoundary"
	metaJSON := `{"tags":["nahan-wizard"]}`
	var buf bytes.Buffer
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Disposition: form-data; name=\"metadata\"\r\n")
	buf.WriteString("Content-Type: application/json\r\n\r\n")
	buf.WriteString(metaJSON)
	buf.WriteString("\r\n--" + boundary + "--\r\n")
	url := fmt.Sprintf("%s/accounts/%s/workers/scripts/%s/settings", cfAPI, accountID, workerName)
	req, err := http.NewRequest("PATCH", url, &buf)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	httpClient.Do(req)
}

func getWorkerTags(accountID, workerName, token string) []string {
	result, err := cfRequest("GET",
		fmt.Sprintf("/accounts/%s/workers/scripts/%s/script-settings", accountID, workerName),
		token, nil,
	)
	if err != nil {
		return nil
	}
	// API ممکنه result رو مستقیم یا داخل result بده
	var raw []interface{}
	if res, ok := result["result"].(map[string]interface{}); ok {
		raw, _ = res["tags"].([]interface{})
	} else {
		raw, _ = result["tags"].([]interface{})
	}
	var tags []string
	for _, t := range raw {
		if s, ok := t.(string); ok {
			tags = append(tags, s)
		}
	}
	return tags
}

func hasWizardTag(tags []string) bool {
	for _, t := range tags {
		if t == wizardTag {
			return true
		}
	}
	return false
}

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

func listWorkersForAccount(accountID, token string) ([]map[string]interface{}, error) {
	result, err := cfRequest("GET",
		fmt.Sprintf("/accounts/%s/workers/scripts", accountID),
		token, nil,
	)
	if err != nil {
		return nil, err
	}
	raw, _ := result["result"].([]interface{})
	var workers []map[string]interface{}
	for _, w := range raw {
		if worker, ok := w.(map[string]interface{}); ok {
			workers = append(workers, worker)
		}
	}
	return workers, nil
}

func getWorkerSubdomain(accountID, workerName, token string) string {
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
			return fmt.Sprintf("%s.%s.workers.dev", workerName, sub)
		}
	}
	return ""
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
				fmt.Printf("\n %s Found existing DB: %s%s%s\n", OK, CYAN, dbName, NC)
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
	return getWorkerSubdomain(accountID, workerName, token)
}

func listAllWorkers(token string) ([]WorkerEntry, error) {
	accounts, err := getAccounts(token)
	if err != nil {
		return nil, err
	}

	done := make(chan bool)
	go spinner(done, "Fetching workers from all accounts...")

	type result struct {
		entries []WorkerEntry
		order   int
	}

	ch := make(chan result, len(accounts))

	for i, acc := range accounts {
		go func(idx int, acc map[string]interface{}) {
			accID, _ := acc["id"].(string)
			accName, _ := acc["name"].(string)
			workers, err := listWorkersForAccount(accID, token)
			var entries []WorkerEntry
			if err == nil {
				for _, w := range workers {
					name, _ := w["id"].(string)
					domain := getWorkerSubdomain(accID, name, token)
					entries = append(entries, WorkerEntry{
						AccountID:   accID,
						AccountName: accName,
						WorkerName:  name,
						WorkerURL:   domain,
					})
				}
			}
			ch <- result{entries: entries, order: idx}
		}(i, acc)
	}

	// جمع‌آوری نتایج با حفظ ترتیب
	results := make([][]WorkerEntry, len(accounts))
	for range accounts {
		r := <-ch
		results[r.order] = r.entries
	}
	done <- true

	var all []WorkerEntry
	for _, entries := range results {
		all = append(all, entries...)
	}
	return all, nil
}

func printOutputURLs(entries []WorkerEntry) {
	fmt.Printf("\n%s-- [ OUTPUT URLS ] --%s\n\n", GREEN+BOLD, NC)
	for _, e := range entries {
		fmt.Println(e.WorkerURL)
	}
	fmt.Println()
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(e.WorkerURL + "\n")
	}
	os.WriteFile("nahan-urls.txt", []byte(sb.String()), 0644)
	fmt.Printf(" %s URLs saved to: %snahan-urls.txt%s\n", OK, CYAN, NC)
	type URLEntry struct {
		Account string `json:"account"`
		Worker  string `json:"worker"`
		URL     string `json:"url"`
	}
	var jsonEntries []URLEntry
	for _, e := range entries {
		jsonEntries = append(jsonEntries, URLEntry{
			Account: e.AccountName,
			Worker:  e.WorkerName,
			URL:     e.WorkerURL,
		})
	}
	jsonData, _ := json.MarshalIndent(jsonEntries, "", "  ")
	os.WriteFile("nahan-urls.json", jsonData, 0644)
	fmt.Printf(" %s JSON saved to:  %snahan-urls.json%s\n", OK, CYAN, NC)
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
	// نمایش وضعیت توکن
	tokenStatus := RED + "Not set" + NC
	shortToken := ""
	if sessionToken != "" {
		shortToken = sessionToken[:8] + "..." + sessionToken[len(sessionToken)-4:]
		tokenStatus = GREEN + "Active " + NC + DIM + "(" + shortToken + ")" + NC
	}
	fmt.Println(CYAN + "+----------------------------------------------------+" + NC)
	fmt.Printf(CYAN+"|"+NC+"   "+BOLD+"Nahan Edge Gateway Wizard  --  Go Edition"+NC+"   "+CYAN+"|"+NC+"\n")
	fmt.Printf(CYAN+"|"+NC+"   "+DIM+"No Wrangler. Pure API. Works on Android."+NC+"    "+CYAN+"|"+NC+"\n")
	fmt.Printf(CYAN+"|"+NC+"   "+DIM+"Version: %-38s"+NC+CYAN+"|"+NC+"\n", VERSION)
	fmt.Printf(CYAN+"|"+NC+"   Token: %-50s"+CYAN+"|"+NC+"\n", tokenStatus)
	fmt.Println(CYAN + "+----------------------------------------------------+" + NC)
}

// ─── SET TOKEN ────────────────────────────────────────────────────────────────
func setToken() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ SET API TOKEN ] --%s\n\n", MAGENTA+BOLD, NC)

	box(INFO+" How to get your API Token", []string{
		"1. Go to: dash.cloudflare.com/profile/api-tokens",
		"2. Click 'Create Token'",
		"3. Use template: 'Edit Cloudflare Workers'",
		"4. Also add permission: D1 - Edit",
		"5. Set Account access to 'All accounts' for multi-account",
		"6. Copy the token and paste it below",
	})

	if sessionToken != "" {
		fmt.Printf("\n %s Current token: %s%s...%s\n", INFO, CYAN, sessionToken[:8], NC)
		fmt.Printf(" %s Press Enter to keep current token, or paste a new one:\n > ", ASK)
	} else {
		fmt.Printf("\n %s Paste your Cloudflare API Token:\n > ", ASK)
	}

	raw, _ := reader.ReadString('\n')
	input := strings.TrimSpace(raw)

	if input == "" && sessionToken != "" {
		fmt.Printf(" %s Keeping current token.\n", OK)
		pressEnter("Press Enter to return...")
		return
	}
	if input == "" {
		fmt.Printf(" %s Token cannot be empty.\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	// تأیید توکن
	done := make(chan bool)
	go spinner(done, "Verifying token...")
	result, err := cfRequest("GET", "/accounts?per_page=1", input, nil)
	done <- true

	if err != nil || func() bool { s, _ := result["success"].(bool); return !s }() {
		fmt.Printf("\n %s Invalid token or connection error.\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	accounts, _ := result["result"].([]interface{})
	accountCount := len(accounts)

	sessionToken = input
	saveToken(input)

	fmt.Printf("\n %s Token verified! %s%d%s account(s) accessible.\n", OK, CYAN, accountCount, NC)
	pressEnter("Press Enter to return to menu...")
}

// ─── INSTALL ──────────────────────────────────────────────────────────────────
func installNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ INSTALL ] DEPLOY TO CLOUDFLARE --%s\n\n", MAGENTA+BOLD, NC)

	if sessionToken == "" {
		fmt.Printf(" %s No API token set. Please set a token first (option 6).\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	accounts, err := getAccounts(sessionToken)
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

	fmt.Printf(" %s Found %s%d%s account(s):\n\n", OK, CYAN, len(accounts), NC)
	for i, acc := range accounts {
		name, _ := acc["name"].(string)
		id, _ := acc["id"].(string)
		fmt.Printf("   %s%d)%s %s %s(%s...)%s\n", GREEN, i+1, NC, name, DIM, id[:8], NC)
	}

	var selectedAccounts []map[string]interface{}
	if len(accounts) == 1 {
		selectedAccounts = accounts
		fmt.Printf("\n %s Single account, using it automatically.\n", INFO)
	} else {
		fmt.Printf("\n %s Enter numbers to deploy to (e.g: 1,3) or 'all':\n > ", ASK)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "0" || strings.ToLower(input) == "back" {
			return
		}
		if strings.ToLower(input) == "all" {
			selectedAccounts = accounts
		} else {
			for _, part := range strings.Split(input, ",") {
				idx := 0
				fmt.Sscanf(strings.TrimSpace(part), "%d", &idx)
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

	scriptContent, err := downloadWorkerJS()
	if err != nil {
		fmt.Printf("\n %s Failed to download worker.js: %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	fmt.Printf(" %s worker.js downloaded (%d KB)\n", OK, len(scriptContent)/1024)

	var deployedEntries []WorkerEntry

	fmt.Println("\n" + CYAN + strings.Repeat("-", 50) + NC)
	for _, acc := range selectedAccounts {
		accID, _ := acc["id"].(string)
		accName, _ := acc["name"].(string)
		workerName := randomName(32)
		dbName := "nahan-" + randomName(16)

		fmt.Printf("\n %s Account: %s%s%s\n", INFO, CYAN, accName, NC)
		fmt.Printf(" %s Worker : %s%s%s\n", INFO, DIM, workerName, NC)

		dbID, err := createD1DB(accID, dbName, sessionToken)
		if err != nil {
			fmt.Printf(" %s DB failed: %s\n", ERR, err.Error())
			fmt.Println(CYAN + strings.Repeat("-", 50) + NC)
			continue
		}

		done := make(chan bool)
		go spinner(done, "Deploying Worker...")
		err = cfUploadWorker(accID, workerName, sessionToken, scriptContent, dbID)
		done <- true
		if err != nil {
			fmt.Printf("\n %s Deploy failed: %s\n", ERR, err.Error())
			fmt.Println(CYAN + strings.Repeat("-", 50) + NC)
			continue
		}

		workerDomain := enableWorkerSubdomain(accID, workerName, sessionToken)
		if workerDomain == "" {
			workerDomain = workerName + ".YOUR-SUBDOMAIN.workers.dev"
		}

		fmt.Printf("\n %s Deployed! %s%s%s\n", OK, CYAN, workerDomain, NC)
		addWizardTag(accID, workerName, sessionToken)

		deployedEntries = append(deployedEntries, WorkerEntry{
			AccountID:   accID,
			AccountName: accName,
			WorkerName:  workerName,
			WorkerURL:   workerDomain,
			DBID:        dbID,
			DBName:      dbName,
		})

		cfg := Config{
			AccountID:  accID,
			APIToken:   sessionToken,
			WorkerName: workerName,
			DBName:     dbName,
			DBID:       dbID,
			WorkerURL:  workerDomain,
		}
		cfgFile := fmt.Sprintf(".nahan-%s.json", accID[:8])
		data, _ := json.MarshalIndent(cfg, "", "  ")
		os.WriteFile(cfgFile, data, 0644)
		saveConfig(cfg)

		fmt.Println(CYAN + strings.Repeat("-", 50) + NC)
	}

	if len(deployedEntries) > 0 {
		printOutputURLs(deployedEntries)
	}

	pressEnter("Press Enter to return to menu...")
}

// ─── UPDATE ───────────────────────────────────────────────────────────────────
func printWorkerList(workers []WorkerEntry, color string) {
	// گروه‌بندی بر اساس اکانت
	type group struct {
		name    string
		workers []WorkerEntry
	}
	var groups []group
	seen := map[string]int{}
	for _, w := range workers {
		if idx, ok := seen[w.AccountID]; ok {
			groups[idx].workers = append(groups[idx].workers, w)
		} else {
			seen[w.AccountID] = len(groups)
			groups = append(groups, group{name: w.AccountName, workers: []WorkerEntry{w}})
		}
	}

	counter := 1
	for _, g := range groups {
		fmt.Printf(" %s%s%s\n", GREEN+BOLD, g.name, NC)
		for _, w := range g.workers {
			if w.Tagged {
				fmt.Printf("   %s%d)%s %s%s%s %s[nahan]%s", color, counter, NC, CYAN, w.WorkerName, NC, GREEN+BOLD, NC)
			} else {
				fmt.Printf("   %s%d)%s %s%s%s", DIM, counter, NC, DIM, w.WorkerName, NC)
			}
			if w.WorkerURL != "" {
				fmt.Printf(" %s(%s)%s", DIM, w.WorkerURL, NC)
			}
			fmt.Println()
			counter++
		}
		fmt.Println()
	}
}

func updateNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ UPDATE ] SELECT WORKER TO UPDATE --%s\n\n", CYAN+BOLD, NC)

	if sessionToken == "" {
		fmt.Printf(" %s No API token set. Please set a token first (option 6).\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	workers, err := listAllWorkers(sessionToken)
	if err != nil {
		fmt.Printf("\n %s %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	if len(workers) == 0 {
		fmt.Printf(" %s No workers found in any account.\n", WARN)
		pressEnter("Press Enter to return...")
		return
	}

	fmt.Printf(" %s Found %s%d%s worker(s):\n\n", OK, CYAN, len(workers), NC)
	printWorkerList(workers, GREEN)
	fmt.Printf(" %s Enter numbers to update (e.g: 1,3) or 'all' or '0' to cancel:\n > ", ASK)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "0" || strings.ToLower(input) == "back" {
		return
	}

	var selected []WorkerEntry
	if strings.ToLower(input) == "all" {
		selected = workers
	} else {
		for _, part := range strings.Split(input, ",") {
			idx := 0
			fmt.Sscanf(strings.TrimSpace(part), "%d", &idx)
			if idx >= 1 && idx <= len(workers) {
				selected = append(selected, workers[idx-1])
			}
		}
	}

	if len(selected) == 0 {
		fmt.Printf(" %s No valid selection.\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	scriptContent, err := downloadWorkerJS()
	if err != nil {
		fmt.Printf("\n %s Download failed: %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	fmt.Printf(" %s worker.js downloaded (%d KB)\n\n", OK, len(scriptContent)/1024)

	for _, w := range selected {
		fmt.Printf(" %s Updating: %s%s%s\n", INFO, CYAN, w.WorkerName, NC)
		dbID := ""
		cfgFile := fmt.Sprintf(".nahan-%s.json", w.AccountID[:8])
		if data, err := os.ReadFile(cfgFile); err == nil {
			var c Config
			if json.Unmarshal(data, &c) == nil {
				dbID = c.DBID
			}
		}
		if dbID == "" {
			if cfg, ok := loadConfig(); ok {
				dbID = cfg.DBID
			}
		}
		done := make(chan bool)
		go spinner(done, "Redeploying...")
		err = cfUploadWorker(w.AccountID, w.WorkerName, sessionToken, scriptContent, dbID)
		done <- true
		if err != nil {
			fmt.Printf("\n %s Failed: %s\n", ERR, err.Error())
		} else {
			fmt.Printf(" %s Updated!\n", OK)
			addWizardTag(w.AccountID, w.WorkerName, sessionToken)
		}
	}

	pressEnter("\nPress Enter to return...")
}

// ─── STATUS ───────────────────────────────────────────────────────────────────
func statusNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ STATUS ] ALL DEPLOYED WORKERS --%s\n\n", CYAN+BOLD, NC)

	if sessionToken == "" {
		fmt.Printf(" %s No API token set. Please set a token first (option 6).\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	workers, err := listAllWorkers(sessionToken)
	if err != nil {
		fmt.Printf("\n %s %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	if len(workers) == 0 {
		fmt.Printf(" %s No workers found in any account.\n", WARN)
		pressEnter("Press Enter to return...")
		return
	}

	fmt.Printf(" %s Found %s%d%s worker(s):\n\n", OK, CYAN, len(workers), NC)
	printWorkerList(workers, CYAN)

	pressEnter("Press Enter to return...")
}

// ─── UNINSTALL ────────────────────────────────────────────────────────────────
func uninstallNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s-- [ UNINSTALL ] SELECT WORKER TO DELETE --%s\n\n", RED+BOLD, NC)

	if sessionToken == "" {
		fmt.Printf(" %s No API token set. Please set a token first (option 6).\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	workers, err := listAllWorkers(sessionToken)
	if err != nil {
		fmt.Printf("\n %s %s\n", ERR, err.Error())
		pressEnter("Press Enter to return...")
		return
	}
	if len(workers) == 0 {
		fmt.Printf(" %s No workers found.\n", WARN)
		pressEnter("Press Enter to return...")
		return
	}

	fmt.Printf(" %s Found %s%d%s worker(s):\n\n", OK, CYAN, len(workers), NC)
	printWorkerList(workers, RED)
	fmt.Printf(" %s Enter numbers to delete (e.g: 1,3) or 'all' or '0' to cancel:\n > ", ASK)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "0" || strings.ToLower(input) == "back" {
		return
	}

	var selected []WorkerEntry
	if strings.ToLower(input) == "all" {
		selected = workers
	} else {
		for _, part := range strings.Split(input, ",") {
			idx := 0
			fmt.Sscanf(strings.TrimSpace(part), "%d", &idx)
			if idx >= 1 && idx <= len(workers) {
				selected = append(selected, workers[idx-1])
			}
		}
	}

	if len(selected) == 0 {
		fmt.Printf(" %s No valid selection.\n", ERR)
		pressEnter("Press Enter to return...")
		return
	}

	box(RED+"[!] DANGER: PERMANENT DELETION"+NC, []string{
		fmt.Sprintf("About to delete %d worker(s).", len(selected)),
		RED + BOLD + "THIS CANNOT BE UNDONE." + NC,
	})
	fmt.Println()

	fmt.Printf(" %s Are you sure? (y/N)\n > ", ASK)
	ans, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(ans)) != "y" {
		fmt.Printf(" %s Cancelled.\n", OK)
		time.Sleep(time.Second)
		return
	}
	fmt.Printf(" %s Type DESTROY to confirm:\n > ", ASK)
	ans2, _ := reader.ReadString('\n')
	if strings.TrimSpace(ans2) != "DESTROY" {
		fmt.Printf(" %s Confirmation failed.\n", ERR)
		time.Sleep(time.Second)
		return
	}

	fmt.Println()
	for _, w := range selected {
		fmt.Printf(" %s Deleting: %s%s%s\n", INFO, CYAN, w.WorkerName, NC)
		done := make(chan bool)
		go spinner(done, "Deleting Worker...")
		result, err := cfRequest("DELETE",
			fmt.Sprintf("/accounts/%s/workers/scripts/%s", w.AccountID, w.WorkerName),
			sessionToken, nil,
		)
		done <- true
		deleted := err == nil
		if r, ok2 := result["success"].(bool); ok2 {
			deleted = r
		}
		if deleted {
			fmt.Printf(" %s Worker deleted\n", OK)
		} else {
			fmt.Printf(" %s Not found or already deleted\n", WARN)
		}
		cfgFile := fmt.Sprintf(".nahan-%s.json", w.AccountID[:8])
		if data, err := os.ReadFile(cfgFile); err == nil {
			var c Config
			if json.Unmarshal(data, &c) == nil && c.DBID != "" {
				done2 := make(chan bool)
				go spinner(done2, "Deleting D1 database...")
				cfRequest("DELETE",
					fmt.Sprintf("/accounts/%s/d1/database/%s", w.AccountID, c.DBID),
					sessionToken, nil,
				)
				done2 <- true
				fmt.Printf(" %s Database deleted\n", OK)
				os.Remove(cfgFile)
			}
		}
		fmt.Println()
	}

	pressEnter("Press Enter to return...")
}

// ─── MAIN MENU ────────────────────────────────────────────────────────────────
func mainMenu() {
	// بارگذاری توکن ذخیره شده
	saved := loadToken()
	if saved != "" {
		sessionToken = saved
	}

	for {
		clearScreen()
		showHeader()
		fmt.Printf("\n%s-- [ MAIN MENU ] --%s\n\n", CYAN+BOLD, NC)

		box(MAGENTA+"SELECT ACTION"+NC, []string{
			"",
			" " + GREEN + "1)" + NC + "  Install Nahan to Cloudflare",
			" " + YELLOW + "2)" + NC + "  Update Worker(s)",
			" " + CYAN + "3)" + NC + "  View all deployed Workers",
			" " + RED + "4)" + NC + "  Uninstall Worker(s)",
			" " + MAGENTA + "5)" + NC + "  Exit",
			" " + BLUE + "6)" + NC + "  Set / Change API Token",
			"",
		})

		fmt.Printf("\n %s Enter choice [1-6]:\n > ", ASK)
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
		case "6":
			setToken()
		default:
			fmt.Printf("\n %s Invalid. Use 1-6.\n", ERR)
			time.Sleep(time.Second)
		}
	}
}

func main() {
	mainMenu()
}

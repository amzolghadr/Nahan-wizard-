package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ─── ANSI Colors ───────────────────────────────────────────────────────────────
const (
	NC      = "\033[0m"
	RED     = "\033[1;31m"
	GREEN   = "\033[1;32m"
	YELLOW  = "\033[1;33m"
	BLUE    = "\033[1;34m"
	MAGENTA = "\033[1;35m"
	CYAN    = "\033[1;36m"
	WHITE   = "\033[1;37m"
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

// ─── Config ────────────────────────────────────────────────────────────────────
type NahanConfig struct {
	WorkerName string `json:"worker_name"`
	DBName     string `json:"db_name"`
	DBID       string `json:"db_id"`
	WorkerURL  string `json:"worker_url"`
}

const configFile = ".nahan-wizard.json"
const workerJSURL = "https://raw.githubusercontent.com/itsyebekhe/nahan/main/_worker.js"

var reader = bufio.NewReader(os.Stdin)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func clearScreen() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J")
	}
}

func prompt(label string) string {
	fmt.Print(" " + ASK + " " + label + "\n ❯ ")
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func promptDefault(label, def string) string {
	fmt.Printf(" "+ASK+" %s [Default: %s%s%s]:\n ❯ ", label, CYAN, def, NC)
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
	width := 76
	fmt.Println(CYAN + "┌" + strings.Repeat("─", width) + "┐" + NC)
	fmt.Printf(CYAN+"│"+NC+" %-*s "+CYAN+"│"+NC+"\n", width-1, title)
	fmt.Println(CYAN + "├" + strings.Repeat("─", width) + "┤" + NC)
	for _, l := range lines {
		// strip ansi for width calc
		plain := stripANSI(l)
		pad := width - 1 - len([]rune(plain))
		if pad < 0 {
			pad = 0
		}
		fmt.Printf(CYAN+"│"+NC+" %s%s "+CYAN+"│"+NC+"\n", l, strings.Repeat(" ", pad))
	}
	fmt.Println(CYAN + "└" + strings.Repeat("─", width) + "┘" + NC)
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func spinner(done chan bool) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	for {
		select {
		case <-done:
			fmt.Print("\r")
			return
		default:
			fmt.Printf("\r "+CYAN+"%s"+NC+" ", frames[i%len(frames)])
			i++
			time.Sleep(80 * time.Millisecond)
		}
	}
}

func runCmd(label string, args ...string) (string, error) {
	fmt.Printf("\n %s %s", INFO, label)
	done := make(chan bool)
	go spinner(done)

	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	done <- true

	if err != nil {
		fmt.Printf(" %s[FAILED]%s\n", RED, NC)
		fmt.Printf("\n%s── ERROR ──%s\n%s\n%s───────────%s\n", RED, NC, string(out), RED, NC)
		pressEnter("Press Enter to acknowledge and continue...")
		return string(out), err
	}
	fmt.Printf(" %s[OK]%s\n", GREEN, NC)
	return string(out), nil
}

func runCmdSilent(args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func wrangler(args ...string) string {
	if commandExists("wrangler") {
		return "wrangler"
	}
	return "npx"
}

func wranglerArgs(extra ...string) []string {
	w := wrangler()
	if w == "npx" {
		args := []string{"npx", "wrangler"}
		return append(args, extra...)
	}
	return append([]string{"wrangler"}, extra...)
}

func saveConfig(cfg NahanConfig) {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configFile, data, 0644)
}

func loadConfig() (NahanConfig, bool) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return NahanConfig{}, false
	}
	var cfg NahanConfig
	json.Unmarshal(data, &cfg)
	return cfg, true
}

func readWranglerTOML() (workerName, dbName string) {
	data, err := os.ReadFile("wrangler.toml")
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "name") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				workerName = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}
		if strings.HasPrefix(line, "database_name") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				dbName = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}
	}
	return
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
	fmt.Println(CYAN + "┌──────────────────────────────────────────────────┐" + NC)
	fmt.Println(CYAN + "│" + NC + "   " + BOLD + "Nahan Edge Gateway Wizard — Go Edition" + NC + "         " + CYAN + "│" + NC)
	fmt.Println(CYAN + "└──────────────────────────────────────────────────┘" + NC)
}

// ─── Dependency Check ─────────────────────────────────────────────────────────

// commandExists checks if a command exists by searching PATH manually
// This avoids faccessat syscall which crashes on Termux/Android
func commandExists(name string) bool {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		pathEnv = "/usr/local/bin:/usr/bin:/bin:/data/data/com.termux/files/usr/bin"
	}
	for _, dir := range strings.Split(pathEnv, ":") {
		full := dir + "/" + name
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func checkDependencies() bool {
	fmt.Printf("\n %s Checking dependencies...\n", INFO)

	// Check Node.js
	if !commandExists("node") {
		box(RED+"[!] Node.js not found"+NC, []string{
			"Node.js is required to run Wrangler.",
			"In Termux: pkg install nodejs",
		})
		return false
	}
	fmt.Printf(" %s Node.js: %sOK%s\n", OK, GREEN, NC)

	// Check npm
	if !commandExists("npm") {
		fmt.Printf(" %s npm not found. In Termux: pkg install nodejs\n", ERR)
		return false
	}
	fmt.Printf(" %s npm: %sOK%s\n", OK, GREEN, NC)

	// Check wrangler
	if commandExists("wrangler") {
		out, _ := runCmdSilent("wrangler", "--version")
		fmt.Printf(" %s Wrangler (global): %s%s%s\n", OK, CYAN, strings.TrimSpace(out), NC)
	} else {
		fmt.Printf(" %s Wrangler not found globally, will use npx\n", WARN)
		ans := prompt("Install Wrangler globally now? (Y/n)")
		if ans == "" || strings.ToLower(ans) == "y" {
			runCmd("Installing Wrangler globally...", "npm", "install", "-g", "wrangler")
		}
	}

	return true
}

// ─── INSTALL ──────────────────────────────────────────────────────────────────

func installNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s── [ PHASE 1 ] PREREQUISITE CHECK ──%s\n", MAGENTA+BOLD, NC)

	if !checkDependencies() {
		pressEnter("Fix dependencies and re-run. Press Enter to return...")
		return
	}

	pressEnter("Phase 1 complete! Press Enter to log in to Cloudflare...")

	// Phase 2: Login
	clearScreen()
	showHeader()
	fmt.Printf("\n%s── [ PHASE 2 ] CLOUDFLARE AUTHENTICATION ──%s\n", MAGENTA+BOLD, NC)
	box(YELLOW+"[!] VPN WARNING"+NC, []string{
		"Disconnect any VPN before authenticating.",
		"VPNs can break the browser redirect handshake.",
	})
	pressEnter("Press Enter to open browser login...")

	args := wranglerArgs("login")
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Run()

	pressEnter("Authenticated! Press Enter to provision D1 database...")

	// Phase 3: D1 Database
	clearScreen()
	showHeader()
	fmt.Printf("\n%s── [ PHASE 3 ] D1 DATABASE PROVISIONING ──%s\n", MAGENTA+BOLD, NC)

	dbName := promptDefault("D1 Database name", "nahan-db")
	dbID := ""

	out2, err := runCmd("Creating D1 database '"+dbName+"'...", wranglerArgs("d1", "create", dbName)...)
	if err != nil {
		// DB may already exist — try listing to find the ID
		fmt.Printf(" %s Database may already exist, fetching list...\n", WARN)
		out2, _ = runCmdSilent(wranglerArgs("d1", "list", "--json")...)
	}

	// Extract UUID from output
	uuidRe := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	matches := uuidRe.FindAllString(out2, -1)
	if len(matches) > 0 {
		dbID = matches[0]
		fmt.Printf(" %s Database ID: %s%s%s\n", OK, CYAN, dbID, NC)
	}

	// Manual fallback
	if dbID == "" {
		fmt.Printf(" %s Could not auto-detect Database ID.\n", WARN)
		for dbID == "" {
			dbID = prompt("Paste your D1 Database UUID manually:")
			if !uuidRe.MatchString(dbID) {
				fmt.Printf(" %s Invalid UUID format, try again.\n", ERR)
				dbID = ""
			}
		}
	}

	pressEnter("Phase 3 complete! Press Enter to configure...")

	// Phase 4: wrangler.toml
	clearScreen()
	showHeader()
	fmt.Printf("\n%s── [ PHASE 4 ] CONFIGURATION ──%s\n", MAGENTA+BOLD, NC)

	workerName := promptDefault("Cloudflare Worker name", "nahan-core")

	toml := fmt.Sprintf(`# Generated by Nahan Wizard
name = "%s"
main = "_worker.js"
compatibility_date = "2023-10-30"

[[d1_databases]]
binding = "IOT_DB"
database_name = "%s"
database_id = "%s"
`, workerName, dbName, dbID)

	os.WriteFile("wrangler.toml", []byte(toml), 0644)
	fmt.Printf(" %s wrangler.toml written successfully\n", OK)
	fmt.Printf("   • Worker name : %s%s%s\n", CYAN, workerName, NC)
	fmt.Printf("   • Database    : %s%s%s (%s)\n", CYAN, dbName, NC, dbID)

	// Download _worker.js
	fmt.Printf("\n %s Downloading latest nahan worker.js...\n", INFO)
	dlArgs := []string{}
	if commandExists("curl") {
		dlArgs = []string{"curl", "-fsSL", "-o", "_worker.js", workerJSURL}
	} else if commandExists("wget") {
		dlArgs = []string{"wget", "-q", "-O", "_worker.js", workerJSURL}
	}
	if len(dlArgs) > 0 {
		runCmd("Downloading _worker.js...", dlArgs...)
	} else {
		fmt.Printf(" %s Neither curl nor wget found. Please manually download _worker.js from:\n", WARN)
		fmt.Printf("   %s%s%s\n", CYAN, workerJSURL, NC)
		pressEnter("Once downloaded, press Enter to continue...")
	}

	pressEnter("Phase 4 complete! Press Enter to deploy...")

	// Phase 5: Deploy
	clearScreen()
	showHeader()
	fmt.Printf("\n%s── [ PHASE 5 ] DEPLOYING TO CLOUDFLARE ──%s\n", MAGENTA+BOLD, NC)

	deployOut, deployErr := runCmd("Deploying Worker to Cloudflare Edge...", wranglerArgs("deploy")...)
	if deployErr != nil {
		pressEnter("Deployment failed. Press Enter to return to menu...")
		return
	}

	// Extract URL
	urlRe := regexp.MustCompile(`https://[a-zA-Z0-9._-]+\.workers\.dev`)
	deployURL := ""
	if m := urlRe.FindString(deployOut); m != "" {
		deployURL = m
	}
	if deployURL == "" {
		u := prompt("Could not auto-detect Worker URL. Enter it manually (e.g. nahan.user.workers.dev):")
		if !strings.HasPrefix(u, "https://") {
			deployURL = "https://" + u
		} else {
			deployURL = u
		}
	}

	// Save config
	cfg := NahanConfig{WorkerName: workerName, DBName: dbName, DBID: dbID, WorkerURL: deployURL}
	saveConfig(cfg)

	// Phase 6: Success
	clearScreen()
	fmt.Println(GREEN + BOLD)
	fmt.Println(`  ██████╗ ██████╗ ███╗   ██╗██╗     ██╗███╗   ██╗███████╗`)
	fmt.Println(`  ██╔══██╗██╔══██╗████╗  ██║██║     ██║████╗  ██║██╔════╝`)
	fmt.Println(`  ██║  ██║██║  ██║██╔██╗ ██║██║     ██║██╔██╗ ██║█████╗  `)
	fmt.Println(`  ██║  ██║██║  ██║██║╚██╗██║██║     ██║██║╚██╗██║██╔══╝  `)
	fmt.Println(`  ██████╔╝██████╔╝██║ ╚████║███████╗██║██║ ╚████║███████╗`)
	fmt.Println(`  ╚═════╝ ╚═════╝ ╚═╝  ╚═══╝╚══════╝╚═╝╚═╝  ╚═══╝╚══════╝`)
	fmt.Println(NC)

	box(GREEN+"NAHAN IS ONLINE!"+NC, []string{
		OK + " Dependencies verified",
		OK + " Cloudflare login successful",
		OK + " D1 Database created: " + CYAN + dbName + NC,
		OK + " Worker deployed: " + CYAN + workerName + NC,
		"",
		BOLD + "Dashboard URL:" + NC,
		"  >> " + CYAN + deployURL + "/sync/dash" + NC,
		"",
		YELLOW + "[!] Default password: " + RED + BOLD + "admin" + NC,
		YELLOW + "  Change it immediately in System settings!" + NC,
	})

	pressEnter("Press Enter to return to main menu...")
}

// ─── UNINSTALL ────────────────────────────────────────────────────────────────

func uninstallNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s── [ UNINSTALL ] REMOVE NAHAN FROM CLOUDFLARE ──%s\n", RED+BOLD, NC)

	box(RED+"[!] DANGER: PERMANENT DELETION"+NC, []string{
		"This will permanently delete your Nahan Worker and D1 Database.",
		"All data will be lost. " + RED + BOLD + "THIS CANNOT BE UNDONE." + NC,
	})

	ans1 := prompt("Are you sure you want to wipe Nahan? (y/N)")
	if strings.ToLower(ans1) != "y" {
		fmt.Printf("\n %s Cancelled.\n", OK)
		time.Sleep(time.Second)
		return
	}

	ans2 := prompt("Type DESTROY to confirm:")
	if ans2 != "DESTROY" {
		fmt.Printf("\n %s Confirmation failed. Returning to menu.\n", ERR)
		time.Sleep(time.Second)
		return
	}

	// Detect names
	workerName, dbName := readWranglerTOML()
	if cfg, ok := loadConfig(); ok {
		if workerName == "" {
			workerName = cfg.WorkerName
		}
		if dbName == "" {
			dbName = cfg.DBName
		}
	}

	if workerName == "" {
		workerName = promptDefault("Worker name to delete", "nahan-core")
	} else {
		fmt.Printf(" %s Worker: %s%s%s\n", OK, CYAN, workerName, NC)
	}
	if dbName == "" {
		dbName = promptDefault("D1 Database name to delete", "nahan-db")
	} else {
		fmt.Printf(" %s Database: %s%s%s\n", OK, CYAN, dbName, NC)
	}

	fmt.Printf("\n%s── TEARDOWN SEQUENCE ──%s\n\n", RED+BOLD, NC)

	workerDeleted := false
	if _, err := os.Stat("wrangler.toml"); err == nil {
		_, e := runCmd("Deleting Worker '"+workerName+"'...", wranglerArgs("delete", "--force")...)
		workerDeleted = (e == nil)
	} else {
		_, e := runCmd("Deleting Worker '"+workerName+"'...", wranglerArgs("delete", "--name", workerName, "--force")...)
		workerDeleted = (e == nil)
	}

	_, dbErr := runCmd("Deleting D1 database '"+dbName+"'...", wranglerArgs("d1", "delete", dbName, "-y")...)
	dbDeleted := (dbErr == nil)

	tomlRemoved := false
	if err := os.Remove("wrangler.toml"); err == nil {
		fmt.Printf(" %s wrangler.toml removed\n", OK)
		tomlRemoved = true
	}

	os.Remove("_worker.js")
	os.Remove(configFile)

	clearScreen()
	fmt.Println(RED + BOLD + "\n  UNINSTALL COMPLETE\n" + NC)

	statusIcon := func(ok bool) string {
		if ok {
			return RED + "[-]" + NC
		}
		return YELLOW + "[!]" + NC
	}
	statusText := func(ok bool, msg string) string {
		if ok {
			return RED + msg + NC
		}
		return YELLOW + "NOT FOUND / SKIPPED" + NC
	}

	box("UNINSTALL SUMMARY", []string{
		statusIcon(workerDeleted) + " Worker '" + BLUE + workerName + NC + "': " + statusText(workerDeleted, "DELETED"),
		statusIcon(dbDeleted) + " Database '" + BLUE + dbName + NC + "': " + statusText(dbDeleted, "DELETED"),
		statusIcon(tomlRemoved) + " wrangler.toml: " + statusText(tomlRemoved, "DELETED"),
	})

	pressEnter("Press Enter to return to main menu...")
}

// ─── UPDATE ───────────────────────────────────────────────────────────────────

func updateNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s── [ UPDATE ] UPGRADE NAHAN TO LATEST VERSION ──%s\n", CYAN+BOLD, NC)

	workerName, _ := readWranglerTOML()
	if cfg, ok := loadConfig(); ok && workerName == "" {
		workerName = cfg.WorkerName
	}
	if workerName == "" {
		workerName = promptDefault("Worker name", "nahan-core")
	}
	fmt.Printf(" %s Updating Worker: %s%s%s\n", INFO, CYAN, workerName, NC)

	if _, err := os.Stat("wrangler.toml"); err != nil {
		fmt.Printf(" %s wrangler.toml not found in current directory.\n", WARN)
		fmt.Printf("   Run install first, or switch to the project directory.\n")
		pressEnter("Press Enter to return...")
		return
	}

	// Download latest worker.js
	dlArgs := []string{}
	if commandExists("curl") {
		dlArgs = []string{"curl", "-fsSL", "-o", "_worker.js", workerJSURL}
	} else if commandExists("wget") {
		dlArgs = []string{"wget", "-q", "-O", "_worker.js", workerJSURL}
	}

	if len(dlArgs) > 0 {
		_, err := runCmd("Downloading latest nahan worker.js...", dlArgs...)
		if err != nil {
			pressEnter("Download failed. Press Enter to return...")
			return
		}
	} else {
		fmt.Printf(" %s No download tool found. Trying to deploy existing _worker.js...\n", WARN)
	}

	_, err := runCmd("Deploying updated Worker...", wranglerArgs("deploy")...)
	if err != nil {
		pressEnter("Update failed. Press Enter to return...")
		return
	}

	box(GREEN+"UPDATE COMPLETE"+NC, []string{
		OK + " Latest nahan worker.js downloaded",
		OK + " Worker '" + CYAN + workerName + NC + "' redeployed successfully",
	})

	pressEnter("Press Enter to return to main menu...")
}

// ─── STATUS ───────────────────────────────────────────────────────────────────

func statusNahan() {
	clearScreen()
	showHeader()
	fmt.Printf("\n%s── [ STATUS ] NAHAN DEPLOYMENT INFO ──%s\n\n", CYAN+BOLD, NC)

	cfg, hasCfg := loadConfig()
	workerName, dbName := readWranglerTOML()
	if hasCfg {
		if workerName == "" {
			workerName = cfg.WorkerName
		}
		if dbName == "" {
			dbName = cfg.DBName
		}
	}

	if workerName == "" && dbName == "" {
		box(YELLOW+"[!] No deployment found"+NC, []string{
			"No wrangler.toml or saved config detected.",
			"Run Install first.",
		})
		pressEnter("Press Enter to return...")
		return
	}

	lines := []string{}

	// Local config info
	if workerName != "" {
		lines = append(lines, BOLD+"Worker Name  : "+NC+CYAN+workerName+NC)
	}
	if dbName != "" {
		lines = append(lines, BOLD+"Database     : "+NC+CYAN+dbName+NC)
	}
	if hasCfg && cfg.DBID != "" {
		lines = append(lines, BOLD+"Database ID  : "+NC+DIM+cfg.DBID+NC)
	}
	if hasCfg && cfg.WorkerURL != "" {
		lines = append(lines, BOLD+"Worker URL   : "+NC+CYAN+cfg.WorkerURL+NC)
		lines = append(lines, BOLD+"Dashboard    : "+NC+CYAN+cfg.WorkerURL+"/sync/dash"+NC)
	}

	lines = append(lines, "")
	lines = append(lines, DIM+"─ Live Status from Cloudflare ─"+NC)

	// Live check: worker
	if workerName != "" {
		out, err := runCmdSilent(append(wranglerArgs("deployments", "list", "--name", workerName))...)
		if err == nil && strings.Contains(out, workerName) {
			lines = append(lines, OK+" Worker status: "+GREEN+BOLD+"ACTIVE"+NC)
		} else {
			lines = append(lines, WARN+" Worker status: "+YELLOW+"UNKNOWN / NOT FOUND"+NC)
		}
	}

	// Live check: D1 list
	if dbName != "" {
		out, err := runCmdSilent(append(wranglerArgs("d1", "list"))...)
		if err == nil && strings.Contains(out, dbName) {
			lines = append(lines, OK+" D1 Database:   "+GREEN+BOLD+"EXISTS"+NC)
		} else {
			lines = append(lines, WARN+" D1 Database:   "+YELLOW+"NOT FOUND"+NC)
		}
	}

	// wrangler.toml
	if _, err := os.Stat("wrangler.toml"); err == nil {
		lines = append(lines, OK+" wrangler.toml: "+GREEN+"PRESENT"+NC)
	} else {
		lines = append(lines, WARN+" wrangler.toml: "+YELLOW+"NOT IN CURRENT DIR"+NC)
	}

	box(CYAN+"NAHAN STATUS"+NC, lines)
	pressEnter("Press Enter to return to main menu...")
}

// ─── MAIN MENU ────────────────────────────────────────────────────────────────

func mainMenu() {
	// Handle Ctrl+C gracefully (os.Interrupt only — Termux compatible)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		fmt.Printf("\n\n %s Goodbye! \n\n", OK)
		os.Exit(0)
	}()

	for {
		clearScreen()
		showHeader()
		fmt.Printf("\n%s── [ MAIN MENU ] ──%s\n\n", CYAN+BOLD, NC)

		box(MAGENTA+"[?] SELECT ACTION"+NC, []string{
			"",
			" " + GREEN + "1)" + NC + "  Install Nahan to Cloudflare",
			" " + YELLOW + "2)" + NC + "  Update existing Nahan Worker",
			" " + CYAN + "3)" + NC + "  View deployment status",
			" " + RED + "4)" + NC + "  Uninstall Nahan from Cloudflare",
			" " + WHITE + "5)" + NC + "  Exit",
			"",
		})

		fmt.Printf("\n %s Enter choice [1-5]:\n ❯ ", ASK)
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
			fmt.Printf("\n %s Goodbye! \n\n", OK)
			os.Exit(0)
		default:
			fmt.Printf("\n %s Invalid option. Use 1-5.\n", ERR)
			time.Sleep(time.Second)
		}
	}
}

func main() {
	mainMenu()
}

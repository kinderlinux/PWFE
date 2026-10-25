package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Port        int      `json:"port"`
	Directories []string `json:"directories"`
}

var configDir string
var configFile string
var currentConfig Config

func initConfigPath() {
	if env := strings.TrimSpace(os.Getenv("PWFE_CONFIG")); env != "" {
		configFile = env
		configDir = filepath.Dir(env)
		_ = os.MkdirAll(configDir, 0755)
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		configDir = filepath.Join(appData, "pwfe")
	} else if os.Geteuid() == 0 {
		// Sistem servisi olarak çalışırken kullanıcı oturumundan bağımsız,
		// öngörülebilir bir konum kullan.
		configDir = "/etc/pwfe"
	} else {
		configDir = filepath.Join(home, ".config", "pwfe")
	}

	_ = os.MkdirAll(configDir, 0755)
	configFile = filepath.Join(configDir, "config.json")
}

func loadConfig() {
	initConfigPath()
	file, err := os.ReadFile(configFile)
	if err != nil {
		currentConfig = Config{
			Port:        7933,
			Directories: []string{},
		}
		saveConfig()
		return
	}
	json.Unmarshal(file, &currentConfig)
	if currentConfig.Port == 0 {
		currentConfig.Port = 7933
	}
	if currentConfig.Directories == nil {
		currentConfig.Directories = []string{}
	}
}

func saveConfig() {
	if configFile == "" {
		initConfigPath()
	}
	if currentConfig.Directories == nil {
		currentConfig.Directories = []string{}
	}
	data, err := json.MarshalIndent(currentConfig, "", "  ")
	if err != nil {
		log.Println("Error marshalling config:", err)
		return
	}
	_ = os.MkdirAll(filepath.Dir(configFile), 0755)
	os.WriteFile(configFile, data, 0644)
}

func isAllowedPath(targetPath string) bool {
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	absTargetClean := filepath.Clean(absTarget)

	for _, dir := range currentConfig.Directories {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		absDirClean := filepath.Clean(absDir)

		targetCmp := absTargetClean
		dirCmp := absDirClean
		if runtime.GOOS == "windows" {
			targetCmp = strings.ToLower(targetCmp)
			dirCmp = strings.ToLower(dirCmp)
		}

		rel, err := filepath.Rel(dirCmp, targetCmp)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if currentConfig.Directories == nil {
		currentConfig.Directories = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(currentConfig)
}

func handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var newCfg Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	currentConfig.Directories = newCfg.Directories
	if currentConfig.Directories == nil {
		currentConfig.Directories = []string{}
	}
	if newCfg.Port != 0 {
		currentConfig.Port = newCfg.Port
	}
	saveConfig()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(currentConfig)
}

func handleListFiles(w http.ResponseWriter, r *http.Request) {
	dirPath := r.URL.Query().Get("dir")

	if currentConfig.Directories == nil {
		currentConfig.Directories = []string{}
	}

	if dirPath == "" {
		var items []map[string]interface{}
		for _, d := range currentConfig.Directories {
			info, err := os.Stat(d)
			var modTime string = ""
			if err == nil {
				modTime = info.ModTime().Format(time.RFC3339)
			}
			items = append(items, map[string]interface{}{
				"name":    filepath.Base(d),
				"path":    d,
				"size":    int64(0),
				"modTime": modTime,
				"isDir":   true,
				"ext":     "Folder",
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
		return
	}

	decodedPath := dirPath

	if !isAllowedPath(decodedPath) {
		http.Error(w, "Access denied: Path not in allowed list", http.StatusForbidden)
		return
	}

	entries, err := os.ReadDir(decodedPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []map[string]interface{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(decodedPath, entry.Name())
		ext := ""
		if entry.IsDir() {
			ext = "Folder"
		} else {
			ext = strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
			if ext == "" {
				ext = "File"
			}
		}

		items = append(items, map[string]interface{}{
			"name":    entry.Name(),
			"path":    fullPath,
			"size":    info.Size(),
			"modTime": info.ModTime().Format(time.RFC3339),
			"isDir":   entry.IsDir(),
			"ext":     strings.ToUpper(ext),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		iIsDir := items[i]["isDir"].(bool)
		jIsDir := items[j]["isDir"].(bool)
		if iIsDir != jIsDir {
			return iIsDir
		}
		iName := items[i]["name"].(string)
		jName := items[j]["name"].(string)
		return strings.ToLower(iName) < strings.ToLower(jName)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	decodedPath := filePath

	if !isAllowedPath(decodedPath) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	info, err := os.Stat(decodedPath)
	if err != nil || info.IsDir() {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(decodedPath)))
	http.ServeFile(w, r, decodedPath)
}

func handlePreview(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	decodedPath := filePath

	if !isAllowedPath(decodedPath) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	info, err := os.Stat(decodedPath)
	if err != nil || info.IsDir() {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, decodedPath)
}

func main() {
	os.Args = normalizeUpdatePasswordArgs(os.Args)
	showVersion := flag.Bool("version", false, "Sürümü yazdır ve çık")
	setUpdatePassword := flag.String("set-update-password", "", "Güncelleme şifresini belirle (değer boş bırakılırsa rastgele üretilir)")
	flag.Parse()

	if *showVersion {
		fmt.Println(currentVersion())
		return
	}

	if flagWasSet("set-update-password") {
		initConfigPath()
		password := *setUpdatePassword
		generated := false
		if password == "" {
			var err error
			password, err = generateUpdatePassword(16)
			if err != nil {
				log.Fatal("Güncelleme şifresi üretilemedi: ", err)
			}
			generated = true
		}
		if err := writeUpdatePassword(password); err != nil {
			log.Fatal("Güncelleme şifresi yazılamadı: ", err)
		}
		if generated {
			fmt.Printf("Güncelleme şifresi: %s\n", password)
		} else {
			fmt.Println("Güncelleme şifresi güncellendi.")
		}
		return
	}

	loadConfig()

	exePath, err := os.Executable()
	staticDir := "./static"
	if err == nil {
		exeDir := filepath.Dir(exePath)
		candidate := filepath.Join(exeDir, "static")
		if _, err := os.Stat(candidate); err == nil {
			staticDir = candidate
		}
	}

	fs := http.FileServer(http.Dir(staticDir))
	http.Handle("/", fs)

	http.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleGetConfig(w, r)
		} else if r.Method == http.MethodPost {
			handleSaveConfig(w, r)
		}
	})
	http.HandleFunc("/api/files", handleListFiles)
	http.HandleFunc("/api/download", handleDownload)
	http.HandleFunc("/api/preview", handlePreview)
	http.HandleFunc("/api/version", handleVersion)
	http.HandleFunc("/api/update/check", handleUpdateCheck)
	http.HandleFunc("/api/update/apply", handleUpdateApply)
	http.HandleFunc("/api/update/status", handleUpdateStatus)

	addr := fmt.Sprintf("0.0.0.0:%d", currentConfig.Port)
	log.Printf("PWFE başlatıldı. Port: %d, Sürüm: %s", currentConfig.Port, currentVersion())
	log.Fatal(http.ListenAndServe(addr, nil))
}

func flagWasSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// normalizeUpdatePasswordArgs, son argüman olarak verilen
// "--set-update-password" bayrağını "--set-update-password=" biçimine çevirir.
// Böylece rastgele şifre üretimi flag paketi hata vermeden çalışır. Değer
// bekleyen bayrağın yanındaki argümanlara (ör. "-ile-baslayan-sifre") dokunmaz.
func normalizeUpdatePasswordArgs(args []string) []string {
	if len(args) < 2 {
		return args
	}
	last := len(args) - 1
	if args[last] == "--set-update-password" || args[last] == "-set-update-password" {
		args[last] += "="
	}
	return args
}

// version is set at build time:
//
//	go build -ldflags "-X main.version=1"
var version = "dev"

const (
	defaultUpdateRepo   = "kinderlinux/PWFE"
	defaultUpdateBranch = "main"
	maxVersionBytes     = 64
	updateCheckTTL      = 5 * time.Minute
	maxUpdateAttempts   = 5
	updateAttemptWindow = 5 * time.Minute

	updatePasswordHeader = "X-PWFE-Update-Password"
)

var errPasswordNotConfigured = errors.New("güncelleme şifresi belirlenmemiş")

type updateStatus struct {
	Stage   string `json:"stage"`
	Version int    `json:"version,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Time    string `json:"time,omitempty"`
}

func installRoot() string {
	if root := strings.TrimSpace(os.Getenv("PWFE_ROOT")); root != "" {
		return root
	}
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func currentVersion() string {
	v := strings.TrimSpace(version)
	if v != "" && v != "dev" {
		return v
	}
	if data, err := os.ReadFile(filepath.Join(installRoot(), "version.txt")); err == nil {
		if s := strings.TrimSpace(string(data)); s != "" {
			return s
		}
	}
	return "dev"
}

func parseVersion(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("sürüm boş")
	}
	v, err := strconv.Atoi(value)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("geçersiz sürüm: %q", value)
	}
	return v, nil
}

func updateRepo() string {
	if repo := strings.TrimSpace(os.Getenv("PWFE_UPDATE_REPO")); repo != "" {
		return repo
	}
	return defaultUpdateRepo
}

func updateBranch() string {
	if branch := strings.TrimSpace(os.Getenv("PWFE_UPDATE_BRANCH")); branch != "" {
		return branch
	}
	return defaultUpdateBranch
}

func updateRawBase() string {
	if base := strings.TrimSpace(os.Getenv("PWFE_UPDATE_RAW_BASE")); base != "" {
		return base
	}
	return "https://raw.githubusercontent.com"
}

func updateRawURL(path string) string {
	base := strings.TrimRight(updateRawBase(), "/")
	return fmt.Sprintf("%s/%s/%s/%s", base, updateRepo(), updateBranch(), path)
}

type remoteVersionCacheState struct {
	sync.Mutex
	value     int
	fetchedAt time.Time
}

var remoteVersionCache remoteVersionCacheState

func fetchRemoteVersion(ctx context.Context) (int, error) {
	remoteVersionCache.Lock()
	defer remoteVersionCache.Unlock()

	if remoteVersionCache.value > 0 && time.Since(remoteVersionCache.fetchedAt) < updateCheckTTL {
		return remoteVersionCache.value, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateRawURL("version.txt"), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "pwfe/"+currentVersion())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("sürüm dosyası alınamadı: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("sürüm dosyası alınamadı: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxVersionBytes+1))
	if err != nil {
		return 0, err
	}
	if len(body) > maxVersionBytes {
		return 0, errors.New("sürüm dosyası çok büyük")
	}

	remote, err := parseVersion(string(body))
	if err != nil {
		return 0, err
	}

	remoteVersionCache.value = remote
	remoteVersionCache.fetchedAt = time.Now()
	return remote, nil
}

func updatePasswordPath() string {
	if path := strings.TrimSpace(os.Getenv("PWFE_UPDATE_PASSWORD_FILE")); path != "" {
		return path
	}
	return filepath.Join(configDir, "update.passwd")
}

func hashUpdatePassword(saltHex, password string) string {
	sum := sha256.Sum256([]byte(saltHex + password))
	return hex.EncodeToString(sum[:])
}

func writeUpdatePassword(password string) error {
	if password == "" {
		return errors.New("şifre boş olamaz")
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	saltHex := hex.EncodeToString(salt)
	data := saltHex + ":" + hashUpdatePassword(saltHex, password) + "\n"

	path := updatePasswordPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func verifyUpdatePassword(password string) (bool, error) {
	data, err := os.ReadFile(updatePasswordPath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, errPasswordNotConfigured
		}
		return false, err
	}

	parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false, errors.New("geçersiz şifre dosyası")
	}

	expected := hashUpdatePassword(parts[0], password)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(parts[1])) == 1, nil
}

const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

func generateUpdatePassword(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("geçersiz şifre uzunluğu")
	}

	alphabetLen := len(passwordAlphabet)
	limit := 256 - (256 % alphabetLen)
	out := make([]byte, 0, length)
	buf := make([]byte, length*2)

	for len(out) < length {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, value := range buf {
			if int(value) >= limit {
				continue
			}
			out = append(out, passwordAlphabet[int(value)%alphabetLen])
			if len(out) == length {
				break
			}
		}
	}
	return string(out), nil
}

type updateAttempt struct {
	count int
	first time.Time
}

var updateAttempts = struct {
	sync.Mutex
	items map[string]*updateAttempt
}{items: make(map[string]*updateAttempt)}

func updateAttemptAllowed(ip string) bool {
	updateAttempts.Lock()
	defer updateAttempts.Unlock()

	item, ok := updateAttempts.items[ip]
	if !ok {
		return true
	}
	if time.Since(item.first) > updateAttemptWindow {
		delete(updateAttempts.items, ip)
		return true
	}
	return item.count < maxUpdateAttempts
}

func recordUpdateFailure(ip string) {
	updateAttempts.Lock()
	defer updateAttempts.Unlock()

	item, ok := updateAttempts.items[ip]
	if !ok || time.Since(item.first) > updateAttemptWindow {
		updateAttempts.items[ip] = &updateAttempt{count: 1, first: time.Now()}
		return
	}
	item.count++
}

func clearUpdateFailures(ip string) {
	updateAttempts.Lock()
	defer updateAttempts.Unlock()
	delete(updateAttempts.items, ip)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func readUpdateStatus() updateStatus {
	data, err := os.ReadFile(filepath.Join(installRoot(), "update.state"))
	if err != nil {
		return updateStatus{Stage: "idle"}
	}
	var status updateStatus
	if err := json.Unmarshal(data, &status); err != nil || status.Stage == "" {
		return updateStatus{Stage: "idle"}
	}
	return status
}

// writeUpdateStatusFile, web arayüzünün güncelleme başlatılırken eski bir
// hata durumunu görmesini engellemek için kullanılır.
func writeUpdateStatusFile(status updateStatus) {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(installRoot(), "update.state")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{
		"code":  code,
		"error": message,
	})
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Sadece GET destekleniyor")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": currentVersion()})
}

func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Sadece GET destekleniyor")
		return
	}

	current := currentVersion()
	currentInt, _ := parseVersion(current)

	latest, err := fetchRemoteVersion(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "check_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current":   current,
		"latest":    latest,
		"available": latest > currentInt,
		"branch":    updateBranch(),
	})
}

func handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Sadece GET destekleniyor")
		return
	}

	status := readUpdateStatus()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current": currentVersion(),
		"stage":   status.Stage,
		"version": status.Version,
		"message": status.Message,
		"error":   status.Error,
		"time":    status.Time,
	})
}

func handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Sadece POST destekleniyor")
		return
	}

	ip := clientIP(r)
	if !updateAttemptAllowed(ip) {
		writeJSONError(w, http.StatusTooManyRequests, "rate_limited", "Çok fazla hatalı deneme. 5 dakika sonra tekrar deneyin.")
		return
	}

	ok, err := verifyUpdatePassword(r.Header.Get(updatePasswordHeader))
	if err != nil {
		if errors.Is(err, errPasswordNotConfigured) {
			writeJSONError(w, http.StatusServiceUnavailable, "not_configured", "Güncelleme şifresi belirlenmemiş. Sunucuda 'sudo pwfe --set-update-password' çalıştırın.")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "password_error", err.Error())
		return
	}
	if !ok {
		recordUpdateFailure(ip)
		writeJSONError(w, http.StatusForbidden, "wrong_password", "Güncelleme şifresi yanlış.")
		return
	}
	clearUpdateFailures(ip)

	current := currentVersion()
	currentInt, _ := parseVersion(current)
	latest, err := fetchRemoteVersion(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "check_failed", err.Error())
		return
	}
	if latest <= currentInt {
		writeJSONError(w, http.StatusConflict, "no_update", "Sistem zaten güncel.")
		return
	}

	writeUpdateStatusFile(updateStatus{
		Stage:   "queued",
		Version: latest,
		Message: "Güncelleme başlatılıyor",
		Time:    time.Now().Format(time.RFC3339),
	})

	cmd := exec.Command("systemctl", "start", "--no-block", "pwfe-update.service")
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		writeUpdateStatusFile(updateStatus{
			Stage:   "error",
			Version: latest,
			Message: "Güncelleme başlatılamadı",
			Error:   message,
			Time:    time.Now().Format(time.RFC3339),
		})
		writeJSONError(w, http.StatusInternalServerError, "trigger_failed", "Güncelleyici başlatılamadı: "+message)
		return
	}

	log.Printf("Güncelleme başlatıldı: v%s -> v%d", current, latest)
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":  "started",
		"current": current,
		"latest":  latest,
	})
}

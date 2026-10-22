package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Config struct {
	Port        int      `json:"port"`
	Directories []string `json:"directories"`
}

var configDir string
var configFile string
var currentConfig Config

func initConfigPath() {
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
		if err == nil && !strings.HasPrefix(rel, "..") {
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
				modTime = info.ModTime().Format("2006-01-02 15:04:05")
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

	decodedPath, err := url.QueryUnescape(dirPath)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

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
			"modTime": info.ModTime().Format("2006-01-02 15:04:05"),
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
	decodedPath, err := url.QueryUnescape(filePath)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

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
	decodedPath, err := url.QueryUnescape(filePath)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

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

	addr := fmt.Sprintf("0.0.0.0:%d", currentConfig.Port)
	log.Printf("PWFE başlatıldı. Port: %d", currentConfig.Port)
	log.Fatal(http.ListenAndServe(addr, nil))
}

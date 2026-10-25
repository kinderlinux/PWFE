// pwfe-update, PWFE'nin kaynak tabanlı güncelleyicisidir.
//
// GitHub'daki version.txt ile yerel sürümü karşılaştırır. Yeni sürüm varsa
// yalnızca main.go ve static/index.html dosyalarını indirir, geçici bir
// dizinde derler, mevcut sürümü yedekler, dosyaları değiştirir ve pwfe
// servisini yeniden başlatır.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var updaterVersion = "dev"

const (
	defaultUpdateRepo   = "kinderlinux/PWFE"
	defaultUpdateBranch = "main"
	maxVersionBytes     = 64
	maxMainGoBytes      = 2 << 20
	maxIndexBytes       = 5 << 20
	keepBackups         = 3
)

type updateState struct {
	Stage   string `json:"stage"`
	Version int    `json:"version,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Time    string `json:"time,omitempty"`
}

type updater struct {
	root      string
	repo      string
	branch    string
	rawBase   string
	noRestart bool
	logger    *log.Logger
}

func main() {
	apply := flag.Bool("apply", false, "Güncellemeyi uygula")
	check := flag.Bool("check", false, "Yalnızca sürüm kontrolü yap")
	rollback := flag.Bool("rollback", false, "Yedekten geri dön")
	toVersion := flag.String("to", "", "--rollback ile geri dönülecek sürüm")
	rootFlag := flag.String("root", "", "Kurulum dizini (varsayılan: /opt/pwfe veya PWFE_ROOT)")
	showVersion := flag.Bool("version", false, "Güncelleyici sürümünü yazdır")
	noRestart := flag.Bool("no-restart", false, "Test için servisi yeniden başlatma")
	flag.Parse()

	if *showVersion {
		fmt.Println(updaterVersion)
		return
	}

	root := resolveRoot(*rootFlag)
	u := &updater{
		root:      root,
		repo:      envOr("PWFE_UPDATE_REPO", defaultUpdateRepo),
		branch:    envOr("PWFE_UPDATE_BRANCH", defaultUpdateBranch),
		rawBase:   strings.TrimRight(envOr("PWFE_UPDATE_RAW_BASE", "https://raw.githubusercontent.com"), "/"),
		noRestart: *noRestart,
		logger:    newLogger(root),
	}

	logError := func(err error) {
		u.logger.Printf("HATA: %v", err)
		u.setState("error", 0, "Güncelleme başarısız", err.Error())
		os.Exit(1)
	}

	switch {
	case *check:
		if err := u.check(); err != nil {
			logError(err)
		}
	case *rollback:
		target := 0
		if strings.TrimSpace(*toVersion) != "" {
			v, err := parseVersion(*toVersion)
			if err != nil {
				logError(err)
			}
			target = v
		}
		if err := u.rollback(target); err != nil {
			logError(err)
		}
	case *apply:
		if err := u.apply(); err != nil {
			logError(err)
		}
	default:
		fmt.Println("Kullanım: pwfe-update --apply | --check | --rollback [--to SURUM] [--root DIZIN]")
		os.Exit(2)
	}
}

func resolveRoot(flagValue string) string {
	if root := strings.TrimSpace(flagValue); root != "" {
		return root
	}
	if root := strings.TrimSpace(os.Getenv("PWFE_ROOT")); root != "" {
		return root
	}
	exe, err := os.Executable()
	if err != nil {
		return "/opt/pwfe"
	}
	return filepath.Dir(exe)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func newLogger(root string) *log.Logger {
	path := filepath.Join(root, "update.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return log.New(os.Stdout, "", log.LstdFlags)
	}
	return log.New(io.MultiWriter(os.Stdout, file), "", log.LstdFlags)
}

func (u *updater) statePath() string {
	return filepath.Join(u.root, "update.state")
}

func (u *updater) setState(stage string, version int, message, errMessage string) {
	state := updateState{
		Stage:   stage,
		Version: version,
		Message: message,
		Error:   errMessage,
		Time:    time.Now().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	tmp := u.statePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		u.logger.Printf("durum dosyası yazılamadı: %v", err)
		return
	}
	if err := os.Rename(tmp, u.statePath()); err != nil {
		u.logger.Printf("durum dosyası taşınamadı: %v", err)
	}
}

func (u *updater) rawURL(path string) string {
	return fmt.Sprintf("%s/%s/%s/%s", u.rawBase, u.repo, u.branch, path)
}

func (u *updater) check() error {
	local, err := u.localVersion()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	remote, err := u.remoteVersion(ctx)
	if err != nil {
		return err
	}

	if remote > local {
		fmt.Printf("Yeni sürüm mevcut: v%d -> v%d\n", local, remote)
	} else {
		fmt.Printf("Sistem güncel: v%d (uzak: v%d)\n", local, remote)
	}
	return nil
}

func (u *updater) apply() error {
	local, err := u.localVersion()
	if err != nil {
		return err
	}

	unlock, err := u.lock()
	if err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			u.logger.Println("Başka bir güncelleme zaten çalışıyor.")
			return nil
		}
		return err
	}
	defer unlock()

	u.setState("checking", local, "Sürüm kontrol ediliyor", "")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	remote, err := u.remoteVersion(ctx)
	if err != nil {
		return err
	}
	if remote <= local {
		u.logger.Printf("Sistem güncel: v%d (uzak: v%d), değişiklik yapılmadı", local, remote)
		u.setState("done", local, "Sistem zaten güncel", "")
		return nil
	}

	u.logger.Printf("Güncelleme başlıyor: v%d -> v%d", local, remote)
	stage := filepath.Join(u.root, ".update", strconv.Itoa(remote))
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	success := false
	defer func() {
		if success {
			_ = os.RemoveAll(stage)
		}
	}()

	u.setState("downloading", remote, "Dosyalar indiriliyor", "")
	if err := u.downloadFiles(ctx, stage); err != nil {
		return err
	}

	u.setState("building", remote, "Yeni sürüm derleniyor", "")
	if err := u.build(ctx, stage, remote); err != nil {
		return err
	}

	u.setState("backing_up", remote, "Mevcut sürüm yedekleniyor", "")
	if err := u.backupCurrent(local); err != nil {
		return err
	}

	u.setState("installing", remote, "Yeni sürüm kuruluyor", "")
	defer func() {
		if !success {
			u.logger.Println("Kurulum tamamlanamadı, önceki sürüme dönülüyor...")
			if rbErr := u.restore(local); rbErr != nil {
				u.logger.Printf("Geri dönüş başarısız: %v", rbErr)
			}
		}
	}()

	if err := u.install(stage, remote); err != nil {
		return err
	}

	if !u.noRestart {
		u.setState("restarting", remote, "Servis yeniden başlatılıyor", "")
		if err := u.restart(); err != nil {
			return err
		}
	}

	success = true
	u.logger.Printf("Güncelleme tamamlandı: v%d", remote)
	u.setState("done", remote, fmt.Sprintf("Sürüm v%d kuruldu", remote), "")
	u.pruneBackups()
	return nil
}

func (u *updater) localVersion() (int, error) {
	data, err := os.ReadFile(filepath.Join(u.root, "version.txt"))
	if err != nil {
		return 0, fmt.Errorf("yerel sürüm okunamadı: %w", err)
	}
	return parseVersion(string(data))
}

func (u *updater) remoteVersion(ctx context.Context) (int, error) {
	body, err := u.fetchFile(ctx, "version.txt", maxVersionBytes)
	if err != nil {
		return 0, err
	}
	return parseVersion(string(body))
}

func (u *updater) fetchFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.rawURL(path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "pwfe-update/"+updaterVersion)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s indirilemedi: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s indirilemedi: HTTP %d", path, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("%s beklenenden büyük", path)
	}
	return body, nil
}

func (u *updater) downloadFiles(ctx context.Context, stage string) error {
	mainGo, err := u.fetchFile(ctx, "main.go", maxMainGoBytes)
	if err != nil {
		return err
	}
	if !bytes.Contains(mainGo, []byte("package main")) {
		return errors.New("indirilen main.go geçerli görünmüyor")
	}

	indexHTML, err := u.fetchFile(ctx, "static/index.html", maxIndexBytes)
	if err != nil {
		return err
	}
	if !bytes.Contains(indexHTML, []byte("</html>")) {
		return errors.New("indirilen index.html geçerli görünmüyor")
	}

	if err := os.MkdirAll(filepath.Join(stage, "static"), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "main.go"), mainGo, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "static", "index.html"), indexHTML, 0644); err != nil {
		return err
	}

	goMod := "module pwfe\n\ngo 1.18\n"
	return os.WriteFile(filepath.Join(stage, "go.mod"), []byte(goMod), 0644)
}

func (u *updater) build(ctx context.Context, stage string, remote int) error {
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("Go derleyicisi bulunamadı. Arch Linux'ta 'sudo pacman -S go' ile kurun")
	}

	buildCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	ldflags := fmt.Sprintf("-s -w -X main.version=%d", remote)
	cmd := exec.CommandContext(buildCtx, "go", "build", "-trimpath", "-ldflags", ldflags, "-o", "pwfe.new", "main.go")
	cmd.Dir = stage
	cmd.Env = append(os.Environ(),
		"GOTOOLCHAIN=local",
		"GOPROXY=off",
		"CGO_ENABLED=0",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("derleme başarısız: %w\n%s", err, strings.TrimSpace(string(output)))
	}

	binary := filepath.Join(stage, "pwfe.new")
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 15*time.Second)
	defer verifyCancel()
	check := exec.CommandContext(verifyCtx, binary, "--version")
	check.Dir = stage
	versionOutput, err := check.CombinedOutput()
	if err != nil {
		return fmt.Errorf("yeni binary çalıştırılamadı: %w\n%s", err, strings.TrimSpace(string(versionOutput)))
	}
	if strings.TrimSpace(string(versionOutput)) != strconv.Itoa(remote) {
		return fmt.Errorf("yeni binary sürümü hatalı: %q", strings.TrimSpace(string(versionOutput)))
	}
	return nil
}

func (u *updater) backupCurrent(version int) error {
	dir := filepath.Join(u.root, "backups", strconv.Itoa(version))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(u.root, "pwfe"), filepath.Join(dir, "pwfe"), 0755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(u.root, "static", "index.html"), filepath.Join(dir, "index.html"), 0644); err != nil {
		return err
	}
	return copyFile(filepath.Join(u.root, "version.txt"), filepath.Join(dir, "version.txt"), 0644)
}

func (u *updater) install(stage string, remote int) error {
	if err := os.MkdirAll(filepath.Join(u.root, "static"), 0755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(stage, "static", "index.html"), filepath.Join(u.root, "static", "index.html"), 0644); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(stage, "pwfe.new"), filepath.Join(u.root, "pwfe"), 0755); err != nil {
		return err
	}
	return writeVersion(filepath.Join(u.root, "version.txt"), remote)
}

func (u *updater) restore(version int) error {
	dir := filepath.Join(u.root, "backups", strconv.Itoa(version))
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("v%d yedeği bulunamadı: %w", version, err)
	}
	if err := copyFile(filepath.Join(dir, "pwfe"), filepath.Join(u.root, "pwfe"), 0755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(dir, "index.html"), filepath.Join(u.root, "static", "index.html"), 0644); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(dir, "version.txt"), filepath.Join(u.root, "version.txt"), 0644); err != nil {
		return err
	}
	if !u.noRestart {
		if err := u.restart(); err != nil {
			return err
		}
	}
	return nil
}

func (u *updater) rollback(target int) error {
	unlock, err := u.lock()
	if err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return errors.New("başka bir güncelleme çalışıyor, geri dönüş yapılamadı")
		}
		return err
	}
	defer unlock()

	local, err := u.localVersion()
	if err != nil {
		return err
	}
	if target == 0 {
		target, err = u.latestBackup(local)
		if err != nil {
			return err
		}
	}
	if target == local {
		return fmt.Errorf("zaten v%d sürümü çalışıyor", local)
	}

	u.setState("rollback", target, "Önceki sürüme dönülüyor", "")
	if err := u.backupCurrent(local); err != nil {
		u.logger.Printf("mevcut sürüm yedeklenemedi: %v", err)
	}
	if err := u.restore(target); err != nil {
		return err
	}
	u.logger.Printf("Geri dönüş tamamlandı: v%d -> v%d", local, target)
	u.setState("done", target, fmt.Sprintf("Sürüm v%d geri yüklendi", target), "")
	fmt.Printf("Geri dönüldü: v%d -> v%d\n", local, target)
	return nil
}

func (u *updater) latestBackup(current int) (int, error) {
	entries, err := os.ReadDir(filepath.Join(u.root, "backups"))
	if err != nil {
		return 0, fmt.Errorf("yedekler okunamadı: %w", err)
	}
	best := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version, err := parseVersion(entry.Name())
		if err != nil || version >= current {
			continue
		}
		if version > best {
			best = version
		}
	}
	if best == 0 {
		return 0, errors.New("geri dönülecek yedek bulunamadı")
	}
	return best, nil
}

func (u *updater) restart() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemctl bulunamadı")
	}
	output, err := exec.Command("systemctl", "restart", "pwfe.service").CombinedOutput()
	if err != nil {
		return fmt.Errorf("pwfe servisi yeniden başlatılamadı: %w\n%s", err, strings.TrimSpace(string(output)))
	}

	// Type=simple ile systemctl, süreç henüz hazır olmadan dönebilir.
	// Kısa bir bekleme sonrası servisin art arda aktif kaldığını doğrula.
	time.Sleep(time.Second)
	for i := 0; i < 10; i++ {
		if err := exec.Command("systemctl", "is-active", "--quiet", "pwfe.service").Run(); err != nil {
			return errors.New("pwfe servisi yeniden başlatıldıktan sonra aktif değil")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func (u *updater) pruneBackups() {
	entries, err := os.ReadDir(filepath.Join(u.root, "backups"))
	if err != nil {
		return
	}
	type backup struct {
		version int
		name    string
	}
	var backups []backup
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version, err := parseVersion(entry.Name())
		if err != nil {
			continue
		}
		backups = append(backups, backup{version: version, name: entry.Name()})
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].version > backups[j].version
	})
	for i := keepBackups; i < len(backups); i++ {
		_ = os.RemoveAll(filepath.Join(u.root, "backups", backups[i].name))
	}
}

func (u *updater) lock() (func(), error) {
	file, err := os.OpenFile(filepath.Join(u.root, ".update.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func parseVersion(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("sürüm boş")
	}
	version, err := strconv.Atoi(value)
	if err != nil || version < 0 {
		return 0, fmt.Errorf("geçersiz sürüm: %q", value)
	}
	return version, nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	tmp := destination + ".tmp"
	output, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := output.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, destination)
}

func writeVersion(path string, version int) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(version)+"\n"), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

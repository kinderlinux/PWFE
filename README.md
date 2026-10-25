# Panda Web File Explorer (PWFE)

Panda Web File Explorer (PWFE), yerel ağınızda (varsayılan **7933** portu) çalışan, hafif ve sade bir web dosya gezginidir. Go ile geliştirilmiştir; eski bilgisayarlarda düşük RAM/CPU tüketimiyle çalışır. Arch Linux ve diğer Linux dağıtımları ile Windows üzerinde çalışır.

## Özellikler

- 🚀 **Hafif ve hızlı:** Go ile yazılmış, harici çalışma zamanı gerektirmeyen iki küçük binary (`pwfe`, `pwfe-update`).
- 📂 **Şifresiz web paneli:** Paylaşılacak klasörleri Ayarlar panelinden ekleyip çıkarabilirsiniz.
- 📥 **Dosya indirme ve önizleme:** Resim, PDF, metin, JSON, HTML gibi dosyaları tarayıcıda önizleyin.
- 📋 **Ayrıntılı listeleme:** Tür, boyut ve değiştirme tarihi; klasör içinde arama ve sıralama.
- 🌐 **3 dil:** Türkçe, İngilizce, Almanca.
- ⚙️ **Systemd servisi:** Açılışta kullanıcı oturumu gerekmeden otomatik başlar.
- 🔄 **Web'den güncelleme:** Sol üstteki **Güncelle** butonu ile yeni sürümü tek tıkla kurun.
- 🛡️ **Güncelleme şifresi:** Güncelleme işlemi, kurulum sırasında belirlenen şifre ile korunur.

---

## Arch Linux kurulumu (önerilen)

### 1. Kurulum betiğini indirin

```bash
curl -fsSL https://raw.githubusercontent.com/kinderlinux/PWFE/main/install.sh -o install.sh
```

### 2. İnceleyip çalıştırın

```bash
less install.sh
sudo bash install.sh
```

Betik sırasıyla:

1. **`go`, `git` ve `curl`** araçlarını kontrol eder.
2. Eksik varsa **onayınızı sorar** ve `pacman` ile kurar.
3. Kaynak kodu geçici bir dizine klonlar.
4. `pwfe` ve `pwfe-update` programlarını derler.
5. Dosyaları `/opt/pwfe` altına, ayarları `/etc/pwfe` altına kurar.
6. Güncelleme şifresi yoksa **rastgele bir şifre üretip ekrana yazar**.
7. Systemd servisini kurar ve **açılışta otomatik başlayacak şekilde etkinleştirir**.

Kurulum sonunda ekrana şuna benzer bir özet basılır:

```text
PWFE v1 kuruldu.
  Arayüz      : http://192.168.1.20:7933
  Ayarlar     : /etc/pwfe/config.json
  Güncelleyici: /opt/pwfe/pwfe-update
  Yedekler    : /opt/pwfe/backups
  Güncelleme şifresi: Xy7...
```

> **Not:** Güncelleme şifresini not edin. Bu şifre yalnızca güncelleme başlatmak için kullanılır; dosya erişiminde şifre yoktur.

### Kurulum seçenekleri

```bash
sudo bash install.sh --branch main             # farklı dal
sudo bash install.sh --local                   # betiğin yanındaki kaynak ağacından kur
sudo bash install.sh --update-repo sahip/depo  # güncellemelerin izleyeceği GitHub deposu
sudo bash install.sh --update-password 'gizli' # şifreyi kendiniz belirleyin
sudo bash install.sh --no-deps                 # pacman çağrısı yapma
sudo bash install.sh -y                        # onay sormadan
sudo bash install.sh --force                   # Arch kontrolünü atla
```

`--local`, betiğin bulunduğu dizindeki kaynak ağacından derler; değişiklikleri GitHub'a push etmeden test etmek için kullanışlıdır. Güncellemeler yine `--branch` / `--update-repo` ile belirlenen uzak kaynağı izler.

`--branch` ve GitHub deposu (`--repo`) bilgileri systemd birimlerine yazılır; web arayüzü ve güncelleyici aynı dalı/depoyu izler. GitHub dışı bir `--repo` kullanırsanız güncelleme deposunu `--update-repo sahip/depo` ile ayrıca belirtin.

Tek satır kurulum (indirip incelemeden çalıştırmak isterseniz):

```bash
curl -fsSL https://raw.githubusercontent.com/kinderlinux/PWFE/main/install.sh | sudo bash
```

---

## Kullanım

Tarayıcıdan sunucu IP adresine ve **7933** portuna gidin:

```text
http://<SUNUCU_IP>:7933
```

1. Sağ üstteki **Ayarlar** butonundan paylaşmak istediğiniz klasörleri ekleyin.
2. Klasörlere göz atın, dosyaları önizleyin veya indirin.

### Servis komutları

```bash
sudo systemctl status pwfe
sudo systemctl restart pwfe
sudo journalctl -u pwfe -n 50
```

Servis `multi-user.target` altında etkinleştirildiği için sistem açılışında, **kullanıcı oturumu açılmadan** çalışır.

---

## Güncelleme

PWFE, GitHub'daki `version.txt` dosyasındaki sürüm numarası ile yerel sürümü karşılaştırır.

- Uzak sürüm **yerelden büyükse**: yalnızca `main.go` ve `static/index.html` indirilir, geçici dizinde derlenir, mevcut sürüm yedeklenir ve servis yeniden başlatılır.
- Uzak sürüm **eşit veya küçükse**: kurulu dosyalara dokunulmaz.

Güncelleme sonrası servis sağlık kontrolü başarısız olursa güncelleyici otomatik olarak önceki sürüme döner. Yedeklerden en yeni 3 tanesi saklanır (`/opt/pwfe/backups`).

> Web güncellemesi yalnızca sunucu kodunu (`main.go`) ve arayüzü yeniler. `pwfe-update` güncelleyicisinin kendisi değiştiyse yeni sürümü almak için `install.sh` bir kez daha çalıştırılmalıdır.

Sürüm numarası düz artan bir sayıdır (`1`, `2`, `3` ...). Güncelleme yayınlamak için `version.txt` dosyasını artırıp değişiklikleri push etmek yeterlidir.

### Web arayüzünden

1. Sol üstteki **⬆ Güncelle** butonuna tıklayın.
2. **Kontrol Et** ile yeni sürüm olup olmadığını görün.
3. Güncelleme şifresini girip **Güncelle**'ye basın.
4. İşlem tamamlanınca sayfa otomatik yenilenir.

### Komut satırından

```bash
sudo /opt/pwfe/pwfe-update --check       # sürüm kontrolü
sudo systemctl start pwfe-update.service # güncellemeyi uygula
sudo /opt/pwfe/pwfe-update --rollback    # son yedeğe dön
sudo /opt/pwfe/pwfe-update --rollback --to 1
```

Güncelleme kayıtları: `/opt/pwfe/update.log`

### Güncelleme şifresini değiştirme

```bash
sudo /opt/pwfe/pwfe --set-update-password 'yeni-sifre'
```

Şifre `/etc/pwfe/update.passwd` içinde, tuzlanmış SHA-256 özeti olarak tutulur (`0600`, root).

---

## Dizin düzeni

```text
/opt/pwfe/
├── pwfe                 # web sunucusu
├── pwfe-update          # güncelleyici
├── static/index.html    # arayüz
├── version.txt          # yerel sürüm
├── update.log           # güncelleme günlüğü
├── update.state         # son güncelleme durumu
├── backups/             # eski sürümler (pwfe + index.html + version.txt)
└── .update/             # geçici indirme/derleme dizini

/etc/pwfe/
├── config.json          # port + paylaşılan dizinler
└── update.passwd        # güncelleme şifresi (0600)

/etc/systemd/system/
├── pwfe.service         # ana servis (açılışta otomatik)
└── pwfe-update.service  # tek seferlik güncelleyici
```

Ayarlar:

```json
{
  "port": 7933,
  "directories": []
}
```

> Eski bir sürümden geliyorsanız (örneğin `/root/.config/pwfe/config.json`) paylaşılan klasörler otomatik taşınmaz; **Ayarlar** panelinden yeniden ekleyin.

---

## Elle derleme (isteğe bağlı)

Arch Linux'ta:

```bash
sudo pacman -S --needed go git curl
git clone https://github.com/kinderlinux/PWFE.git
cd PWFE

VERSION="$(tr -d '[:space:]' < version.txt)"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o pwfe .
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.updaterVersion=$VERSION" -o pwfe-update ./updater
```

Windows'ta yalnızca ana program derlenir:

```powershell
go build -o pwfe.exe .
```

Windows'ta config varsayılan olarak `%APPDATA%\pwfe\config.json` altında tutulur. `pwfe-update` yalnızca Linux/systemd içindir.

Testleri çalıştırmak için:

```bash
go test ./...
```

---

## Kaldırma

```bash
sudo bash install.sh --uninstall          # config ve şifre korunur
sudo bash install.sh --uninstall --purge  # config ve şifre de silinir
```

---

## Güvenlik notları

- Güncelleme, GitHub deposundaki kodu **root olarak** derleyip çalıştırır. Depoya yazma yetkisi olan kişiler tüm kurulumlarda root erişimi elde eder. Depo hesabınızda 2FA ve korumalı dal kullanın.
- Güncelleme şifresi düz HTTP üzerinden gönderilir; aynı ağdaki biri trafiği dinleyebilir. Gerekirse PWFE'yi yalnızca güvendiğiniz ağda çalıştırın veya önüne TLS sonlandıran bir reverse proxy koyun.
- Dosya erişiminde şifre yoktur; paylaşılan klasörleri minimumda tutun.

---

## Sorun giderme

| Sorun | Çözüm |
|---|---|
| Güncelle butonu "şifre belirlenmemiş" diyor | `sudo /opt/pwfe/pwfe --set-update-password 'yeni'` |
| Güncelleme derlenemiyor | `go` kurulu mu? `sudo pacman -S --needed go` |
| Güncelleme sonrası servis açılmıyor | `journalctl -u pwfe -n 50` ve `/opt/pwfe/update.log`; `sudo /opt/pwfe/pwfe-update --rollback` |
| Güncelleme yanlış depoyu/dalı izliyor | Birimlerdeki `PWFE_UPDATE_REPO` / `PWFE_UPDATE_BRANCH` değerlerini düzeltin veya `--update-repo` ile yeniden kurun |
| Arayüze erişemiyorum | `sudo systemctl status pwfe`, port çakışması ve güvenlik duvarında 7933/TCP iznini kontrol edin |
| Açılışta servis başlamıyor | `sudo systemctl enable --now pwfe` |
| Port değiştirmek | `/etc/pwfe/config.json` içinde `port`, ardından `sudo systemctl restart pwfe` |

---

*KinderLinux tarafından ❤️*

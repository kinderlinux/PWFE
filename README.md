# Panda Web File Explorer (PWFE)

Yerel ağınızda çalışan, hafif bir web dosya gezginidir. Go ile yazılmıştır; varsayılan port **7933**'tür. Arch Linux için kurulum betiği ve systemd servisi içerir.

## Özellikler

- 🚀 Hafif: harici çalışma zamanı gerektirmeyen `pwfe` ve `pwfe-update` binary'leri.
- 📂 Şifresiz web paneli: paylaşılan klasörleri **Ayarlar**'dan ekleyin.
- 📥 Dosya indirme ve tarayıcıda önizleme.
- 🔄 Web arayüzünden tek tıkla güncelleme.
- 🛡️ Güncelleme işlemi şifre ile korunur.
- ⚙️ Açılışta kullanıcı girişi gerekmeden otomatik başlar.

---

## Kurulum (Arch Linux)

Tek satır kurulum (betiği önce indirip incelemek istemiyorsanız):

```bash
curl -fsSL https://raw.githubusercontent.com/kinderlinux/PWFE/main/install.sh | sudo bash
```

Betik sırasıyla:

1. `go`, `git` ve `curl` araçlarını kontrol eder; eksikse onayınızla `pacman` ile kurar.
2. İlk kurulumda **güncelleme şifresini sorar**.
3. `pwfe` ve `pwfe-update` programlarını derleyip `/opt/pwfe` altına kurar.
4. Servisi kurar ve açılışta otomatik başlayacak şekilde etkinleştirir.

> Güncelleme şifresi ilk kurulumda sorulur ve sonraki kurulumlarda korunur. Yalnızca güncelleme başlatmak için kullanılır; dosya erişiminde şifre yoktur.

Kurulumdan sonra tarayıcıdan sunucu adresini açın:

```text
http://<SUNUCU_IP>:7933
```

**Ayarlar** butonundan paylaşmak istediğiniz klasörleri ekleyin.

---

## Güncelleme

Web arayüzünün sol üstündeki **Güncelle** butonunu kullanın. Yeni sürüm yoksa dosyalara dokunulmaz.

Komut satırından:

```bash
sudo /opt/pwfe/pwfe-update --check
sudo systemctl start pwfe-update.service
sudo /opt/pwfe/pwfe-update --rollback
```

- Uzak sürüm, GitHub'daki `version.txt` ile karşılaştırılır.
- Yeni sürüm varsa yalnızca `main.go` ve `static/index.html` indirilir ve derlenir.
- Güncelleme başarısız olursa otomatik olarak önceki sürüme dönülür.
- Günlük: `/opt/pwfe/update.log`

Yeni sürüm yayınlamak için `version.txt` dosyasını artırıp `main` dalına push etmek yeterlidir.

### Güncelleme şifresini değiştirme

```bash
sudo /opt/pwfe/pwfe --set-update-password 'yeni-sifre'
```

---

## Kaldırma

```bash
sudo bash install.sh --uninstall
```

Bu komut servisi durdurur, systemd birimlerini, `/opt/pwfe` ve `/etc/pwfe` dizinlerini (config ve güncelleme şifresi dahil) siler.

---

## Dizin düzeni

```text
/opt/pwfe/
├── pwfe                 # web sunucusu
├── pwfe-update          # güncelleyici
├── static/index.html    # arayüz
├── version.txt          # yerel sürüm
├── update.log           # güncelleme günlüğü
├── backups/             # eski sürümler
└── .update/             # geçici derleme dizini

/etc/pwfe/
├── config.json          # port ve paylaşılan klasörler
└── update.passwd        # güncelleme şifresi (0600)
```

Ayarlar:

```json
{
  "port": 7933,
  "directories": []
}
```

---

## Güvenlik

- Güncelleme, GitHub deposundaki kodu **root olarak** derleyip çalıştırır. Depo hesabınızda 2FA ve korumalı dal kullanın.
- Güncelleme şifresi düz HTTP üzerinden gönderilir. Gerekirse PWFE'yi yalnızca güvendiğiniz ağda çalıştırın veya önüne TLS sonlandıran bir reverse proxy koyun.
- Dosya erişiminde şifre yoktur; paylaşılan klasörleri minimumda tutun.

---

## Sorun giderme

| Sorun | Çözüm |
|---|---|
| Güncelle butonu "şifre belirlenmemiş" diyor | `sudo /opt/pwfe/pwfe --set-update-password 'yeni'` |
| Güncelleme derlenemiyor | `sudo pacman -S --needed go` |
| Güncelleme sonrası servis açılmıyor | `journalctl -u pwfe -n 50` ve `/opt/pwfe/update.log`; `sudo /opt/pwfe/pwfe-update --rollback` |
| Arayüze erişemiyorum | `sudo systemctl status pwfe`; port çakışması ve güvenlik duvarı |
| Açılışta servis başlamıyor | `sudo systemctl enable --now pwfe` |

---

*KinderLinux tarafından ❤️*

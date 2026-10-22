# Panda Web File Explorer (PWFE)

Panda Web File Explorer (PWFE), yerel ağınızda (7933 portunda) çalışan, şifresiz ve web paneli üzerinden yönetilebilen son derece hafif, hızlı ve sade bir dosya gezginidir. Eski bilgisayarlar ve Arch Linux başta olmak üzere tüm Linux dağıtımları ile Windows'ta kusursuz çalışacak şekilde tasarlanmıştır.

## Özellikler

- 🚀 **Yüksek Performans & Düşük Kaynak:** Go (Golang) ile geliştirilmiştir, eski PC'lerde RAM/CPU tüketimi minimumdur.
- 📂 **Şifresiz Web Paneli ile Klasör Yönetimi:** İstediğiniz klasörleri web arayüzündeki Ayarlar panelinden anında ekleyebilir/çıkarabilirsiniz.
- 📥 **Dosya İndirme:** Paylaşılan dosyalara tek tıkla tarayıcı üzerinden hızlı erişim ve indirme.
- 📋 **Detaylı Dosya Bilgileri:** Dosya türleri (uzantıları), boyutları ve son değiştirme tarihleri.
- 🌐 **3 Dil Desteği:** Türkçe, İngilizce ve Almanca dil seçenekleri (anlık geçiş).
- ⚙️ **Systemd Servis Desteği:** Linux sistemlerde her açılışta otomatik başlama.

---

## Kurulum ve Çalıştırma (Arch Linux / Linux)

### 1. Derleme (Build)
**Size direkt derlenmiş bir dosya sunmuyoruz!** Go yüklü bir bilgisayarda (veya hedef makinede) projeyi derleyin:
```bash
go build -o pwfe main.go
```

### 2. Konumlandırma ve Çalıştırma
Dosyaları uygun bir dizine (örneğin `/opt/pwfe`) taşıyın:
```bash
sudo mkdir -p /opt/pwfe/static
sudo cp pwfe /opt/pwfe/
sudo cp -r static/* /opt/pwfe/static/
sudo cp config.json /opt/pwfe/
```

### 3. Systemd Servisi Olarak Ayarlama (Her Açılışta Başlaması İçin)
Servis dosyasını systemd dizinine kopyalayın:
```bash
sudo cp pwfe.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable pwfe
sudo systemctl start pwfe
```

Servisin durumunu kontrol etmek için:
```bash
sudo systemctl status pwfe
```

---

## Kullanım

Tarayıcınızı açıp yerel ağınızdaki sunucu IP adresine ve **7933** portuna gidin:
```text
http://<SUNUCU_IP>:7933
```
veya yerelde:
```text
http://localhost:7933
```

1. Sağ üstteki **Ayarlar** butonuna tıklayarak paylaşmak istediğiniz klasör yollarını (Örn: `/home/kullanici/Belgeler` veya `/mnt/depo`) şifresiz bir şekilde ekleyin.
2. Ana ekrandan klasörlere göz atın ve istediğiniz dosyaları indirin!

---

*KinderLinux tarafından ❤️*

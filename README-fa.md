<div dir="rtl">

# moav-client

![Go](https://img.shields.io/badge/Go-1.25-blue?logo=go) ![License: MIT](https://img.shields.io/badge/License-MIT-green)

**[English](README.md)** | فارسی

کلاینتی برای سرورهای **[MoaV — مادر همه‌ی VPNها](https://github.com/shayanb/MoaV)**. یک باندل اشتراک چندپروتکلی را می‌خواند، رمزنگاری واقعی هر پروتکل را به sing-box و مجموعه‌ای از sidecarهای اختیاری (MasterDNS، AmneziaWG، Psiphon، TrustTunnel، Tor) واگذار می‌کند، تأخیر هر endpoint را به‌صورت سرتاسری از داخل تونل اندازه می‌گیرد، بار را روی مجموعه‌ی سالم پخش می‌کند، و یک پروکسی محلی واحد SOCKS5 / HTTP CONNECT ارائه می‌دهد. یک داشبورد React با ظاهری هماهنگ با پنل ادمین MoaV دید زنده‌ای از سلامت endpointها، پهنای‌باند هر پروتکل، ویرایش قوانین پلاگین و لاگ زنده می‌دهد.

<!-- screenshot: dashboard overview (Endpoints tab) -->
<!-- ![moav-client dashboard](docs/assets/dashboard.png) -->

---

## شروع سریع

```bash
curl -fsSL https://raw.githubusercontent.com/MotherofallVPNs/moav-client/main/install.sh | bash
```

نصب‌کننده پیش‌نیازها را بررسی می‌کند، مخزن را clone می‌کند، اجازه می‌دهد sidecarها را انتخاب کنید، `config.yaml` را می‌سازد، ایمیج‌ها را build می‌کند، استک را بالا می‌آورد و دستور سراسری `moav-client` را نصب می‌کند. هم تعاملی و هم بدون تعامل کار می‌کند — برای نصب headless/با فلگ به [docs/INSTALL.md](docs/INSTALL.md) نگاه کنید.

سپس استک را با `moav-client` مدیریت کنید:

```bash
moav-client status                # docker compose ps
moav-client logs proxy-core       # دنبال‌کردن لاگ‌ها
moav-client probe                 # اجرای probe از طریق API
moav-client sidecar add tor       # فعال‌سازی + build + اجرای sidecar تور
moav-client update                # git pull + rebuild + restart
```

آدرس‌های ارائه‌شده:

| چیست | آدرس |
|---|---|
| داشبورد | http://localhost:3001 |
| پروکسی SOCKS5 | `socks5h://localhost:1080` |
| پروکسی HTTP CONNECT | http://localhost:8081 |
| REST + WS API | http://localhost:8088 |

مرورگر یا پروکسی سیستم را روی `socks5h://localhost:1080` تنظیم کنید. هر اتصال از سالم‌ترین endpoint عبور می‌کند.

### منابع

دیسک هر کانتینر + دانلود نصب اولیه (amd64). سرویس‌های هسته همیشه اجرا می‌شوند؛ sidecarها اختیاری‌اند (با `--profile`).

| سرویس | دیسک | دانلود | پروفایل |
|---|---|---|---|
| proxy-core | ~۱۸ MB | هسته | همیشه |
| web-ui | ~۷۵ MB | هسته | همیشه |
| sing-box | ~۱۱۶ MB | هسته | همیشه |
| xray | ~۱۰۴ MB | هسته | همیشه |
| MasterDNS | ~۱۳۸ MB | sidecar | `masterdns` |
| AmneziaWG | ~۱۴۹ MB | sidecar | `amneziawg` |
| Psiphon | ~۱۷۶ MB | sidecar | `psiphon` |
| TrustTunnel | ~۹۰ MB | sidecar | `trusttunnel` |
| Tor | ~۸۵ MB | sidecar | `tor` |

| مصرف | فقط هسته | استک کامل |
|---|---|---|
| دیسک (ایمیج‌های runtime) | ~۳۱۳ MB | ~۹۴۵ MB |
| دانلود نصب اولیه | ~۱۹۰ MB | ~۸۱۰ MB |
| RAM (بی‌کار) | ~۱۵۰ MB | ~۴۰۰ MB |

یک build کامل حدود ۴ GB کش build هم می‌گذارد که با `docker builder prune` قابل پاک‌سازی است. به‌روزرسانی‌ها فقط لایه‌های تغییریافته را دانلود می‌کنند.

---

## پروتکل‌های پشتیبانی‌شده

پارسر باندل، فرمت استاندارد اشتراک MoaV (URIهای سبک V2Ray با base64) به‌علاوه‌ی فایل‌های اختیاری `.conf` وایرگارد را می‌پذیرد.

| پروتکل | مسیر اتصال | توضیح |
|---|---|---|
| VLESS / Reality | خروجی sing-box | اثرانگشت utls، کلید عمومی و short-id ریالیتی |
| VLESS + WS + TLS (CDN) | خروجی sing-box | utls + ALPN + path / host |
| Trojan + TLS | خروجی sing-box | اثرانگشت uTLS، SNI |
| Shadowsocks-2022 | خروجی sing-box | 2022-blake3-aes-128-gcm |
| Hysteria 2 (+obfs) | خروجی sing-box | مبهم‌سازی salamander |
| VLESS + XHTTP + Reality | خروجی xray | xhttp فقط در Xray است؛ روی ‎11800+‎ |
| WireGuard | بلوک `endpoints[]` در sing-box | از `wireguard.conf` |
| AmneziaWG | sidecar `amneziawg` | `amneziawg-go` فضای‌کاربر + microsocks روی مسیر پیش‌فرض awg0 |
| TrustTunnel | sidecar `trusttunnel` | کلاینت آماده‌ی بالادست (HTTP/2 + HTTP/3) در حالت SOCKS5 |
| MasterDNS | sidecar `masterdns` | باینری بالادست از `masterking32/MasterDnsVPN` |
| Psiphon | sidecar `psiphon` | از سورس `Psiphon-Labs/psiphon-tunnel-core`؛ با کانفیگ توکار بدون نیاز به اعتبارنامه تونل می‌زند |
| Tor | sidecar `tor` | `peterdavehello/tor-socks-proxy` — SOCKS5 روی ‎:9150‎، بدون اعتبارنامه |

هر sidecar ورودی SOCKS5 خودش را روی شبکه‌ی داکری `moav-net` ارائه می‌دهد؛ moav-client هرکدام را یک عضو در استخر متعادل‌کننده می‌بیند.

---

## داشبورد وب

| تب | کاری که می‌توانید انجام دهید |
|---|---|
| **Endpoints** | وضعیت و تأخیر زنده. روشن/خاموش‌کردن هرکدام (toggle برای sidecar کانتینر را هم متوقف/شروع می‌کند). ویرایش اولویت درجا. ردیف‌های غیرفعال نشان `DISABLED` می‌گیرند. |
| **Sources** | وارد کردن باندل سرور دیگر با رهاکردن فایل `.zip` — زیر `data/<name>/` استخراج و یک منبع اضافه می‌شود. فهرست/حذف منابع و reload. |
| **Analytics** | کارت‌های آپلود/دانلود هر پروتکل با نمودار ۲ دقیقه‌ای، نمودار سطحی هم‌پوشان، و جدول هر endpoint با شمارش dial/خطا/failover. |
| **Plugins** | فهرست، مرتب‌سازی، ویرایش و حذف قوانین مسیریابی. افزودن از کاتالوگ آماده. تغییرات بدون restart اعمال می‌شوند. |
| **Settings** | تغییر زنده‌ی استراتژی متعادل‌سازی، probe همه، **سطح دسترسی شبکه** (loopback / lan / public با احراز هویت اختیاری)، کلید SNI-spoof، و پشتیبان‌گیری/بازیابی. |
| **Debug** | لاگ زنده (بافرهای حلقوی per-level، ~۸۰۰ رویداد برای هر سطح). فیلتر، pause/autoscroll/copy/clear. به‌علاوه جدول flow هر اتصال. |
| **Diagnostics** | بررسی اتصال از خود proxy-core: TCP، DNS یا traceroute — اختیاراً *از داخل* تونل یک endpoint مشخص. |
| **Config** | بارگذاری زنده‌ی `config.yaml`. ویرایش و ذخیره. برای تغییرات ساختاری پیام restart نمایش داده می‌شود. |

<!-- screenshot/gif: dashboard tabs walkthrough -->
<!-- ![dashboard walkthrough](docs/assets/dashboard.gif) -->

---

## پیکربندی

فایل `config.yaml` همه‌چیز را کنترل می‌کند؛ sing-box و xray به‌صورت پیش‌فرض روشن‌اند (رمزنگاری پروتکل‌ها). فایل کامل و کامنت‌گذاری‌شده‌ی [`config.yaml.example`](config.yaml.example) مرجع است — کپی و ویرایش کنید. بخش‌های کلیدی:

- `proxy` — پورت‌های listener + احراز هویت اختیاری SOCKS5
- `subscription` — `file` / `url` / `wireguard_files` یا چند `sources`
- `load_balancing.strategy` — `latency` | `priority` | `weighted`
- `plugins` — `torrent_block`، `block_direct`، `routing_rules`
- `singbox` / `xray` / `sni_spoof` — sidecarهای dialer (پیش‌فرض روشن)
- `sidecars` — `masterdns` / `amneziawg` / `psiphon` / `trusttunnel` / `tor`

اکثر کاربران هرگز `config.yaml` را دستی ویرایش نمی‌کنند — وارد کردن باندل (تب Sources) و toggle کردن در داشبورد آن را برایتان می‌نویسد.

---

## پلاگین‌ها

زنجیره‌ی قوانین «اولین تطابق برنده». هم `config.yaml` و هم تب Plugins داشبورد یک موتور را تغذیه می‌کنند؛ تغییرات داشبورد بدون restart اعمال می‌شوند.

انواع تطابق: `domain`، `domain_suffix`، `domain_keyword`، `ip_cidr`، `geoip`، `port`، `protocol`.
عمل‌ها: `proxy` (پیش‌فرض)، `direct` (دور زدن)، `block` (انداختن).

### مسدودسازی مستقیم (کلید قطع)

`plugins.block_direct: true` یک کلید قطع نشت است: هر اتصالی که قرار است مستقیم برود انداخته می‌شود — هم یک قانون `direct` و هم fallback آخرین‌چاره‌ی متعادل‌کننده وقتی همه‌ی endpointها down هستند. پیش‌فرض `false`. وقتی روشن است قوانین `direct` مثل `lan-direct` را هم می‌شکند.

### GeoIP

قوانین `geoip:<cc>` یک IP مقصد را با لیست CIDR در `geoip/<cc>.txt` تطبیق می‌دهند (لیست ایران در مخزن هست و هفتگی توسط CI به‌روز می‌شود). تطابق فقط روی **IP** است — میزبان‌های نام‌دار resolve نمی‌شوند. به [geoip/README.md](geoip/README.md) نگاه کنید.

---

## CLI

```
moav-client [command] [flags]

Commands:
  serve       اجرای پروکسی + API (پیش‌فرض)
  probe       probe یک‌باره‌ی تأخیر همه‌ی endpointها
  list        فهرست endpointها بدون probe
  fetch-sub   گرفتن و پارس یک URL اشتراک
  healthcheck بررسی API محلی و خروج 0/1 (healthcheck کانتینر)
  version     چاپ نسخه
  help        راهنما

Global flags:
  --config    مسیر config.yaml (پیش‌فرض: config.yaml)
```

---

## مستندات

- [docs/INSTALL.md](docs/INSTALL.md) — نصب headless / با فلگ، سطح دسترسی شبکه
- [docs/SIDECARS.md](docs/SIDECARS.md) — TrustTunnel، Psiphon، Tor، MasterDNS، AmneziaWG
- [docs/SNI_SPOOFING.md](docs/SNI_SPOOFING.md) — sidecar اختیاری SNI-spoofing
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — پل sing-box، متعادل‌کننده/failover، prober، کنترل داکر
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) — مشکلات رایج
- [docs/MOAV_BUNDLE.md](docs/MOAV_BUNDLE.md) — پیشنهاد فرمت باندل `moav://`
- [CLAUDE.md](CLAUDE.md) — راهنمای عامل LLM

---

## مجوز

MIT — رجوع به [LICENSE](LICENSE).

</div>

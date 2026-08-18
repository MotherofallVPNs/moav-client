<div dir="rtl">

<div align="center">

# moav-client

**یک پروکسی محلی، جلوی همه‌ی پروتکل‌هایی که سرور MoaV شما صحبت می‌کند.**

[![Go](https://img.shields.io/badge/Go-1.25-06b6d4.svg?logo=go&logoColor=white)](https://go.dev) [![License: MIT](https://img.shields.io/badge/license-MIT-22c55e.svg)](LICENSE) [![Release](https://img.shields.io/github/v/release/MotherofallVPNs/moav-client?label=release&color=16a34a&logo=github&logoColor=white)](https://github.com/MotherofallVPNs/moav-client/releases/latest)

[![MoaV server](https://img.shields.io/badge/server-MoaV-ef4444.svg?logo=github&logoColor=white)](https://github.com/MotherofallVPNs/MoaV) [![Protocols](https://img.shields.io/badge/protocols-13%2B-8b5cf6.svg)](#protocols) [![Telegram](https://img.shields.io/badge/Telegram-motherofallvpns-2CA5E0.svg?logo=telegram)](https://t.me/motherofallvpns) [![X](https://img.shields.io/badge/X-@motherofallvpns-000000.svg?logo=x)](https://x.com/motherofallvpns)

🇬🇧 [English](README.md) &nbsp;·&nbsp; 🇮🇷 [فارسی](README-fa.md)

ساخته و نگهداری‌شده توسط جامعه‌ی **[MoaV](https://github.com/MotherofallVPNs)**.

</div>

---

<a id="why"></a>

## چرا moav-client

سرور MoaV عمداً چند پروتکل به شما می‌دهد: هیچ transport واحدی از دست هر سانسورچی جان به‌در نمی‌برد، پس وقتی یک مسیر انگشت‌نگاری (fingerprint) شد، به مسیر دیگری سوئیچ می‌کنید. اما یک گوشی یا لپ‌تاپ معمولاً *یک* پروتکل را در هر لحظه صحبت می‌کند، و انتخاب دستیِ مسیر زنده، وسط یک قطعی، دقیقاً بدترین زمان برای ویرایش کانفیگ است.

moav-client کل باندل را می‌گیرد و این کار را برایتان انجام می‌دهد. رمزنگاری واقعی هر پروتکل را به sing-box و مجموعه‌ای از sidecarهای اختیاری (MasterDNS، AmneziaWG، Psiphon، TrustTunnel، Tor) واگذار می‌کند، تأخیر هر endpoint را به‌صورت سرتاسری از داخل تونل خودش اندازه می‌گیرد، بار را روی مجموعه‌ی سالم پخش می‌کند و یک پروکسی محلی واحد SOCKS5 / HTTP CONNECT ارائه می‌دهد. مرورگر یا سیستم را روی همان یک آدرس تنظیم کنید؛ از هر endpointی که همین حالا سریع‌تر و زنده است عبور می‌کند. یک داشبورد React با ظاهری هماهنگ با پنل ادمین MoaV دید زنده‌ای از سلامت endpointها، پهنای‌باند هر پروتکل، ویرایش قوانین مسیریابی و لاگ زنده می‌دهد.

---

## فهرست مطالب

**لینک‌ها** &nbsp;·&nbsp; [سرور MoaV](https://github.com/MotherofallVPNs/MoaV) &nbsp;·&nbsp; [مستندات](docs/) &nbsp;·&nbsp; [تلگرام](https://t.me/motherofallvpns)

**شروع کنید** &nbsp;·&nbsp; [چرا moav-client](#why) &nbsp;·&nbsp; [نمونه‌ی زنده](#demo) &nbsp;·&nbsp; [شروع سریع](#quick-start) &nbsp;·&nbsp; [وارد کردن کانفیگ](#import)

**استفاده** &nbsp;·&nbsp; [پروتکل‌های پشتیبانی‌شده](#protocols) &nbsp;·&nbsp; [داشبورد وب](#dashboard) &nbsp;·&nbsp; [پیکربندی](#config) &nbsp;·&nbsp; [پلاگین‌ها](#plugins) &nbsp;·&nbsp; [CLI](#cli)

**زیر پوسته** &nbsp;·&nbsp; [REST API](#api) &nbsp;·&nbsp; [مستندات](#docs) &nbsp;·&nbsp; [توسعه](#development) &nbsp;·&nbsp; [جامعه](#community)

---

<a id="demo"></a>

## نمونه‌ی زنده

<div align="center">
<a href="https://github.com/MotherofallVPNs/moav-client"><img src="docs/assets/dashboard.gif" alt="داشبورد moav-client — هر endpoint را probe می‌کند و سریع‌ترین را مسیریابی می‌کند" width="90%"></a>
<br><sub><b>داشبورد</b> · سلامت زنده‌ی endpointها، پهنای‌باند هر پروتکل و مسیریابی با یک کلیک</sub>
</div>

---

<a id="quick-start"></a>

## شروع سریع

```bash
curl -fsSL https://raw.githubusercontent.com/MotherofallVPNs/moav-client/main/install.sh | bash
```

نصب‌کننده پیش‌نیازهای نصب‌نشده (docker، git، curl، python3) را **خودکار نصب می‌کند**، مخزن را clone می‌کند، اجازه می‌دهد sidecarها را از یک چک‌لیست انتخاب کنید (فقط ایمیج‌های انتخابی build می‌شوند)، `config.yaml` را می‌سازد، ایمیج‌ها را build می‌کند، استک را بالا می‌آورد، در صورت تمایل آن را روی شبکه‌ی محلی باز می‌کند و دستور سراسری `moavc` را نصب می‌کند. هم تعاملی (حتی وقتی با `bash` پایپ شود) و هم کاملاً headless کار می‌کند — به [docs/INSTALL.md](docs/INSTALL.md) نگاه کنید.

سپس استک را با **`moavc`** مدیریت کنید (نام کامل `moav-client` هم کار می‌کند):

```bash
moavc status                # وضعیت سرویس‌ها + سلامت + آدرس‌ها
moavc info                  # فقط آدرس‌های داشبورد / پروکسی / API
moavc logs -f proxy-core    # دنبال‌کردن لاگ‌ها
moavc probe                 # اجرای probe تأخیر
moavc sidecar add tor       # فعال‌سازی + build + اجرای یک sidecar
moavc expose lan            # سطح دسترسی: loopback | lan | public
moavc update [-b <branch>]  # pull (و در صورت نیاز تعویض شاخه) + rebuild
moavc uninstall [--wipe]    # حذف استک (--wipe کانفیگ/داده را هم پاک می‌کند)
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

اندازه‌ی واقعی ایمیج روی دیسک (amd64). هسته همیشه اجرا می‌شود؛ sidecarها اختیاری‌اند (با `--profile`). هر کانتینر در `docker-compose.yml` محدودیت حافظه و CPU دارد.

| سرویس | دیسک | RAM بی‌کار | سقف | پروفایل |
|---|---|---|---|---|
| proxy-core | ~۱۸ MB | ~۸ MB | 256m / 1.0 | همیشه |
| web-ui | ~۷۶ MB | ~۳ MB | 128m / 0.5 | همیشه |
| sing-box | ~۱۱۶ MB | ~۱۴ MB | 256m / 1.0 | همیشه |
| xray | ~۶۶ MB | ~۱۰ MB | 256m / 0.5 | همیشه (باینری رسمی XTLS، پین‌شده با `XRAY_VERSION`) |
| MasterDNS | ~۱۳۸ MB | — | 128m / 0.5 | `masterdns` |
| AmneziaWG | ~۱۴۹ MB | ~۴ MB | 256m / 0.5 | `amneziawg` |
| Psiphon | ~۱۷۶ MB | ~۶ MB | 256m / 0.5 | `psiphon` |
| TrustTunnel | ~۱۴۷ MB | ~۱۴ MB | 256m / 0.5 | `trusttunnel` |
| Tor | ~۸۶ MB | ~۶۸ MB | 256m / 0.5 | `tor` |

| مصرف | فقط هسته | استک کامل |
|---|---|---|
| دیسک (ایمیج‌های runtime) | ~۲۷۶ MB | ~۹۷۰ MB |
| دانلود نصب اولیه | ~۱۱۵ MB | ~۳۹۰ MB |
| RAM (بی‌کار) | ~۳۵ MB | ~۱۳۰ MB |

مرحله‌ی `[5/5]` نصب‌کننده پیش از build یک جدول تخمینی دانلود/دیسک برای هر مؤلفه نشان می‌دهد. یک build کامل حدود ۸ GB کش build هم می‌گذارد که با `docker builder prune` قابل پاک‌سازی است؛ به‌روزرسانی‌ها فقط لایه‌های تغییریافته را دانلود می‌کنند.

---

<a id="import"></a>

## وارد کردن کانفیگ

هر چیزی که moav-client مسیریابی می‌کند از یک باندل MoaV شروع می‌شود. سه راه برای بارگذاری آن وجود دارد؛ همه در نهایت به `config.yaml` می‌رسند و پس از آن از داشبورد قابل مدیریت‌اند.

### ۱. آدرس باندل `moav://` (پیشنهادی)

فرمت باندل فشرده‌ی MoaV همه‌ی پروتکل‌های یک سرور را در یک خط جمع می‌کند. یک `<defaultHost>` و اعتبارنامه‌های مشترک را یک‌بار حمل می‌کند و سپس یک رکورد `p=` برای هر پروتکل؛ به‌این‌ترتیب سروری با شش پروتکل که به‌صورت URIهای جدا ~۲ کیلوبایت بود، پس از base64 حدود ۶۴۰ بایت می‌شود:

```
moav://<name>@<host>?uuid=…&pw=…&pbk=…&sni_default=…&fp=chrome\
  &p=reality,443,sni=…,flow=xtls-rprx-vision\
  &p=vless-ws,443,host=…,path=…\
  &p=trojan,8443,sni=…\
  &p=hy2,443,obfs=salamander,obfs_pw=…#MoaV
```

(روی سیم یک خط است؛ اینجا فقط برای خوانایی شکسته شده). moav-client آن را به یک endpoint برای هر رکورد `p=` باز می‌کند، پس متعادل‌کننده، prober و داشبورد دقیقاً مثل URIهای جداگانه با آن‌ها رفتار می‌کنند. خط `moav://` را در `subscription.url` بگذارید (یا فایلی در `subscription.file`)، یا هنگام نصب paste کنید. گرامر کامل و کلیدهای هر پروتکل: **[docs/MOAV_BUNDLE.md](docs/MOAV_BUNDLE.md)**.

### ۲. اشتراک base64 / متنی

اشتراک کلاسیک V2Ray هم کار می‌کند. `subscription.url` را روی یک لینک `https://…` یا `subscription.file` را روی یک `subscription.txt` محلی تنظیم کنید؛ محتوا می‌تواند base64 یا متنی باشد، و می‌تواند باندل‌های `moav://` را با URIهای تک‌پروتکلی قدیمی (`vless://`، `trojan://`، `hysteria2://`، …) هر کدام در یک خط ترکیب کند. هر خط مستقل پارس می‌شود و بر اساس URI حذفِ تکراری می‌شود، پس یک باندل و URIهای پراکنده‌ی همان سرور بی‌تداخل کنار هم می‌مانند. فایل‌های `.conf` وایرگارد / AmneziaWG که در `subscription.wireguard_files` فهرست شوند، هرکدام یک endpoint می‌شوند.

### ۳. رهاکردن یک `.zip` سرور (چند سرور)

برای اجرای چند سرور MoaV در کنار هم، از تب **Configs** داشبورد (یا `POST /api/bundles`) استفاده کنید: `.zip` صادرشده‌ی یک سرور را رها کنید تا زیر `data/<name>/` استخراج شود و یک ورودی `subscription.sources` اضافه شود. فهرست، حذف و reload منابع از همان تب، بدون ویرایش دستی.

> اکثر افراد هرگز مستقیم به `config.yaml` دست نمی‌زنند. وارد کردن یک باندل و toggle کردن endpointها در داشبورد آن را برایتان می‌نویسد.

---

<a id="protocols"></a>

## پروتکل‌های پشتیبانی‌شده

پارسر، [فرمت باندل `moav://`](#import) و فرمت استاندارد اشتراک MoaV (URIهای سبک V2Ray با base64) به‌علاوه‌ی فایل‌های اختیاری `.conf` وایرگارد را می‌پذیرد.

| پروتکل | مسیر اتصال | توضیح |
|---|---|---|
| VLESS / Reality | خروجی sing-box | اثرانگشت utls، کلید عمومی و short-id ریالیتی |
| VLESS + WS + TLS (CDN) | خروجی sing-box | utls + ALPN + path / host |
| Trojan + TLS | خروجی sing-box | اثرانگشت uTLS، SNI |
| AnyTLS | خروجی sing-box | TLS + رمز عبور، اثرانگشت uTLS تصادفی، SNI، پرچم `insecure` |
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

> **پشتیبانی کلاینت‌ها از AnyTLS محدودتر از VLESS/Trojan است.** اینجا از طریق sing-box اتصال برقرار می‌شود، و همچنین توسط Hiddify، sing-box (SFA/SFI)، NekoBox، mihomo و Shadowrocket نسخه‌ی ‎2.2.65+‎ پشتیبانی می‌شود. کلاینت‌های قدیمی‌تر یا دیگر ممکن است آن را نفهمند.

---

<a id="dashboard"></a>

## داشبورد وب

| تب | کاری که می‌توانید انجام دهید |
|---|---|
| **Endpoints** | وضعیت و تأخیر زنده. روشن/خاموش‌کردن هرکدام (toggle برای sidecar کانتینر را هم متوقف/شروع می‌کند). ویرایش اولویت درجا. ردیف‌های غیرفعال نشان `DISABLED` می‌گیرند. |
| **Configs** | وارد کردن باندل سرور دیگر با رهاکردن فایل `.zip` — زیر `data/<name>/` استخراج و یک منبع اضافه می‌شود. فهرست/حذف منابع و reload. |
| **Analytics** | کارت‌های آپلود/دانلود هر پروتکل با نمودار ۲ دقیقه‌ای، نمودار سطحی هم‌پوشان، و جدول هر endpoint با شمارش dial/خطا/failover. |
| **Plugins** | فهرست، مرتب‌سازی، ویرایش و حذف قوانین مسیریابی. افزودن از کاتالوگ آماده. تغییرات بدون restart اعمال می‌شوند. |
| **Settings** | در پنل‌های جداگانه: استراتژی متعادل‌سازی + probe، **سطح دسترسی شبکه** (loopback / lan / public با احراز هویت اختیاری SOCKS5 و داشبورد)، آدرس‌ها و URLها، کلید SNI-spoof، پشتیبان‌گیری/بازیابی، و در پایین یک ویرایشگر **پیشرفته‌ی** `config.yaml` به‌صورت تاشو. |
| **Debug** | لاگ زنده (بافرهای حلقوی per-level، ~۸۰۰ رویداد برای هر سطح). فیلتر، pause/autoscroll/copy/clear. به‌علاوه جدول flow هر اتصال. |
| **Diagnostics** | بررسی اتصال از خود proxy-core: TCP، DNS یا traceroute — اختیاراً *از داخل* تونل یک endpoint مشخص. |

![تب Endpoints — وضعیت زنده، تأخیر، و کلید هر endpoint](docs/assets/dashboard.png)

<table>
  <tr>
    <td width="50%"><img src="docs/assets/analytics.png" alt="Analytics — پهنای باند هر پروتکل"><br><sub><b>Analytics</b> — پهنای باند زنده‌ی هر پروتکل</sub></td>
    <td width="50%"><img src="docs/assets/plugins.png" alt="Plugins — قواعد مسیریابی"><br><sub><b>Plugins</b> — قواعد مسیریابی (اولین تطابق برنده)</sub></td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/assets/sources.png" alt="Configs — منابع اشتراک"><br><sub><b>Configs</b> — منابع باندل چندسروره</sub></td>
    <td width="50%"><img src="docs/assets/settings.png" alt="Settings — استراتژی، سطح دسترسی، SNI-spoof"><br><sub><b>Settings</b> — استراتژی، سطح دسترسی، آدرس‌ها</sub></td>
  </tr>
</table>

---

<a id="config"></a>

## پیکربندی

فایل `config.yaml` همه‌چیز را کنترل می‌کند؛ sing-box و xray به‌صورت پیش‌فرض روشن‌اند (رمزنگاری پروتکل‌ها). فایل کامل و کامنت‌گذاری‌شده‌ی [`config.yaml.example`](config.yaml.example) مرجع است — کپی و ویرایش کنید. بخش‌های کلیدی:

- `proxy` — پورت‌های listener + احراز هویت اختیاری SOCKS5
- `subscription` — `file` / `url` / `wireguard_files` یا چند `sources`
- `load_balancing.strategy` — `latency` | `priority` | `weighted`
- `plugins` — `torrent_block`، `block_direct`، `routing_rules`
- `singbox` / `xray` / `sni_spoof` — sidecarهای dialer (پیش‌فرض روشن)
- `sidecars` — `masterdns` / `amneziawg` / `psiphon` / `trusttunnel` / `tor`

اکثر کاربران هرگز `config.yaml` را دستی ویرایش نمی‌کنند — وارد کردن باندل (تب Configs) و toggle کردن در داشبورد آن را برایتان می‌نویسد، یا از ویرایشگر **پیشرفته‌ی** تاشو در پایین تب Settings استفاده کنید.

**نسخه‌ها** در `.env` پین می‌شوند: `XRAY_VERSION` (تگ نسخه‌ی رسمی XTLS)، `IMAGE_SINGBOX` / `IMAGE_TOR` / `IMAGE_CADDY` (رفرنس ایمیج‌های pull‌شده) و `MOAV_VERSION`. نسخه‌ی کلاینت در فایل `VERSION` است. به [`.env.example`](.env.example) نگاه کنید.

---

<a id="plugins"></a>

## پلاگین‌ها

زنجیره‌ی قوانین «اولین تطابق برنده». هم `config.yaml` و هم تب Plugins داشبورد یک موتور را تغذیه می‌کنند؛ تغییرات داشبورد بدون restart اعمال می‌شوند.

انواع تطابق: `domain`، `domain_suffix`، `domain_keyword`، `ip_cidr`، `geoip`، `port`، `protocol`.
عمل‌ها: `proxy` (پیش‌فرض)، `direct` (دور زدن)، `block` (انداختن).

### مسدودسازی مستقیم (کلید قطع)

`plugins.block_direct: true` یک کلید قطع نشت است: هر اتصالی که قرار است مستقیم برود انداخته می‌شود — هم یک قانون `direct` و هم fallback آخرین‌چاره‌ی متعادل‌کننده وقتی همه‌ی endpointها down هستند. پیش‌فرض `false`. وقتی روشن است قوانین `direct` مثل `lan-direct` را هم می‌شکند.

### GeoIP

قوانین `geoip:<cc>` یک IP مقصد را با لیست CIDR در `geoip/<cc>.txt` تطبیق می‌دهند (لیست ایران در مخزن هست و هفتگی توسط CI به‌روز می‌شود). تطابق فقط روی **IP** است — میزبان‌های نام‌دار resolve نمی‌شوند. به [geoip/README.md](geoip/README.md) نگاه کنید.

---

<a id="cli"></a>

## CLI

دو ابزار خط‌فرمان هم‌نام‌اند. **رپر مدیریتی** — در `PATH` با نام **`moavc`** (و `moav-client`) نصب می‌شود — استک داکر را روزمره مدیریت می‌کند:

```
moavc <command>

  up | down | restart        شروع / توقف / rebuild استک
  status                     وضعیت سرویس‌ها + سلامت + آدرس‌ها
  info                       فقط آدرس‌های داشبورد / پروکسی / API
  logs [-f] [service]        دنبال‌کردن لاگ‌ها
  probe | stats              probe endpointها / نمایش شمارنده‌ها
  sidecar add|remove|list    مدیریت sidecarهای اختیاری
  install                    اجرای دوباره‌ی ویزارد نصب
  expose <loopback|lan|public>   تغییر سطح دسترسی شبکه
  update [-b <branch>]       pull (و در صورت نیاز تعویض شاخه) + rebuild
  uninstall [--wipe]         حذف استک (--wipe کانفیگ/داده را پاک می‌کند)
  open | version
```

**باینری proxy-core** (داخل کانتینر، `FROM scratch`) خودِ پروکسی را اجرا می‌کند و زیرفرمان‌های خودش را دارد — `serve` (پیش‌فرض)، `probe`، `list`، `fetch-sub <url>`، `healthcheck`، `version` — همگی با `--config <path>`.

---

<a id="api"></a>

## REST API

سرور API روی `proxy.api_port` (پیش‌فرض ۸۰۸۸) گوش می‌دهد. پاسخ‌ها JSON‌اند؛ همه‌ی مسیرها CORS باز برای داشبورد دارند.

| متد | مسیر | توضیح |
|---|---|---|
| GET | `/api/healthz` | liveness — `{"ok":true}` |
| GET | `/api/version` | نسخه‌ی build + commit، uptime، IP و کشور خروجی نصب/پروکسی |
| GET | `/api/endpoints` | استخر فعلی با وضعیت / تأخیر / کانفیگ |
| PATCH | `/api/endpoints/<id>` | `{enabled, priority}` — endpoint را patch می‌کند و برای sidecar کانتینر داکر را هم متوقف/شروع می‌کند |
| POST | `/api/probe` | اجرای فوری یک پاس probe |
| GET | `/api/stats` | شمارنده‌های هر endpoint (dial، خطا، failover، bytes_up/down، last_error) + استراتژی فعال |
| POST | `/api/strategy` | تعویض استراتژی متعادل‌سازی در زمان اجرا |
| GET | `/api/flows` | رکوردهای اخیر flow هر اتصال (مقصد، endpoint، بایت، نتیجه) |
| GET/PUT | `/api/plugins` | دریافت `{rules, templates}` / جایگزینی اتمیک فهرست قوانین |
| GET | `/api/logs` | بافر حلقوی لاگ؛ فیلتر اختیاری `?level=` |
| GET/POST | `/api/config` | دریافت / نوشتنِ اتمیک `config.yaml` روی دیسک |
| POST | `/api/bundles` | آپلود multipart `.zip` → استخراج زیر `data/<name>/` + ثبت یک منبع |
| GET | `/api/sources` | فهرست منابع اشتراک پیکربندی‌شده |
| DELETE | `/api/sources/<name>` | حذف یک منبع از `config.yaml` |
| POST | `/api/sources/reload` | ری‌استارت خودِ proxy-core برای بارگذاری دوباره‌ی وضعیت اشتراک |
| GET/PUT | `/api/exposure` | سیاست bind (loopback / lan / public) + احراز هویت SOCKS5 → `.env` |
| GET/PUT | `/api/snispoof` | فعال‌سازی SNI-spoof + SNI/uTLS جعلی پیش‌فرض |
| GET | `/api/diag` | `?type=tcp\|dns\|trace&target=…&via=<endpoint>` بررسی اتصال |
| GET | `/api/backup` | دانلود tar.gz از کانفیگ + منابع |
| POST | `/api/restore` | بازیابی از یک backup آپلودشده |
| WS | `/api/ws` | مالتی‌پلکس فریم‌های `endpoints` و `log` |

---

<a id="docs"></a>

## مستندات

- [docs/INSTALL.md](docs/INSTALL.md) — نصب headless / با فلگ، سطح دسترسی شبکه، به‌روزرسانی و حذف
- [docs/PLUGINS.md](docs/PLUGINS.md) — قوانین مسیریابی، کلید قطع، geoip و کاتالوگ کامل قالب‌ها
- [docs/SIDECARS.md](docs/SIDECARS.md) — TrustTunnel، Psiphon، Tor، MasterDNS، AmneziaWG
- [docs/SNI_SPOOFING.md](docs/SNI_SPOOFING.md) — sidecar اختیاری SNI-spoofing
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — پل sing-box، متعادل‌کننده/failover، prober، کنترل داکر
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) — مشکلات رایج
- [docs/MOAV_BUNDLE.md](docs/MOAV_BUNDLE.md) — فرمت باندل `moav://`: گرامر کامل، کلیدهای مشترک در برابر هر پروتکل، و منطق حذف تکراری
- [CLAUDE.md](CLAUDE.md) — راهنمای عامل LLM

---

<a id="development"></a>

## توسعه

### اجرای محلی proxy-core (بدون داکر)

```bash
cd proxy-core
go run . --config ../config.yaml
```

### اجرای محلی web-ui

```bash
cd web-ui
npm install
npm run dev
# سرور توسعه‌ی Vite روی http://localhost:5173
# هدف پیش‌فرض API: http://localhost:8088 (با VITE_API_URL بازنویسی کنید)
```

### تست‌ها

```bash
cd proxy-core && go test ./...
cd web-ui && npm run build  # type-check + بسته‌بندی
```

---

<a id="community"></a>

## جامعه

**سلام کنید.** [تلگرام](https://t.me/motherofallvpns) برای پرسش، کمک و اعلان انتشارها · [ایکس](https://x.com/motherofallvpns) · [GitHub Issues](https://github.com/MotherofallVPNs/moav-client/issues) برای باگ و درخواست ویژگی · [سرور MoaV](https://github.com/MotherofallVPNs/MoaV) · [moav.sh](https://moav.sh).

---

## مجوز

MIT — رجوع به [LICENSE](LICENSE).

</div>

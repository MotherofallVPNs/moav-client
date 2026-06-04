<div dir="rtl">

# moav-client

![Go](https://img.shields.io/badge/Go-1.25-blue?logo=go) ![License: MIT](https://img.shields.io/badge/License-MIT-green)

**[English](README.md)** | فارسی

کلاینتی برای سرورهای **[MoaV — مادر همه‌ی VPNها](https://github.com/shayanb/MoaV)**. یک باندل اشتراک چندپروتکلی را می‌خواند، رمزنگاری واقعی هر پروتکل را به sing-box و مجموعه‌ای از sidecarهای اختیاری (MasterDNS، AmneziaWG، Psiphon، TrustTunnel، Tor) واگذار می‌کند، تأخیر هر endpoint را به‌صورت سرتاسری از داخل تونل آن اندازه می‌گیرد، بار را روی مجموعه‌ی سالم پخش می‌کند، و یک پروکسی محلی واحد SOCKS5 / HTTP CONNECT ارائه می‌دهد. یک داشبورد React با ظاهری هماهنگ با پنل ادمین MoaV، دید زنده‌ای از سلامت endpointها، پهنای‌باند هر پروتکل، ویرایش قوانین پلاگین و لاگ زنده‌ی دیباگ می‌دهد.

---

## شروع سریع

**نصب با یک دستور** (توصیه‌شده):

```bash
curl -fsSL https://raw.githubusercontent.com/MotherofallVPNs/moav-client/main/install.sh | bash
```

نصب‌کننده پیش‌نیازها را بررسی می‌کند، مخزن را clone می‌کند، شما را برای فعال‌سازی sidecarها راهنمایی می‌کند (با تخمین حجم دیسک برای هر انتخاب)، فایل `config.yaml` را می‌سازد، ایمیج‌های داکر را build می‌کند و استک را بالا می‌آورد. هم به‌صورت تعاملی (با TTY) و هم کاملاً بدون تعامل (با متغیرهای محیطی / فلگ‌ها) کار می‌کند.

**نمونه‌های بدون تعامل:**

```bash
# همه‌چیز از طریق متغیرهای محیطی (بدون پرسش).
MOAV_HEADLESS=1 \
MOAV_DIR=/opt/moav-client \
MOAV_SUBSCRIPTION=/etc/moav/subscription.txt \
MOAV_SIDECARS=masterdns,psiphon \
  bash -c "$(curl -fsSL https://raw.githubusercontent.com/MotherofallVPNs/moav-client/main/install.sh)"

# یا با فلگ‌ها پس از clone محلی.
git clone https://github.com/MotherofallVPNs/moav-client && cd moav-client
./install.sh --headless --dir /opt/moav-client --sidecars masterdns,psiphon
```

پس از نصب، `./moav-client` یک wrapper سبک روی docker-compose است:

```bash
./moav-client status                # docker compose ps
./moav-client logs proxy-core       # دنبال‌کردن لاگ‌ها
./moav-client probe                 # اجرای probe از طریق API
./moav-client stats                 # خروجی JSON از /api/stats
./moav-client sidecar add tor       # فعال‌سازی + build + اجرای sidecar تور
./moav-client sidecar remove psiphon
./moav-client update                # git pull + rebuild + restart
```

آدرس‌های ارائه‌شده:

| چیست | آدرس |
|---|---|
| داشبورد | http://localhost:3001 |
| پروکسی SOCKS5 | `socks5h://localhost:1080` |
| پروکسی HTTP CONNECT | http://localhost:8081 |
| REST + WS API | http://localhost:8088 |

مرورگر یا پروکسی سیستم را روی `socks5h://localhost:1080` تنظیم کنید. هر اتصال از سالم‌ترین endpoint سرور MoaV عبور می‌کند.

### مصرف منابع به ازای هر کانتینر

حجم ایمیج روی دیسک (اندازه‌گیری‌شده روی Ubuntu 24.04 / amd64). ستون «شبکه» مقداری است که نصب اولیه دانلود می‌کند: ایمیج‌های pull‌شده لایه‌های فشرده را دانلود می‌کنند؛ ایمیج‌های build‌شده به‌صورت محلی کامپایل می‌شوند ولی یک ایمیج پایه (golang / debian / node) می‌کشند — آن لایه‌های پایه میان کانتینرهای build‌شده مشترک‌اند، پس مجموع واقعی بسیار کمتر از جمع تک‌تک است.

| سرویس | حجم ایمیج | شبکه (اولین اجرا) | کِی بالا می‌آید |
|---|---|---|---|
| **proxy-core** | ~۱۸ MB | build محلی (Go روی scratch؛ پایه golang-alpine) | همیشه |
| **web-ui** | ~۷۵ MB | build محلی (Vite → nginx:alpine، پایه ~۹۴ MB) | همیشه |
| **sing-box** | ~۱۱۶ MB | ~۵۰ MB دانلود (ghcr.io/sagernet/sing-box) | همیشه |
| **xray** | ~۱۰۴ MB | ~۴۵ MB دانلود (teddysun/xray — xhttp/splithttp + MTProxy) | همیشه |
| MasterDNS | ~۱۳۸ MB | build محلی (golang + debian) | `--profile masterdns` |
| AmneziaWG | ~۱۴۹ MB | build محلی (golang + debian) | `--profile amneziawg` |
| Psiphon | ~۱۷۶ MB | build محلی (clone از psiphon-tunnel-core) | `--profile psiphon` |
| TrustTunnel | ~۸۵ MB | build محلی (placeholder) | `--profile trusttunnel` |
| Tor | ~۸۵ MB | ~۳۰ MB دانلود (peterdavehello/tor-socks-proxy) | `--profile tor` |

استک پایه (همیشه روشن): حدود **۳۱۳ MB** روی دیسک. استک کامل با همه‌ی sidecarها: حدود **۹۴۵ MB**، به‌علاوه‌ی حدود ۵۰۰ MB کش build در اولین اجرا. یک نصب تازه تقریباً ۶۰۰ تا ۸۰۰ MB لایه‌ی پایه و runtime دانلود می‌کند که بیشترش میان sidecarهای build‌شده مشترک است.

حافظه (RAM): استک پایه در حالت بی‌کاری حدود ~۱۵۰ MB مصرف می‌کند؛ هر sidecar ۲۰ تا ۸۰ MB اضافه می‌کند. ۱ GB برای حالت پایه کافی است؛ ۲ GB اگر چند sidecar را فعال کنید.

---

## پروتکل‌های پشتیبانی‌شده

پارسر باندل، فرمت استاندارد اشتراک MoaV (URIهای سبک V2Ray با کدگذاری base64) به‌علاوه‌ی فایل‌های اختیاری `.conf` وایرگارد را می‌پذیرد.

| پروتکل | مسیر اتصال | توضیح |
|---|---|---|
| VLESS / Reality | خروجی sing-box | اثرانگشت utls، کلید عمومی و short-id ریالیتی |
| VLESS + WS + TLS (CDN) | خروجی sing-box | utls + ALPN + path / host |
| Trojan + TLS | خروجی sing-box | اثرانگشت uTLS، SNI |
| Shadowsocks-2022 | خروجی sing-box | 2022-blake3-aes-128-gcm |
| Hysteria 2 (+obfs) | خروجی sing-box | مبهم‌سازی salamander |
| VLESS + XHTTP + Reality | خروجی xray | xhttp فقط در Xray است؛ sidecar مربوطه آن را روی ‎11800+‎ مدیریت می‌کند |
| WireGuard | بلوک `endpoints[]` در sing-box | از `wireguard.conf` پارس می‌شود |
| AmneziaWG | sidecar `amneziawg` | `amneziawg-go` فضای‌کاربر + `awg setconf` + microsocks روی مسیر پیش‌فرض awg0 |
| TrustTunnel | sidecar `trusttunnel` | placeholder — برای فعال‌سازی باینری کلاینت بالادست را mount کنید |
| MasterDNS | sidecar `masterdns` | باینری بالادست از ریلیزهای `masterking32/MasterDnsVPN` |
| Psiphon | sidecar `psiphon` | از سورس `Psiphon-Labs/psiphon-tunnel-core` build می‌شود؛ با کانفیگ توکار خودش بدون نیاز به اعتبارنامه تونل می‌زند |
| Tor | sidecar `tor` | `peterdavehello/tor-socks-proxy` — SOCKS5 روی ‎:9150‎، بدون اعتبارنامه |

هر sidecar ورودی SOCKS5 خودش را روی شبکه‌ی داکری `moav-net` ارائه می‌دهد؛ moav-client هرکدام را به‌عنوان یک عضو در استخر متعادل‌کننده‌ی بار می‌بیند.

---

## داشبورد وب

| تب | کاری که می‌توانید انجام دهید |
|---|---|
| **Endpoints** | وضعیت و تأخیر زنده. روشن/خاموش‌کردن هرکدام (toggle برای sidecarها کانتینر داکر را هم متوقف/شروع می‌کند). ویرایش اولویت به‌صورت درجا. ردیف‌های غیرفعال نشان `DISABLED` می‌گیرند نه وضعیت کهنه. |
| **Sources** | وارد کردن باندل یک سرور MoaV دیگر با رهاکردن فایل `.zip` آن — زیر `data/<name>/` استخراج و یک ورودی `subscription.sources` اضافه می‌شود. فهرست/حذف منابع و اجرای reload. |
| **Analytics** | کارت‌های آپلود/دانلود هر پروتکل با نمودارهای کوچک ۲ دقیقه‌ای، نمودار سطحی هم‌پوشان از پهنای‌باند همه‌ی پروتکل‌ها، و جدول هر endpoint با شمارش dial / خطا / failover و آخرین خطا. |
| **Plugins** | فهرست، مرتب‌سازی، ویرایش، فعال/غیرفعال و حذف قوانین مسیریابی. افزودن از کاتالوگ آماده — همه به‌صورت پیش‌فرض غیرفعال. تغییرات بدون restart اعمال می‌شوند. |
| **Settings** | تغییر زنده‌ی استراتژی متعادل‌سازی بار (latency / priority / weighted)، دکمه‌ی «probe همه‌ی endpointها»، **سطح دسترسی شبکه** (loopback / LAN / public با احراز هویت اختیاری SOCKS5، نوشته‌شده در `.env`)، کلید SNI-spoof و پشتیبان‌گیری/بازیابی کانفیگ. |
| **Debug** | لاگ زنده (بافرهای حلقوی جداگانه برای هر سطح، حدود ۸۰۰ رویداد برای هرکدام از info / warn / error تا هشدارها زیر انبوه info گم نشوند). فیلتر سطح و متن، pause / autoscroll / copy / clear. به‌علاوه جدول flow هر اتصال. |
| **Diagnostics** | اجرای بررسی اتصال از خود proxy-core: TCP، DNS یا traceroute مبتنی بر TCP-TTL — به‌صورت اختیاری *از داخل* تونل یک endpoint مشخص، تا «روتر من به این میزبان نمی‌رسد» را از «تونل این endpoint قطع است» تشخیص دهید. |
| **Config** | بارگذاری زنده‌ی `config.yaml` از دیسک. ویرایش و ذخیره (نوشتن اتمیک). برای تغییرات ساختاری پیام «proxy-core را restart کنید» نمایش داده می‌شود. |

دکمه‌ی `↻ Refresh` در نوار بالا همه‌ی تب‌ها را در جا تازه می‌کند؛ نشان سلامت کنار آن `سالم/کل` را نشان می‌دهد.

---

## مرجع پیکربندی

فایل `config.yaml` همه‌چیز را کنترل می‌کند. مقادیر پیش‌فرض در `config.Defaults()` داخل `proxy-core/config/config.go` تنظیم شده‌اند. نکته‌ی مهم: **`singbox.enabled` به‌صورت پیش‌فرض روشن است** — رمزنگاری همه‌ی پروتکل‌ها توسط sing-box انجام می‌شود، پس بدون آن هیچ endpointی قابل اتصال نیست. در اجرای خارج از داکر، `dial_host` را به `127.0.0.1` تغییر دهید.

برای فهرست کامل کلیدها به `config.yaml.example` در ریشه‌ی مخزن نگاه کنید.

---

## محدودیت‌های شناخته‌شده

این موارد باگ moav-client نیستند ولی تا وقتی اقدامی نکنید به‌صورت ردیف قرمز در داشبورد دیده می‌شوند:

- **TrustTunnel کلاینت عمومی لینوکس ندارد.** Dockerfile این sidecar یک باینری در مسیر `/usr/local/bin/trusttunnel-client` به‌علاوه‌ی `client.toml` می‌پذیرد. تا وقتی بالادست منتشرش کند، این مورد خطا می‌ماند.
- **کانتینر Tor ممکن است `unhealthy` نشان دهد ولی کار کند.** ایمیج `peterdavehello/tor-socks-proxy` healthcheck خودش را دارد که یک آدرس `.onion` فیسبوک را از داخل مدار Tor می‌کشد — سخت‌گیرانه و روی برخی شبکه‌ها کند/مسدود. پروکسی SOCKS5 روی `:9150` صرف‌نظر از آن کار می‌کند؛ probe داخل داشبورد سیگنال معتبر است.
- **اعتبار کلید Reality سمت سرور است.** اگر `pbk` / `sid` باندل دیگر با کلید خصوصی سرور هماهنگ نباشد، خطای `connection: EOF` می‌بینید — حلقه‌ی failability خودکار از کنارش رد می‌شود؛ اگر همه‌ی endpointهای Reality هم‌زمان خراب باشند، از اپراتور بخواهید کلیدها را بچرخاند. رجوع به [shayanb/MoaV#115](https://github.com/shayanb/MoaV/issues/115).
- **AmneziaWG به دسترسی‌های کانتینری نیاز دارد.** زنجیره‌ی `amneziawg-go` + `awg setconf` + microsocks به `cap_add: NET_ADMIN` و `/dev/net/tun` نیاز دارد (در `docker-compose.yml` تنظیم شده). روی میزبان‌های به‌شدت سخت‌گیر ممکن است پروفایل seccomp آزادتری لازم باشد.

---

## CLI

باینری `moav-client` به‌صورت تک‌فایل عرضه می‌شود. زیردستورها:

```
moav-client [command] [flags]

Commands:
  serve       اجرای پروکسی + API (پیش‌فرض اگر دستوری داده نشود)
  probe       probe یک‌باره‌ی تأخیر همه‌ی endpointها
  list        فهرست endpointها بدون probe
  fetch-sub   گرفتن و پارس یک URL اشتراک
  healthcheck بررسی API محلی و خروج با کد 0/1 (healthcheck کانتینر؛ روی ایمیج scratch هم کار می‌کند)
  version     چاپ نسخه
  help        چاپ راهنما

Global flags:
  --config    مسیر config.yaml (پیش‌فرض: config.yaml)
```

---

## معماری داخلی

برای جزئیات پیاده‌سازی (پل sing-box، failover متعادل‌کننده، معناشناسی probe آگاه از تونل، تولید کانفیگ sidecar، کنترل داکر) به **[docs/INTERNALS.md](docs/INTERNALS.md)** نگاه کنید. راهنمای عامل LLM هم در **[CLAUDE.md](CLAUDE.md)** هست.

---

## مجوز

MIT — رجوع به [LICENSE](LICENSE).

</div>

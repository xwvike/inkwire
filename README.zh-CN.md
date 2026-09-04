# Inkwire

使用 HTML、CSS 和 SVG 编写固定尺寸电子纸页面的渲染器。

| | Gicisky | EPD-nRF5 |
|---|---|---|
| 固件 | 出厂 | nRF51/nRF52 [替换固件](https://github.com/tsl0922/EPD-nRF5) |
| 名称 | `PICKSMART`、`NEMR<mac8>` | `NRF_EPD_<mac4>` |
| 型号来自 | 广播 | 连接 |
| 尺寸 | 212x104 … 960x640 | 400x300 … 880x528 |
| 颜色 | BW、BWR、BWRY | BW、BWR |

Markup 遵循 Web 布局规则，实现面向电子纸面板的 CSS 子集。支持范围和渲染差异见
[MARKUP.zh-CN.md](MARKUP.zh-CN.md)。

## CLI

| 命令 | 用途 |
|---|---|
| `inkwire scan [-timeout 15s]` | 列出当前可扫描的标签 |
| `inkwire render (-size WxH \| -panel FAMILY:ID) [-o out.png] [-asset SRC=FILE] <page.html>` | 渲染为 PNG |
| `inkwire compile [-o scene.json] [-asset SRC=FILE] <page.html>` | 输出内部场景 JSON |
| `inkwire measure (-size WxH \| -panel FAMILY:ID) [-json] [-asset SRC=FILE] <page.html>` | 输出节点布局 |
| `inkwire push -device NAME [-family gicisky\|nrfepd] [-settle 30s] [-asset SRC=FILE] <page.html>` | 渲染并写入 |
| `inkwire mode -device NAME [-mode picture\|calendar\|clock] [-week-start sunday\|monday] [-settle 30s]` | EPD-nRF5 时钟/日历 |
| `inkwire serve [-listen ADDR] [-assets DIR]` | 启动 HTTP 服务 |
| `inkwire help` | `-h`、`--help` |
| `inkwire version` | `-v`、`--version` |

### scan

列出当前可扫描的标签。输出的名称或地址可用于 `-device`。

```bash
inkwire scan -timeout 10s
```

```
ADDRESS                                NAME           RSSI  BATT  FAMILY   MODEL           SIZE      PALETTE
FF:FF:92:94:38:61                      NEMR92943861    -50  3.0V  gicisky  EPD 2.9" BWR    296x128   BWR
C1:57:DD:3F:C1:F8                      NRF_EPD_C1F8    -62     -  nrfepd   ask on connect            not advertised
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-timeout` | `15s` | 扫描窗口时长 |

Gicisky 通过厂商数据 `0x5053` 识别，EPD-nRF5 通过 service UUID 或名称前缀识别。

无扫描结果时退出码为 1。

### render

将页面渲染为 PNG。

```bash
inkwire render -size 296x128 -o preview.png page.html
```

```
wrote preview.png (296x128)
```

| 参数 | 要求 | 说明 |
|---|---|---|
| `-o` | 页面同名的 `.png` | PNG 输出路径 |
| `-size` | `-size`、`-panel` 二选一且必填 | 按任意 `WxH` 视口排版 |
| `-panel` | `-size`、`-panel` 二选一且必填 | 按指定面板的完整尺寸排版并检查墨水 |
| `-asset` | 未设置 | 注入本地资源，格式为 `SRC=FILE`，可重复 |

`-size` 与 `-panel` 互斥。`-size` 是完整的输出视口，可大于或小于任意实际面板；
`-panel` 使用面板表中的完整尺寸和可用墨水。

```
$ inkwire render page.html
render needs -size WxH or -panel family:id
```

`-size` 指定完整的排版和输出尺寸，不读取面板表。

```bash
inkwire render -size 400x300 page.html
```

`-panel` 按面板型号获取完整尺寸和可用墨水。

```bash
inkwire render -panel gicisky:0x0033 page.html
inkwire render -panel nrfepd:UC8176_420_BWR page.html
```

```
wrote page.png (296x128)
wrote page.png (400x300)
```

报告面板不支持的内容：

```bash
inkwire render -panel gicisky:0x0028 page.html
```

```
wrote page.png (296x128)
BW panel cannot show red ink at (10,10)
```

即使面板拒绝，仍输出预览图。面板或尺寸错误时退出码为 2，排版失败时为 1。

页面结构和 CSS 支持范围见 [MARKUP.zh-CN.md](MARKUP.zh-CN.md)。

### measure

输出每个节点的布局框；不渲染。

```bash
inkwire measure -size 120x40 page.html
```

```
row         0,0    120x40
    text    0,11    40x17   wants 56x17
    text   44,11    76x17   wants 35x17

warning root.children[0] [text-clipped]: "LAST REF" does not fit 40x17: 16 pixels along the line cut off
```

| 参数 | 要求 | 含义 |
|---|---|---|
| `-size` | `-size`、`-panel` 二选一且必填 | 按任意 `WxH` 视口排版 |
| `-panel` | `-size`、`-panel` 二选一且必填 | 按指定面板的完整尺寸排版 |
| `-json` | 关 | 输出 JSON 而不是树 |
| `-asset` | 未设置 | 注入本地资源，格式为 `SRC=FILE`，可重复 |

`wants` 仅在节点期望尺寸与实际布局框不同时显示。布局框不足不一定报错，
`text-clipped` 表示内容已被裁切。

### push

渲染并写入标签。

```bash
inkwire push -device NEMR92943861 page.html
```

```
writing to NEMR92943861 (e2ada7d1-187a-caea-21e4-d895f8240b62, gicisky)
pushing 9472 bytes (EPD 2.9" BWR 296x128 BWR)
tag requested 244 byte messages -> 240 byte blocks
upload complete, tag is refreshing
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-device` | 必填 | 广播名（`NAME` 列）或 BLE 地址 |
| `-family` | `auto` | 指定设备家族，跳过自动识别 |
| `-settle` | `30s` | 仅 EPD-nRF5：`REFRESH` 后保持连接的时长，`0` 表示立即断开，见[写入间隔](#写入间隔) |
| `-asset` | 未设置 | 注入本地资源，格式为 `SRC=FILE`，可重复 |

页面尺寸与面板不符时按面板重排，并报告 `size-mismatch`。面板不支持的墨水改画为黑色，
每种墨水报告一条 `unsupported-ink`。

`push` 从已连接设备获取目标尺寸和可用墨水，不接受 `-size` 或 `-panel`。

渲染警告会写入日志，见[警告](#警告)。

### mode

设置 EPD-nRF5 标签的时钟或日历，并同步时间。

```bash
inkwire mode -device NRF_EPD_C1F8 -mode clock
```

```
connecting to NRF_EPD_C1F8 (C1:57:DD:3F:C1:F8)
firmware version 0x76
panel is UC8176_420_BWR 400x300 BWR
setting the clock to 2026-08-21 16:04:28 +0800 and the mode to clock
staying connected 30s while the panel refreshes; disconnecting now would cancel it
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-device` | 必填 | 广播名或 BLE 地址 |
| `-mode` | `calendar` | `picture`、`calendar` 或 `clock` |
| `-week-start` | 保留标签原值 | 日历每周第一列：`sunday` 或 `monday` |
| `-settle` | `30s` | 重绘期间保持连接的时长，`0` 表示立即断开 |

仅 EPD-nRF5 支持该命令。指定 Gicisky 标签时拒绝：

```
NEMR92943861 (e2ada7d1-…, gicisky) is a gicisky tag, but the family asked for is nrfepd
```

`push` 将标签切回 `picture` 模式，详见[时钟与日历](#时钟与日历)。

### serve

启动 HTTP 服务。

```bash
inkwire serve -listen 127.0.0.1:8080
```

```
listening on http://127.0.0.1:8080
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-listen` | `127.0.0.1:8080` | 监听地址，仅限回环 |
| `-assets` | `.` | JSON 场景相对资源的根目录 |

无鉴权，写入请求直达硬件；仅允许绑定回环地址。JSON 场景的相对资源从 `-assets` 指定的目录
读取；multipart 页面必须上传本地资源。路由和错误码见[创建 HTTP 服务](#创建-http-服务)。

## Gicisky

| | |
|---|---|
| 面板 | 从广播识别；212x104 … 960x640，BW/BWR/BWRY |
| 方向 | 横屏使用 profile 尺寸，竖屏交换宽高 |
| Payload | 按型号选择平面、变换与压缩 |
| GATT | 服务 `FEF0`，控制 `FEF1`，数据 `FEF2` |

厂商数据（公司 ID `0x5053`）：

```
byte   0        1        2   3        4
       id low   battery  firmware     id high
```

型号 id = `(data[4] << 8) | data[0]` 的低 14 位。名称不含型号，写入前需读取厂商数据。

### 型号

从广播读取，无需连接。

| ID | 名称 | 尺寸 | 颜色 | 变换与打包 |
|---|---|---|---|---|
| `0x000B` | EPD 2.1" BWR | 212x104 | BWR | 旋转 270°，X 镜像 |
| `0x0028` | EPD 2.9" BW | 296x128 | BW | 旋转 90° |
| `0x002E` | EPD 2.9" BWRY | 296x128 | BWRY | 旋转 90°，四色两位打包 |
| **`0x0033`** | **EPD 2.9" BWR** | **296x128** | **BWR** | 旋转 90° |
| `0x004B` | EPD 4.2" BWR | 400x300 | BWR | |
| `0x004E` | EPD 4.2" BWRY | 400x300 | BWRY | 四色两位打包 |
| `0x008B` | EPD 10.2" BWR | 960x640 | BWR | QuickLZ |
| `0x00A0` | TFT 2.1" BW | 250x132 | BW | TFT，旋转 90°，X 镜像 |
| `0x010B` | EPD 2.1" BWR | 250x128 | BWR | 旋转 270°，X 镜像 |
| `0x012B` | EPD 7.5" BWR | 800x480 | BWR | Y 镜像，反色，QuickLZ |
| `0x022B` | EPD 3.7" BWR | 240x416 | BWR | 旋转 180°，X 镜像，列压缩 |

`0x012B` 在固件 `0x8101` 上改用列压缩。

型号表来自 [hass-gicisky](https://github.com/eigger/hass-gicisky)（MIT，© 2025 eigger）。
仅 `0x0033` 已实机验证，其余在 `inkwire scan` 中标记为 `unverified`：

```
FF:FF:92:94:38:61                      NEMR92943861    -50  3.0V  gicisky  EPD 4.2" BWR    400x300   BWR (unverified)
```

未知 id 列出并标记为 `unrecognised model`，不驱动。

## EPD-nRF5

| | |
|---|---|
| GATT | 服务 `62750001-d828-918d-fb46-b6c11c675aec`，读写+通知 `…0002`，版本 `…0003` |
| 命令 | `INIT=0x01` `REFRESH=0x05` `WRITE_IMAGE=0x30` `SET_TIME=0x20` |
| 平面 | 黑白 + 颜色，逐行，高位在前，置位为白，颜色平面取反 |
| 压缩 | 固件 RLE；空白 400x300 平面 15000 → 232 字节 |


### 型号

`INIT` 连接后读取。尺寸不符时按实际面板重排，并报告 `size-mismatch`。面板不支持的墨水改画为黑色，并报告 `unsupported-ink`。

| ID | 名称 | 尺寸 | 颜色 | 打包 |
|---|---|---|---|---|
| `0x01` | UC8176_420_BW | 400x300 | BW | 双平面 |
| `0x02` | SSD1619_420_BWR | 400x300 | BWR | 双平面 |
| **`0x03`** | **UC8176_420_BWR** | **400x300** | **BWR** | 双平面 |
| `0x04` | SSD1619_420_BW | 400x300 | BW | 双平面 |
| `0x06` | UC8179_750_BW | 800x480 | BW | 双平面 |
| `0x07` | UC8179_750_BWR | 800x480 | BWR | 双平面 |
| `0x0a` | SSD1677_750_HD_BW | 880x528 | BW | 双平面 |
| `0x0b` | SSD1677_750_HD_BWR | 880x528 | BWR | 双平面 |
| `0x10` | UC8179_583_BWR | 648x480 | BWR | 双平面 |
| `0x11` | UC8179_583_BW | 648x480 | BW | 双平面 |
| `0x08` `0x09` `0x0e` `0x0f` | UC8159 | 640x384 / 600x448 | BW/BWR | 半字节，**未实现** |
| `0x05` `0x0c` `0x0d` | JD796xx | 400x300 / 800x480 / 648x480 | BWRY | **未实现** |

`0x03` 已验证，其余打印 `unverified`：

```
panel is UC8179_750_BWR 800x480 BWR (unverified), link carries 244 bytes and can decompress
```

未实现的打包被拒绝，不尝试。

### 时钟与日历

| 模式 | 重绘 |
|---|---|
| `picture` | 不重绘 |
| `calendar` | 每天 `00:00:00` |
| `clock` | 每分钟 |

`push` 将标签重置为 `picture`。`mode` 恢复并同步时间：

```bash
inkwire mode -device NRF_EPD_C1F8 -mode clock
```

`-week-start` 不写则保留标签原值。

## 蓝牙链路


### 信号与距离

RSSI 受发射端、天线、路径和接收机影响。以下数据仅适用于测试适配器。

测试条件：Realtek RTL8761BU dongle（Debian，USB `0bda:8771`，蓝牙 5.1，内置天线，USB 2.0 全速）；标签立放，视距无遮挡。

| 距离 | Gicisky | EPD-nRF5 | 推送 |
|---|---|---|---|
| 1 m | −66 dBm | −68 dBm | 4/4 |
| 2 m | −74 dBm | −78 dBm | 2/2 |
| 3 m | −76 dBm | −80 dBm | 2/2 |
| 5 m | −80 dBm | −86 dBm | 2/2 |


### 写入间隔

同一块标签两次写入间隔 ≥45 秒。刷新期间拒绝连接；实测 26 秒时失败。

`-settle` 控制写完后保持连接的时长，提前断开会取消该次绘制。取消的绘制会导致画面显示异常，
且无法获取验证原因。

4.2" 400x300 BWR 标签实测需要 90 秒，30 秒不足。该时长取决于面板刷新时间，与页面内容无关。

`mode` 同样使用 `-settle`。设置时间会触发重绘，时长不足会取消绘制。

`-settle 0` 写入完成后立即断开并取消绘制。省略参数使用 30 秒默认值，负数会被拒绝。

### 连接失败

| 现象 | 出现阶段 |
|---|---|
| `le-connection-abort-by-local` | 连接建立 |
| 面板未在 5 秒内自报型号 | `INIT` 之后（仅 EPD-nRF5） |
| `ATT error: 0x0e` | 传输中的随机一帧 |

失败自动重试 3 次，间隔 2 秒；全部失败后报错退出。

广播窗口外静默是正常现象；扫描不到不代表设备不存在。

## 创建 HTTP 服务

```bash
inkwire serve -listen 127.0.0.1:8080
```

路由与同名 CLI 子命令行为一致。

| 路由 | 对应命令 | 功能 |
|---|---|---|
| `GET /v1/scan` | `inkwire scan` | 列出当前能扫描到的标签 |
| `POST /v1/render` | `inkwire render` | 本地渲染，返回包含 Base64 PNG 预览的 JSON |
| `POST /v1/push` | `inkwire push` | 渲染页面并写入标签 |
| `POST /v1/mode` | `inkwire mode` | 设置 EPD-nRF5 时钟或日历 |

所有写入路由必须提供 `?device=`；服务端不自动选择设备。

`/v1/render` 必须且只能提供一个 `?size=WxH` 或 `?panel=family:id`。请求参数提供完整视口，
HTML 根元素尺寸不能替代它。

```bash
curl http://127.0.0.1:8080/v1/scan

curl -F 'page=@examples/markup_quickstart/page.html;type=text/html' \
  'http://127.0.0.1:8080/v1/render?size=296x128'

curl -F 'page=@examples/markup_quickstart/page.html;type=text/html' \
  'http://127.0.0.1:8080/v1/render?panel=gicisky:0x0033'

curl -F 'page=@examples/markup_quickstart/page.html;type=text/html' \
  'http://127.0.0.1:8080/v1/push?device=NRF_EPD_C1F8'

curl -X POST 'http://127.0.0.1:8080/v1/mode?device=NRF_EPD_C1F8&mode=clock'
```

| 查询参数 | 用在 | 说明 |
|---|---|---|
| `size` | render | 按任意 `WxH` 视口排版 |
| `panel` | render | 按指定面板的完整尺寸排版，并检查它的墨水 |
| `device` | push、mode | 广播名或 BLE 地址，必填 |
| `family` | push | 指定设备家族，跳过自动识别 |
| `mode` | mode | `picture`、`calendar` 或 `clock`，默认 `calendar` |
| `week-start` | mode | `sunday` 或 `monday`，不写则保留标签原值 |

`/v1/render` 和 `/v1/push` 返回完整 `report`；`/v1/push` 和 `/v1/mode` 另含设备状态。
`?panel=` 拒绝时返回 `unprocessable-scene`，响应体仍包含 `pngBase64`。


### 警告

非致命。

| `code` | 含义 |
|---|---|
| `text-clipped` | 内容超出布局框 |
| `layout-overflow` | 子节点超出容器主轴 |
| `empty-layout` | 可绘制区域为零 |
| `missing-runes` | 字库缺少字形 |
| `size-mismatch` | 页面尺寸与面板不符，按面板重排并裁剪超出部分 |
| `unsupported-ink` | 面板不支持该墨水，改画为黑色，每种墨水一条 |
| `unsupported-declaration` | 不支持的 CSS 属性或取值，忽略 |
| `unsupported-at-rule` | 不支持的 at-rule，忽略 |
| `unresolved-drawing` | SVG 无尺寸或无可绘制内容 |
| `unresolved-image` | 图片无法读取 |
| `no-stylesheet` | 页面无任何样式来源 |
| `unresolved-stylesheet` | 样式表无法读取 |
| `duplicate-stylesheet` | 同一份样式表来了两次，只读一次 |
| `over-constrained` | 一个轴上同时给了两条边和一个尺寸，按 CSS 的规则丢掉末端那条边 |
| `unsupported-selector` | 选择器无法解析，跳过该规则 |
| `unreadable-rule` | 语法错误规则，跳过 |
| `substituted-font-size` | 字体或字号不可用，使用最近值 |

### 错误

| `code` | 状态码 | 含义 |
|---|---|---|
| `unsupported-media-type` | 415 | `Content-Type` 不是 `application/json` 或 `multipart/form-data` |
| `invalid-request` | 400 | 缺少 `device` 或渲染目标、参数值无效或 multipart 结构错误 |
| `request-too-large` | 413 | 超出体积上限 |
| `invalid-scene` | 422 | 无法解码或渲染 |
| `invalid-page` | 422 | `page` 部分无法编译 |
| `unprocessable-scene` | 422 | 渲染成功，但无法为该面板打包 |
| `render-failed` | 500 | PNG 编码失败 |
| `device-busy` | 409 | 适配器占用中 |
| `device-identify-failed` | 502 | 设备不可达、家族不符、型号未知或扫描失败 |
| `push-failed` | 502 | 标签报错或连接失败 |
| `device-timeout` | 504 | 完整设备操作超过该家族的总时限 |
| `scan-failed` | 502 | 扫描失败 |

### 并发

同一适配器同时仅允许一个会话。并发写入返回占用者状态：

```json
{
  "error": "device PICKSMART is being written",
  "code": "device-busy",
  "status": {
    "device": "PICKSMART",
    "state": "pushing",
    "since": "2026-08-13T02:41:07Z"
  }
}
```

`status` 随每个写入结果返回。

| 步骤 | 实测 |
|---|---|
| 扫描 | 4.3 – 11.5 秒 |
| 连接 + 首次应答 | 4.3 – 7.8 秒 |
| 每块 | 约 105 毫秒 |
| 一次 Gicisky 写入 | 14.6 – 20.5 秒 |

默认超时：扫描 15 秒、应答 5 秒、重试间隔 2 秒、最多 3 次。

## 图片资源

HTML 使用 `img src` 引用图片。`src` 可以是 HTTP/HTTPS 链接，也可以是相对路径；`.png`、
`.jpg`、`.jpeg` 为位图，`.svg` 为外部 SVG。

```html
<img src="assets/portrait.png" class="portrait">
<img src="assets/chart.svg" class="chart">
<img src="https://example.com/portrait.png" class="remote">
```

SVG 的 `viewBox` 按浏览器默认的 `xMidYMid meet` 映射到元素盒；支持小数缩放、非零原点和带符号的轴向变换。SVG 视口会裁剪其内容。填充和描边颜色必须是面板墨色（`black`、`white`、`red`、`yellow` 及等价十六进制写法）；其他颜色会报告并跳过。开放线段、折线和路径支持 `stroke-linecap: butt|round|square`；路径连接支持 `stroke-linejoin: miter|round|bevel`。`<use href="#id">` 和 `xlink:href` 可引用同一 SVG 中带 `id` 的已支持元素或分组。

CLI 使用 `-asset SRC=FILE` 注入相对路径资源，可重复指定：

```bash
inkwire render \
     -size 296x128 \
     -asset assets/portrait.png=photos/portrait.png \
     -asset assets/chart.svg=charts/chart.svg \
     page.html
```

未指定映射时，CLI 从页面目录读取相对资源。HTTP 使用 multipart：`page` 是 HTML；每个
本地资源是二进制文件字段，字段名必须与 `src` 完全一致，包括目录前缀。HTTP/HTTPS
资源不需要文件字段。

客户端本机路径对服务端不可见；HTTP 页面引用的本地资源必须随请求上传。

```bash
curl -F 'page=@page.html;type=text/html' \
     -F 'assets/portrait.png=@assets/portrait.png;type=image/png' \
     -F 'assets/chart.svg=@assets/chart.svg;type=image/svg+xml' \
     'http://127.0.0.1:8080/v1/render?size=296x128'
```

`link` 的 `href` 遵循相同的相对资源规则，`stylesheet` 也可以作为独立字段传入。

| 上限 | |
|---|---:|
| 页面 | 16 MiB |
| 单个资源 | 32 MiB |
| 请求 | 64 MiB |
| 资源数量 | 32 |
| 渲染页面或解码后图片 | 16,777,216 像素 |

## 示例

### Markup 能力

| 页面 | 尺寸 | 内容 |
|---|---:|---|
| [layout](examples/markup_capabilities/layout.html) | 400x300 | 盒模型、flex、grid、尺寸、定位 |
| [inline](examples/markup_capabilities/inline.html) | 400x300 | 行内流、原子盒、垂直对齐 |
| [paint](examples/markup_capabilities/paint.html) | 400x300 | 墨水、边框、裁剪、可见性、变换 |
| [svg](examples/markup_capabilities/svg.html) | 400x300 | SVG 图元、路径、图案、分组 |
| [resources](examples/markup_capabilities/resources.html) | 400x300 | 本地图片、外部 SVG、object-fit |
| [cascade](examples/markup_capabilities/cascade.html) | 400x300 | 来源顺序、优先级、important、继承 |
| [potrace](examples/markup_capabilities/potrace.html) | 500x500 | 带 viewBox 和带符号变换的外部 SVG |

### 场景与工具

| 页面 | 尺寸 |
|---|---|
| [markup_quickstart](examples/markup_quickstart/page.html) | 296x128 |
| [panel_check](examples/panel_check/)：[primitives](examples/panel_check/primitives.html) [polarity](examples/panel_check/polarity.html) | 400x300 |
| [fridge](examples/fridge/page.html) | 400x300 |
| [claude_usage](examples/claude_usage/page.html) —— Claude Code 用量快照 | 296x128 |
| [card_showcase](examples/card_showcase/page.html) | 296x128 |
| [cookbook](examples/cookbook/main.go) —— display API | 308x944 |
| [gallery](examples/gallery/main.go) —— 图片资源 | 508x392 |

重新生成参考图：`INKWIRE_UPDATE_REFERENCES=1 go test ./...`

<table>
  <tr>
    <td><a href="examples/markup_capabilities/layout.html"><img src="examples/markup_capabilities/layout.png" alt="layout"></a></td>
    <td><a href="examples/markup_capabilities/inline.html"><img src="examples/markup_capabilities/inline.png" alt="inline"></a></td>
  </tr>
  <tr>
    <td><a href="examples/markup_capabilities/paint.html"><img src="examples/markup_capabilities/paint.png" alt="paint"></a></td>
    <td><a href="examples/markup_capabilities/svg.html"><img src="examples/markup_capabilities/svg.png" alt="svg"></a></td>
  </tr>
  <tr>
    <td><a href="examples/markup_capabilities/resources.html"><img src="examples/markup_capabilities/resources.png" alt="resources"></a></td>
    <td><a href="examples/markup_capabilities/cascade.html"><img src="examples/markup_capabilities/cascade.png" alt="cascade"></a></td>
  </tr>
  <tr>
    <td><a href="examples/card_showcase/page.html"><img src="examples/card_showcase/card_showcase.png" alt="card"></a></td>
    <td><a href="examples/fridge/page.html"><img src="examples/fridge/fridge.png" alt="fridge" width="400"></a></td>
  </tr>
  <tr>
    <td><a href="examples/claude_usage/page.html"><img src="examples/claude_usage/claude_usage.png" alt="claude usage"></a></td>
  </tr>
</table>

## 参考

- https://github.com/atc1441/ATC_GICISKY_ESL —— 协议
- https://atc1441.github.io/ATC_GICISKY_Paper_Image_Upload.html —— 上传工具
- https://github.com/tinygo-org/bluetooth —— Go BLE
- https://github.com/fpoli/gicisky-tag —— Python
- https://github.com/eigger/hass-gicisky —— Home Assistant
- https://github.com/tsl0922/EPD-nRF5 —— EPD-nRF5 固件

[English](README.md)

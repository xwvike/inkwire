# Inkwire

Json Schema驱动的电子纸标签渲染器。

| | Gicisky | EPD-nRF5 |
|---|---|---|
| 固件 | 出厂 | nRF51/nRF52 [替换固件](https://github.com/tsl0922/EPD-nRF5) |
| 名称 | `PICKSMART`、`NEMR<mac8>` | `NRF_EPD_<mac4>` |
| 型号来自 | 广播 | 连接 |
| 尺寸 | 212x104 … 960x640 | 400x300 … 880x528 |
| 颜色 | BW、BWR、BWRY | BW、BWR |


## CLI

| 命令 | 用途 |
|---|---|
| `inkwire scan [-timeout 15s]` | 列出当前可被扫描出来的标签 |
| `inkwire render [-o out.png] [-size WxH \| -panel FAMILY:ID] <scene.json>` | PNG 预览 |
| `inkwire push -device NAME [-family gicisky\|nrfepd] [-settle 30s] <scene.json>` | 渲染并写入 |
| `inkwire mode -device NAME [-mode picture\|calendar\|clock] [-week-start sunday\|monday] [-settle 30s]` | EPD-nRF5 时钟/日历 模式 |
| `inkwire serve [-listen ADDR] [-assets DIR]` | 启动 HTTP 服务 |
| `inkwire schema [-lang en\|zh]` | 打印 Json Schema 参考 |
| `inkwire help` | `-h`、`--help` |
| `inkwire version` | `-v`、`--version` |

### scan

列出当前环境中能扫描到的标签。不确定目标详细信息情况下，尽量先扫描，并取得 `-device` 要填的名字或地址。

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

扫描同时区分两个家族：Gicisky 通过厂商数据 `0x5053`，EPD-nRF5 通过 service UUID
或名字前缀。

标签扫描结果为空时退出码为 1。

### render

把场景渲染成 PNG，预览效果。

```bash
inkwire render -o preview.png page.json
```

```
wrote preview.png (296x128)
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-o` | 场景同名的 `.png` | PNG 输出路径 |
| `-size` | 场景自己的 `size` | 改按 `WxH` 排版 |
| `-panel` | 未设置 | 按指定面板排版，并检查它的墨水 |

场景要么自己说多大，要么由别人替它说。**没有默认页面尺寸**了——以前有一个，
是某家族的 2.9" 面板，写死在 display 层里，于是一个什么都没说的场景拿到的是
别人桌上那块标签的尺寸，而且从来没人告诉它。

```
$ inkwire render page.json
this scene states no size: give the document a size, or render with -size WxH or -panel family:id
```

`-size` 直接说明尺寸，不查任何面板表：

```bash
inkwire render -size 400x300 page.json
```

`-panel` 更进一步：尺寸取自面板，然后真的为它打包一遍，把字节丢掉。这是唯一
能抓出面板显示不了的墨水的办法，而预览抓不到——一个红色标题的 PNG，无论目标
面板有没有彩色平面，看上去都是对的。

```bash
inkwire render -panel gicisky:0x0033 page.json
inkwire render -panel nrfepd:UC8176_420_BWR page.json
```

```
wrote page.png (296x128)
wrote page.png (400x300)
```

面板显示不了的一页会说出来，预览照样留下：

```bash
inkwire render -panel gicisky:0x0028 page.json
```

```
wrote page.png (296x128)
BW panel cannot show red ink at (10,10)
```

面板拒绝这一页时预览照样写出来，因为它长什么样才是「该改哪里」的答案；退出码
才是「它上不了这块屏」的答案。面板或尺寸写错退出 2，跟家族写错一样；场景排不
出来退出 1。

家族必须写明，因为两家的编号各排各的；而 Gicisky 面板只能按 id 要：`0x000B`
和 `0x010B` 都叫 `EPD 2.1" BWR`，尺寸却不同。EPD-nRF5 面板则可以用固件给的名
字或 id。两家的 id 都列在 [Gicisky](#gicisky) 和 [EPD-nRF5](#epd-nrf5) 两节。

`-size` 和 `-panel` 是同一件事的两种说法，同时给会被拒绝，而不是替你挑一个。

页面级字段的完整说明见 [SCHEMA.zh-CN.md](SCHEMA.zh-CN.md) 的「页面」一节。

### push

渲染并写入标签。

```bash
inkwire push -device NEMR92943861 page.json
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
| `-family` | `auto` | 指定则断言设备属于这个家族，不再内部判断 |
| `-settle` | `30s` | 仅 EPD-nRF5：`REFRESH` 后保持连接的时长，见[写入间隔](#写入间隔) |

场景声明的尺寸与面板不符不会拒绝，按面板重排并给出 `size-mismatch` 警告。

渲染报告中的警告打印在写入日志里，见[警告](#警告)。

### mode

把 EPD-nRF5 标签交还给它自己的时钟或日历，并顺带对时。

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
| `-settle` | `30s` | 重绘期间保持连接的时长 |

只有 EPD-nRF5 有这个功能。指向 Gicisky 标签会指名拒绝：

```
NEMR92943861 (e2ada7d1-…, gicisky) is a gicisky tag, but the family asked for is nrfepd
```

`push` 会把标签切回 picture 模式，所以这条命令是它的反向操作，详见[时钟与日历](#时钟与日历)。

### serve

启动 HTTP 服务，供不是命令行的调用方使用。

```bash
inkwire serve -listen 127.0.0.1:8080
```

```
listening on http://127.0.0.1:8080
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-listen` | `127.0.0.1:8080` | 监听地址，仅限回环 |
| `-assets` | `.` | 相对图片路径可以读取的目录 |

没有任何鉴权，每个请求都会写硬件，所以只允许绑定回环地址，绑别的会被拒绝。
路由与错误码见[创建HTTP服务](#创建http服务)。

### schema

打印 Json Schema 参考。内容随二进制一同分发。

```bash
inkwire schema -lang zh > SCHEMA.zh-CN.md
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-lang` | `en` | 打印哪一份翻译：`en` 或 `zh` |



## Gicisky

| | |
|---|---|
| 面板 | 从广播识别；212x104 … 960x640，BW/BWR/BWRY |
| 方向 | 横屏使用 profile 尺寸，竖屏交换宽高 |
| Payload | 按型号选择平面、变换与压缩 |
| GATT | 服务 `FEF0`，控制 `FEF1`，数据 `FEF2` |

厂商数据，公司 `0x5053`：

```
byte   0        1        2   3        4
       id low   battery  firmware     id high
```

型号 id = `(data[4] << 8) | data[0]` 低 14 位。名称不携带型号，因此写入
Gicisky 标签前必须先看到厂商数据，再按该型号渲染。

### 型号

广播里读出，不需要连接。

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

表来自 [hass-gicisky](https://github.com/eigger/hass-gicisky)（MIT，© 2025 eigger）。
只有 `0x0033` 经过实机核对，其余在 `inkwire scan` 里标 `unverified`：

```
FF:FF:92:94:38:61                      NEMR92943861    -50  3.0V  gicisky  EPD 4.2" BWR    400x300   BWR (unverified)
```

不在表里的 id 仍会列出，但标为 `unrecognised model`，不会被驱动。

## EPD-nRF5

| | |
|---|---|
| GATT | 服务 `62750001-d828-918d-fb46-b6c11c675aec`，读写+通知 `…0002`，版本 `…0003` |
| 命令 | `INIT=0x01` `REFRESH=0x05` `WRITE_IMAGE=0x30` `SET_TIME=0x20` |
| 平面 | 黑白 + 颜色，逐行，高位在前，置位为白，颜色平面取反 |
| 压缩 | 固件 RLE；空白 400x300 平面 15000 → 232 字节 |


### 型号

`INIT` 连接后读出。尺寸不符不会拒绝，按实际面板重新排版并给出 `size-mismatch` 警告。

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

`push` 将标签重置为 `picture`。`mode` 恢复并对时：

```bash
inkwire mode -device NRF_EPD_C1F8 -mode clock
```

`-week-start` 不写则保留标签原值。

## 蓝牙链路


### 信号与距离

RSSI 属于整条链路——标签的发射、两端天线、路径与接收机——所以下表是**某一个适配器**的状况。

测试适配器 Realtek RTL8761BU dongle（OS: Debian，USB `0bda:8771`，蓝牙 5.1，内置天线，USB 2.0 全速），标签立放且视距无遮挡：

| 距离 | Gicisky | EPD-nRF5 | 推送 |
|---|---|---|---|
| 1 m | −66 dBm | −68 dBm | 4/4 |
| 2 m | −74 dBm | −78 dBm | 2/2 |
| 3 m | −76 dBm | −80 dBm | 2/2 |
| 5 m | −80 dBm | −86 dBm | 2/2 |


### 写入间隔

同一块标签两次写入间隔 ≥45 秒。刷新期间标签拒绝连接，实测 26 秒时失败。

`-settle` 控制的是写完之后继续保持连接的时长，提前断开会取消这次绘制。

### 连接失败

| 现象 | 出现阶段 |
|---|---|
| `le-connection-abort-by-local` | 连接建立 |
| 面板未在 5 秒内自报型号 | `INIT` 之后（仅 EPD-nRF5） |
| `ATT error: 0x0e` | 传输中的随机一帧 |

实际失败会自动重试，默认 3 次、间隔 2 秒。三次都失败才报错退出。

标签在广播窗口之间静默是正常的，扫不到不等于不在——这也是重试存在的原因。

## 创建HTTP服务

```bash
inkwire serve -listen 127.0.0.1:8080
```

路由与同名子命令做同一件事，参数也同名。

| 路由 | 对应命令 | 功能 |
|---|---|---|
| `GET /v1/scan` | `inkwire scan` | 列出当前能扫描到的标签 |
| `POST /v1/render` | `inkwire render` | 本地渲染，返回 PNG 预览 |
| `POST /v1/push` | `inkwire push` | 渲染并写入标签 |
| `POST /v1/mode` | `inkwire mode` | 把 EPD-nRF5 标签交还给它自己的时钟或日历 |

`?device=` 所有写入路由必填，和 CLI 的 `-device` 一样，服务端不会替请求猜设备。

```bash
curl http://127.0.0.1:8080/v1/scan

curl -H 'Content-Type: application/json' --data-binary @page.json \
  http://127.0.0.1:8080/v1/render -o render.json

curl -H 'Content-Type: application/json' --data-binary @page.json \
  'http://127.0.0.1:8080/v1/render?panel=gicisky:0x0033' -o render.json

curl -H 'Content-Type: application/json' --data-binary @page.json \
  'http://127.0.0.1:8080/v1/push?device=NRF_EPD_C1F8'

curl -X POST 'http://127.0.0.1:8080/v1/mode?device=NRF_EPD_C1F8&mode=clock'
```

| 查询参数 | 用在 | 说明 |
|---|---|---|
| `size` | render | 改按 `WxH` 排版，而不是场景声明的尺寸 |
| `panel` | render | 按指定面板排版，并检查它的墨水 |
| `device` | push、mode | 广播名或 BLE 地址，必填 |
| `family` | push | 指定则断言设备属于这个家族 |
| `mode` | mode | `picture`、`calendar` 或 `clock`，默认 `calendar` |
| `week-start` | mode | `sunday` 或 `monday`，不写则保留标签原值 |

`/v1/render` 和 `/v1/push` 的响应含完整 `report`，`/v1/push` 和 `/v1/mode` 另含设备状态。
`?panel=` 拒绝的那一页返回 `unprocessable-scene`，但 `pngBase64` 仍在响应体里：
这一页画出来了，它长什么样才是「该改哪里」的答案。


### 警告

不致命。

| `code` | 含义 |
|---|---|
| `text-clipped` | 文字超出所在框，字符或整行被裁 |
| `layout-overflow` | 子节点主轴超出容器 |
| `empty-layout` | 内边距或尺寸使可绘制区域为零 |
| `missing-runes` | 字库缺少这些字形 |
| `size-mismatch` | 场景声明的尺寸与面板不符，已按面板重排，超出部分被裁 |

### 错误

| `code` | 状态码 | 含义 |
|---|---|---|
| `unsupported-media-type` | 415 | 非 JSON 或 multipart |
| `invalid-request` | 400 | 未指名 `device`，或 `family`、`mode`、`week-start` 取值未知，或 multipart 结构错误 |
| `request-too-large` | 413 | 超出体积上限 |
| `invalid-scene` | 422 | 无法解码或渲染 |
| `unprocessable-scene` | 422 | 渲染成功，但无法为该面板打包 |
| `render-failed` | 500 | PNG 编码失败 |
| `device-busy` | 409 | 适配器占用中 |
| `device-identify-failed` | 502 | 目标不在范围内、属于另一个家族、未广播已知型号，或扫描失败 |
| `push-failed` | 502 | 标签报错或连接失败 |
| `device-timeout` | 504 | 完整设备操作超过该家族的总时限 |
| `scan-failed` | 502 | 扫描失败 |

### 并发

一个适配器一次会话，跨设备互斥。第二个写入被拒，附带占用者状态：

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

`status` 出现在每个写入结果中。

| 步骤 | 实测 |
|---|---|
| 扫描 | 4.3 – 11.5 秒 |
| 连接 + 首次应答 | 4.3 – 7.8 秒 |
| 每块 | 约 105 毫秒 |
| 一次 Gicisky 写入 | 14.6 – 20.5 秒 |

扫描 15 秒，应答 5 秒，重试 2 秒，3 次。

## 图片资源

| 场景 | `source` 解析为 |
|---|---|
| CLI | 相对场景文档的路径；也接受绝对路径、`file:`、data URL |
| `serve -assets DIR` | `DIR` 下的路径；绝对路径和 `..` 被拒 |
| multipart | 同一请求中的文件字段名 |

```json
{
  "type": "image",
  "source": "assets/portrait.png",
  "processing": "auto"
}
```

```bash
inkwire render -o output/dashboard.png scenes/dashboard/page.json
```

```json
{
  "version": 1,
  "size": {"width": 296, "height": 128},
  "root": {
    "type": "image",
    "source": "portrait",
    "processing": "auto"
  }
}
```

```bash
curl -F 'scene=@page.json;type=application/json' \
     -F 'portrait=@photos/portrait.png;type=image/png' \
     http://127.0.0.1:8080/v1/render -o render.json
```

`scene` 是文档，其他文件字段是资源，并在该请求内覆盖 `-assets`。

| 上限 | |
|---|---:|
| Scene JSON | 16 MiB |
| 单个资源 | 32 MiB |
| 请求 | 64 MiB |
| 资源数量 | 32 |
| 渲染页面或解码后图片 | 16,777,216 像素 |

## 示例

| 页面 | 尺寸 |
|---|---|
| [desk](examples/desk/)：[claude](examples/desk/claude.json) [disk](examples/desk/disk.json) [tasks](examples/desk/tasks.json) [btc](examples/desk/btc.json) [chart](examples/desk/chart.json) | 296x128 |
| [panel_check](examples/panel_check/)：[primitives](examples/panel_check/primitives.json) [polarity](examples/panel_check/polarity.json) | 400x300 |
| [layout_showcase](examples/layout_showcase/page.json) —— `grid` `anchored` `transformed` `clip` `clipShape` | 296x128 |
| [fridge](examples/fridge/page.json) | 400x300 |
| [compose_showcase](examples/compose_showcase/page.json) | 296x128 |
| [showcase](examples/showcase/page.json) —— 图元与文字 | 296x128 |
| [paint_showcase](examples/paint_showcase/page.json) —— 裁剪、图案、虚线 | 296x128 |
| [state_showcase](examples/state_showcase/page.json) | 296x128 |
| [card_showcase](examples/card_showcase/page.json) | 296x128 |
| [text_showcase](examples/text_showcase/page.json) | 296x128 |
| [cookbook](examples/cookbook/main.go) —— display API | 296x128 |

重新生成参考图：`INKWIRE_UPDATE_REFERENCES=1 go test ./...`

<table>
  <tr>
    <td><a href="examples/desk/btc.json"><img src="examples/desk/btc.png" alt="btc"></a></td>
    <td><a href="examples/desk/chart.json"><img src="examples/desk/chart.png" alt="chart"></a></td>
  </tr>
  <tr>
    <td><a href="examples/desk/disk.json"><img src="examples/desk/disk.png" alt="disk"></a></td>
    <td><a href="examples/desk/tasks.json"><img src="examples/desk/tasks.png" alt="tasks"></a></td>
  </tr>
  <tr>
    <td><a href="examples/layout_showcase/page.json"><img src="examples/layout_showcase/layout_showcase.png" alt="layout"></a></td>
    <td><a href="examples/compose_showcase/page.json"><img src="examples/compose_showcase/compose_showcase.png" alt="compose"></a></td>
  </tr>
  <tr>
    <td><a href="examples/card_showcase/page.json"><img src="examples/card_showcase/card_showcase.png" alt="card"></a></td>
    <td><a href="examples/showcase/page.json"><img src="examples/showcase/showcase.png" alt="showcase"></a></td>
  </tr>
  <tr>
    <td><a href="examples/paint_showcase/page.json"><img src="examples/paint_showcase/paint_showcase.png" alt="paint"></a></td>
    <td><a href="examples/state_showcase/page.json"><img src="examples/state_showcase/state_showcase.png" alt="state"></a></td>
  </tr>
  <tr>
    <td><a href="examples/text_showcase/page.json"><img src="examples/text_showcase/text_showcase.png" alt="text"></a></td>
    <td><a href="examples/fridge/page.json"><img src="examples/fridge/fridge.png" alt="fridge" width="400"></a></td>
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

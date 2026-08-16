# Inkwire

Scene Schema → 电子纸标签。两个家族，一个渲染器。

| | Gicisky | EPD-nRF5 |
|---|---|---|
| 固件 | 出厂 | nRF51/nRF52 [替换固件](https://github.com/tsl0922/EPD-nRF5) |
| 名称 | `PICKSMART`、`NEMR<mac8>` | `NRF_EPD_<mac4>` |
| 型号来自 | 广播 | 连接 |
| 尺寸 | 296x128 | 400x300 … 880x528 |
| 颜色 | BWR | BW、BWR |

## CLI

| 命令 | 用途 |
|---|---|
| `inkwire render [-o out.png] <scene.json>` | PNG 预览 |
| `inkwire encode [-o out.bin] <scene.json>` | 设备 payload，仅 Gicisky |
| `inkwire push [-device NAME] [-family auto\|gicisky\|nrfepd] [-settle 30s] <scene.json>` | 渲染并写入 |
| `inkwire scan [-timeout 15s]` | 列出标签 |
| `inkwire mode [-device NAME] [-mode picture\|calendar\|clock] [-week-start sunday\|monday] [-settle 30s]` | EPD-nRF5 时钟与模式 |
| `inkwire serve [-listen ADDR] [-device NAME] [-assets DIR]` | HTTP 服务 |
| `inkwire push-payload [NAME] <payload.bin>` | 写入原始 payload |

```
$ inkwire scan
ADDRESS                                NAME           RSSI  BATT  FAMILY   MODEL           SIZE      PALETTE
FF:FF:92:94:38:61                      NEMR92943861    -50  3.0V  gicisky  EPD 2.9" BWR    296x128   BWR
C1:57:DD:3F:C1:F8                      NRF_EPD_C1F8    -62     -  nrfepd   ask on connect            not advertised
```

```bash
inkwire render -o preview.png page.json
inkwire push -device NEMR92943861 page.json
```

| 参数 | 说明 |
|---|---|
| `-device` | 广播名（`NAME` 列）或 BLE 地址。默认 `FF:FF:92:94:38:61` |
| `-family` | `auto` 把 `NRF_EPD*` 映射为 nrfepd，其余 gicisky。地址不携带家族 |
| `-timeout` | 按家族计时，两个家族依次扫描 |
| `-settle` | `REFRESH` 后保持连接的时长，见[刷新](#刷新) |

## Gicisky

| | |
|---|---|
| 面板 | 296x128，BWR，无灰度 |
| 方向 | 横屏 296x128，竖屏 128x296 |
| Payload | 黑白平面 + 红色平面，9472 字节 |
| GATT | 服务 `FEF0`，控制 `FEF1`，数据 `FEF2` |
| 名称 | `PICKSMART` 仅开机期间；稳定后 `NEMR<mac8>` |

厂商数据，公司 `0x5053`：

```
byte   0        1        2   3        4
       id low   battery  firmware     id high
```

型号 id = `(data[4] << 8) | data[0]` 低 14 位。名称不携带型号。

11 个型号（212x104 … 960x640，BW/BWR/BWRY），来自 [hass-gicisky](https://github.com/eigger/hass-gicisky)（MIT，© 2025 eigger）。`0x0033` 已验证，其余打印 `unverified`。未知 id 列出但不可驱动。

## EPD-nRF5

| | |
|---|---|
| GATT | 服务 `62750001-d828-918d-fb46-b6c11c675aec`，读写+通知 `…0002`，版本 `…0003` |
| 命令 | `INIT=0x01` `REFRESH=0x05` `WRITE_IMAGE=0x30` `SET_TIME=0x20` |
| 平面 | 黑白 + 颜色，逐行，高位在前，置位为白，颜色平面取反 |
| 压缩 | 固件 RLE；空白 400x300 平面 15000 → 232 字节 |

```bash
inkwire push -device NRF_EPD_C1F8 examples/fridge/page.json
```

### 型号

`INIT` 之后读出。尺寸不符被拒：

```
the page is 296x128 and the panel is UC8176_420_BWR 400x300 BWR; render it at the panel's size
```

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

### 刷新

```
sending 40 frames compressed
staying connected 30s while the panel refreshes; disconnecting now would cancel it
```

| | |
|---|---|
| `-settle` | 默认 30 秒。提前断开取消绘制。大屏或 BWR 用 60 秒 |
| 写入间隔 | 同一标签 ≥45 秒；26 秒失败——刷新期间标签拒绝连接 |

可靠性取决于 RSSI，与帧数无关：

| RSSI | 39 帧页面 |
|---|---|
| −88 dBm | 0/8 |
| −50 dBm | 12/12 |

弱链路表现为连接阶段 `le-connection-abort-by-local`、缺少 config 回复、或随机帧 `ATT error: 0x0e`。先查 RSSI。

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

## HTTP

```bash
inkwire serve -listen 127.0.0.1:8080 -device NEMR92943861
```

| 路由 | 响应 |
|---|---|
| `POST /v1/render` | `image/png` |
| `POST /v1/encode` | 9472 字节，`application/octet-stream`，仅 Gicisky |
| `POST /v1/display` | JSON 结果 |
| `GET /v1/devices` | 标签列表，每条带 `family` |

`-listen` 仅回环。`/v1/display` 接受 `?device=` 与 `?family=auto|gicisky|nrfepd`。预算：Gicisky 45 秒，EPD-nRF5 150 秒。`/v1/encode` 对 EPD-nRF5 目标以 `size-unknown` 拒绝：该面板连接后才报出自身尺寸。请改用 `/v1/display`。

```bash
curl -H 'Content-Type: application/json' --data-binary @page.json \
  http://127.0.0.1:8080/v1/render -o preview.png

curl -H 'Content-Type: application/json' --data-binary @page.json \
  'http://127.0.0.1:8080/v1/display?device=NRF_EPD_C1F8'
```

响应头：`X-Inkwire-Warnings`、`X-Inkwire-Missing-Runes`、`X-Inkwire-Image-Decisions`。

### 警告

不致命。

| `code` | 含义 |
|---|---|
| `text-clipped` | 文字超出其框，字符或整行被裁 |
| `layout-overflow` | 子节点主轴超出容器 |
| `empty-layout` | 内边距或尺寸使可绘制区域为零 |
| `missing-runes` | 字库缺少这些字形 |

### 错误

| `code` | 状态码 | 含义 |
|---|---|---|
| `unsupported-media-type` | 415 | 非 JSON 或 multipart |
| `invalid-request` | 400 | multipart 结构错误，或 `family` 未知 |
| `request-too-large` | 413 | 超出体积上限 |
| `invalid-scene` | 422 | 无法解码或渲染 |
| `unprocessable-scene` | 422 | 能渲染，无法编码 |
| `size-unknown` | 400 | 对 EPD-nRF5 目标调用 `/v1/encode` |
| `render-failed` | 500 | PNG 编码失败 |
| `device-busy` | 409 | 适配器占用中 |
| `push-failed` | 502 | 标签报错或连接失败 |
| `device-timeout` | 504 | 重试用尽 |
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
     http://127.0.0.1:8080/v1/render -o preview.png
```

`scene` 是文档，其他文件字段是资源，并在该请求内覆盖 `-assets`。

| 上限 | |
|---|---:|
| Scene JSON | 16 MiB |
| 单个资源 | 32 MiB |
| 请求 | 64 MiB |
| 资源数量 | 32 |

## Scene Schema

全部整数像素。

```json
{
  "version": 1,
  "orientation": "landscape",
  "background": "white",
  "root": {
    "type": "absolute",
    "clip": true,
    "children": [
      {
        "bounds": {"x": 4, "y": 4, "width": 288, "height": 24},
        "node": {
          "type": "rectangle",
          "radius": 4,
          "fill": "black"
        }
      },
      {
        "bounds": {"x": 10, "y": 8, "width": 276, "height": 16},
        "node": {
          "type": "text",
          "runs": [
            {"text": "INKWIRE ", "font": "monaco", "size": 12, "ink": "white"},
            {"text": "23.5℃", "font": "ui", "size": 14, "ink": "red"}
          ]
        }
      },
      {
        "bounds": {"x": 12, "y": 42, "width": 272, "height": 72},
        "node": {
          "type": "text",
          "wrap": "runes",
          "lineHeight": 16,
          "runs": [
            {"text": "今天的电子纸页面由 JSON 直接生成。", "font": "ui", "size": 14, "ink": "black"}
          ]
        }
      }
    ]
  }
}
```

![quickstart](examples/schema_quickstart/schema_quickstart.png)

### 页面

| 字段 | 类型 | 取值 | 默认 |
|---|---|---|---|
| `version` | integer | `1` | 必填 |
| `orientation` | string | `landscape`、`portraitClockwise`、`portraitCounterClockwise` | `landscape` |
| `size` | size | 仅预览 | 设备尺寸 |
| `background` | ink | `white`、`black`、`red` | `white` |
| `root` | node | | 空 |

### 基本值

```json
{
  "size": {"width": 100, "height": 40},
  "point": {"x": 10, "y": 5},
  "rect": {"x": 10, "y": 5, "width": 100, "height": 40}
}
```

`ink` 为 `black`、`white` 或 `red`；可选颜色默认 `black`。

```json
{
  "ink": "black",
  "width": 2,
  "dash": [4, 2],
  "dashOffset": 0
}
```

| 字段 | 类型 | | 默认 |
|---|---|---|---|
| `ink` | ink | | `black` |
| `width` | integer | > `0` | 必填 |
| `dash` | integer[] | 实线/空白长度，每项 > `0` | 实线 |
| `dashOffset` | integer | | `0` |

所有节点需要 `type`。`size` 是流式布局的首选尺寸，在 `absolute.children[].bounds` 内被忽略。

### 长度

| 写法 | 含义 |
|---|---|
| `64` | 像素 |
| `"25%"` | 容器该轴尺寸的比例 |
| `"calc(100% - 12px)"` | 百分比 ± 像素，两项都必需 |

```json
{"basis": 64, "cross": "25%", "maxMain": "calc(100% - 12px)"}
```

`0` 是长度；字段缺省才是自动。`anchored` 上 `"right": 0` 贴边，不写 `right` 则该边自由。

距离接受负数——`anchored` 各边、`clipShape` 的 `inset`、`circle` 与 `ellipse` 的 `center`、`polygon` 的 `points`：

```json
{"left": -6, "right": "-10%", "top": "calc(0% - 6px)"}
```

尺寸不接受：`basis`、`cross`、`width`、`height`、`min*`/`max*`、轨道、`radius`、`corner`。百分比与 `fr` 不计入固有尺寸。

## 布局

### absolute

```json
{
  "type": "absolute",
  "clip": true,
  "children": [
    {
      "bounds": {"x": 4, "y": 4, "width": 100, "height": 20},
      "node": {"type": "rectangle", "fill": "black"}
    }
  ]
}
```

| 字段 | 类型 | | 默认 |
|---|---|---|---|
| `size` | size | | 内容 |
| `clip` | boolean | 裁到 bounds | `false` |
| `children[].bounds` | rect | | 必填 |
| `children[].node` | node | | 必填 |

### row / column

```json
{
  "type": "row",
  "gap": 4,
  "mainAlign": "start",
  "crossAlign": "center",
  "children": [
    {
      "basis": 64,
      "node": {"type": "text", "runs": [{"text": "LEFT"}]}
    },
    {
      "grow": 1,
      "node": {"type": "text", "align": "end", "runs": [{"text": "RIGHT"}]}
    }
  ]
}
```

| 字段 | 类型 | | 默认 |
|---|---|---|---|
| `gap` | integer | | `0` |
| `mainAlign` | string | `start`、`center`、`end` | `start` |
| `crossAlign` | string | `start`、`center`、`end`、`stretch` | `stretch` |
| `children[].grow` | integer | 剩余空间权重 | `0` |
| `children[].basis` | length | 分配前的主轴尺寸 | 内容 |
| `children[].cross` | length | 交叉轴尺寸 | 容器 |
| `children[].minMain` / `maxMain` | length | | 无 |
| `children[].minCross` / `maxCross` | length | | 无 |
| `children[].ratio` | number | 主轴 ÷ 交叉轴 | 无 |
| `children[].alignSelf` | string | 覆盖 `crossAlign` | 容器 |

### grid

```json
{
  "type": "grid",
  "columns": ["auto", "1fr", "auto"],
  "rows": [15, 15],
  "columnGap": 5,
  "rowGap": 3,
  "children": [
    {"node": {"type": "text", "runs": [{"text": "/data"}]}},
    {"node": {"type": "rectangle", "stroke": {"ink": "black", "width": 1}}},
    {"node": {"type": "text", "align": "end", "runs": [{"text": "1180G"}]}},
    {"column": 1, "columnSpan": 3, "node": {"type": "spacer"}}
  ]
}
```

| 字段 | 类型 | | 默认 |
|---|---|---|---|
| `columns` / `rows` | track[] | `"auto"`、长度、`"1fr"` | 一条自动轨道 |
| `columnGap` / `rowGap` | integer | | `0` |
| `alignItems` / `justifyItems` | string | `start`、`center`、`end`、`stretch` | `stretch` |
| `children[].column` / `row` | integer | 从 1 起的线号；`0` = 下一空位 | `0` |
| `children[].columnSpan` / `rowSpan` | integer | | `1` |
| `children[].alignSelf` / `justifySelf` | string | | 网格 |

自动轨道取最宽内容；`fr` 分配剩余。

### anchored

```json
{
  "type": "anchored",
  "children": [
    {"left": "50%", "top": 0, "width": 40, "height": 12,
     "node": {"type": "rectangle", "fill": "black"}},
    {"right": 0, "bottom": 0, "width": 20, "height": 20, "layer": 1,
     "node": {"type": "rectangle", "fill": "red"}}
  ]
}
```

| 字段 | 类型 | |
|---|---|---|
| `children[].top` `right` `bottom` `left` | length | 距该边的距离 |
| `children[].width` `height` | length | 该轴尺寸 |

同轴两条边加一个尺寸被拒。

### transformed

```json
{
  "type": "transformed",
  "scale": 2,
  "turns": 1,
  "child": {"type": "text", "runs": [{"text": "42"}]}
}
```

| 字段 | 类型 | | 默认 |
|---|---|---|---|
| `scale` | integer | 仅整数 | `1` |
| `turns` | integer | 顺时针 90° 次数 | `0` |
| `child` | node | | 必填 |

需要重采样的一律拒绝。

### stack / padding / spacer

```json
{
  "type": "stack",
  "children": [
    {"type": "rectangle", "fill": "white", "stroke": {"ink": "black", "width": 1}},
    {"type": "text", "align": "center", "verticalAlign": "middle", "runs": [{"text": "OK"}]}
  ]
}
```

```json
{
  "type": "padding",
  "insets": {"top": 4, "right": 8, "bottom": 4, "left": 8},
  "child": {"type": "text", "runs": [{"text": "PAD"}]}
}
```

```json
{"type": "spacer", "size": {"width": 8, "height": 8}}
```

| 节点 | 字段 |
|---|---|
| `stack` | `size`、`children[]` —— 同一区域内按序绘制 |
| `padding` | `insets`（`top` `right` `bottom` `left`）、`child` |
| `spacer` | `size` |

## 文字

```json
{
  "type": "text",
  "align": "start",
  "verticalAlign": "top",
  "wrap": "none",
  "lineHeight": 14,
  "runs": [
    {"text": "温度 ", "font": "hzk", "size": 14, "ink": "black"},
    {"text": "23.5", "font": "monaco", "size": 14, "ink": "red"},
    {"text": "℃", "font": "hzk", "size": 14, "ink": "red"}
  ]
}
```

| 字段 | 类型 | | 默认 |
|---|---|---|---|
| `runs[].text` | string | | 必填 |
| `runs[].font` | string | `ui`、`hzk`、`monaco` | `ui` |
| `runs[].size` | integer | | `12` |
| `runs[].ink` | ink | | `black` |
| `align` | string | `start`、`center`、`end` | `start` |
| `verticalAlign` | string | `top`、`middle`、`bottom` | `top` |
| `wrap` | string | `none`、`runes` | `none` |
| `lineHeight` | integer | | 字体 |

| 字族 | 尺寸 | 覆盖 |
|---|---|---|
| `ui` | 12 14 16 24 28 32 36 42 48 | 拉丁 + 中日韩 |
| `hzk` | 12 14 16 24 28 32 36 42 48 | 拉丁 + 中日韩 |
| `monaco` | 10 12 14 16 20 24 28 30 32 36 42 48 | **仅 ASCII** |

16 以上为整数倍放大。混排拆成多个 run。

## 图片

```json
{
  "type": "image",
  "source": "portrait.png",
  "processing": "manual",
  "options": {
    "fit": "cover",
    "sampling": "bilinear",
    "dither": "floydSteinberg",
    "threshold": 128,
    "redThreshold": 170,
    "redMaxGreen": 170,
    "disableRed": false
  },
  "contrast": {
    "radius": 4,
    "amount": 1.3
  }
}
```

| 字段 | 类型 | | 默认 |
|---|---|---|---|
| `source` | string | 见[图片资源](#图片资源) | 必填 |
| `processing` | string | `manual`、`auto` | `manual` |
| `options` | image options | 手动参数 | 默认值 |
| `overrides` | image overrides | 覆盖 `auto` 选出的指定字段 | 无 |
| `contrast.radius` | integer | 局部对比度半径，≥0 | 关闭 |
| `contrast.amount` | number | 局部对比度强度 | 关闭 |

options 与 overrides 字段相同；overrides 按字段生效。

| 字段 | 类型 | | 默认 |
|---|---|---|---|
| `fit` | string | `stretch`、`contain`、`cover` | `stretch` |
| `sampling` | string | `nearest`、`bilinear` | `nearest` |
| `dither` | string | `threshold`、`floydSteinberg`、`ordered` | `threshold` |
| `threshold` | integer | 0–255 亮度阈值 | `128` |
| `redThreshold` | integer | 0–255 红色通道阈值 | |
| `redMaxGreen` | integer | 0–255 判红的绿色上限 | |
| `disableRed` | boolean | 丢弃红色平面 | `false` |

`auto` 依据图像选择阈值、抖动与红色提取，并在 `X-Inkwire-Image-Decisions` 中报告。

```json
{
  "type": "image",
  "source": "portrait.png",
  "processing": "auto",
  "overrides": {
    "fit": "cover",
    "sampling": "bilinear",
    "dither": "floydSteinberg",
    "disableRed": true
  },
  "contrast": {
    "radius": 4,
    "amount": 1.3
  }
}
```

```json
{
  "type": "image",
  "source": "data:image/png;base64,iVBORw0KGgoAAA...",
  "processing": "manual"
}
```

## 图元

区域来自 `absolute` 的 bounds 或共享的 `stack`。

| `type` | 字段 | 可选 |
|---|---|---|
| `pixel` | `at`、`ink` | `size` |
| `rectangle` | `fill` 或 `stroke` | `size`、`radius` |
| `line` | `from`、`to`、`stroke` | `size` |
| `polyline` | `points` ≥2、`stroke` | `size` |
| `polygon` | `points` ≥3、`fill` 或 `stroke` | `size` |
| `circle` | `center`、`radius`、`fill` 或 `stroke` | `size` |
| `ellipse` | `fill` 或 `stroke` | `size` |
| `arc` | `start`、`sweep`、`stroke` | `size` |
| `pie` | `start`、`sweep`、`ink` | `size` |
| `chord` | `start`、`sweep`、`ink` | `size` |

角度为度。`ellipse` `arc` `pie` `chord` 使用整个矩形。

```json
{
  "type": "absolute",
  "children": [
    {
      "bounds": {"x": 2, "y": 2, "width": 40, "height": 24},
      "node": {"type": "rectangle", "radius": 4, "fill": "white", "stroke": {"ink": "black", "width": 1}}
    },
    {
      "bounds": {"x": 46, "y": 2, "width": 32, "height": 32},
      "node": {"type": "circle", "center": {"x": 16, "y": 16}, "radius": 14, "stroke": {"ink": "red", "width": 2}}
    },
    {
      "bounds": {"x": 82, "y": 2, "width": 40, "height": 24},
      "node": {"type": "ellipse", "fill": "red", "stroke": {"ink": "black", "width": 1}}
    },
    {
      "bounds": {"x": 126, "y": 2, "width": 40, "height": 24},
      "node": {"type": "arc", "start": -90, "sweep": 270, "stroke": {"ink": "black", "width": 3}}
    },
    {
      "bounds": {"x": 170, "y": 2, "width": 40, "height": 24},
      "node": {"type": "pie", "start": -90, "sweep": 120, "ink": "red"}
    },
    {
      "bounds": {"x": 214, "y": 2, "width": 40, "height": 24},
      "node": {"type": "chord", "start": 20, "sweep": 220, "ink": "black"}
    }
  ]
}
```

```json
{
  "type": "stack",
  "children": [
    {"type": "pixel", "at": {"x": 3, "y": 3}, "ink": "black"},
    {"type": "line", "from": {"x": 0, "y": 20}, "to": {"x": 60, "y": 0}, "stroke": {"ink": "red", "width": 2, "dash": [4, 2]}},
    {"type": "polyline", "points": [{"x": 0, "y": 8}, {"x": 6, "y": 2}, {"x": 12, "y": 8}], "stroke": {"ink": "black", "width": 1}},
    {"type": "polygon", "points": [{"x": 20, "y": 0}, {"x": 40, "y": 10}, {"x": 20, "y": 20}, {"x": 0, "y": 10}], "fill": "red"}
  ]
}
```

## Path

```json
{
  "type": "path",
  "commands": [
    {"op": "move", "to": {"x": 0, "y": 20}},
    {"op": "line", "to": {"x": 20, "y": 0}},
    {"op": "quadratic", "control": {"x": 30, "y": 20}, "to": {"x": 40, "y": 5}},
    {"op": "cubic", "control1": {"x": 45, "y": 0}, "control2": {"x": 50, "y": 20}, "to": {"x": 60, "y": 10}},
    {"op": "arc", "bounds": {"x": 60, "y": 0, "width": 30, "height": 30}, "start": 0, "sweep": 180},
    {"op": "close"}
  ],
  "fill": "black",
  "stroke": {"ink": "red", "width": 1}
}
```

| `op` | 字段 |
|---|---|
| `move` | `to` |
| `line` | `to` |
| `quadratic` | `control`、`to` |
| `cubic` | `control1`、`control2`、`to` |
| `arc` | `bounds`、`start`、`sweep` |
| `close` | |

≥1 条命令，且需 `fill` 或 `stroke`。

## Pattern

```json
{
  "type": "pattern",
  "rows": [
    "x...",
    ".r..",
    "..x.",
    "...r"
  ],
  "inks": {
    "x": "black",
    "r": "red"
  }
}
```

各行等长。`inks` 键为单字符；未映射字符不修改画面。在区域内平铺。

## 裁剪

| 节点 | 裁到 |
|---|---|
| `clip` | 子节点自身的框 |
| `clipRect` | 一个矩形 |
| `clipShape` | `inset`、`circle`、`ellipse`、`polygon` |
| `clipPath` | 一条路径 |

| 节点 | 字段 |
|---|---|
| `clip` | `child`、`layer` |
| `clipRect` | `rect`、`child`、`layer` |
| `clipShape` | `shape`、`child`、`layer` |
| `clipPath` | `commands`、`child`、`layer` |

`shape.kind` 为 `inset`、`circle`、`ellipse` 或 `polygon`，按需带 `insets`、`corner`、`radius`、`radiusX`、`radiusY`、`center`、`points`。`layer` 决定嵌套裁剪的顺序。

```json
{"type": "clip", "child": {"type": "text", "runs": [{"text": "很长的一行"}]}}
```

```json
{
  "type": "clipRect",
  "rect": {"x": 0, "y": 0, "width": 80, "height": 40},
  "child": {"type": "rectangle", "fill": "red"}
}
```

```json
{
  "type": "clipShape",
  "shape": {"kind": "circle", "radius": "50%"},
  "child": {"type": "image", "source": "portrait.png", "processing": "auto"}
}
```

```json
{
  "type": "clipPath",
  "path": {
    "commands": [
      {"op": "arc", "bounds": {"x": 0, "y": 0, "width": 64, "height": 64}, "start": 0, "sweep": 360},
      {"op": "close"}
    ]
  },
  "child": {
    "type": "image",
    "source": "portrait.png",
    "processing": "auto",
    "overrides": {"fit": "cover", "disableRed": true}
  }
}
```

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

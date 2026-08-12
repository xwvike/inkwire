# Gicisky / PICKSMART BLE 电子价签 — 协议与移植说明

本文是项目的唯一说明文档，记录当前驱动的构建与使用、字节级协议、编码规则和真机实验结论。

配套代码：`cmd/inkwire` + `internal/gicisky`（Go 传输层）以及 `internal/display`（Go 三色画布、状态栈、显示列表、字体、文字布局、PNG 预览与像素编码）。

本文档区分 **已在真机验证** 和 **未验证/推测**。请不要把推测部分当作事实来写代码。

---

## 1. 目标与边界

把任意内容推到一块 BLE 电子价签上，不使用厂商 App。

分层设计：

| 层 | 职责 | 状态 |
|---|---|---|
| 传输层 | BLE 连接、握手、分块上传、刷新 | Go 驱动已实现并真机验证 |
| 编码层 | 图像 → 双位平面字节串 | 驱动外；规则已真机验证 |
| 内容层 | 生成要显示的画面 | 驱动外 |

传输层对 payload 内容完全无知。当前只保证手里这块 296×128 标签可用，不据此承诺其它型号兼容。

### 构建与运行

项目使用 Go 1.23.8 或更高版本和 `tinygo.org/x/bluetooth`，不需要 Python 环境。Module path 为 `github.com/xwvike/inkwire`：

```bash
go test ./...
go build -trimpath -ldflags '-s -w' -o inkwire ./cmd/inkwire

./inkwire scan
./inkwire payload.bin
./inkwire PICKSMART payload.bin
```

默认目标是 `FF:FF:92:94:38:61`。macOS 无法通过公开 CoreBluetooth API 获取真实 MAC 时，驱动自动按 `PICKSMART` / `NEMR92943861` 精确匹配广播名称。命令行显式传入其它名称、MAC 或 CoreBluetooth UUID 后，只匹配该目标，不再回退到默认名称。当前本机构建产物为 macOS arm64，约 2 MB。

---

## 2. 硬件事实（已验证）

| 项 | 值 |
|---|---|
| 设备名 | `PICKSMART`；`NEMR92943861` 作为备用名称（macOS Go 驱动均已实测） |
| GATT DeviceName | `PICKSMART` |
| MAC | `FF:FF:92:94:38:61`（Public，不轮换，可硬编） |
| 面板 | 296×128 三色（黑/白/红） |
| SoC | Telink TLSR |
| 标签外壳印字 | `92.94.38.61`（= MAC 后 4 字节） |

### GATT 结构

```
Service 0xFEF0
├── 0xFEF1  "ext control"  — 命令写入 + Notify 来源（同一个特征）
├── 0xFEF2  "write block"  — 图像数据块写入
└── 0xFEF3               — 只读，返回 "DEPG..."（疑似面板型号，未读全）

Service 00010203-0405-0607-0809-0a0b0c0d1912
└── ...2b12  "OTAv1.0"    — Telink 标准 OTA，可刷第三方固件（未尝试，有变砖风险）
```

**注意**：Notify 订阅在 `FEF1` 上，不是单独的 notify 特征。命令和通知走同一个特征。

广播包里另有 5 字节厂商数据编码了屏幕规格（工具里叫 Raw Type，取 `末字节+首字节`）。本机实测值 `0032` 对应 296×128 BWR，`0030` 对应 296×128 BW。**这两个样本推出"bit1 = 三色标志"，样本量不足，不要据此推广到其它型号。**

---

## 3. 协议状态机（已验证，字节级）

全流程由 Notify 驱动。所有写入使用 **write with response**。

```
主机 → FEF1: 01
标签 → notify: 01 F4 00
                 └┬─┘
                  └ uint16 LE = 244，块消息大小
                    实际数据块 = 244 - 4 = 240 字节
                    （4 字节给块号）

主机 → FEF1: 02 <总长度 uint32 LE> 00 00 00     共 8 字节
             例：02 00 25 00 00 00 00 00   （0x2500 = 9472）
标签 → notify: 02 ...

主机 → FEF1: 03
标签 → notify: 05 00 <期望块号 uint32 LE>

  循环 {
    主机 → FEF2: <块号 uint32 LE><数据 240 字节>
    标签 → notify: 05 00 <下一块号>     继续
                或 05 08 ...            全部完成
                或 05 XX ...            出错，中止
  }
```

### 关键行为

- **块大小由标签决定**，不要写死，也不要按 MTU 推算。从 stage 1 响应读取。
- **严格 ACK 驱动**。ACK 与本地计数器一致时才前进到下一块；不一致时**原样重发上一块**，既不前进，也不跳到 ACK 指定的块号。禁止流水线并发。
- **末块允许不足长**。9472 ÷ 240 = 39 块整 + 112 字节，共 40 块（0x00–0x27）。
- **数据发完不代表结束**。最后一块送出后继续等待，直到收到 `05 08`。
- **收到 `05 08` 后标签主动断开连接**，这是正常流程，不是错误。
- **订阅 Notify 后不要立刻发 `01`。** macOS CoreBluetooth 真机实测，立即写入会丢失 `01 F4 00`；等待 2 秒后早期验证程序和 Go 正式驱动均能稳定完成全流程。
- 参考实现在 Notify 回调里延迟 50ms 再处理。Telink 缓冲区小，建议保留。

---

## 4. 像素编码（驱动外）

以下规则从已验证能出正确画面的 JS 实现中逐行提取。

### 4.1 平面顺序

payload = `[黑白平面][红平面]`，各 `296 × 128 / 8 = 4736` 字节，共 9472 字节。

### 4.2 位规则

- MSB first，`bitPosition` 从 7 递减到 0
- **黑白平面**：`0.2126R + 0.7152G + 0.0722B > 128` → 置 1（**1 = 白，0 = 黑**）
- **红平面**：`R > 170 且 G < 170` → 置 1

### 4.3 扫描顺序 ⚠️

canvas 为 **width=128, height=296**。原实现的取值索引是：

```javascript
for (i = 0; i < canvas.width; i++)        // 0..127
  for (x = 0; x < canvas.height; x++)     // 0..295
    idx = ((i * canvas.height) + x) * 4;  // = (i * 296 + x) * 4
```

这个式子按 canvas 真实行主序寻址是对不上的（常规公式应为 `(y * 128 + x) * 4`），但原网页按该布局能正确刷屏。

### 真机验证结果

以肉眼期望的横向画面 `296×128` 为起点：

1. 将画面**逆时针旋转 90°**，得到编码画布 `128×296`。
2. 按编码画布的行主序依次取像素。
3. 每 8 个像素按 MSB first 打包，再依次拼接黑白平面和红平面。

三种排列已经在同一块标签上实测：

| 输入排列 | 真机结果 |
|---|---|
| 直接打包 `296×128` 常规行主序 | 斜切乱码 |
| 主对角线转置为 `128×296` | 画面清晰但左右镜像 |
| 逆时针旋转 90° 为 `128×296` | **方向与文字均正确** |

因此编码层不要直接打包横向画面的行主序，也不要使用主对角线转置。Pillow 对应变换为 `image.transpose(Image.Transpose.ROTATE_90)`。该规则属于编码层，不应写进 Go 传输驱动。

### 4.4 Dithering 与抗锯齿：按内容选择

Dithering 属于画面生成策略，不是协议要求，也没有适用于所有内容的固定开关。参考实现使用 Floyd–Steinberg 误差扩散，其价值和副作用取决于画面：

- 照片、渐变和需要模拟中间灰阶的图形可能从 Dithering 中受益。
- 小字号文字、细线和纯色 UI 中，误差扩散可能把连续笔画打散成点阵，关闭后通常更干净。
- 图文混排可以分区域处理：照片区域抖动，文字和图标保持纯色，而不是整张画面共用一个策略。
- 是否启用应保留为上层选项，由用户根据本地 1-bit 预览和真机效果决定。

抗锯齿同样不是简单的开或关。灰色边缘经过 1-bit 阈值后可能丢失细笔画或使相邻笔画粘连，但不同字体、字号、hinting 和阈值会产生不同结果。单色栅格化是当前文字测试中较稳定的基线，不代表以后无需继续比较其它渲染方式。

### 4.5 小字号中文（已真机验证）

在这块 296×128、1-bit 黑白平面上，使用 macOS 字体和 Pillow 当前渲染流程，对 11px、13px、14px、15px 中文以及灰度阈值、4 倍超采样、Floyd–Steinberg、红色文字和多种字体做了对比。本节只记录这轮候选的结果，不把它当作面板或中文显示的最终极限：

- 在已测试组合中，15px 笔画完整；14px 是当前稳定可用的最小字号；13px 已开始缺笔；11px 仅能勉强辨认。
- 14px 配 18px 行高，可在 128px 高度内稳定排 7 行。
- `Hiragino Sans GB W3`（字体集合 index 0）+ Pillow 单色栅格化只是在本轮有限候选中表现最好：笔画较完整，但局部笔画位置并不总是理想，不能称为最优字体方案。
- 早期 128×128 手机能够显示更好的小字号中文，说明分辨率本身不能解释当前效果；针对小像素网格设计的字形、人工 hinting 和点阵取舍仍有明显优化空间。
- 1-bit 墨水屏确实无法保留 ClearType 的 RGB 亚像素信息，但仍可继续测试专用 12×12、14×14、16×16 中文点阵字库、包含 bitmap strike 的字体、不同 FreeType hinting 模式以及逐字像素修正。
- 灰度抗锯齿、阈值、超采样和误差扩散在本轮样本中没有超过单色 W3；这只是实测记录，不排除其它字体或参数组合得到更好结果。

macOS 上的已验证 Pillow 配置：

```python
font = ImageFont.truetype(
    "/System/Library/Fonts/Hiragino Sans GB.ttc",
    14,
    index=0,
)
draw.fontmode = "1"
draw.text(position, text, font=font, fill=0)
```

该配置用于复现当前基线，不是最终字体规范。字体路径是 macOS 特有配置；后续无论继续使用 macOS 还是迁移到 Linux，都应优先寻找专为小字号屏幕设计、带点阵或强 hinting 的 CJK 字体，并重新真机比较。

后续中文方案改为 HZK12 + Monaco 混排，见 4.7；本节保留作为历史记录和 Hiragino 路线的参考基线。

### 4.6 小字号英文与数字（已真机验证）

英文和阿拉伯数字优先使用 `Monaco` 10px 单色栅格化。该字体在 10px 下仍能稳定区分小写字母、数字和标点，且等宽排列适合时间、日期、地址、日志与状态数据：

```python
font = ImageFont.truetype(
    "/System/Library/Fonts/Monaco.ttf",
    10,
)
draw.fontmode = "1"
```

- 正文行高使用 14px，128px 高度内可放置 8 行正文和一条 16px 标题栏。
- 重要数值或标题可使用 Monaco 11–12px，正文默认保持 10px。
- 当前 MO10 基线使用整数坐标和单色栅格化；其它视觉方案仍可按第 4.4 节单独比较。
- Monaco 是 macOS 系统字体；迁移到 Linux 时需要重新验证可部署的等宽点阵字体，例如 Terminus。

### 4.7 中文字体决定：HZK12 + Monaco 混排（已真机验证）

设备不在身边时，用本机预生成 PNG 对比了 Hiragino 14px 基线、Zpix、Cubic 11，以及取自 1980 年代 GB2312 国标点阵字库的 HZK16、HZK12，并用高密度笔画字（餐/罐/蟹/藏/酱，13–24 画）做了压力测试。结论：**中文改用 HZK12，与 Monaco 10px 混排**，替代上一节的 Hiragino 方案。

HZK12 + Monaco 10px 已在同一块标签上完成真机验证：9 行、14px 行高的中英数字混排正常，复杂字仍可辨认，用户确认显示效果几乎与本地逐像素预览一致。测试 payload 为 9472 字节，完成 40/40 分块并收到最终刷新确认。

**为什么是 HZK12**：压力测试下 HZK12 和 Zpix 基本战平，餐/罐/蟹/藏/酱 都没有糊成一团；但 HZK12 摆到和已选定的 Monaco 数字方案同一行时观感更协调。Cubic 11、HZK16 在更早一轮已经出局，未再比较。

**数据来源与取字方式**：HZK12 是原始二进制点阵数据，不是 TrueType，Pillow 读不了，需要按 GB2312 区位码自己算字节偏移：

```python
raw = ch.encode("gb2312")                  # 2 字节
b1, b2 = raw[0], raw[1]
index = (b1 - 0xA1) * 94 + (b2 - 0xA1)
offset = index * 24                        # 12×12，每行 2 字节 × 12 行
glyph = hzk12_bytes[offset : offset + 24]  # MSB first，逐行取位
```

文件来自 GitHub `aguegu/BitmapFont` 仓库的 `font/HZK12`（196,272 字节 = 87 区 × 94 位 × 24 字节，无文件头）。此前使用的 `zfj-hash/BitmapFont` 镜像与其 Git blob SHA 完全一致：`877acf27cf08376ec3635c9f9603554d70d67734`。项目接受上游 GPL-3.0-or-later 及历史字体来源风险，选中的字体文件、上游 `COPYING`、固定提交与来源记录由 `internal/display/fonts` 保存；七个内嵌字体文件的摘要记在 `fonts/SOURCE` 并由 `TestEmbeddedFontsMatchRecordedDigests` 强制校验，新增文件未登记摘要会直接测试失败。这一选择对整个项目许可的影响见第 8 节。

**已知坑**：半角 `¥`（U+00A5）不在 GB2312 字符集里，`encode("gb2312")` 会直接报错；必须用全角 `￥`（U+FFE5）取字，否则容错逻辑可能悄悄换成空白字形，价格符号会无声消失，不是渲染 bug。

**同类坑：区位码在范围内不等于字库有这个字。** 用 GBK 解码器建立「Unicode → 区位索引」映射时，会把若干 GBK 相对 GB2312 的扩展字一起收进来，而 1980 年代的 HZK 文件在那些槽位上是全零。三个字库对这些扩展槽的填法还不一致，实测：

- `€`（U+20AC，GBK 的 A2E3）：HZK12/14/16 **全部**为空。
- `ⅰ`–`ⅹ`、`︵︶﹁﹂` 等竖排标点、`┆┇┈┉┊┋` 虚线制表符，共 35 个：**只有 HZK16 为空**，12/14px 有字。

后果是同一串文字在 12/14px 正常、切到 16px 无声消失。因此 `BitmapFace.Glyph` 现在把全零记录当作「本字库没有这个字」，让它走缺字叉框并进入 `TextLayout.MissingRunes`；唯一的例外是本来就该空白的半角空格和全角空格 `　`（U+3000）。ASCII strike 的 0x00–0x1F 控制字符同理，现在也会显式报缺字而不是画一格空白。

推 payload 前检查 `MissingRunes()` 是目前唯一能在上机前发现缺字的手段，别让它撒谎。

**与 Monaco 混排**：一行文字里 `ord(ch) < 128` 的字符走 Monaco 10px（Pillow `anchor="ls"`，基线定在这一行往下第 10px 处），其余字符走 HZK12（12×12 格子里从左上角逐位铺满）。行高统一 14px（12px 字身 + 2px 行距），128px 面板高度可排 9 行。

**已知不足**：HZK12 是 1980 年代印刷宋体点阵，笔画比 Zpix 更"老气"，这是预期的风格取舍，不是缺陷。

**原生字号**：为了保持整套界面的字形一致，只内嵌标准 HZK12、HZK14、HZK16，不纳入 HZK16F/S 和 24px 以上的宋/黑/楷/仿宋变体。`ui` 字体族提供 12/14/16px 中文档位，分别配 Monaco 10/12/14；Monaco 16 另作大号英文和数字。没有原生档位的字号默认报错，不静默缩放。

**试过没用上的候选**：Ark Pixel Font（`TakWolf/ark-pixel-font`）12px monospaced 的 zh_cn 字库缺字（用 fontTools 查 cmap 确认缺"餐""酱"），价签场景直接淘汰；Dotted Songti（`wixette/dotted-chinese-fonts`，文泉驿矢量化而来）是大尺寸展示用的矢量点阵字体，12–16px 下直接单色栅格化会碎成噪点，不适合这套流程。方正除了国标点阵（即 HZK 系列）外，没找到专门为像素网格设计的字体。

### 4.8 待验证：红色区域的黑平面

纯红 `(255,0,0)` 亮度 = 54 < 128，因此在黑白平面被置为 **黑**，同时红平面置 1，即红色下方压着一层黑。

小面积红字实测正常显示。**大面积纯红是否会因此偏暗发闷，未验证。** 若出现问题，尝试在红平面为 1 的位置强制把黑白平面置为白，对比效果。

### 4.9 Go 显示底层

`internal/display` 使用只包含黑、白、红三种语义颜色的 `Frame` 作为稳定中间结果：文字、图形和图片先绘制到 Frame，再由设备编码器完成旋转和双位平面打包。

- `TextStyle` 通过字体族和整数像素字号选择原生 strike；`TextRun` 支持同一文字块内的多字号与黑/红混排。
- `ui` 字体族把普通 ASCII 路由到 Monaco strike，把中文和全角字符路由到对应 HZK；原始 HZK 文件本身不包含半角 ASCII。字库中全零的槽位一律按缺字处理，见 4.7 的说明。
- `Canvas` 使用整数像素和半开矩形坐标，可绘制矩形、带宽度直线、折线、圆、椭圆、圆角矩形、圆弧、扇形、弓形及填充/描边多边形；所有描边统一使用 `StrokeStyle`。
- `Canvas.Save` / `Restore` 保存和恢复当前状态，`ClipRect` 叠加矩形裁剪，`ClipPath` 叠加任意路径裁剪（even-odd，嵌套轮廓可挖孔），`Translate` 提供整数平移。裁剪建立后固定在 Frame 坐标中；之后继续平移不会移动已有裁剪区域。`Clip(rect)` 仍可创建共享 Frame 的子 Canvas，子 Canvas 继承当时的平移和裁剪，但拥有独立状态栈，不会污染父 Canvas。
- 路径裁剪以掩膜形式挂在裁剪状态上，判定发生在 `Canvas.Set`，因此图元、文字和图片一律遵守，无需各自感知。掩膜安装后不再修改，保存的状态和子 Canvas 可以安全共享同一份。
- 闭合形状的描边一律**内对齐**，绝不画到对应填充之外；开放路径（直线、折线、未闭合轮廓、不足 360° 的圆弧）保持**居中**。矩形、圆角矩形、圆、椭圆用自身的双尺寸隐式判定得到环带，多边形和路径用「填充减去腐蚀」得到环带，两种方式都由构造保证描边 ⊆ 填充。虚线只决定环带上哪些部分被绘制，不移动环带位置。
- 闭合形状的虚线按**真实弧长**计量（像素投影到最近的轮廓点），因此斜边上的实线段长度与水平边一致。开放路径的虚线仍按栅格步数计量，45° 上会偏长 √2，尚未统一。
- `StrokeStyle.Dash` 以栅格像素数交替描述开/关段，`DashOffset` 控制起点；虚线状态沿整条折线和闭合轮廓连续，不在拐点重新开始。奇数项 pattern 会按标准做法重复一遍后循环，非正数 pattern 视为无效描边。
- 通用 `Path` 支持 `MoveTo`、`LineTo`、`QuadraticTo`、`CubicTo`、椭圆弧和 `Close`。曲线绘制时按最多 1px 步长压平到整数网格；多个轮廓统一使用 even-odd 规则填充，因此可以直接形成孔洞。
- 标准圆和圆环必须优先使用专用 `FillCircle` / `StrokeCircle`：`StrokeCircle` 支持任意正整数像素宽度，1–6px 已逐级做连通性和轴向厚度测试。Path 的多轮廓孔洞用于任意复杂轮廓，不用它代替精确圆栅格。
- 圆弧角度遵循屏幕坐标：0° 向右，正角度顺时针；绝对值达到 360° 的 sweep 视为完整椭圆。图元不做抗锯齿，只生成确定性的黑、白、红像素；粗线固定使用方形笔刷、方形端点和实心连接。
- 图元层明确不承担灰度、渐变、阴影、模糊、任意浮点变换或通用矢量排版。需要的新轮廓由 `Path` 表达，裁剪后的子 Canvas 与父 Canvas 共享同一个 Frame。
- 每个图片节点独立选择 `stretch` / `contain` / `cover`、nearest / bilinear，以及 threshold / Floyd–Steinberg / ordered Dithering，不存在强制全局开关。
- `DisplayList` 记录与 Canvas 对应的状态、文字、图片和图元命令，可查询命令数与实际逻辑边界，支持 `Replay`、`Clone` 和 `Reset`。重放复用同一组确定性栅格化代码，并且不会改变目标 Canvas 的状态栈、平移或裁剪。
- DisplayList 在记录时复制点集、虚线 pattern 和 Path，并把 `image.Image` 转成内部 NRGBA 快照；因此调用方在记录后修改原切片、Path 或图片不会改变重放结果。已经测量的 `TextLayout` 对外不可变，可安全复用。
- 横屏逻辑画布为 296×128；竖屏为 128×296，可明确选择顺时针或逆时针映射到物理面板。设备编码器仍统一执行协议所需的逆时针旋转和位打包。
- 字体、文字、图元、图像、Canvas 状态和 DisplayList 都有 Go 单元测试；测试覆盖直接绘制与重放逐像素一致、可变输入快照、Clone 独立性、边界计算及状态下溢。运行时不调用 Python 或浏览器。任意角度旋转、缩放和通用布局树仍不属于这一层。
- `examples/showcase/showcase.png` 是 296×128 图元与文字综合展示图；生成器现通过 DisplayList 重放产生画面，并逐字节比对已真机确认的基准 PNG。运行 `go run ./examples/showcase -png examples/showcase/showcase.png -payload /tmp/inkwire-showcase.bin` 可重新生成 PNG 和真机 payload。
- `examples/state_showcase/state_showcase.png` 是 Canvas 状态和 DisplayList 专项展示图：画面直接标出 `CLIP RECT`、`TRANSLATE`、`SAVE / RESTORE`、命令数与逻辑边界，生成过程还实际执行 `Clone`、`Reset` 和 `Replay`。2026-08-12 已通过 Go 驱动上传真机，9472 字节 payload 完成 40/40 分块并进入刷新。

---

## 5. 已验证可用的参考配置

参考页面（atc1441 uploader）中这组参数在本机成功刷屏：

```
Type:         296x128 BWR
Raw Type:     0032
Width/Height: 128 / 296
Dithering:    按内容选择（网页成功配置曾开启，见 4.4）
Compression:  ✗
Second Color: ✓
Mirror:       ✗
```

**Compression 必须关闭。** 开启时走的是另一套 RLE 打包格式（`0x75` 行头），本机未验证标签是否支持，实测开启时屏幕无反应。

---

## 6. 已知坑

1. **刷屏期间拒绝连接。** 收到 `05 08` 后标签断开并刷新面板，此时连接会成功建立 GATT server 但立刻断开。实测需重试 2–3 次、间隔 5 秒才能重连。驱动中重试次数应放宽到 5 次以上。
2. **连接间隔激进。** 广播中 `SlaveConnectionIntervalRange = 08-00-08-00`，即固定 10ms。对蓝牙适配器要求较高，劣质 dongle 可能传一半掉线。
3. **USB 3.0 干扰 2.4GHz。** 若用 USB 蓝牙 dongle，插 USB 2.0 口或用延长线拉离机箱。
4. **不需要配对。** 无绑定 GATT 连接，执行 pair 反而可能失败。
5. **假 CSR8510 dongle。** 市面上大量克隆芯片，表现为随机掉线。推荐 RTL8761B（需 `firmware-realtek`）。
6. **墨水屏刷新慢。** 单次全刷十几秒，纽扣电池供电。合理刷新频率是分钟级到小时级，不要按秒轮询。
7. **macOS 返回 CoreBluetooth UUID，不是公开 MAC。** 每次上传/重试都重新扫描；若地址无法匹配固定 MAC，则按 `PICKSMART` / `NEMR92943861` 精确匹配名称，并使用当次扫描结果中的 CoreBluetooth UUID 连接。不要跨机器保存日志里的 UUID。

---

## 7. 当前状态与待办

**已完成**
- 协议逆向完成，参考实现真机验证通过
- Go 驱动已在 macOS arm64 + `tinygo.org/x/bluetooth` v0.15.0 真机验证：完成 9472 字节、40 个数据块上传并收到 `05 08`
- Go 渲染层已实现三色 `Frame/Canvas`、Canvas 状态栈与整数平移/裁剪、不可变 `DisplayList`、内嵌标准 HZK12/14/16 与 Monaco 10/12/14/16 strike、GB2312 严格取字、富文本 runs、原生字号匹配、换行/对齐、缺字叉框、图片缩放与逐节点 Dithering、横竖屏、PNG 预览和 Gicisky payload 编码
- `go test ./...` 覆盖正常上传、异常 ACK 重发、截断握手、首次异常块号与空 Payload；`go vet ./...` 通过
- 当前纯传输命令构建为约 2 MB；显示命令接入后，内嵌约 0.7 MB 的字体资源会进入同一个二进制，运行时仍不依赖 Python、Pillow、Bleak 或虚拟环境
- 黑白文字 payload 的方向已真机验证：横向画面逆时针旋转 90° 后打包可正确显示
- 14px `Hiragino Sans GB W3` 单色栅格化已真机验证：18px 行高可排 7 行；默认正文已改为真机验证过的 HZK12 + Monaco 10（见 4.7）
- 10px `Monaco` 单色栅格化已真机验证：14px 行高适合作为英文和数字的默认正文配置

- 源码已发布到 `github.com/xwvike/inkwire`，module path 与仓库地址一致，根目录 `LICENSE` 为 GPL-3.0-or-later（见第 8 节）

**待办（按优先级）**
1. 肉眼确认标准 HZK16 与 Monaco 14、独立 Monaco 16 的基线和建议行高。
2. 在当前 `Canvas + TextLayout + DisplayList` 底层之上实现布局树、序列化接口和调度层。
3. 准备迁移主机时，在 Linux/BlueZ 上验证固定 MAC 直连与 Notify 行为。

**未尝试**
- 读取 `FEF3` 完整值确认面板型号
- 压缩模式（`0x75` RLE 格式）
- Telink OTA 刷第三方固件

---

## 8. 许可证

本项目整体按 **GPL-3.0-or-later** 分发，根目录 `LICENSE` 为 GPL-3.0 官方全文。

选择 copyleft 的原因是字体：`internal/display/fonts` 下的 HZK12/14/16 取自 `aguegu/BitmapFont`，上游声明 GPL-3.0-or-later，而这些数据通过 `go:embed` 直接编进了 inkwire 二进制。为了保住「单二进制、运行时零外部依赖」这条设计线，选择让整个项目跟随上游许可，而不是把字体拆成运行时加载的外部文件。

- 根目录 `LICENSE` 与 `internal/display/fonts/COPYING` 是同一份 GPL-3.0 文本，sha256 `8ceb4b9ee5adedde47b31e975c1d90c73ad27b6b165a1dcd80c7c545eb65b903`。
- `internal/display/fonts/SOURCE` 记录每个字体文件的上游仓库、固定 commit 与 SHA-256。

**未清理的风险**：MONACO10/12/14/16 是从 macOS `/System/Library/Fonts/Monaco.ttf` 生成的 1-bit ASCII strike，属于 Apple 系统字体的衍生数据，随本仓库一起分发的许可状态**没有核实过**，和 4.7 记录的 HZK 历史来源风险是两件独立的事。若要正经分发，替换为明确可再分发的等宽点阵字体（例如 Terminus）是更干净的做法，这件事也和第 7 节里 Linux 迁移那条待办撞在一起。

---

## 9. 参考

- 协议逆向与参考实现：https://github.com/atc1441/ATC_GICISKY_ESL
- 在线上传工具（源码即页面，可直接 curl）：https://atc1441.github.io/ATC_GICISKY_Paper_Image_Upload.html
- Go BLE 库：https://github.com/tinygo-org/bluetooth
- 另一 Python 实现（仅在 250×122 BWR 上测过）：https://github.com/fpoli/gicisky-tag
- Home Assistant 集成：https://github.com/eigger/hass-gicisky

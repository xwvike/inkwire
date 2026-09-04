# Markup 手册

Markup 使用 HTML 表达内容，CSS 表达布局和绘制，SVG 表达几何图形。`render`、`measure` 和
HTTP `render` 必须指定 `size` 或 `panel`；`push` 从目标设备获取面板尺寸和墨色。
根元素可以省略 `width` 和 `height`，它们只参与 CSS 布局，不决定渲染目标。根元素的
`orientation` 支持 `landscape`、`portrait-cw` 和 `portrait-ccw`，默认值为 `landscape`。

## 支持的 CSS 属性

属性未列出取值时，该取值不实现并产生 `unsupported-declaration`。表中的简写属性保留 CSS
的语法，但仅接受表中规定的子项。

| 类别 | 属性 | 支持的取值或语法 | 注意 |
|---|---|---|---|
| 盒模型 | `display` | `block`、`inline`、`inline-block`、`flex`、`inline-flex`、`grid`、`inline-grid`、`contents`、`none` | `inline-*` 建立对应的原子盒；`contents` 不生成自身盒子 |
| 尺寸 | `width`、`height`、`min-width`、`max-width`、`min-height`、`max-height`、`flex-basis` | `px`、`%`、`calc()`、`auto`（按属性语义） | 百分比按包含块解析；`flex-basis` 参与 flex 初始尺寸 |
| 比例 | `aspect-ratio` | `number` 或 `number / number` | 两个数必须为正数 |
| 盒模型 | `box-sizing` | `content-box`、`border-box` | 影响 `width`、`height`、`min-*`、`max-*` 和 `flex-basis` 的尺寸含义 |
| 内边距 | `padding`、`padding-top`、`padding-right`、`padding-bottom`、`padding-left` | 非负 `px`、`%`、`calc()` | 百分比按包含块行向尺寸解析，在布局时取整 |
| 外边距 | `margin`、`margin-top`、`margin-right`、`margin-bottom`、`margin-left` | `px`、`%`、`calc()`、`auto` | `auto` 用于可用空间分配；百分比按包含块行向尺寸解析 |
| Flex | `flex` | `none`、`auto`、`<grow>`、`<grow> <shrink>`、`<grow> <shrink> <basis>` | `<grow>` 的单值形式使用 `0%` basis；`<basis>` 支持 `auto`、`px`、`%`、`calc()` |
| Flex | `flex-direction` | `row`、`column` | — |
| Flex | `flex-grow`、`flex-shrink` | 非负数 | `flex-shrink` 默认值为 `1` |
| 间距 | `gap`、`row-gap`、`column-gap` | 非负 `px`、`%`、`calc()` | `row-gap` 百分比按包含块的块向尺寸解析，`column-gap` 按行向尺寸解析；`gap` 的第一个值为 row-gap、第二个值为 column-gap；布局时取整 |
| 对齐 | `align-items`、`align-self`、`justify-items`、`justify-self` | `stretch`、`flex-start`、`start`、`center`、`flex-end`、`end`；`align-self` 和 `justify-self` 另支持 `auto` | `justify-*` 使用相同的对齐值集合 |
| 对齐 | `justify-content` | `flex-start`、`start`、`normal`、`center`、`flex-end`、`end`、`space-between` | `space-around`、`space-evenly` 等不实现 |
| Grid | `grid-template-columns`、`grid-template-rows` | `px`、`%`、`calc()`、正数 `fr`、`auto`、`min-content`、`max-content`、`repeat()` | `repeat()` 的轨道数上限为 400 |
| Grid | `grid-column`、`grid-row` | `auto`、行号、`span N`、`N / M`、`N / span M` | 行号和跨度必须为正整数 |
| 定位 | `position` | `static`、`relative`、`absolute` | 不实现 `fixed` 和 `sticky` |
| 定位 | `top`、`right`、`bottom`、`left`、`inset` | `px`、`%`、`calc()`、`auto` | 百分比按定位包含块解析 |
| 定位 | `z-index` | 数字 | 仅改变已定位元素的绘制层级 |
| 颜色 | `background`、`background-color`、`color`、`border-color`、`border-top-color`、`border-right-color`、`border-bottom-color`、`border-left-color` | `black`、`white`、`red`、`yellow`，及对应三位或六位十六进制 | 不支持透明度、函数颜色和其他命名色；未支持颜色不会近似 |
| 边框 | `border`、`border-top`、`border-right`、`border-bottom`、`border-left` | 一个宽度、线型和颜色，顺序任意 | 线型省略时为 `none`，不会绘制 |
| 边框 | `border-width`、`border-top-width`、`border-right-width`、`border-bottom-width`、`border-left-width` | 非负 `px` 或仅含像素的 `calc()` | 布局时取整；不接受百分比或多值边框宽度 |
| 边框 | `border-style`、`border-top-style`、`border-right-style`、`border-bottom-style`、`border-left-style` | `solid`、`dashed`、`dotted`、`none`、`hidden` | `dotted` 的点和间距等于线宽 |
| 边框 | `border-radius` | 非负 `px` 或仅含像素的 `calc()` | 布局时取整；不接受百分比或椭圆双值 |
| 可见性 | `visibility` | `visible`、`hidden` | `hidden` 保留布局盒但不绘制 |
| 裁剪 | `overflow` | `visible`、`hidden`、`clip` | 不提供滚动 |
| 裁剪 | `clip-path` | `none`、`inset()`、`circle()`、`ellipse()`、`polygon()` | 函数参数支持 `px`、`%`、`calc()`；`inset()` 只支持单一圆角半径 |
| 变换 | `transform` | `none`、`scale()`、`rotate()` | 仅支持 `scale` 和 `rotate` 函数 |
| 变换 | `rotate` | `deg`、`grad`、`rad`、`turn` 或无单位角度 | — |
| 变换 | `transform-origin` | 一个或两个关键词、`px`、`%`、`calc()` | 关键词为 `left`、`center`、`right`、`top`、`bottom` |
| 变换 | `scale` | 一个或两个相同的整数，且不小于 1 | 不做重采样；不支持小数或非等比缩放 |
| SVG 绘制 | `fill`、`stroke` | 支持的墨色、`none`、`transparent` | 这些属性可继承；内联 SVG 中的 CSS 声明覆盖 SVG 呈现属性 |
| SVG 绘制 | `stroke-width` | 非负 `px`、仅含像素的 `calc()` 或 SVG 无单位数值 | 布局时取整；小于 1 像素时按 1 像素绘制并产生警告 |
| SVG 绘制 | `stroke-dasharray`、`stroke-dashoffset` | 空格或逗号分隔的整数像素 | — |
| 字体 | `font` | `size[/line-height] family` | 仅读取字号、行高和字体族；样式、粗细等字段会产生警告 |
| 字体 | `font-family` | `ui`、`hzk`、`monaco`，可写字体栈 | 使用第一个可用字体族；都不可用时使用默认字体并产生警告 |
| 字体 | `font-size` | `px` 或仅含像素的 `calc()` | 使用最近可用位图字号并产生 `substituted-font-size` |
| 文本 | `line-height` | `px`、仅含像素的 `calc()`、无单位比例、相对字号的百分比 | 无单位值按当前元素字号解析；百分比解析后继承计算值 |
| 文本 | `text-align` | `left`、`start`、`center`、`right`、`end` | — |
| 文本 | `vertical-align` | `baseline`、`top`、`middle`、`bottom` | 只作用于 inline 级盒；初始值为 `baseline`，不继承 |
| 文本 | `white-space` | `normal`、`nowrap`、`pre`、`pre-wrap` | — |
| 图片 | `object-fit` | `fill`、`contain`、`cover` | 只影响 `img` 和外部绘图的盒内适配 |

所有已实现属性接受 `inherit`、`initial`、`unset` 和 `revert`。`color`、`fill`、`stroke`、
`stroke-width`、`stroke-dasharray`、`stroke-dashoffset`、`font`、`font-family`、`font-size`、
`line-height`、`text-align`、`white-space` 和 `visibility` 默认继承；`vertical-align` 不继承。自定义
属性 `--name` 可声明并供 `var()` 使用，按相同规则层叠并继承。

## 不支持的属性和取值

未列入上表的 CSS 属性全部不实现。对已实现属性使用未列出的取值，结果相同：声明被忽略并
产生 `unsupported-declaration`。以下是常见的不支持项：

- 布局：`float`、`clear`、`order`、`flex-wrap`、`align-content`、`grid-template-areas`、
  `grid-auto-columns`、`grid-auto-rows`、`grid-auto-flow`、`place-content`、`place-items`、
  `place-self`、`table-layout`。
- 定位和效果：`position: fixed`、`position: sticky`、`opacity`、`filter`、`box-shadow`、
  `text-shadow`、`mix-blend-mode`、`background-image`、`background-repeat`、`background-size`。
- 文本：`font-weight`、`font-style`、`font-variant`、`text-decoration`、`text-indent`、
  `letter-spacing`、`word-spacing`、`text-transform`、`text-overflow`、`hyphens`。
- 图片：`object-position`。
- 颜色与变换取值：`rgb()`、`rgba()`、`hsl()`、渐变、透明度；`transform` 中除 `scale()` 和
  `rotate()` 以外的函数；CSS `scale` 的小数、负数或不等比双值；`border-style` 的 `double`、
  `groove`、`ridge`、`inset`、`outset`；`overflow: auto` 和 `overflow: scroll`。

## 使用注意

- `size` 或 `panel` 决定画布边界。根元素的 `width` 和 `height` 不会扩大或缩小该边界；超出
  画布的内容产生布局或裁剪警告。
- `box-sizing`、padding 和 border 都参与盒尺寸。绝对定位子元素相对于最近的已定位祖先的
  padding box，位于其 border 内侧、padding 外侧。
- Flex 简写保留 grow、shrink 和 basis 三个分量；`flex: 1` 是 `1 1 0%`。未指定 `flex-shrink`
  时初始值为 `1`。
- Inline、图片和 inline SVG 共用行框。`vertical-align` 仅对 inline 级盒有效；block、flex
  item 或 grid item 上的声明会产生警告。单行文本的盒内垂直对齐可使用与盒高相等的
  `line-height`。
- 样式来源顺序为 CLI 样式表、页面 `<style>`、按文档顺序加载的 `link` 样式表。普通声明按
  specificity 和来源顺序比较；行内样式高于普通选择器，`!important` 高于普通声明。自定义
  属性按相同规则层叠并继承；未定义或循环的 `var()` 会丢弃所在声明。
- 百分比内边距和外边距按包含块行向尺寸换算；`row-gap` 按块向尺寸、`column-gap` 按行向尺寸
  换算，均在布局时取整。边框宽度、圆角和字号只接受像素值或仅含像素的 `calc()`；`calc()`
  只支持 `px`、`%` 的加减。
- 字体来自构建时内置位图字库。未找到字体族使用默认字体；未找到对应字号使用最近字号。
- 页面会继续编译，但不支持的声明、选择器、at-rule、颜色、字体和资源都会产生警告。警告
  不是渲染失败标志，需根据类型检查最终画面。

## SVG

支持的元素：`rect`、`circle`、`ellipse`、`line`、`polyline`、`polygon`、`path`、`g`、
`clipPath`、`pattern`、`defs`、`title`、`desc`。支持 `path` 的 `M`、`L`、`H`、`V`、`C`、
`S`、`Q`、`T`、`A`、`Z` 及相对命令。

SVG 支持 `fill`、`stroke`、`stroke-width`、`stroke-dasharray`、`stroke-dashoffset` 和
`clip-path`。`viewBox` 按浏览器的 `xMidYMid meet` 规则映射到 viewport，viewport 外的图形
会被裁剪。SVG 的 `transform` 支持 `translate`、`scale` 和 `rotate`；`scale` 接受有限非零
数值，`rotate` 接受一个角度或角度加旋转中心。CSS 的 `transform` 仅支持本手册中列出的函数。

## 图片与资源

`img src` 支持 HTTP/HTTPS URL 和相对路径。`.png`、`.jpg`、`.jpeg` 按位图读取，`.svg` 按
外部 SVG 绘图读取。相对路径始终以页面资源映射为准，HTTP 服务不会读取客户端路径或服务端
当前目录中的任意文件。

`<img>` 中的 SVG 按替换型图片处理：页面 CSS 只控制图片盒的尺寸和裁剪，不进入 SVG 文档参与
级联；已支持的 SVG 绘制属性和 viewport 保留。内联 `<svg>` 仍参与页面 CSS 级联。

CLI 通过可重复的 `-asset SRC=FILE` 注入本地资源；未提供映射时，从页面所在目录读取：

```bash
inkwire render -size 296x128 \
  -asset assets/portrait.png=photos/portrait.png \
  -asset assets/chart.svg=charts/chart.svg page.html
```

HTTP 使用 multipart：`page` 为 HTML；本地资源作为文件字段上传，字段名必须与 `src` 完全一致，
包括目录前缀。远程 URL 不需要文件字段；`link` 的 `href` 遵循相同规则，stylesheet 可以作为
独立字段上传。

## 警告

Markup 可能产生以下警告：`text-clipped`、`layout-overflow`、`empty-layout`、`missing-runes`、
`size-mismatch`、`unsupported-ink`、`unsupported-declaration`、`unsupported-at-rule`、
`unresolved-drawing`、`unresolved-image`、`no-stylesheet`、`unresolved-stylesheet`、
`duplicate-stylesheet`、`over-constrained`、`unsupported-selector`、`unreadable-rule`、
`substituted-font-size`。

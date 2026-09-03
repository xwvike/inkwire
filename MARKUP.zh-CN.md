# Markup 手册

HTML 定义内容，CSS 定义布局和绘制，SVG 定义几何图形。CLI 的 `render`、`measure` 和 HTTP
`render` 必须指定 `size` 或 `panel`；`push` 从目标设备获取面板尺寸和墨色。
根元素可以省略 `width` 和 `height`。声明后，它们只参与 CSS 布局，不决定渲染目标。

根元素的 `orientation` 可以是 `landscape`、`portrait-cw` 或 `portrait-ccw`，默认是
`landscape`。

## 支持的属性

属性名仅表示属性已实现，不表示接受浏览器中的全部取值。超出下列范围的取值会产生警告并被忽略。

| 分组 | 属性 |
|---|---|
| 盒模型 | `display` `width` `height` `min-width` `max-width` `min-height` `max-height` `aspect-ratio` `box-sizing` `padding` `padding-top` `padding-right` `padding-bottom` `padding-left` `margin` `margin-top` `margin-right` `margin-bottom` `margin-left` |
| flex 与 grid | `flex` `flex-direction` `flex-basis` `flex-grow` `flex-shrink` `gap` `row-gap` `column-gap` `align-items` `align-self` `justify-content` `justify-items` `justify-self` `grid-template-columns` `grid-template-rows` `grid-column` `grid-row` |
| 定位 | `position` `top` `right` `bottom` `left` `inset` `z-index` |
| 绘制 | `background` `background-color` `color` `border` `border-width` `border-style` `border-color` `border-top` `border-right` `border-bottom` `border-left` `border-top-width` `border-right-width` `border-bottom-width` `border-left-width` `border-top-style` `border-right-style` `border-bottom-style` `border-left-style` `border-top-color` `border-right-color` `border-bottom-color` `border-left-color` `border-radius` `visibility` |
| 裁剪与变换 | `overflow` `clip-path` `transform` `rotate` `transform-origin` `scale` |
| SVG 绘制 | `fill` `stroke` `stroke-width` `stroke-dasharray` `stroke-dashoffset` |
| 文本 | `font` `font-family` `font-size` `line-height` `text-align` `vertical-align` `white-space` |
| 图片 | `object-fit` |

所有已实现属性均接受 `inherit`、`initial`、`unset` 和 `revert`。关键字、单位和函数名按 CSS
规则不区分大小写；字体族名同样如此，但警告按作者书写的原样回显。`color`、`font-family`、
`font-size`、`line-height`、`text-align`、`white-space` 和自定义属性会继承；其他属性从各元素的初始值开始。

### 取值

- 长度支持 `px`、百分比和 `calc()`。
- `width`、`height`、`min-*`、`max-*`、`flex-basis`、`top`、`right`、`bottom`、`left`、
  `inset`、`transform-origin` 和轨道尺寸接受百分比。
- 内边距、外边距和间隙按包含块的行向尺寸在布局时解析并取整。边框宽度、圆角和字号
  只接受像素值。
- `line-height` 接受像素长度、无单位比例，或相对于字号的百分比。
- 墨色支持 `black`、`white`、`red`、`yellow` 及其三位或六位十六进制写法（如 `#000`、`#000000`）。
  其他颜色会被报告，不会近似成可用颜色。
- 字体族受构建内置字库限制。未找到字体族时使用默认字体并报告
  `unsupported-declaration`；字号无对应位图字形时使用最近字号并报告 `substituted-font-size`。

### 层叠与继承

样式表按来源顺序合并：CLI 传入的样式表在前，页面中的 `<style>` 和 `link rel="stylesheet"`
按文档顺序追加。同一元素的普通声明按选择器优先级排序，优先级相同则后声明生效；普通行内
`style` 声明优先于普通选择器声明。`!important` 声明优先于所有普通声明，并在 important 声明
之间按相同规则比较。

自定义属性按相同规则层叠并从父元素继承。`var()` 在继承链上解析自定义属性；未定义或循环引用
会产生警告并忽略使用它的声明。

### 显示与布局

`display` 支持 `block`、`inline`、`inline-block`、`flex`、`inline-flex`、`grid`、
`inline-grid`、`contents` 和 `none`。

Flex 容器的 `flex-direction` 支持 `row` 和 `column`。`flex-grow` 分配剩余空间，
`flex-shrink` 吸收不足空间，`flex-basis` 设置初始尺寸。`flex` 简写支持 CSS 的
`flex: <grow>`、`<grow> <shrink>` 和 `<grow> <shrink> <basis>` 形式；单数字形式使用
`0%` basis，与浏览器一致。`gap`、`row-gap` 和 `column-gap` 分隔子项。`align-items`、
`align-self`、`justify-content`、`justify-items` 和 `justify-self` 控制对齐。
`margin: auto` 可以把子项推到容器另一侧。

Grid 容器使用 `grid-template-columns` 和 `grid-template-rows` 定义轨道，支持像素、百分比、
`fr` 和 `auto`。`grid-column` 和 `grid-row` 放置子项，使用 `span` 指定跨越范围。

`overflow: hidden` 把内容裁剪在盒子内。`white-space: pre` 保留连续空格，默认会合并
空白字符。

### 盒模型

盒模型遵循 CSS；尺寸、padding 和 border 均参与布局。

```css
.card { width: 100px; padding: 10px; border: 5px solid black; }   /* 宽 130 */
.same { width: 130px; padding: 10px; border: 5px solid black;
        box-sizing: border-box; }                                 /* 宽 130 */
```

`box-sizing` 默认是 `content-box`：`width`、`height`、`min-*`、`max-*` 和 `flex-basis` 表示
内容尺寸，padding 和 border 追加在外部。`border-box` 使这些属性表示整个盒子，内容使用剩余空间。

边框占据空间。内容位于边框和 padding 之内；绝对定位的子元素相对于内边距盒定位，即边框
以内、padding 以外的区域。

`border-top`、`border-right`、`border-bottom` 和 `border-left` 可分别设置。单边边框参与盒模型，
支持与 `border` 相同的宽度、墨色和线型。

### 定位

```css
.page { position: relative; }
.badge { position: absolute; right: 4px; top: 4px; }
.hint { position: relative; left: 2px; top: -1px; }
```

`position: relative` 保留元素在正常流中的位置，并按 `top`、`right`、`bottom`、`left` 或 `inset`
偏移绘制盒。`position: absolute` 将元素移出正常流，并相对于最近的已定位祖先放置。`z-index`
控制已定位元素的绘制顺序。

### 绘制与变换

`background` 和 `border` 接受上述墨色。`border-style` 支持 `solid`、`dashed`、`dotted`、`none`
和 `hidden`；`dotted` 的点和间距均等于边框宽度。`border-radius` 绘制圆角；`visibility: hidden`
保留盒子但不绘制内容。

`border-style` 的初始值是 `none`；未指定线型时，`border: 1px` 和单独的 `border-width` 均不绘制。
需要多条线或多种明暗的线型（`double`、`groove`、`ridge`、`inset`、`outset`）会产生警告并跳过。

`transform` 支持 `rotate` 和整数倍 `scale`。`rotate` 接受角度，`transform-origin` 控制
旋转中心。不支持的变换函数会被报告。

### 盒子里的文字

`line-height` 按 CSS 规则计算。它与文字高度的差值为 leading，平均分配到行框上下；增大
`line-height` 会扩大行框，不会单独向下移动文字。每行依据自身字体指标定位。

Inline 内容按行框排版。文本、`inline-block`、`inline-flex`、`inline-grid`、图片和 inline
SVG 共用行框，并在 `white-space` 允许时换行。padding、margin、background、border、
`position: relative` 和 `vertical-align` 均作用于各自的 inline 项。

`vertical-align` 支持 `baseline`、`top`、`middle` 和 `bottom`，初始值为 `baseline`。
`text-top`、`text-bottom`、`sub`、`super` 和长度值会产生警告并被忽略。该属性只作用于
inline 级盒子，不继承；用于 block、flex item 或 grid item 时会产生警告。

`inline-block`、`inline-flex` 和 `inline-grid` 是原子盒子，盒内子元素分别按普通
block、flex 或 grid 规则排版。

要在盒内对齐内容，使用容器的 `align-items`、项目的 `align-self`，或为单行文本设置与盒高
相等的 `line-height`。

```css
.chip { display: inline-block; height: 20px; vertical-align: middle; }

.row    { display: flex; align-items: center; }   /* 每一项都居中 */
.figure { align-self: center; }                   /* 只让这一项居中 */
.label  { height: 20px; line-height: 20px; }      /* 单行文字在盒子里居中 */
```

## SVG

```html
<svg viewBox="0 0 214 74">
  <polyline points="0,8 6,2 12,8" />
</svg>
<img src="chart-plot.svg" />
```

支持的元素是 `rect`、`circle`、`ellipse`、`line`、`polyline`、`polygon`、`path`、`g`、
`clipPath`、`pattern`、`defs`、`title` 和 `desc`。支持的绘制属性是 `fill`、`stroke`、
`stroke-width`、`stroke-dasharray`、`stroke-dashoffset` 和 `clip-path`。

`path` 支持 `M L H V C S Q T A Z` 及其相对命令。支持 `translate` 和整数倍 `scale` 变换。
同一属性同时出现在 SVG 呈现属性和 CSS 中时，CSS 优先。
`fill` 和 `stroke` 仅接受上述墨色、`none` 和 `transparent`；其他颜色会产生警告并跳过该绘制。

## 图片与文件

`img` 通过 `src` 引用资源。`src` 可以是 HTTP/HTTPS 链接或相对路径。`.png`、`.jpg` 和
`.jpeg` 按位图加载，`.svg` 编译为外部 SVG。

```
page.html
assets/
  portrait.png
  chart.svg
```

```html
<img src="assets/portrait.png" class="portrait" />
<img src="assets/chart.svg" class="chart" />
<img src="https://example.com/portrait.png" class="remote" />
```

CLI 使用可重复的 `-asset SRC=FILE` 注入本地资源：

```bash
inkwire render \
     -size 296x128 \
     -asset assets/portrait.png=photos/portrait.png \
     -asset assets/chart.svg=charts/chart.svg \
     page.html
```

未指定映射时，CLI 从页面目录读取相对资源。HTTP 使用 multipart：`page` 是 HTML；本地资源
作为二进制文件字段上传，字段名必须与 `src` 完全一致，包括目录前缀。HTTP/HTTPS 资源不需要
文件字段。客户端本机路径不会传递给服务端。

```bash
curl -F 'page=@page.html;type=text/html' \
     -F 'assets/portrait.png=@assets/portrait.png;type=image/png' \
     -F 'assets/chart.svg=@assets/chart.svg;type=image/svg+xml' \
     'http://127.0.0.1:8080/v1/render?size=296x128'
```

`link` 的 `href` 遵循相同的相对资源规则；stylesheet 也可以作为独立字段传入。

## 警告

不支持的属性、取值、选择器、at-rule、颜色、图片和字体会产生警告，其他内容继续编译。
`text-clipped`、`layout-overflow` 和 `empty-layout` 等布局警告指出内容无法完整放入固定画布。

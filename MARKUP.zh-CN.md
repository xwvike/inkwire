# Markup 手册

页面使用 HTML 编写内容、CSS 负责布局和绘制、SVG 负责几何图形。渲染目标提供视口尺寸：
CLI 的 `render`、`measure` 和 HTTP 的 `render` 必须指定 `size` 或 `panel`，`push` 从已连接设备获取面板。
根元素可以省略 `width` 和 `height`；如果声明，它们只参与 CSS 布局，不选择渲染目标。

```html
<main class="page">
  <h1>Inkwire</h1>
</main>
```

```css
.page {
  background: white;
}
```

根元素的 `orientation` 可以是 `landscape`、`portrait-cw` 或 `portrait-ccw`，默认是
`landscape`。


## 支持的属性

列出属性名不等于接受浏览器中的所有取值。超出下列范围的取值会产生警告并被忽略。

| 分组 | 属性 |
|---|---|
| 盒模型 | `display` `width` `height` `min-width` `max-width` `min-height` `max-height` `aspect-ratio` `box-sizing` `padding` `padding-top` `padding-right` `padding-bottom` `padding-left` `margin` `margin-top` `margin-right` `margin-bottom` `margin-left` |
| flex 与 grid | `flex` `flex-direction` `flex-basis` `flex-grow` `gap` `row-gap` `column-gap` `align-items` `align-self` `justify-content` `justify-items` `justify-self` `grid-template-columns` `grid-template-rows` `grid-column` `grid-row` |
| 定位 | `position` `top` `right` `bottom` `left` `inset` `z-index` |
| 绘制 | `background` `background-color` `color` `border` `border-width` `border-style` `border-color` `border-radius` `visibility` |
| 裁剪与变换 | `overflow` `clip-path` `transform` `rotate` `transform-origin` `scale` |
| SVG 绘制 | `fill` `stroke` `stroke-width` `stroke-dasharray` `stroke-dashoffset` |
| 文本 | `font` `font-family` `font-size` `line-height` `text-align` `vertical-align` `white-space` |
| 图片 | `object-fit` |

所有已实现属性都接受 `inherit`、`initial`、`unset` 和 `revert`。会继承的属性是 `color`、
`font-family`、`font-size`、`line-height`、`text-align` 和 `vertical-align`。其他属性在每个
元素上从默认值开始。

### 取值

- 长度支持 `px`、百分比，`calc()`。
- `width`、`height`、`min-*`、`max-*`、`flex-basis`、`top`、`right`、`bottom`、`left`、
  `inset`、`transform-origin` 和轨道尺寸接受百分比。
- 内边距、外边距、间隙、边框宽度、圆角和字号按整像素计算。间距使用百分比会被拒绝。
- `line-height` 接受像素长度、无单位比例，或相对于字号的百分比。
- 墨色是 `black`、`white`、`red` 和 `yellow`。其他颜色会被报告，不会近似成可用颜色。
- 字体和字号受当前构建内置的位图字库限制。没有对应字库或字号时使用最接近的字形，并
  给出 `substituted-font-size` 警告。

### 显示与布局

`display` 支持 `block`、`inline`、`inline-block`、`flex`、`inline-flex`、`grid`、
`inline-grid`、`contents` 和 `none`。

Flex 容器的 `flex-direction` 支持 `row` 和 `column`。`flex-grow` 分配剩余空间，
`flex-basis` 设置初始尺寸，`gap`、`row-gap` 和 `column-gap` 分隔子项。`align-items`、
`align-self`、`justify-content`、`justify-items` 和 `justify-self` 控制对齐。
`margin: auto` 可以把子项推到容器另一侧。

Grid 容器使用 `grid-template-columns` 和 `grid-template-rows` 定义轨道，支持像素、百分比、
`fr` 和 `auto`。`grid-column` 和 `grid-row` 放置子项，使用 `span` 指定跨越范围。

`box-sizing` 支持 `content-box` 和 `border-box`。`overflow: hidden` 把内容裁剪在盒子内。
`white-space: pre` 保留连续空格，默认会合并空白字符。

### 定位

```css
.page { position: relative; }
.badge { position: absolute; right: 4px; top: 4px; }
.hint { position: relative; left: 2px; top: -1px; }
```

`position: relative` 保持元素在正常流中，并按 `top`、`right`、`bottom`、`left` 或 `inset`
偏移绘制出来的盒子。`position: absolute` 将元素移出正常流，并相对最近的已定位祖先放置。
`z-index` 控制已定位元素的绘制顺序。

### 绘制与变换

`background` 和 `border` 接受上述墨色。边框的 `border-style` 支持 `solid`、`dashed` 和
`none`，`border-radius` 绘制圆角。`visibility: hidden` 保留盒子但不绘制它的内容。

`transform` 支持 `rotate` 和整数倍 `scale`。`rotate` 接受角度，`transform-origin` 控制
旋转中心。不支持的变换函数会被报告。

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
同一个值同时写在 SVG 呈现属性和 CSS 中时，CSS 优先。

## 图片与文件

`img` 通过 `src` 引用资源。`src` 可以是 HTTP/HTTPS 链接，也可以是相对路径。`.png`、
`.jpg` 和 `.jpeg` 作为位图加载，`.svg` 作为外部 SVG 编译。

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

未指定映射时，CLI 仍从页面目录读取相对资源。HTTP 使用 multipart：`page` 是 HTML；每个
本地资源是二进制文件字段，字段名必须与 `src` 完全一致，包括目录前缀。HTTP/HTTPS
资源不需要文件字段。

客户端本机路径对服务端不可见；HTTP 本地资源必须随请求上传。

```bash
curl -F 'page=@page.html;type=text/html' \
     -F 'assets/portrait.png=@assets/portrait.png;type=image/png' \
     -F 'assets/chart.svg=@assets/chart.svg;type=image/svg+xml' \
     'http://127.0.0.1:8080/v1/render?size=296x128'
```

`link` 的 `href` 遵循相同的相对资源规则，`stylesheet` 也可以作为独立字段传入。

## 警告

不支持的属性、取值、选择器、at-rule、颜色、图片和字体都会产生警告，页面其余部分继续
编译。`text-clipped`、`layout-overflow` 和 `empty-layout` 等布局警告会指出固定画布中放不下
的内容。

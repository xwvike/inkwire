# Markup 手册

Markup 页面使用 HTML 编写内容、CSS 负责布局和绘制、SVG 负责几何图形。页面尺寸由根元素
提供：

```html
<main class="page">
  <h1>Inkwire</h1>
</main>
```

```css
.page {
  width: 296px;
  height: 128px;
  background: white;
}
```

根元素的 `orientation` 可以是 `landscape`、`portrait-cw` 或 `portrait-ccw`，默认是
`landscape`。

## HTML

元素都会形成盒子。`div`、`main`、`section`、`header`、`footer`、`p`、标题和其他块级
元素默认是块盒。`span`、`b`、`i`、`small`、`em`、`strong`、`a`、`label` 和 `code` 默认
是行内元素，直到 CSS 修改它们的 `display`。`style`、`link`、`script`、`title`、`meta`、
`head`、`base` 和 `template` 不会绘制。

按通常方式使用 `class`、`id` 和 `style` 属性。`img` 用来放置位图或外部 SVG 图形，内联
`svg` 用来放置矢量几何图形。

## CSS 层叠

样式按以下顺序收集：

1. 页面旁边的同名 `.css` 文件。
2. 按文档顺序出现的 `style` 元素和链接样式表。

`style` 属性只作用于当前元素，普通优先级高于样式表规则。对于同一个属性，生效顺序为：

```text
带 !important 的声明
          |
普通行内 style
          |
普通选择器规则：先比权重，再比源码顺序
          |
继承值或属性默认值
```

选择器权重遵循 CSS：

```css
/* 元素 */
p { color: black; }

/* class */
.warning { color: red; }

/* id */
#title { color: yellow; }

/* 组合与关系 */
main.warning { color: red; }
main .warning { color: red; }      /* 后代 */
main > .warning { color: red; }    /* 直接子级 */
```

先比较 `id`，再比较 `class`、属性和伪类，最后比较元素名称。同一类别中匹配项越多，该类别
权重越高，但一个 `id` 仍高于任意数量的 `class` 或元素。权重相同则后出现的声明生效。
`!important` 使用 CSS 的写法，并覆盖普通声明。行内的 `!important` 高于样式表中的
`!important`。多个 `!important` 之间仍按选择器权重和源码顺序决定：

```css
.warning { color: black !important; }
#title { color: red; }
```

自定义属性使用同样的层叠规则，并沿元素树继承：

```css
:root { --accent: red; }
.badge { color: var(--accent); }
.badge span { color: var(--accent, black); }
```

`var(--name, fallback)` 在名称未声明时使用 fallback。没有 fallback 的缺失名称，或自定义
属性之间形成循环引用，会产生 `unsupported-declaration` 警告。

编译器无法读取的选择器会跳过，并给出 `unsupported-selector` 警告。不支持的伪元素和
`:is()`、`:has()` 等选择器函数不会被静默近似。

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

- 长度使用 `px`、百分比，或组合两者的 `calc()`。
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

未指定时，文本使用 `vertical-align: middle`。需要贴近边缘时显式使用 `top` 或 `bottom`。

### 绘制与变换

`background` 和 `border` 接受上述墨色。边框的 `border-style` 支持 `solid`、`dashed` 和
`none`，`border-radius` 绘制圆角。`visibility: hidden` 保留盒子但不绘制它的内容。

`transform` 支持 `rotate` 和整数倍 `scale`。`rotate` 接受角度，`transform-origin` 控制
旋转中心。不支持的变换函数会被报告。

## SVG

CSS 没有描述的几何图形使用 SVG：

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

`src` 以 `.svg` 结尾的 `img` 会作为图形编译，其他支持的格式作为位图加载。CLI 中的相对
路径相对页面解析；HTTP 服务中相对 `-assets` 目录解析；multipart 请求从同名文件部分读取。

```html
<img src="assets/portrait.png" class="portrait" />
<img src="chart-plot.svg" class="chart" />
```

## 警告

不支持的属性、取值、选择器、at-rule、颜色、图片和字体都会产生警告，页面其余部分继续
编译。`text-clipped`、`layout-overflow` 和 `empty-layout` 等布局警告会指出固定画布中放不下
的内容。

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
| 盒模型 | `display` | `block`、`list-item`、`inline`、`inline-block`、`flex`、`inline-flex`、`grid`、`inline-grid`、`contents`、`none` | `inline-*` 建立对应的原子盒；`contents` 不生成自身盒子；`list-item` 是带标记的块，`li` 默认就是它 |
| 尺寸 | `width`、`height` | `px`、`%`、`calc()`、`auto`、`fit-content` | 百分比按包含块解析；`fit-content` 盒的宽（高）取内容尺寸，不被容器拉伸；`max-content` 按 `fit-content` 绘制并警告，`min-content` 同样如此 |
| 尺寸 | `min-width`、`min-height`、`flex-basis` | `px`、`%`、`calc()`、`auto` | `flex-basis` 参与 flex 初始尺寸；与浏览器一致，flex 项的 `min-width`/`min-height` 默认为 `auto`，不会收缩到比最长的单词更窄（列方向则不低于内容高度），除非设为 `0` 或设置 `overflow: hidden`；`min-content` 等同 `auto` |
| 尺寸 | `max-width`、`max-height` | `px`、`%`、`calc()`、`none` | `none` 表示不设上限 |
| 比例 | `aspect-ratio` | `number` 或 `number / number` | 两个数必须为正数；未给出的那一边由给出的一边、或块铺满的宽度推出；两边都给出时忽略 |
| 盒模型 | `box-sizing` | `content-box`、`border-box` | 影响 `width`、`height`、`min-*`、`max-*` 和 `flex-basis` 的尺寸含义 |
| 内边距 | `padding`、`padding-top`、`padding-right`、`padding-bottom`、`padding-left` | 非负 `px`、`%`、`calc()` | 百分比按包含块行向尺寸解析，在布局时取整 |
| 外边距 | `margin`、`margin-top`、`margin-right`、`margin-bottom`、`margin-left` | `px`、`%`、`calc()`、`auto`；可为负 | `auto` 在 `justify-content` 之前占用 flex 行的剩余空间，并在交叉轴上定位 flex 和 grid 项；`margin: 0 auto` 让有宽度的块居中；百分比按包含块行向尺寸解析；块级流中的上下外边距与浏览器一致地合并（见下文） |
| Flex | `flex` | `none`、`auto`、`<grow>`、`<grow> <shrink>`、`<grow> <shrink> <basis>` | `<grow>` 的单值形式使用 `0%` basis；`<basis>` 支持 `auto`、`px`、`%`、`calc()` |
| Flex | `flex-direction` | `row`、`row-reverse`、`column`、`column-reverse` | 反向时从末端开始排列；与浏览器一致，`justify-content` 的 `start`、`end`、`left`、`right` 仍指书写方向的两端 |
| Flex | `flex-wrap` | `nowrap`、`wrap` | 与浏览器一致，放不下的项目换到下一行：每行单独做 flex 分配，行间距为 `row-gap`（列方向为 `column-gap`），由 `align-content` 放置各行；`wrap-reverse` 按 `wrap` 绘制并警告 |
| Flex | `flex-flow` | `row`、`row-reverse`、`column`、`column-reverse`、`nowrap`、`wrap` | 方向与换行，顺序任意 |
| Flex | `flex-grow`、`flex-shrink` | 非负数 | `flex-shrink` 默认值为 `1` |
| 间距 | `gap`、`row-gap`、`column-gap` | `normal`（无间距）、非负 `px`、`%`、`calc()` | `row-gap` 百分比按包含块的块向尺寸解析，`column-gap` 按行向尺寸解析；`gap` 的第一个值为 row-gap、第二个值为 column-gap；布局时取整 |
| 对齐 | `align-items`、`align-self`、`justify-items`、`justify-self` | `stretch`、`flex-start`、`start`、`center`、`flex-end`、`end`；`align-self` 和 `justify-self` 另支持 `auto` | `justify-*` 使用相同的对齐值集合 |
| 对齐 | `justify-content` | `flex-start`、`start`、`normal`、`center`、`flex-end`、`end`、`space-between`、`space-around`、`space-evenly` | 只分配没有被 grow 占用的空间；在 grid 上用来放置列，`normal` 则拉伸 `auto` 列铺满 |
| 对齐 | `align-content` | `flex-start`、`start`、`normal`、`stretch`、`center`、`flex-end`、`end`、`space-between`、`space-around`、`space-evenly` | 像 `justify-content` 放置列那样放置 grid 的行，也放置换行 flex 的各行；`normal` 和 `stretch` 把它们拉伸填满剩余空间；与浏览器一致，不换行的 flex 只有一行，忽略此属性 |
| 对齐 | `place-items`、`place-self` | `stretch`、`flex-start`、`start`、`center`、`flex-end`、`end`；`place-self` 另支持 `auto` | 先 `align-*` 值，后 `justify-*` 值；只写一个值时两者相同 |
| 对齐 | `place-content` | `flex-start`、`start`、`normal`、`stretch`、`center`、`flex-end`、`end`、`space-between`、`space-around`、`space-evenly` | 先 `align-content`，后 `justify-content`；只写一个值时两者相同 |
| Grid | `grid-template-columns`、`grid-template-rows` | `px`、`%`、`calc()`、正数 `fr`、`auto`、`min-content`、`max-content`、`repeat()` | `repeat()` 的轨道数上限为 400 |
| Grid | `grid-auto-columns`、`grid-auto-rows` | `px`、`%`、`calc()`、正数 `fr`、`auto`、`min-content`、`max-content`；一个或多个尺寸 | 决定网格在声明轨道之外自动补出的行列尺寸；多个尺寸依次轮流使用，与浏览器一致 |
| Grid | `grid-column`、`grid-row` | `auto`、行号、`span N`、`N / M`、`N / span M` | 行号和跨度必须为正整数 |
| 定位 | `position` | `static`、`relative`、`absolute` | 不实现 `fixed` 和 `sticky` |
| 定位 | `top`、`right`、`bottom`、`left`、`inset` | `px`、`%`、`calc()`、`auto` | 百分比按定位包含块解析 |
| 定位 | `z-index` | 数字 | 仅改变已定位元素的绘制层级 |
| 颜色 | `color` | `black`、`white`、`red`、`yellow`，可写名称、十六进制，或 `rgb()`、`rgba()`、`hsl()`、`hsla()`、`oklab()`、`oklch()` | 灰色或带色偏的灰会画成黑色并给出警告；不支持透明度、其他颜色函数和其他命名色 |
| 颜色 | `background`、`background-color`、`border-color`、`border-top-color`、`border-right-color`、`border-bottom-color`、`border-left-color` | `black`、`white`、`red`、`yellow`、灰色 `gray`、`grey`、`silver`、`lightgray`、`lightgrey`、`darkgray`、`darkgrey`、`dimgray`、`dimgrey`、`gainsboro`、`whitesmoke`，及这些颜色或任意灰色的十六进制、`rgb()`、`rgba()`、`hsl()`、`hsla()`、`oklab()`、`oklch()` 写法 | 灰色由黑白抖动而成。带色偏的灰（红绿蓝相差不超过 48，如 `#6b7280` 或 Tailwind 4 的 `oklch(55.1% 0.027 264.364)`）按其亮度画成灰色并给出警告。不支持透明度、其他颜色函数和其他命名色；其他颜色不会近似 |
| 颜色 | `background`、`background-image` | `none`，或 `linear-gradient()`，方向为 `to top`、`to right`、`to bottom`、`to left`，或 `0deg`、`90deg`、`180deg`、`270deg` 及以 `turn`、`rad`、`grad` 表示的同样角度；两个及以上色标，取白色和同一种墨色或其灰色，每个色标可带一个 `%` 位置 | 背景图画在 `background-color` 之上。对角方向、其他角度、一个渐变里两种墨色、颜色提示、双位置色标、长度位置、`radial-gradient()`、`conic-gradient()`、`repeating-*-gradient()` 和 `url()` 会给出警告并丢弃该声明 |
| 边框 | `border`、`border-top`、`border-right`、`border-bottom`、`border-left` | 一个宽度、线型和颜色，顺序任意 | 线型省略时为 `none`，不会绘制；颜色省略时与任一边框颜色写 `currentColor` 一样，取元素的文字颜色 |
| 边框 | `border-width`、`border-top-width`、`border-right-width`、`border-bottom-width`、`border-left-width` | 非负 `px`、仅含像素的 `calc()`，或 `thin`、`medium`、`thick`（1、3、5px） | 布局时取整；不接受百分比或多值边框宽度 |
| 边框 | `border-style`、`border-top-style`、`border-right-style`、`border-bottom-style`、`border-left-style` | `solid`、`dashed`、`dotted`、`none`、`hidden` | `dotted` 的点和间距等于线宽 |
| 边框 | `border-radius` | 四角共用一个非负 `px`、`%` 或 `calc()` 半径 | 布局时取整；百分比按盒子尺寸解析，达到 `50%` 及以上时绘制为圆或椭圆；非正方形盒子上更小的百分比本应形成椭圆角，此时按较小半径绘制圆角并警告；逐角半径和 `/` 椭圆半径产生警告 |
| 可见性 | `visibility` | `visible`、`hidden` | `hidden` 保留布局盒但不绘制 |
| 裁剪 | `overflow` | `visible`、`hidden`、`clip` | 不提供滚动 |
| 裁剪 | `clip-path` | `none`、`inset()`、`circle()`、`ellipse()`、`polygon()` | 函数参数支持 `px`、`%`、`calc()`；`inset()` 只支持单一圆角半径 |
| 变换 | `transform` | `none`、`scale()`、`rotate()` | 仅支持 `scale` 和 `rotate` 函数 |
| 变换 | `rotate` | `deg`、`grad`、`rad`、`turn` 或无单位角度 | — |
| 变换 | `transform-origin` | 一个或两个关键词、`px`、`%`、`calc()` | 关键词为 `left`、`center`、`right`、`top`、`bottom` |
| 变换 | `scale` | 一个或两个相同的整数，且不小于 1 | 不做重采样；不支持小数或非等比缩放 |
| SVG 绘制 | `fill`、`stroke` | 支持的墨色和灰色、`none`、`transparent` | 这些属性可继承；内联 SVG 中的 CSS 声明覆盖 SVG 呈现属性 |
| SVG 绘制 | `stroke-width` | 非负 `px`、仅含像素的 `calc()` 或 SVG 无单位数值 | 布局时取整；小于 1 像素时按 1 像素绘制并产生警告 |
| SVG 绘制 | `stroke-dasharray`、`stroke-dashoffset` | 空格或逗号分隔的整数像素 | — |
| SVG 绘制 | `stroke-linecap` | `butt`、`round`、`square` | 作用于 SVG 开放线段、折线和路径；SVG 默认值为 `butt` |
| SVG 绘制 | `stroke-linejoin` | `miter`、`round`、`bevel` | 作用于 SVG 路径相邻线段的连接处；SVG 默认值为 `miter` |
| 字体 | `font` | `size[/line-height] family` | 仅读取字号、行高和字体族；样式、粗细等字段会产生警告 |
| 字体 | `font-family` | `ui`、`hzk`、`monaco`，可写字体栈 | 按声明顺序逐字尝试字体族；未知名称跳过，字体栈全部不可用时使用默认字体并产生警告 |
| 字体 | `font-size` | `px`、仅含像素的 `calc()`，或父元素字号的百分比 | 使用最近可用位图字号并产生 `substituted-font-size` |
| 文本 | `line-height` | `normal`、`px`、仅含像素的 `calc()`、无单位比例、相对字号的百分比 | 无单位值按当前元素字号解析；百分比解析后继承计算值 |
| 文本 | `text-align` | `left`、`start`、`center`、`right`、`end` | — |
| 文本 | `vertical-align` | `baseline`、`top`、`middle`、`bottom` | 只作用于 inline 级盒；初始值为 `baseline`，不继承 |
| 文本 | `white-space` | `normal`、`nowrap`、`pre`、`pre-wrap` | 按浏览器习惯在词之间折行：空格和连字符之后，以及中日韩文字之间（不在闭合标点前折行） |
| 文本 | `word-break` | `normal`、`break-all`、`break-word` | `break-all` 允许在任意两个字符之间折行；`keep-all` 会警告并按 `normal` 处理 |
| 文本 | `overflow-wrap`、`word-wrap` | `normal`、`break-word`、`anywhere` | 比整行还长的单词总会在放不下的地方断开，所以各值效果相同 |
| 文本 | `font-weight` | `normal`、`bold`、`bolder`、`lighter` 或 1 到 1000 的数字 | 600 及以上按粗体绘制：内置字库只有一种字重，每个字形向右偏移一像素再绘制一次，并多占一像素宽度 |
| 文本 | `font-style` | `normal` | `italic` 和 `oblique` 产生警告；内置字库只有正体 |
| 文本 | `letter-spacing` | `normal`、`px`、仅含像素的 `calc()` | 加在每个字符之后，可为负 |
| 文本 | `text-decoration`、`text-decoration-line` | `none`、`underline`、`line-through`、`overline` | 以文字颜色绘制实线，字体高度每 16 像素粗一像素；与 CSS 一致，线条贯穿后代文字，后代的 `none` 无法去掉，遇到 inline-block 和定位盒子时停止；简写中的线型、颜色或粗细产生警告 |
| 文本 | `text-decoration-style` | `solid` | 其他线型产生警告 |
| 文本 | `text-decoration-color` | `currentColor` | 装饰线以文字颜色绘制；其他颜色产生警告 |
| 文本 | `text-decoration-thickness`、`text-underline-offset` | `auto` | 其他取值产生警告 |
| 文本 | `text-overflow` | `clip`、`ellipsis` | 与浏览器一致，`ellipsis` 在被 `overflow: hidden` 截断的行末显示“…”，仅作用于不含子元素的文字；其他情况产生警告 |
| 列表 | `list-style`、`list-style-type` | `disc`、`circle`、`square`、`none`、`decimal`、`decimal-leading-zero`、`lower-alpha`、`upper-alpha`、`lower-latin`、`upper-latin`、`lower-roman`、`upper-roman`、`lower-greek`、`cjk-decimal`、`simp-chinese-informal`、`trad-chinese-informal`、`cjk-ideographic`、`disclosure-open`、`disclosure-closed`，或一个字符串；简写还可带位置 | 每个 `display: list-item` 盒子都按其颜色和字体画出标记，序号从 `ol` 的 `start`、`reversed` 和各 `li` 的 `value` 算起；这里没有的计数样式按 `decimal` 绘制并警告 |
| 列表 | `list-style-position` | `outside`、`inside` | `outside` 把标记挂在首行旁、列表项边框之外；`inside` 把标记放在行首，列表项以盒子而非文字开头时警告并改画在外侧 |
| 列表 | `list-style-image` | `none` | 图片产生警告，改画类型对应的标记 |
| 图片 | `object-fit` | `fill`、`contain`、`cover` | 只影响 `img` 和外部绘图的盒内适配 |

所有已实现属性接受 `inherit`、`initial`、`unset` 和 `revert`。`color`、`fill`、`stroke`、
`stroke-width`、`stroke-dasharray`、`stroke-dashoffset`、`stroke-linecap`、`stroke-linejoin`、`font`、`font-family`、`font-size`、
`font-weight`、`font-style`、`letter-spacing`、`line-height`、`text-align`、`white-space`、`word-break`、`overflow-wrap`、`list-style` 及其子属性和 `visibility` 默认继承；`vertical-align` 不继承。自定义
属性 `--name` 可声明并供 `var()` 使用，按相同规则层叠并继承。

内置位图字号：

| 字体族 | 可用字号（px） |
|---|---|
| `ui`、`hzk` | `12`、`14`、`16`、`24`、`28`、`32`、`36`、`42`、`48` |
| `monaco` | `10`、`12`、`14`、`16`、`20`、`24`、`28`、`30`、`32`、`42`、`48` |

## 不支持的属性和取值

未列入上表的 CSS 属性全部不实现。对已实现属性使用未列出的取值，结果相同：声明被忽略并
产生 `unsupported-declaration`。以下是常见的不支持项：

- 布局：`float`、`clear`、`order`、`grid-template-areas`、
  `grid-auto-columns`、`grid-auto-rows`、`grid-auto-flow`、`table-layout`。
- 定位和效果：`position: fixed`、`position: sticky`、`opacity`、`filter`、`box-shadow`、
  `text-shadow`、`mix-blend-mode`、`background-repeat`、`background-size`。
- 文本：`font-variant`、`text-indent`、`word-spacing`、`text-transform`、`hyphens`。
- 图片：`object-position`。
- 颜色与变换取值：`hwb()`、`lab()`、`lch()`、`color()`、`color-mix()` 等上面未列出的颜色函数，`radial-gradient()`、`conic-gradient()`、重复渐变、透明度；`transform` 中除 `scale()` 和
  `rotate()` 以外的函数；CSS `scale` 的小数、负数或不等比双值；`border-style` 的 `double`、
  `groove`、`ridge`、`inset`、`outset`；`overflow: auto` 和 `overflow: scroll`。

## 使用注意

- `size` 或 `panel` 决定画布边界。根元素的 `width` 和 `height` 不会扩大或缩小该边界；超出
  画布的内容产生布局或裁剪警告。
- `box-sizing`、padding 和 border 都参与盒尺寸。绝对定位子元素相对于最近的已定位祖先的
  padding box，位于其 border 内侧、padding 外侧。
- Flex 简写保留 grow、shrink 和 basis 三个分量；`flex: 1` 是 `1 1 0%`。未指定 `flex-shrink`
  时初始值为 `1`。
- 块级流中的上下外边距与浏览器一致地合并：相接的外边距取最大的正值加上最小的负值，可穿过空盒子；
  父元素在对应一侧没有 padding、border（下方还要求没有自身高度）时，首个或末个子元素的外边距会移到父元素
  外面。flex 和 grid 项、inline-block、定位盒子、`overflow: hidden` 盒子和页面根元素把子元素的外边距
  留在内部。百分比外边距不参与合并；与其他外边距相接时产生警告，两者相加。
- 元素按浏览器自带样式表的样子绘制，页面写的任何规则都优先于它，与选择器无关：`h1` 到 `h6` 为粗体，
  字号是父元素的 2、1.5、1.17、1、0.83、0.67 倍，并静默对齐到字体已有的最近字号；`p`、`ul`、`ol`、
  `dl`、`menu`、`pre` 和标题带浏览器的上下外边距；`blockquote` 和 `figure` 左右各缩进 40px，`dd`
  左缩进 40px；`ul`、`ol` 和 `menu` 缩进 40px 并画出列表标记，嵌套列表没有上下外边距，标记依次为空心圆、
  方块；`ol` 和 `li` 的 `type` 属性指定标记；`b`、`strong` 和 `th` 为粗体，`th`、`caption` 和
  `center` 居中；`u`、`ins` 和带 `href` 的 `a` 带下划线；`s`、`del` 和 `strike` 带删除线；`small`
  为 0.83 倍；`hr` 是文字颜色的 1px 横线；`pre` 保留空白。不绘制链接颜色。浏览器会画而这里不画的，
  `unsupported-element` 会指出来，页面写明想要的效果后即不再警告：表单控件、`canvas`、`video`、
  `audio`、`iframe`、`object` 和 `embed` 不绘制；没有边框和背景的 `button` 不画框；没有 `display`
  的 `table` 按块排布；`em`、`i`、`cite`、`var`、`dfn` 和 `address` 以正体绘制。每种斜体元素在一页中
  只警告一次。
- Inline、图片和 inline SVG 共用行框。`vertical-align` 仅对 inline 级盒有效；block、flex
  item 或 grid item 上的声明会产生警告。单行文本的盒内垂直对齐可使用与盒高相等的
  `line-height`。
- 样式来源顺序为 CLI 样式表、页面 `<style>`、按文档顺序加载的 `link` 样式表。普通声明按
  specificity 和来源顺序比较；行内样式高于普通选择器，`!important` 高于普通声明。自定义
  属性按相同规则层叠并继承；未定义或循环的 `var()` 会丢弃所在声明。
- 百分比内边距和外边距按包含块行向尺寸换算；`row-gap` 按块向尺寸、`column-gap` 按行向尺寸
  换算，均在布局时取整。边框宽度和字号只接受像素值或仅含像素的 `calc()`。`calc()` 支持长度的
  加减、长度与数字的乘除以及括号嵌套；`min()`、`max()`、`clamp()` 可用于任何接受 `calc()` 之处，
  前提是参数中没有百分比。
- 凡接受像素长度之处（包括 `calc()` 与自定义属性内部），也可以使用 `em`（当前元素字号；在
  `font-size` 中为父元素字号）、`rem`（根元素字号，根元素未设置时为 12px）、`vw`、`vh`、`vmin`、
  `vmax`（根元素 `width` 与 `height` 的比例，二者都须以像素给出，否则警告并丢弃该声明），以及按
  每英寸 96px 换算的 `pt`、`pc`、`in`、`cm`、`mm`、`Q`。`ch` 与 `ex` 产生警告。
- 字体来自构建时内置位图字库。未找到字体族使用默认字体；字体栈按字形逐级回退；未找到对应字号使用最近字号。
  内置字库只有一种字重，粗体由它合成。
- 灰色是黑色按 4x4 有序抖动铺在白底上，取最接近的十六分之几覆盖率：`#808080` 是 8/16，四舍五入到
  0 或 16 的灰色就是白色或黑色。抖动图案按画面坐标对齐，相邻的灰色区域无缝衔接，同一页面每次渲染
  像素一致。画在灰色上的文字会在字形周围清出 1px 的白边。文字本身不能是灰色：抖动后的 1px 笔画会
  丢掉一半像素。1px 的灰色边框或描边同理，看起来是一条点线。`compile` 把灰色写成 `black 8/16`。
  带色偏的灰按红绿蓝加权（0.2126、0.7152、0.0722）得到亮度，色偏丢弃。带任何透明度的颜色——
  `#0008`、`rgb(0 0 0 / 50%)`——都会被拒绝，因为绘制时不知道透过去的是什么；请直接写出应有的颜色。
- `linear-gradient()` 用同一套抖动绘制：每个像素按它在盒子里的位置，从色标算出覆盖率，取最接近的
  十七级之一，所以一个渐变最多十七条色带，两端的色带只有中间的一半宽。色标位置按 CSS 规则推算，
  可以落在 0%–100% 之外。渐变只能从白色过渡到一种墨色——黑色经过各级灰，或红色、黄色经过各自的
  浅色——因为一个像素不能一半是这种墨色一半是那种。圆角盒子按与填充相同的圆角裁剪渐变。面板画不了
  的渐变会给出警告并丢弃该声明，和浏览器丢弃读不懂的声明一样，所以写在前面作为回退的 `background`
  仍然生效。
- 页面会继续编译，但不支持的声明、选择器、at-rule、颜色、字体和资源都会产生警告。警告
  不是渲染失败标志，需根据类型检查最终画面。

## SVG

支持的元素：`rect`、`circle`、`ellipse`、`line`、`polyline`、`polygon`、`path`、`g`、`use`、
`clipPath`、`pattern`、`defs`、`title`、`desc`。支持 `path` 的 `M`、`L`、`H`、`V`、`C`、
`S`、`Q`、`T`、`A`、`Z` 及相对命令。`<use href="#id">` 和 `xlink:href` 可引用同一 SVG 中
带 `id` 的已支持元素或分组。

SVG 支持 `fill`、`stroke`、`stroke-width`、`stroke-dasharray`、`stroke-dashoffset`、
`stroke-linecap`、`stroke-linejoin` 和
`clip-path`。`viewBox` 按浏览器的 `xMidYMid meet` 规则映射到 viewport，viewport 外的图形
会被裁剪。SVG 的 `transform` 支持 `translate`、`scale` 和 `rotate`；`scale` 接受有限非零
数值，`rotate` 接受一个角度或角度加旋转中心。CSS 的 `transform` 仅支持本手册中列出的函数。

## 图片与资源

`img src` 支持 HTTP/HTTPS URL 和相对路径。`.png`、`.jpg`、`.jpeg` 按位图读取，`.svg` 按
外部 SVG 绘图读取。相对路径始终以页面资源映射为准，HTTP 服务不会读取客户端路径或服务端
当前目录中的任意文件。HTTP 服务只从公网拉取 URL，不会访问自身所在机器或所在局域网；命令行不受此限。

`<img>` 中的 SVG 按替换型图片处理：页面 CSS 只控制图片盒的尺寸和裁剪，不进入 SVG 文档参与
级联；已支持的 SVG 绘制属性和 viewport 保留。页面 CSS 在该图片上声明 SVG 绘画属性时产生
`unsupported-declaration` 警告。内联 `<svg>` 仍参与页面 CSS 级联。

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
`substituted-font-size`、`approximate-corner`、`unsupported-element`。

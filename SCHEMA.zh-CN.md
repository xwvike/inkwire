# Inkwire Scene Schema

`inkwire render`、`push` 与 `/v1/push` 所解码的文档。
运行 `inkwire schema` 可打印本文。[English](SCHEMA.md) · [README](README.zh-CN.md)

## Scene Schema

全部整数像素。

```json
{
  "version": 1,
  "orientation": "landscape",
  "size": {"width": 296, "height": 128},
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

### 页面

| 字段 | 类型 | 取值 | 默认 |
|---|---|---|---|
| `version` | integer | `1` | 必填 |
| `orientation` | string | `landscape`、`portraitClockwise`、`portraitCounterClockwise` | `landscape` |
| `size` | size | 这一页排版所用的画布 | 必填，除非调用方给了尺寸 |
| `background` | ink | `white`、`black`、`red`、`yellow` | `white` |
| `root` | node | | 空 |

渲染页面和解码后的源图片最多为 16,777,216 像素。

### 基本值

本文其余部分按名称引用的三种复合值。

| 值 | 字段 | 类型 | 取值 |
|---|---|---|---|
| size | `width`、`height` | 整数 | ≥ `0` |
| point | `x`、`y` | 整数 | 可为负 |
| rect | `x`、`y`、`width`、`height` | 整数 | `x`、`y` 可为负 |

```json
{
  "size": {"width": 100, "height": 40},
  "point": {"x": 10, "y": 5},
  "rect": {"x": 10, "y": 5, "width": 100, "height": 40}
}
```

`ink` 为 `black`、`white`、`red` 或 `yellow`；可选颜色默认 `black`。

```json
{
  "ink": "black",
  "width": 2,
  "dash": [4, 2],
  "dashOffset": 0
}
```

| 字段 | 类型 | 取值 | 默认 |
|---|---|---|---|
| `ink` | ink | `black`、`white`、`red`、`yellow` | `black` |
| `width` | integer | > `0` | 必填 |
| `dash` | integer 数组 | 实线/空白长度，每项 > `0` | 实线 |
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

自动轨道取最宽内容；`fr` 分配剩余。放置采用 CSS Grid 默认的稀疏行流：双轴明确定位的子节点可以重叠；只指定 `row` 的子节点在该行已满时创建隐式列；只指定 `column` 的子节点向下寻找；完全自动的子节点创建隐式行。隐式轨道均为自动轨道。

CLI 命令逐项打印扩展；HTTP 渲染响应在 JSON body 的
`report.GridExpansions` 中保留每个 grid 的完整记录。

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
| `children[].layer` | integer | 数值越高越晚绘制；相同时保持文档顺序 |
| `children[].node` | node | 内容 |

同轴两条边加一个尺寸被拒。

### relative

```json
{
  "type": "relative",
  "top": "10%",
  "left": 8,
  "child": {"type": "rectangle", "fill": "red"}
}
```

| 字段 | 类型 | | 默认 |
|---|---|---|---|
| `top` `right` `bottom` `left` | length | 相对于正常 flow 位置的偏移 | 不偏移 |
| `child` | node | | 必填 |

子节点仍保留正常布局得到的尺寸和位置，但整个绘制子树会按这些边距移动。
同一轴同时写两条边时，`left` 优先于 `right`，`top` 优先于 `bottom`；只写末端边时
会向相反方向移动。百分比相对于 containing box 解析。这个包装节点不会改变父级测量到的尺寸。

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

| 字段 | 类型 | 取值 | 默认 |
|---|---|---|---|
| `type` | 字符串 | `stack`、`padding`、`spacer` | 必填 |
| `size` | size | 仅 `stack` 与 `spacer` | `stack` 为容器的，`spacer` 为 `0`×`0` |
| `children[]` | node 数组 | 仅 `stack`：同一区域内按序绘制，后面的在上层 | 空 |
| `insets` | 对象 | 仅 `padding`：`top`、`right`、`bottom`、`left`，各为整数 | 每边 `0` |
| `child` | node | 仅 `padding` | `padding` 必填 |

`spacer` 不需要写 `size`。不写就是 0×0 —— 当占多少空间由 `grow` 决定时，这正是它
该有的样子。

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
| `lineHeight` | integer | 行盒高度；多出的部分在字形上下均分 | 字体 |

| 字族 | 尺寸 | 覆盖 |
|---|---|---|
| `ui` | 12 14 16 24 28 32 36 42 48 | 拉丁 + 中日韩 |
| `hzk` | 12 14 16 24 28 32 36 42 48 | **仅中日韩** —— GB2312，不含 ASCII |
| `monaco` | 10 12 14 16 20 24 28 30 32 36 42 48 | **仅 ASCII** |

只有 `ui` 两者都覆盖：它是 `monaco` 与 `hzk` 配对而成，`monaco` 优先。用 `hzk` 写
数字会得到一条 `missing-runes` 警告和一个占位方块，所以混排要么用 `ui`，要么按字符
类别拆成多个 run。

16 以上为整数倍放大。

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
| `source` | string | 见[图片资源](README.zh-CN.md#图片资源) | 必填 |
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

`auto` 依据图像选择阈值、抖动与红色提取。`/v1/render` 和 `/v1/push` 在
`report.Images` 中返回决策，CLI 命令也会打印。成功的 `/v1/render` 固定返回
JSON，PNG 以 base64 `pngBase64` 与报告一起返回。

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

| 字段 | 类型 | 取值 | 默认 |
|---|---|---|---|
| `type` | 字符串 | `pixel`、`rectangle`、`line`、`polyline`、`polygon`、`circle`、`ellipse`、`arc`、`pie`、`chord` | 必填 |
| `size` | size | 绘制区域 | 容器的 |
| `fill` | ink | 仅 `rectangle`、`polygon`、`circle`、`ellipse` | 不填充 |
| `stroke` | stroke | 除 `pixel`、`pie`、`chord` 外的所有类型 | 不描边 |
| `ink` | ink | 仅 `pixel`、`pie`、`chord` | `black` |
| `at` | point | 仅 `pixel`：像素位置 | `pixel` 必填 |
| `radius` | length | `rectangle` 的圆角，或 `circle` 的半径 | `0` |
| `from` | point | 仅 `line`：一端 | `line` 必填 |
| `to` | point | 仅 `line`：另一端 | `line` 必填 |
| `points` | point 数组 | `polyline` ≥2，`polygon` ≥3 | 两者必填 |
| `center` | point | 仅 `circle`：圆心 | 区域中心 |
| `start` | 数值 | 仅 `arc`、`pie`、`chord`：起始角度（度） | 这三者必填 |
| `sweep` | 数值 | 仅 `arc`、`pie`、`chord`：扫过角度（度） | 这三者必填 |

图元需要 `fill` 或 `stroke` 才会画出东西；`pixel`、`pie`、`chord` 用 `ink`。
`ellipse`、`arc`、`pie`、`chord` 使用整个矩形，而不是中心加半径。

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
| `arc` | `bounds`、`start`、`sweep`、`rotation` |
| `close` | |

≥1 条命令，且需 `fill` 或 `stroke`。

## Pattern

| 字段 | 类型 | 取值 | 默认 |
|---|---|---|---|
| `type` | 字符串 | `pattern` | 必填 |
| `size` | size | 平铺区域 | 容器的 |
| `rows` | 字符串数组 | 每行一个字符串，各行等长 | 必填 |
| `inks` | 对象 | 单字符键映射到 `black`、`white`、`red`、`yellow` | 必填 |

`inks` 中没有对应项的字符不修改画面。各行在区域内平铺。

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

## Rotated

把子节点绕一个点转任意角度。

| 字段 | 类型 | 取值 | 默认 |
|---|---|---|---|
| `type` | 字符串 | `rotated` | 必填 |
| `size` | size | 要旋转的区域 | 容器的 |
| `degrees` | 数值 | 顺时针角度 | `0` |
| `origin` | 含 `x`、`y` 两个长度的对象 | 绕着转的那个点，从区域左上角量起。用长度而不是数字，这样在区域还没有尺寸的时候就能说“到一半的地方” | 区域中心 |
| `child` | node | 被转的东西 | 必填 |

```json
{
  "type": "rotated",
  "degrees": 37,
  "origin": {"x": "0%", "y": "100%"},
  "child": {"type": "rectangle", "fill": "black"}
}
```

它不改变排版，这跟 CSS 的 transform 一致，也是唯一能组合的答案：一个尺寸随角度变化的
盒子没法跟别的东西并排放，因为它一转，邻居就得跟着挪。转过的子节点会伸到自己区域外面，
跟圆心贴着区域边缘的 circle 一样。

凡是从几何算出来的东西，任意角度都精确——形状是先算清楚自己在哪儿，再决定盖住哪些像素。
文字和图片本身已经是像素，只能重采样：四分之一转是精确的，其余角度会糙，十二像素下看得
出来。仍然照画，因为样式表转一个盒子就是要连里面写的字一起转。

`transformed` 是另一个节点，做的是另一件事：它把子节点画到一张离屏表面上再整块搬，所以
只接受整数倍的四分之一转和整数放大。要放大用它，要旋转用这个。

## 裁剪

四个节点各带一个子节点，区别只在裁到什么。

| 字段 | 类型 | 取值 | 默认 |
|---|---|---|---|
| `type` | 字符串 | `clip`、`clipRect`、`clipShape`、`clipPath` | 必填 |
| `size` | size | 裁剪所在区域 | 容器的 |
| `child` | node | 被裁剪的节点 | 必填 |
| `rect` | rect | 仅 `clipRect`：保留的矩形 | `clipRect` 必填 |
| `shape` | shape | 仅 `clipShape`：见下 | `clipShape` 必填 |
| `path` | path | 仅 `clipPath`：命令位于 `path.commands` | `clipPath` 必填 |

`clip` 不接受后三者：它裁到子节点自身的框。

### shape

| 字段 | 类型 | 取值 | 默认 |
|---|---|---|---|
| `kind` | 字符串 | `inset`、`circle`、`ellipse`、`polygon` | 必填 |
| `insets` | 4 个 length | 仅 `inset`：上、右、下、左 | `inset` 必填 |
| `corner` | length | 仅 `inset`：圆角半径 | `0` |
| `radius` | length | 仅 `circle` | `circle` 必填 |
| `radiusX` | length | 仅 `ellipse` | `ellipse` 必填 |
| `radiusY` | length | 仅 `ellipse` | `ellipse` 必填 |
| `center` | length 组成的 point | 仅 `circle` 与 `ellipse`，其余类型给了会被拒绝 | 区域中心 |
| `points` | length 组成的 point 数组 | 仅 `polygon`，至少三个 | `polygon` 必填 |

裁剪节点自身不决定绘制顺序：每个节点只有一个子节点，嵌套裁剪取交集。有重叠的同级裁剪可放入 `stack`（后面的子节点在上层）；若还需要定位框，使用 `anchored.children[].layer`。

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

## 报告

每次渲染都会附带一份报告，说明编译过程测量到了什么。它不改变文档本身，只陈述
这份描述最终意味着什么。`inkwire render` 会打印它，`/v1/render` 和 `/v1/push`
以 `report` 字段返回它。

```json
{
  "bounds": {"x": 0, "y": 0, "width": 296, "height": 128},
  "missingRunes": "翯𡃁",
  "warnings": [
    {"path": "root.children[2]", "code": "text-clipped", "message": "\"3260/3720G\" does not fit 40x15: 6 pixels of the line and 0 whole lines are cut off"}
  ],
  "gridExpansions": [
    {"path": "root", "implicitColumns": 0, "implicitRows": 2}
  ],
  "images": [
    {
      "path": "root.children[0]",
      "options": {"fit": "contain", "sampling": "bilinear", "dither": "floydSteinberg", "threshold": 128, "redThreshold": 170, "redMaxGreen": 170, "disableRed": false},
      "profile": {"midToneFraction": 0.61, "threshold": 97, "redSeparation": 0, "lostToLuminance": 0.02, "photographic": true, "redIsMeaningful": false, "colourCarriesStructure": false},
      "toneByColourDistance": false,
      "contrastEnhanced": true
    }
  ]
}
```

| 字段 | 类型 | 含义 |
|---|---|---|
| `bounds` | rect | 这一页实际画到的范围，写成原点加尺寸 |
| `missingRunes` | string | 没有任何内置字体能画的字符，就是那些字符本身 |
| `warnings` | array | 描述带来的非致命后果；代码表见 README。`unsupported-ink` 只在指定了面板时才可能出现，不指定面板就不知道有哪些墨水 |
| `gridExpansions` | array | 自动布置在 grid 声明之外新建的轨道 |
| `images` | array | 自动图片处理做了什么决定，每个 `"processing": "auto"` 的图片一条 |

只有 `bounds` 一定存在。四个数组在无话可说时整个省略，所以一次干净渲染的报告
就只有它的 bounds。

`images[].options` 用的是 `image` 节点接受的同一批拼写，所以一个决定读回来时
用的是它本该被写下的词汇。`images[].profile` 是这些选项被选出来之前图片测量到
的结果，它存在是为了让自动决定可以被推翻——节点上的 `overrides` 就是推翻的方式。

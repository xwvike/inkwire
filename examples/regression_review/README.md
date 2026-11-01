# 回归修复对比

打开 [index.html](index.html)，查看相同输入在修复前后的结果。所有 PNG
均为真实渲染输出放大 12 倍，使用最近邻采样。

- `before/`：在修改实现之前，由提交 `bbddbfd` 的实现生成。
- `after/`：修改实现后，用同一个程序、相同输入生成。
- `page.html`：方向问题的固定 HTML 输入。
- `main.go`：两个绘制用例和 HTML 编译用例的生成程序，没有模拟缺陷或修复。
- `result.json`：像素颜色及方向的原始观测值。

从仓库根目录重新生成当前实现的结果：

```sh
go run ./examples/regression_review -out examples/regression_review/after
```

修复前，新增回归测试确认失败：斜线缺失应覆盖的像素、零半径点消失、圆环
中心被填黑、body 声明尺寸后 wrapper 方向丢失。修复后，同一组断言通过：

```sh
go test ./internal/display ./internal/markup -run 'TestEvenStrokeCoversHalfPixelSamples|TestZeroRadiusCircleContainsItsCenter|TestCircleStrokeKeepsZeroRadiusHole|TestABodyIsDrawnWholeAsABrowserDrawsIt' -count=1
```

方向不直接旋转逻辑画布 PNG；它作为场景元数据传递给设备打包器。
因此该用例应比较 `scene.json` 的 `orientation` 字段，而不是期待 PNG 旋转。

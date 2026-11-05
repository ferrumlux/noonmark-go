# Noonmark · MyGo 原生界面原型

以 Go + MyGo 的 `ui` 包复现 Noonmark 的界面。没有 HTML、CSS、JavaScript 或 WebView；文字、控件、图标及分栏由 MyGo 原生 UI 构造。当前是第一轮 UI 原型，不是完整应用。

## 已覆盖的界面场景

共20个原生视图：官方图库18张有对应实现，另有2个无公开截图的源码推导设置页。

- 计划与轨迹：Day Todo、任务池、未来计划、重复计划（含展开轨迹）、未完成、已完成、日历、烛龙对话。
- 札记：飞光、飞光行内编辑、Sticky Note 便签墙、Sticky Note 清单流。
- 设置：偏好、组织、数据、写入与隐私边界。
- 浮窗：快速记录、飞光速记。
- 源码推导预览：同步、烛龙与自动化 Provider。页面注明无公开截图和无真实连接，不计入18张原图对照。

可交互：十个主导航切换、便签墙/流切换、飞光展开/编辑/取消、中文多行输入、任务勾选/取消、日期选择、侧栏收放、设置分段选项与开关、分类预览显示/关闭。业务操作按钮只呈现外观，不会产生实际副作用。

所有业务内容都是原项目公开演示图对应的固定样例。未实现存储、任务状态机、数据同步、AI、真实任务新增/发送、数据导入/导出、全局快捷键注册。复盘计数不随演示勾选重新计算。所有状态只保留在本次进程内。

这不是“所有交互状态全部完成”：全局搜索结果、完整任务编辑/排期对话框、重复规则编辑器、分类生命周期操作、Provider凭证配置、同步冲突/错误/确认态等仍未覆盖。画面差异见 VALIDATION.md。

## 运行

要求 Go 1.27.3 或更新版本。MyGo 已锁定至提交 `977ea57e0baf78da62698ba7371420d192260466`，无需相邻源码目录。

```sh
go run -buildvcs=false .
# 直接打开任务池或日历
go run -buildvcs=false . -screen pool
go run -buildvcs=false . -screen calendar
# 直接打开其他页面/浮窗
go run -buildvcs=false . -screen flylight
go run -buildvcs=false . -screen settings-general
go run -buildvcs=false . -screen quick-entry
# 列出全部20个页面及尺寸
go run -buildvcs=false . -list
```

Linux 桌面需要 GTK 3 及可用的 X11/Wayland 会话。离屏渲染不需要显示服务器，但中文文字需要 Pango、Cairo、Fontconfig 及 Noto Sans CJK SC 字体。macOS 使用平台默认字体；本轮未做 macOS 实机验证。

桌面运行时，Cmd（Linux/Windows为Ctrl）+, 打开独立设置窗，Cmd+N 打开快速记录，Cmd+Shift+I 打开飞光速记。Esc 关闭设置/浮窗。它们只是应用内快捷键，不注册系统全局快捷键。

## 无窗口生成截图

```sh
go run -buildvcs=false . -render screenshots
```

生成20个PNG：主界面2400×1536，设置1720×1320，快速记录1040×336，飞光速记1040×524，均为2×。这些是同一份UI代码经 `ui.Render` 输出的原生视图帧，不是将原项目截图作为背景，也不是网页截图。单页可用 `-screen flylight -render-one screenshots`。

## 检查

```sh
go test -buildvcs=false -v .
go vet -buildvcs=false .
```

内存较少的环境可用 `GOGC=20 GOMEMLIMIT=700MiB GOMAXPROCS=2 go build -p=1 -buildvcs=false .`。首次编译需要下载 Go 依赖。

## 视觉边界

主要布局、信息层次、默认尺寸和颜色接近原图，但不是像素级一致：Linux 使用 Noto Sans CJK，原版 macOS 使用 SF/PingFang；图标是 SVG 近似而非原版 SF Symbols；圆角、字重、抗锯齿、个别间距/截断及任务摘要仍有差异。本轮目标为 1200×768 内容区，没有复制 macOS 系统标题栏。

达到“一模一样”的下一步应在 macOS 上使用相同字体、SF Symbols、窗口样式和缩放比例进行截图差分与交互验收。原项目 AppKit/SwiftUI 的系统行为不会因外观复刻自动得到。

## 来源与许可

- Noonmark：<https://github.com/quboliu/noonmark>，参考提交 `43e6eefaae3a04be89cab3e234d470111afd80ef`。
- 视觉基准：该提交 `docs/assets/screenshots/readme/` 内的全部18张中文截图，以及 `Sources/NoonmarkMacUIContract/MacUIDesignContract.swift`。未使用已归档 HTML 原型作为验收依据。
- MyGo：<https://github.com/egoist/mygo>，MIT 许可，当前依赖锁定见 `go.mod`。
- 此 UI 复刻原型按 AGPL-3.0 提供，完整许可见 `LICENSE`。保留 Noonmark 的来源及其 AGPL-3.0 许可义务。MyGo 与其他依赖保留各自许可。

没有修改或发布上游仓库，没有上传或接入任何用户业务数据。

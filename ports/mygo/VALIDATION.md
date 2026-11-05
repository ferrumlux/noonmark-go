# 当前标题接线验证入口

2026-11-06：隔离 development 日待办标题原生编辑已接入，完整范围、实际UI／磁盘fault回归、有序读视图与未验收项目见 [NATIVE-UI.md](docs/tasks/connect-title-autosave/NATIVE-UI.md)。下文保留已有其他批次验证记录，不能将旧原型截图或编译当作真实系统IME／Swift oracle。

# 验证结果与局限

## 执行环境

Linux amd64；Go 1.27.3；CGO_ENABLED=0；MyGo 固定提交 977ea57e0baf78da62698ba7371420d192260466。测试和渲染使用 Noto Sans CJK SC、Pango/Cairo/Fontconfig。

## 已通过

- 编译当前原型。
- go vet 静态检查通过（退出码0），记录见 vet-results.txt。
- TestNativeScreensAndNavigation：三页往返、中文输入、重复勾选/取消、日期选择、侧栏收起/展开。
- TestNativeSnapshotSizes：三个视图均输出2400×1536像素。
- TestNarrowWindow：960×720下导航与日历标题的冒烟测试。这不是小窗口像素布局验收。
- 逐张目检导出的 PNG，并对照同名官方截图；已修正首轮的子任务徽章重叠、右侧段落重叠及日历工具条覆盖。

原始测试输出见 test-results.txt；截图渲染输出见 render-results.txt。

## 未通过/未运行

桌面窗口启动尝试因环境没有 DISPLAY / WAYLAND_DISPLAY 而退出，原始错误为 `mygo: cannot open display (is DISPLAY or WAYLAND_DISPLAY set?)`。这不是应用业务逻辑错误，但表示本轮没有验证操作系统窗口/GPU路径。

未运行 macOS 或 Windows 实机验证、系统快捷键、菜单、无障碍读屏、输入法候选窗、平台窗口行为。截图不应描述为 macOS 实机截图或像素级完全一致。

## 主要视觉差异

- 字体/字重与抗锯齿：Linux Noto/Pango 对比 macOS SF/PingFang/CoreText。
- 图标：手写 SVG 近似原版 SF Symbols，图形与细节尚不相同。
- 若干行间距、徽章宽度、文本截断和日历状态摘要未做到逐像素对齐。
- 三个页面的可视区域已搭建，其余七页仅为占位说明；不是十页完整复刻。
- 固定演示内容、进程内状态，复盘数字不会根据勾选动态重新计算。

## 第二轮覆盖与检查

已扩展到20个视图，详见 SCREEN_INDEX.json：其中18个分别对应官方中文截图，Sync/Provider两个设置页由源码推导，尚无公开参考图验收。10个主页面已不再使用占位说明。

最终检查包含：20个注册视图的渲染尺寸；10个主导航往返2轮；便签布局往返3轮；飞光编辑/取消及中文草稿；设置选择/撤销、分类预览开关；快速记录取消；飞光速记中文多行；打开独立工具窗的应用内快捷键；Esc关闭回调。go test 与 go vet 均须以包内最新日志为准。

各组新增页面均逐张对照参考和集成截图，已修正重复计划状态文字、完成时间对齐、飞光输入区遮挡工具条、设置分段控件标签命中、浮窗文字垂直位置等问题。原来“其他7页占位”的说明仅描述第一轮，第二轮已覆盖这些主页面。

尚未覆盖完整任务编辑、全局搜索、排期菜单、重复规则编辑、分类生命周期操作、Provider配置和连接错误、同步冲突/确认等全部交互状态；本轮是主要可视场景原型，不是完整UI行为克隆。平台字体、图标、系统窗口以及部分换行/间距差异仍存在。

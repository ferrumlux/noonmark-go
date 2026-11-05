# MyGo downstream native title boundary

基线：github.com/egoist/mygo v0.7.5-0.20261105040656-977ea57e0baf（977ea57e0baf78da62698ba7371420d192260466）。保留原 MIT LICENSE；应用维持 AGPL-3.0-only。0001 修改七个源文件，0002 新增两个合成回归测试文件，manifest 固定官方归档、patch及每个文件前后hash。

本bundle由统一scripts/environment准备到外置副本。不能直接覆盖Go module cache，不能只改hash消除不明差异。旧根目录0001-preserve-preedit-before-focus-clear.patch是保留的历史最小候选，不再次叠加。

契约与边界见 docs/tasks/connect-title-autosave/DEPENDENCY.md。修复原生已经交付但尚未更新string binding的最后字符，并提供帧间最终化；不代表真实OS IME或应用退出接线已完成。

## Native ContextMenu 生命周期补丁

0003-native-menu-lifetime.patch 在同一官方基线上修改 ui/menu.go、ui/headless.go 和 content.go；0004-native-menu-regression-tests.patch 新增 ui 与根 package 的合成回归。前两份标题补丁保持原字节及 hash，不重新生成。manifest 固定新增 patch 与各目标文件前后 SHA256，准备器继续 fail-closed，正式 module cache 不修改。

每次实际交给 host 的 popup 有独立实例及 host 身份，只接收一次 callback；新菜单请求、owner 消失或禁用、窗口关闭或 host 改变会使旧 callback 失效。业务选中和文字编辑命令共用守门。现有同步 PopupMenu 返回后沿原 int callback 发出 0 表示结束，Tester.CloseMenu 使用相同契约，无新平台 ABI。合法选择同步先于结束通知，结束不会冲掉已接受的选择。

详细证据、取消通知的来源和未验证范围见 docs/tasks/connect-return-menu/DEPENDENCY.md。此补丁不代表真实 OS 菜单、IME、Swift oracle 或平台事件次序已实机验收。

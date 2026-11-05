# 回池菜单：MyGo native 边界

## 范围与实现

官方基线固定 github.com/egoist/mygo v0.7.5-0.20261105040656-977ea57e0baf，Go 1.27.3。保留 native-finalize 的 0001/0002 标题补丁原字节，追加 0003（ui/menu.go、ui/headless.go、content.go）与 0004（两份永久合成回归），仍由外置依赖准备器生成只读副本。不更改原版 MyGo、官方 module cache、参考 Swift 树或远端。

原问题：native host 保存的旧 callback 可在关闭、同 trace 重开后写入同一个 element ID 和 label；只靠应用导航代际不能区分实际 popup 实例。

新契约：

- showMenu 绑定实际 shownMenu 实例和 host，askMenu / askMenuLater 使先前实例失效。
- menuChosen 先检验当前实例，再一次性消费；0、越界 ID、重复交付、旧实例不得变成操作，也不得消费新实例。
- 同一守门覆盖业务项及文字编辑命令，不只保护回池项。
- 已关闭或关闭中的 engine、不同 host、owner 消失、当前帧未建构或禁用时不接受。resolveMenu 在 owner 消失的帧即失效，重建同 key 不能恢复旧授权。
- 应用仍负责稳定业务身份与导航 key；框架不能从同一个 element key 推断它已代表另一业务对象。

## 取消通知与同步顺序

上游 content.go 只给选中项安装 Click callback，没有将 popup 结束传给 ui。原 Tester.CloseMenu 也只丢弃 callback。本补丁在 content.go 已有 postMain 闭包以 defer chosen(0) 报告结束，包括窗口在真正展示前已关闭。Tester.CloseMenu 对仍有 callback 的菜单报告相同 0。

只读检查的同步链：

- macOS internal/darwin/menu.go：NSMenu 的 popUpMenuPositioningItem 在菜单结束后返回，action 直接调用 MenuItemClicked。
- Linux internal/linux/menu.go：activate 直接调用 handler；deactivate 退出嵌套 gMainLoopRun，随后 PopupMenu 返回。
- Windows internal/windows/menu.go：trackPopup 返回命令后，PopupMenu 同步执行 menuCommand；0 不执行命令。
- Web internal/web/menu.go：事件循环结束后，非零选择直接调用 MenuItemClicked，再返回。
- 公共 menuItemClicked 直接执行 Click，contentMenu.Click 直接调用 chosen(id)；onMain 在当前主线程直接执行，不再入队。

因此源码契约是选择先交付、popup 返回后再交付 0；已消费的实例忽略该 0。没有新增平台 ABI。真实平台事件顺序仍需实机验收；此处不是 OS 运行证明。若自定义 host 不报告结束，未换菜单且未变化的单纯取消仍不能凭空被 ui 观察。

## 门禁与证据

永久 ui 回归保存真实 Tester native host 收到的 callback，不直接调用 menuChosen 或伪造业务选择：取消后同 owner 重开、另一 owner 替换、重复选择、选择后结束、显式取消、Tester 取消、非法 ID、owner 消失重建、owner 禁用、engine closed/closing、host 替换及旧文字 Paste。每个替换场景还验证当前合法菜单可用。根 package 通过真实 content connection 加 fake backend 验证无选择结束、窗口关闭而未展示也报告 0。

外置证据根：/tmp/noonmark-menu-boundary-build/evidence。

- native-menu-red-final.log：相同永久测试运行于仅有原 0001/0002 的独立副本，11 个 ui 生命周期分支、文字编辑回归和两项 content 结束分支均红。
- native-check-green-final.log：scripts/check-mygo-native 通过；82 个 ui 顶层测试及 1 个根 package 顶层测试，含原标题 native 回归、上游 menu/context/edit-menu 套件；race 与 ui、根 package vet 均通过。
- dependency-green.log：scripts/check-dependency 51/51，通过新追加补丁次序、逐文件 hash、frozen receipt 及不可重排检查。准备器实现、schema 与拒绝策略未变。
- native-all-ui.log：额外完整 ui 套件已结束（373.006s），仅 TestDiffFrameAllocs 失败，steady 3、warm 9，高于上游阈值 2。baseline-diff-allocs.log 在未加 0003/0004 的独立基线上复现同样 3/9（146.908s）；不改阈值、不跳过此失败，精简门禁不等于全框架通过。
- release-size.log：CGO_ENABLED=0、Linux amd64、trimpath、mygo_noinspector、production=1 的示例大小对照。hello 前后均 9,687,303 bytes；counter-native 从 9,781,511 增至 9,785,607 bytes（+4,096）。ELF text 分别增加 401/1,219 bytes，data/bss 不变；新增同步结束回调与 native 实例守门为小幅代码增量，没有引入新依赖或反射。二进制大小不是性能或实机验证。

首次通过准备器的 recipe ID 为 198cb91063d260e1a9c29a7fc8ea8b8de13610c48845be5ce3813345c12a9fed，receipt 记录 manifest、helper、源码目录与全部 prepared 文件 SHA256。整合到另一源码目录会自然生成不同 recipe ID；必须重新准备、验证 selection 和测试，不复制已生成依赖目录冒充新结果。

应用三批纯 Go、回池四个独立反例及六平台交叉编译的最终接合证据由 VERIFY.md 记录。Linux headless/fake backend 与交叉编译不是 OS、IME、Swift oracle 或非 Linux 实机验收。

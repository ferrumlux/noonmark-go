# 824e966：六 host 运行与窗口生命周期证据

本记录固定于 `824e96614a3c0196f9086505d201aae23b7c6de2` / tree `5509119fc599aec2cb2df4a80640a588f13c6cde`。不是后续代码的验收，也不是正式发行许可。

## 已核实

- [常规 MyGo CI 42297385679](https://github.com/ferrumlux/noonmark-go/actions/runs/42297385679) 成功；四模块 race/vet、Unicode16、依赖与路径边界、六目标编译及 Linux 合成 CLI/headless 通过。完整日志与实际六成员产物逐项核验。
- [六 host CLI/headless 42297385613](https://github.com/ferrumlux/noonmark-go/actions/runs/42297385613) attempt 1 的 build 和六 runtime 全成功。13个实际 ZIP 的 API digest/size/CRC、六 binary 架构/哈希、exact artifact ID、完整日志、30个预期命令退出结果与12张合成 PNG 核验通过。
- [六 host 窗口生命周期 42297385629](https://github.com/ferrumlux/noonmark-go/actions/runs/42297385629) attempt 1 的 build 和六 runtime 全成功。另13个实际 ZIP 与六 binary 核验通过；十二阶段 Create→Open 的应用/driver/READY、正常关闭、首次 Service.Close、进程收割及同会话快照一致。
- 目标为 Linux / Windows / macOS × amd64 / arm64。macOS ARM 本轮两种 smoke 均实际取得 runner 并运行；不是用旧轮失败或其他目标推断。CLI 的 ARM host query 为 hw.optional.arm64=1、sysctl.proc_translated=0。

[逐产物身份与哈希](evidence-824e966.json) 是已验证原始材料的精简索引，只包含公开项目证据。没有运行目录、凭据、环境秘密或生产资料。原始ZIP没有嵌入仓库；Actions当前保留7天，链接过期后不能用摘要冒充原始字节仍可访问。

## 尚未通过的范围

窗口仅 `lifecycle-only`，`display_pixels_verified=false`、`ime_tested=false`。第一帧提交和可见句柄不证明系统实际像素；Linux使用隔离Xvfb/software GL，不是用户桌面。CLI的PNG为空开发会话headless渲染，不是系统截图。中文IME、DPI、完整输入/焦点/剪贴板/文件选择、休眠唤醒与完整业务仍由 [#9](https://github.com/ferrumlux/noonmark-go/issues/9) 保持打开。

[#12](https://github.com/ferrumlux/noonmark-go/issues/12) 只闭环六host CLI/headless smoke。生产迁移、真实AI/同步、安装、签名和发行未因本记录获准或完成。

## 性能风险和历史

本轮常规CI根应用race **587.848秒**，原默认600秒门槛仅余 **12.152秒**。成功不代表稳定余量；[#3](https://github.com/ferrumlux/noonmark-go/issues/3) 继续测量和优化，不修改门槛掩盖风险。

旧a3 ARM Mac两轮未获runner、初次本地Xvfb准备失败及冻结清单回归均保留为独立历史，新的成功不覆盖旧事实。详见 [GO-NATIVE-HOST-001](../../engineering/failure-cases/GO-NATIVE-HOST-001.md) 和 [GO-NATIVE-WINDOW-FREEZE-001](../../engineering/failure-cases/GO-NATIVE-WINDOW-FREEZE-001.md)。

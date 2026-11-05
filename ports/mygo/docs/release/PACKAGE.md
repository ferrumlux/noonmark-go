# Noonmark Go 便携程序包

此包包含一份目标平台二进制、源代码身份、AGPL-3.0-only 许可与依赖许可。请先核对同一发行的 `SHA256SUMS`。完整对应源代码位于同一组资产的 `noonmark-go-<version>-source.tar.gz`，包括构建脚本、全部项目源码、所用依赖源码及 MyGo 补丁；无需向维护者申请获取。

`SOURCE.json` 记录精确版本、commit、上游来源、工具链与依赖。`TARGET.json` 记录本包目标与二进制 SHA-256。`candidate` 包只供隔离开发验证，不能用于真实资料。当前应用仍须显式使用隔离 development session 或静态 prototype 参数；双击不等于完整生产启动流程。

Linux/macOS 包为 tar.gz，Windows 包为 zip。解压后程序为 `noonmark-mygo`（Windows 为 `noonmark-mygo.exe`）。本包是便携命令行启动的桌面程序，不是安装器或 macOS `.app`。请参阅对应源码中 `repository/ports/mygo/README.md` 的实际启动参数与当前功能限制。

## 系统依赖和分发边界

- Linux：原生桌面需要可用的图形会话及 GTK 3、Pango、Cairo、Epoxy、EGL/OpenGL、Fontconfig 和中文字型。Ubuntu 24.04 的开发验证安装命令见源代码 `.github/workflows/mygo.yml`。不捆绑系统共享库或字体。
- macOS、Windows：使用操作系统原生库。最低系统版本及各架构的真实运行支持必须以正式验收报告为准，交叉编译不证明兼容性。
- 没有发布者代码签名、Apple notarization、stapling 或 Windows Authenticode。macOS ARM64 链接器生成的 ad-hoc 签名不是开发者身份签名。本包不宣称 Gatekeeper／SmartScreen 信任。不要绕过操作系统安全警告；有警告时停止并核查来源。
- 打包不启动应用，不读取用户资料、iCloud、Keychain 或 AI 凭证，也不自动上传诊断资料。

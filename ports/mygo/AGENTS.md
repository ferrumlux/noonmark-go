# Go + MyGo 移植工作区

继续遵守上游基线 AGENTS.md（公开来源 quboliu/noonmark，43e6eef），以下为当前用户明确授权的隔离跨平台移植边界。

- 当前源码在新建本地副本，原上游与最初原型、三批模块目录一律不改。本轮明确要求新建副本，取代根文件针对另一台 Linux 主机固定路径的历史指令。
- 这是原生桌面移植，不是既有 Web 服务端提案。禁止把 Go 原型切换为生产实现或声称已全面对等。
- 源码仅放在 ports/mygo；缓存、二进制、运行资料和新证据放在显式外置 NOONMARK_MYGO_BUILD_ROOT，不得建立 .build、artifacts、dist 或兼容链接。
- 本轮只能运行隔离 development/e2e 身份，资料通过领域命令合成。禁止 production、iCloud、Keychain、真实 AI 网络调用。远端 Git 只能用于用户已批准的 ferrumlux/noonmark-go 备份。
- 只允许经过严格验证的空 catalog 分类历史子集；真实非空 catalog 及其他未接通 supplement 仍 fail-closed。真实 macOS Swift 名称 capture 已取得，固定向量的 Go 名称／等价字段1807项对账吻合；仅此有限范围，不证明 Swift Character 分段、完整实体 codec、分类业务或非 Linux 产品／UI 实机对等。证据见 docs/tasks/verify-swift-name-comparison/evidence-42185254910.json；Swift 仅为独立研发对照工具，Go + MyGo 产品无 Swift 运行时依赖。
- 文档使用新加坡中文。UI 改动先阅读 DESIGN.md 和本轮设计；保留原型模式用于视觉回归，真实运行模式不可伪装静态预览功能已接通。

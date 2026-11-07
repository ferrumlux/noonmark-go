# 隔离普通分类开发入口候选 v1

本批依据入口设计 R2（SHA256 ae47397bc172f761ec974b2fe1a096ef6a39db5cca1c3bb78bf0f42fb9e0a80d）。默认仍是 legacy-development；显式 ordinary-development 才接完整 ordinary Service 与分类 UI。不是生产导入、正式缺省切换或 Swift 互通承诺。

## 实验启动

使用既有 scripts/build 与显式外置 NOONMARK_MYGO_BUILD_ROOT，二进制在该外置目录 bin/noonmark-mygo。父目录须已存在且为普通目录；session 必须绝对、clean、无符号链接父路径，新建目标不能已存在。资料仅通过领域命令合成。

以下为 Bash 示例；先将占位路径换成实际隔离会话的绝对路径，保持参数引用。新建目标必须不存在，冷开使用同一会话路径。

新建：`"$NOONMARK_MYGO_BUILD_ROOT/bin/noonmark-mygo" -runtime-profile=ordinary-development -init-session "/absolute/isolated/session"`

冷开：`"$NOONMARK_MYGO_BUILD_ROOT/bin/noonmark-mygo" -runtime-profile=ordinary-development -session "/absolute/isolated/session"`

窗口显示「隔离开发会话 · 普通分类」。标准错误提供真实绝对路径、profile、入口协议版本及重开参数示意。日志中的程序名为 `noonmark`，构建产物名则为 `noonmark-mygo`；重开时须使用实际可执行文件路径、`-runtime-profile=ordinary-development` 和指向原会话的 `-session`。日志路径使用 Go `%q` 格式，并非通用 shell 安全引用，不能直接当作所有 shell 可复制执行的命令；请按所用 shell 正确引用实际路径，尤其留意 `$`、反引号等字符。构建含 VCS 信息时显示修订；既有 buildvcs=false 构建明确显示 source-revision-not-embedded，不捏造提交身份。验收以固定源 SHA 与产物 SHA256 索引为准。

ordinary 与 render-one、prototype、list、全屏导出互斥。未知／重复／空／缺值参数、位置或尾部参数，均在报告、目录或 Service 副作用前拒绝。原 prototype、legacy render-one 与 list 路径保留。native smoke 仍只是生命周期探针，不因支持显式 profile 就成为分类业务或真实输入验收。

## 数据与错误

创建和打开使用同一 factory，创建 hub 后、任何窗口 attach 前启用分类；原 legacy default gate 不改。同日完整冷开不增加 revision，跨日 Open 会独立日结。非空 ordinary 会话送 legacy 必须拒绝，不自动换路径或新建空库。

参数校验失败没有写入。进入 Create/Open 后的失败不作零写保证：初始 revision 或启动日结可能已写入，目录保留，需检查；不重试、回滚、截尾或 fallback。enable 失败先清理 lane/句柄，不创建可编辑窗口。原生事件循环启动失败也报告可能已写入。

## 证据层次与限制

- A 入口候选：纯参数、路径与报告独占负例；真实二进制 CLI 拒绝；统一 factory 创建／同日／跨日冷开；非空 legacy 拒绝；两个 OS 进程写入竞争；两窗口共享与原三项 UI 红例 focused 回归。具体命令及结果在最终报告按固定 SHA 列出，未执行不计 PASS。
- B 平台交付：当前云环境无 DISPLAY、Xvfb 或 xvfb-run，未执行真实 native 分类业务、IME、菜单、DPI 或六平台实机。headless、模拟窗口和跨编译不能替代。
- C 独立声明：Swift 全状态互通、生产准入、硬断电耐久均未由本批验证。

完整四模块、全回归、跨平台构建及最终组合门禁由后续固定组合统一执行，不把本批 focused 结果相加当全门禁通过。Open 内部各持久化阶段 fault 注入需要在后端测试范围补齐；没有将事务方法的 fault 测试冒充 Open 自身返回 nil,error 的逐阶段实证。

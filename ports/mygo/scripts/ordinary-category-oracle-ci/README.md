# 普通单分组 Swift 参考捕获：仓库集成

本目录只管理独立研发参考器的 CI 边界和有界证据导出。Go + MyGo 仍是唯一产品；没有 Swift 运行时依赖。本次仅完成本地仓库候选、Python/static/mock 验证，没有触发 CI、执行 Swift、生成 golden 或改变产品准入。

## 精确来源和审查边界

相邻 `ordinary-category-oracle/` 是自包含参考器。文档／来源描述更新后，工具清单和运行器的来源锁必须同时更新并重新审查，不能把旧清单的验收视为新清单的验收。真实 Swift 源码、冻结输入和14个 Core+LICENSE不因文档变化而改变。

- `TOOL-MANIFEST.json` SHA256：`4d81f1784ef074bba86895dda6127fcce4b293ad1f6a6ac3ed666a28519a0fe8`。
- `SOURCE-CLOSURE.json` SHA256：`ad948c7383b6af6b7cc3a82e0d7752982a9cadd5d38101bd683ccdde6cb0e31d`。
- `runner.py` SHA256：`3464a455677d7e45b77d622aa05feb7674d0ba65bf6382ab7e8e2a2f99bc715c`。
- `OrdinaryCategoryOracle.swift` SHA256：`4fc05129212c72819c8939eaa47dcca08458e902656850c3b58cf54b8dcfa579`。
- `INPUT.json`：6181 bytes，SHA256 `2a3f43ffb5a78165ce9d32958089e0e3973280829fa5f3ab33f69fa44ea70aae`。
- 来源为 [quboliu/noonmark@43e6eef](https://github.com/quboliu/noonmark/tree/43e6eefaae3a04be89cab3e234d470111afd80ef) 的14个 Core 文件和完整 AGPL-3.0 LICENSE，逐项 bytes/SHA256/Git blob 在清单中核对。没有 Storage、App 或完整 snapshot 的执行入口。

独立的 **65 case Swift 原件尚未捕获，Go 对照也未完成**。65 是本 suite 的观察数，不是领域通过数；不能追加到既有 1807 名称套件的分母。旧 name/fullSnapshot/I1 脚本、输入、分母和门禁均未修改，`semanticParityVerified=false`、`runtimeAdmission=false` 继续有效。

## 限定 workflow

`.github/workflows/ordinary-category-oracle.yml` 只允许 `main` 的三个专属路径 push，及显式 `workflow_dispatch`。三个路径是本 workflow、参考器目录和本 CI 辅助目录。没有 PR、PR target、schedule、release、production 或外部仓库触发。

- 权限只有 `contents: read`，checkout 不保存凭据，checkout/upload-artifact 均沿用现有 name oracle 的固定 40 字符 SHA。
- `macos-15`，`DEVELOPER_DIR=/Applications/Xcode_26.2.app/Contents/Developer`。**此版本在实际 hosted image 上是否可用仍需未来 job 核实**。runner 会核 Xcode 26.2、Swift 6 和选定 developer 下的 swiftc；缺失即失败，不下载、不安装、不升级、不改选其他 Xcode。
- `cancel-in-progress: false`，job 上限 30 分钟。不使用 secrets、签名、发布或 deployment。经审查的提交进入 main 且涉及专属路径时，可触发此独立 job；也可显式 workflow_dispatch。
- 外置新 work/evidence 根均为 runner.temp 下含 run_id/run_attempt 的专属路径。原 runner 要求 work root 不存在，导出器同样拒绝覆盖已有 evidence root。测试只用自动删除的外置临时目录，不在源码内建立缓存、二进制或证据。
- 真实命令是 `runner.py capture --reviewed-manifest-sha256 … --work-root … --developer-dir …`。观察器 Swift 可执行程序仅接收冻结 INPUT 和 `work/run` 两个路径参数。

## 来源封存、sandbox 和进程预算

runner 的执行边界如下：任何子进程前，14 Core + LICENSE + 全部已审工具/输入被核 bytes/hash 并复制到 private `work/input`，文件 444、目录 555；每个进程之前和最终 receipt 之前再次核实体身份和内容。编译／运行引用 stage，不读取 live checkout。

五条平台预检每条 20 秒，两个 Swift 编译共享 900 秒，观察器运行 60 秒；每条命令 stdout/stderr 各最多 64KiB。进程监督涵盖 selector/setup/exit/timeout/超量，成功和失败均清理进程组，有界 wait 和残留组核验。清理不确定不能生成成功 receipt。上述 runner 上限加清理、测试和上传留在 30 分钟 job 上限内；job 取消或 runner 崩溃不保证 always 步骤有机会完成。

全部预检、编译和运行使用同一 deny-default/deny-network 策略，只读系统、选定 Xcode 和本次 private work；HOME/TMP/cache 是隔离目录。首次真实 macOS 执行还必须验证 sandbox 对 SDK、Foundation/CryptoKit/ICU 和 dylib 加载的实际兼容性。遇到拒绝应保留有界诊断并停止，不放宽策略绕过。

## 封闭证据导出与失败行为

`export_evidence.py` 不启动子进程、不调用 Swift、不开网络，也不递归归档 work root。上传 action 只指定两个精确文件：`ordinary-category-evidence.zip`、`export-summary.json`。

ZIP 内封闭清单如下：

1. 完整成功捕获：经原 `verify_capture` 验证的 `run/manifest.json` 及其明确引用的 synthetic observations/raw JSON。固定 65 IDs、每个 role/reference、成功阶段原件、原 UUID、Date bits、实际 digest 与重建 payload 绑定均复核；导出时再核每文件 bytes/SHA256。单观察 64KiB、单原件 256KiB、512 原件、总原件 16MiB、清单 128KiB。保存 Foundation 原字节，绝不重编码或替换 ID/bits/digest。
2. 成功 capture receipt、五个精确 provenance 文件（来源/工具清单、INPUT、LICENSE、stage identity）、有界 platform.json。receipt 必须绑定输出清单、source/tool/input、stage identity 和八条命令 receipt，parity/admission 必须为 false。平台的 `officialHostedImageVerified` 仍为 false，不能由捕获器自证 hosted 来源。
3. 八个固定 label 的 command JSON，各自精确 stdout/stderr raw 或 diagnostic-prefix。每管道 64KiB，command JSON 64KiB；核文件角色、路径和摘要。不收 shell 环境、任意日志、工作目录、二进制、HOME/TMP/cache、完整 toolchain 或其他资料。
4. 失败时只保留已核 shape 的 runner-failure.json（最多16KiB）、run/failure.json（最多16KiB）、有界 platform 和已经写出的固定命令证据。未完成 manifest 的部分观察／state 不导出。平台或版本检查在 work 创建前失败时，仍导出小型 `capture-not-started` 摘要。
5. 任一证据不合约、链接、缺件、超量或摘要冲突时，丢弃待导出内容，只写固定 `evidence-validation-failed` 摘要并返回非零；不把未核异常文字复制进附件。原 capture step 非零始终使 job 失败，没有 continue-on-error。所有输出拒绝 symlink/hardlink，ZIP 无可执行属性，只含 UTF-8 文件。总解压内容上限20MiB、文件数上限550；ZIP 本身亦受大小上限约束。

命令诊断可能含本次 runner 临时路径和公开 Swift 源码／编译信息；它们来自此隔离执行。不会收集真实用户资料。export-summary 只表示捕获／导出完整性，不能证明 Go 语义对等。即使将来有真实 `captureComplete=true`，仍需独立检查领域结果及 Go 对照，才能提出后续产品准入变更。

## 本地轻量复现

在仓库根执行：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -B ports/mygo/scripts/ordinary-category-oracle/runner.py verify --reviewed-manifest-sha256 4d81f1784ef074bba86895dda6127fcce4b293ad1f6a6ac3ed666a28519a0fe8
PYTHONDONTWRITEBYTECODE=1 python3 -B ports/mygo/scripts/ordinary-category-oracle/test_contracts.py -v
PYTHONDONTWRITEBYTECODE=1 python3 -B ports/mygo/scripts/ordinary-category-oracle-ci/test_ci.py -v
```

原 34 项 Python 合同与本集成 70 项 Python/static/mock 检查通过。后者包含 24 项 workflow 单点危险变更拒绝、封闭上传／失败保留、路径／预算／链接负例，以及明确标为 synthetic 的协议完整性正控制。所有伪造 receipt/原件只在测试临时目录使用并删除，未保存成 Swift 证据或 golden。

workflow 的安全合同采用 exact-byte SHA 锁，不是部分 YAML 解析器；新增任意 workflow 字节都须重新评审后显式更新锁。已用本机已安装 PyYAML 独立解析并核关键字段；actionlint 未安装，因此没有 actionlint 通过结论。没有 Go 重型构建、Swift 编译、macOS sandbox、产品运行或 hosted CI 通过结论。

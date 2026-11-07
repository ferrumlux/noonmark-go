# 普通单分组 Swift 独立参考器

仅用于合成 ordinary 分类的研发对照。Go + MyGo 仍为唯一产品，产品没有 Swift 运行时依赖。本次只有 Python/static/mock 验证，尚未实际编译或执行 Swift，没有 golden、Go 业务对账结论或产品准入变更。

## 文件和来源

- PLAN.md：设计范围、真实 API、固定序列与运行边界。
- SOURCE-CLOSURE.json：固定公开上游 commit、14 个 Core 文件和 LICENSE 的完整来源／字节身份。
- SOURCE-VERIFICATION.json：该来源核验阶段的逐文件结果，不代表 Swift 执行或当前测试状态。
- SCHEMA.md：65 个独立观察的阶段、原件、摘要与日期位契约。
- OrdinaryCategoryOracle.swift、runner.py、INPUT.json、TOOL-MANIFEST.json：观察器、隔离运行器、6181B 冻结输入和全部工具身份。
- test_contracts.py：仅 Python 协议、source staging 和 mock 进程生命周期检查。所有 synthetic protocol 样本都不是 Swift 原件或 golden。

公开来源为 [quboliu/noonmark@43e6eef](https://github.com/quboliu/noonmark/tree/43e6eefaae3a04be89cab3e234d470111afd80ef)。14 个 Core 文件及完整 AGPL-3.0 LICENSE 保存在 upstream/，原始字节不改。新观察器文件 SPDX 为 AGPL-3.0-only。固定来源 URL、bytes、SHA256 和 Git blob 见 SOURCE-CLOSURE.json。

## 运行与证据

默认 `python3 -B runner.py verify` 仅核来源／工具 hash，不执行 Swift。工具、文档或来源清单任一字节变化，必须更新 TOOL-MANIFEST.json 并重新审查该 exact hash；旧 hash 的审查不能自动覆盖新包。

在确认的 macOS/Xcode26.2 环境中，显式 capture 命令为：

`python3 -B runner.py capture --reviewed-manifest-sha256 <已审工具清单SHA256> --work-root <全新外置绝对目录> --developer-dir /Applications/Xcode_26.2.app/Contents/Developer`

runner 不下载源码、不发布。先核全部来源／工具，复制到只读 stage 并核身份；Core 使用 -enable-testing 和 -package-name Noonmark。全部预检、编译和运行都使用独立 HOME/TMP/cache 以及 deny-default/deny-network sandbox。输出与源码不重叠。真实 prepare/commit/validateIntegrity/Codable 只涉及 classification section，不调用完整 snapshot/DataPackage 或旧 name/snapshot 套件。

仓库有专属 `.github/workflows/ordinary-category-oracle.yml`，只在 main 的专属路径 push 或 workflow_dispatch 下运行；具体权限、平台版本、预算和封闭上传规则见 [CI 说明](../ordinary-category-oracle-ci/README.md)。实际 hosted image 是否提供 Xcode26.2、sandbox 是否兼容 SDK/库加载，仍须实际 job 验证。失败停止，不放宽策略或替换工具链。

没有成功的真实执行便没有 source-bound captureComplete。即使 captureComplete=true，也只表示采集完整，不表示所有领域探针通过、Go semantic parity 或 runtime admission。独立65观察不加入既有1807名称套件分母，fullSnapshot/I1和runtimeAdmission=false保持不动。

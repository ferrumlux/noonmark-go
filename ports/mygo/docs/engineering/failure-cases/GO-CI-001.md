# GO-CI-001：CI context 与有界日志完成条件

状态：本地红→绿及实际 GitHub hosted run 已通过。首次发现：2026-11-05 UTC。修复提交：[f2f81df2defa7d2d71ca1035371d7089bdc73564](https://github.com/ferrumlux/noonmark-go/commit/f2f81df2defa7d2d71ca1035371d7089bdc73564)。

## 影响与归因

初版 CI 草稿存在两项缺陷：job-level env 使用 runner.temp，不符合 GitHub context 可用范围；`tee >(tail ...)` 的异步日志写入不在主 pipeline 等待范围，可在证据未写完时返回成功。均为本次隔离 CI 草稿引入，尚无引入 commit，未进入基线 `7e816a7986dffbf10ea5f5a9dbefb63beac2e8d2`。实际修改来自本轮 CI 实施工作，Git author/committer 尚未生成；不据此推断未来提交者身份。未影响用户资料或生产运行。

## 红→绿证据与根因修正

- actionlint v1.7.7 报错：`context "runner" is not allowed here`（job.env）。改成步骤内读取 RUNNER_TEMP，向 GITHUB_ENV 写入外置根；同版本 actionlint exit 0。validator 新增 job.env runner 引用拒绝样例。
- 独立审查在 tail 写入前注入 3 秒延迟，原实现约 0.05 秒返回成功，result.txt 已存在而三个阶段日志均 0 字节；4 秒后日志才写入。原命令成功不等于证据已完成。
- 现改为同步前台 Python 捕获，64 KiB 分块流式写控制台，内存只保留末尾 256 KiB，完成写入后 pipeline 才结束。三个阶段分别注入 0.2 秒延迟时，主流程等到约 0.69 秒，返回时三个日志已完整；模拟存储写入失败返回非零且无 result.txt。check/cross/CLI 各注入 exit 37，仍返回 37 且不生成成功标记。未用无界临时日志，也未吞掉 logger 错误。
- 两次开发中的本地 shell 曾因运行时原地改写脚本而出现 EOF（文件读取位置漂移），不能用作最终成功证据。最终运行前冻结文件，运行结束再比对六个当时文件的 SHA256 全部一致。

## 永久门禁

实际强制链：`.github/workflows/mygo.yml` 的 Validate workflow contract 步骤 → `ports/mygo/scripts/ci/validate.py` → `scripts/ci/test_runner.py`。

- validator：9 个安全拒绝样例，包括 job.env context、可写权限、浮动 action pin、非限定 artifact 路径；bash 语法；超过 512 KiB 的二进制 stdout 不截断且文件限于末尾 256 KiB；logger 写入失败返回非零。
- test_runner：8 个纯合成控制流 fixture，覆盖三阶段 exit 37、成功、延迟日志完成、writer 失败、非空 build root、源码内 build root。fixture 只在外置 TemporaryDirectory 复制 runner/capture，并明确不能替代真实构建。
- actionlint v1.7.7 固定版本复核命令见 `../../GITHUB-CI.md`；当前不是 workflow 内的 actionlint step，不把自制契约检查冒充完整 GitHub schema validator。
- Go failure registry 仅登记 Go test 名称，本 case 不往该 registry 伪造 Python 测试项；CI 的真实强制链如上。

## 最终本地症状与构建验证

2026-11-05 18:09–18:19 UTC，从全空外置 module/build cache 经公共 proxy/checksum database 下载依赖，实际 `scripts/ci/run` exit 0：四个模块 gofmt/race/vet、50 dependency preparation、37 native-input 回归、六 OS/arch 编译全部通过。使用实际 Linux amd64 产物完成 day/pool 无窗口渲染和 development identity/create/reopen/reuse/无身份拒绝。六份限定证据合计 15435 bytes。

完整框架 TestDiffFrameAllocs 的既存失败不被隐藏：只运行支持的定向 suite。跨编译不是 Windows/macOS 实机运行，Linux headless 也不是桌面窗口、IME、Swift oracle、签名或安装验收。没有正式发行。

最终真实 run 的 workflow/runner/logger SHA256：

- workflow：`f9b3d2838f522812bb8fa6b2be3b0e3b5127f22768120ae039a02a5e5c01cb73`
- run：`66ca0c449b82b0726f0d53c29364bcaa6f110a7e8903ae501f90527202e738a1`
- capture.py：`a71a552db0fd9edb18ab6b2f6c02d038c261027644aecbc2e71b41ac9a17ab2e`

真实 run 后只补充 validator/合成 fixtures 与文档，再次执行全套静态/合成检查与 actionlint exit 0；未修改上述三文件或任何应用源码。

- 最终 validate.py：`9320521565390ea2bb517b480c69be00402a49fd384e34733ead8c55ceb771f5`
- test_runner.py：`adc828bd3e94b9b48b6620a7bce2ee96a0726e00b7b0981747d8ac6356d4fe07`

## 发布／回滚

提交应引用 GO-CI-001，并在实际 Actions 完成后回填 commit/run 及结论。若远端环境失败，应保留失败日志与精确 commit，定位后复跑；不可将本地结果改写成 hosted-run 成功。必要时仅回退此独立开发 CI，不改应用数据、签名或安全设置。

## 实际 GitHub 托管验收

[run 42083897005](https://github.com/ferrumlux/noonmark-go/actions/runs/42083897005) 由上述 commit 的 main push 触发，2026-11-05 18:24:20–18:33:22 UTC；run 与 development job 127443770748 均为 completed/success，所有步骤 success。固定工具链安装、workflow 契约、冷缓存实际测试／六目标编译／合成 CLI 和证据上传均已完成，不是仅创建 workflow 或本地推测。

证据 artifact：mygo-development-evidence-42083897005-1，ID 13442358332，压缩包 4029 bytes，SHA256 `b8aaebb885f071d64758ecd32c5f3d7a28fafb9fb904a991d65f9d76de9f7169`，2026-11-12 18:33 UTC 到期。上传路径仍只限定 ci-evidence/*.txt。此验收只覆盖 f2f81df，后续业务提交必须核对各自 Actions 结果；不扩大为六平台桌面、Swift oracle 或正式发行通过。

独立下载核验 artifact：仅 binary-checksums、checks、cross-build、entrypoints、environment、result 六个 .txt，解压共 15459 bytes，ZIP 实算 hash 与 API 相符。实际日志确认四模块强制 race／vet 链、50 preparation、37 native tests、6 个 COMPILED 目标、day／pool 各 2400×1536 渲染与 CLI guards。权限仅 Contents:read 和 Metadata:read。upload-artifact 产生 Node20 被 runner 强制使用 Node24 的弃用 warning，但上传成功；后续升级仍需固定官方 SHA 和重跑契约，不忽略该维护信号。

第二次真实 [run 42086088522](https://github.com/ferrumlux/noonmark-go/actions/runs/42086088522)／job127451446276 在2026-11-05约18:49 UTC全部success，environment精确记录 `f478b7668c0543a8ac459742708a70cb8214dc1f`。artifact13444800237实下载仅六个预期.txt，解压15459 bytes、ZIP4045 bytes，SHA256 `2ae104b48f3b4579db2ff0f5f35e0ea322e8cf232bda8531dc7dc4b755ab527a` 与API一致；仍无权限／上传范围扩张。新菜单批次尚需独立核实后续run。

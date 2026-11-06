# DayTodo 窄读取 hosted CI 记录

Refs #2 / #3。本记录补齐 DayTodo 窄读取批次的 hosted development 门禁；#2 须待本记录合入后才闭环，不代表 #3 的持续性能裕度或 #9 的跨平台实机验收完成。

## 版本与结果

- [GitHub run 42265932787](https://github.com/ferrumlux/noonmark-go/actions/runs/42265932787)：push、attempt 1，官方结论 `success`，development job `128057714249` 的10个步骤均成功。
- 被测 [commit 0ee21539d87453e630b559a3474c1d23a0d15a21](https://github.com/ferrumlux/noonmark-go/commit/0ee21539d87453e630b559a3474c1d23a0d15a21)，tree `0092877493116d00dadbf997d94dfe13193797d6`。
- Ubuntu 24.04、Go 1.27.3；使用全新外置 module/build cache，没有恢复缓存。job 完成时间为2026-11-06 18:17:38 UTC（新加坡时间11月7日02:17:38）。

此前 `f541dc28f4b6d0e13da36f38151424fe342f8e44` 的本地串行验证包括年度入口与 root 557.039s；本次 hosted 结果绑定上列 `0ee21539`，两者不混用，也不是受控性能 A/B。本记录属于随后纯文档补充，不能将其后文档提交称为本次 CI 已测版本。

## 实际入口与完成范围

精确版本的 [workflow](https://github.com/ferrumlux/noonmark-go/blob/0ee21539d87453e630b559a3474c1d23a0d15a21/.github/workflows/mygo.yml) 先执行 `"$RUNNER_TEMP/noonmark-ci-python/bin/python" -B ports/mygo/scripts/ci/validate.py`，再执行 `bash ports/mygo/scripts/ci/run`。

[CI 入口](https://github.com/ferrumlux/noonmark-go/blob/0ee21539d87453e630b559a3474c1d23a0d15a21/ports/mygo/scripts/ci/run) 依次调用 `bash "$APP/scripts/check"`、`bash "$APP/scripts/cross-build"`，再以 `NOONMARK_MYGO_BINARY="$ROOT/bin/noonmark-mygo-linux-amd64"` 调用 `bash "$APP/scripts/test-entrypoints"`。

- [check](https://github.com/ferrumlux/noonmark-go/blob/0ee21539d87453e630b559a3474c1d23a0d15a21/ports/mygo/scripts/check) 对应用根模块、domain-core、domain-extensions、domain-codec 逐一完成 gofmt、`go test -p=1 -race -buildvcs=false ./...`、`go vet -p=1 -buildvcs=false ./...`。没有追加 timeout、short 或筛选参数。vet 无输出，其成功由精确源码的 fail-fast 顺序及后续阶段完成共同确认。
- root 包491.495s，internal/taskapp 包219.937s，均为包耗时而非整个 job 耗时。默认每个测试二进制600s超时不变；root 本次名义裕度108.505s，只是单次结果，不保证后续 runner 时长或 writer 尾延迟。
- Unicode16：19,965行 normalization、NFC 与 NFD 各99,825项、1,093行 grapheme-break、1,112,064个有效 scalar 检查完成。1,093行不是 folding 数据；本入口固定 `RUN_ICU76=0`。
- dependency 验证51项 Python 测试通过。受支持 native 门禁通过，selected-layout 与全部五个 role 子测试有明确 PASS 输出。
- linux/windows/darwin × amd64/arm64 六目标编译完成；Linux 合成 development 身份、新建／重开／reuse 拒绝护栏及 Day/Pool headless 渲染完成。
- artifact 与完整 job 日志均出现 `All supported development gates passed. No release or non-Linux runtime claim.`。

## 证据摘要

官方 run/job 结果、完整解码日志、artifact 元数据与下载 ZIP 字节已交叉核对；相关17份源码逐一按该 tree 的 Git blob SHA-1 核对。以下 SHA-256 只标识对应字节，不是签名证明：

- Artifact `13528327962`，ZIP 6,458 bytes：`2f75ade2998ed29ecdaa90e9a849936b3890e98b83fe3b3e13dcaebd25446e1e`。与 GitHub 元数据及上传日志一致，ZIP CRC 校验通过。
- 完整解码 job 日志，UTF-8 77,446 bytes、920行：`23a9d19eb14275f77d9410084297e7ba08a4317f2484577ef1b9470380a74574`。此摘要针对解码后的全文，不是 GitHub 日志压缩包。

ZIP 只有 environment、checks、cross-build、entrypoints、binary-checksums、result 六份普通文本，阶段输出与完整日志对应。没有可执行文件、源码压缩包、production 资料或发布包。二进制校验清单已记录，但 ZIP 不含二进制，未以它独立重算二进制字节。

GitHub artifact 保留至2026-11-13 18:17:34 UTC；本文保留身份与摘要，不宣称提供永久附件下载或已签名归档。upload-artifact 的 Node.js 20→24 弃用提示为非致命 warning，本次上传与 job 均成功。

## 尚未验收

六目标仅编译，Linux headless 亦不等于任一目标的真实桌面窗口或系统 IME 验收。没有运行独立 ICU76 differential、完整 framework allocation suite／TestDiffFrameAllocs，亦没有新增 Swift oracle、生产资料验证、签名、部署或正式发布。根模块输出非 verbose，不能据此逐项声称每个应用测试的独立日志可见。此前本地年度入口的结果仍绑定其原版本，不冒充本次 hosted 独立年度入口结果。

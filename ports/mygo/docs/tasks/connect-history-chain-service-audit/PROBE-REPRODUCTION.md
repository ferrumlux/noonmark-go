# 最大完整事件包探针复现

将 `maximum_full_event_envelope_probe.go.txt` 与 `reproduce-maximum-full-event-envelope` 一同保存即可；不用保存原外置绝对路径。只在已经具备冻结三实现、两个独立测试及对应 baseline 的 Go+MyGo 端口运行。

本探针不进入默认测试，不改源文件，不连接会话或 Service。它用外置 Go overlay 在内存编译视图中追加一个测试函数，复用冻结测试的 purecore fixture/canonical helpers。运行前确认其他重型年载荷或最大AST测试空闲。

需预先提供 Go1.27.3、已有外置 cache/module path（包含固定MyGo archive），并新建或选定外置BUILD。不要复制/删除 cache，不要下载或改工具链。本次已实际运行的算法、overlay、log/exit在原审查证据中；下方便携包装在正式整合后已实际重跑，exit0/package4.051s，结果相同；初次独立审查时该包装仅作语法检查。

示例（替换为当前已有实际路径）：

```bash
export NOONMARK_GO=/absolute/existing/go/bin/go
export GOCACHE=/absolute/existing/go-build-cache
export GOPATH=/absolute/existing/go-path
bash ./reproduce-maximum-full-event-envelope \
  /absolute/repository/ports/mygo \
  /absolute/existing/external-build
```

包装会沿用端口 `scripts/environment` 准备固定MyGo补丁与 GOWORK，禁用 GOPROXY/GOSUMDB，使用 -p=1/GOMAXPROCS=2/GOGC=20/GOMEMLIMIT=700MiB。外置 evidence 下保存生成源码、overlay、log和exit。

测试函数依赖冻结 `history_vnext_independent_test.go`（SHA `0cc208feaef690b5142adf50e57d303b38e1e38d9a62898a6298f001fbc787c4`）的 iaPath/iaNow/iaWire/iaObj/iaNumber/iaMust/iaReject 与既有 imports，无需作者测试。包装先核对该SHA，避免在不同fixture上冒称同一结果。

已执行证据：53,285条固定A循环历史、16,777,023-byte完整envelope通过，53,286命中byte limit；实际canonical writer同时检查node/depth上限。该数字只属于此固定合法形状，不是所有snapshot统一event上限。源码树两个测试的SHA保持不变。

# 作者隔离验证记录

task-id: audit-history-lifecycle

2026-11-06。先因约 860 MB 可用空间暂停于测试草稿；父任务完成已获批准的单项旧缓存清理后，实际可用 2,424,352,768 bytes，才启动 real red。本 worker 未清理任何缓存。

## 顺序与结果

- 01-pre-implementation-red：退出 1，新增测试引用缺失 lifecycle API，真实缺符号编译 red。此后才新增生产 helper。
- 02-focused：退出 1，6.532 秒。一个测试先 Continue 形成 lineage，再把前一 U 改 A，预期 after 被 core invalidTraceContinuation 拒绝。修正的是错误 oracle，保留原路径作精确负例并增加明确标示无 lineage 的 multi-U 参数例。
- 03-focused：退出 0，6.769 秒。`-run '^Test(HistoryLifecycle|HistoryCommand|IC)'`，含新 lifecycle 及旧命令独立测试。
- 04-taskapp：退出 0，29.386 秒，完整 taskapp，`-count=1`。
- 05-taskapp-race：退出 0，179.061 秒，完整 taskapp，无 `-run` 过滤。
- 06-vet：退出 0，`go vet -p=1 ./internal/taskapp`。
- 07-final-focused：退出 0，6.509 秒，同 focused filter，对最终 exact production bytes 重跑。

完整 taskapp 与 race 后，仅把旧结果说明中 two audit entries 更正为 four；未改变逻辑。完整/race 的 shared command 文件 SHA 为 `0f2bee9e9529c9414d08c6efb80debe10e4bf0e124686a32d1d5c8a3a70ac63e`；最终注释更正后为 `a7ba8b24314bffb7503c1278ad0b35b306a015f0c60c23cf9b1907670d336c32`，已通过 vet 与 final-focused。新 lifecycle 文件始终为 `acd9060443ca1c160c72cc8346d8074ac2737de7c5e9b537fc7757caef41b4dd`。最终 exact-source 全量复核仍由独立／formal gate 承担，不把注释前 full/race 冒充 exact-byte 后全量运行。

外置 evidence/runs 等由本任务 BUILD 根的 `runs.tsv`、每阶段 `.command`、完整 `.log` 对账；所有失败日志保留。测试成功不掩盖 fixture 修正来源。独立实现复核及精确 formal aggregate 由父任务另行负责。

## 环境与约束

固定 Go 1.27.3；`scripts/environment`、显式外置 BUILD、`GOCACHE=/workspace/shared/noonmark-go-build-cache-20261105`、`GOPATH=/tmp/noonmark-gopath`、GOPROXY/GOSUMDB off、GOMAXPROCS=2、GOGC=20、GOMEMLIMIT=700MiB、`-p=1`。每次顺序运行，guard 检查 >=2 GiB 后才启动；没有增加 timeout、复制 toolchain、创建冷 cache、运行 release／六目标或触碰其余 CI cache。

源码来源 fixture 使用 core APIs；预期许可 delta 与分类 event 字段独立手写。multi-A、无 lineage multi-U 及极值参数不冒称 Service 可达。旧独立测试除显式扩充 AST 入口外语义不改。

## 验证限制

未接线 helper 只证明 frozen profile 下的一条 exact edge，不证明 current head、全图历史可达、写权限或 Store frontier。未运行 Swift oracle、非 Linux 实机、Service/store/UI 持久化、fault/reopen/多 handle 或生产路径。旧 v2 consumer 遇 A 的拒绝未解决；future X+A 仍必须拒绝。独立复核未完成前不称可正式整合。

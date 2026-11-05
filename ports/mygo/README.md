# Noonmark Go + MyGo 移植

当前是完整复刻的研发快照，不是功能齐全的发行版。原生 UI 原型与三批领域模块现已集中在此目录，尚未将 UI 操作连接到持久化。

- 原生 MyGo UI：20 个场景，详见 docs/UI-PROTOTYPE.md 和 VALIDATION.md。
- domain-core：TaskChain / Definition / Trace 有限内核、时间与历史不变量。
- domain-extensions：分类、重复计划生命周期及版本历史。
- domain-codec：候选 v7 规范编码/解码与安全校验，仅合成 fixture；Swift 互通尚未验证，非空分类 audit 默认拒绝。
- docs/migration-audit：完整产品与平台差距、测试库存。

本轮 Linux 原型测试、vet 以及三个领域模块测试已通过。历史模块证据见各自 evidence；六目标交叉编译不等于六平台实机验证。macOS / Windows 实机、Swift oracle、系统集成、SQLite/sync、AI 与完整领域/UI 接线仍待完成。

## 本地验证

Go 1.27.3；MyGo 与依赖已锁定。缓存和构建产物必须放在源码目录之外。

```sh
export GOTOOLCHAIN=local GOTELEMETRY=off
export GOCACHE=/absolute/external/build/go-cache
export GOPATH=/absolute/external/build/go-path
go test -buildvcs=false ./...
go vet -buildvcs=false ./...
(cd domain-core && go test ./...)
(cd domain-extensions && go test ./...)
(cd domain-codec && go test ./...)
go run -buildvcs=false . -list
```

UI 运行与离屏截图见 docs/UI-PROTOTYPE.md。当前默认仍为内存静态原型；不接入任何现有 Noonmark 数据。禁止生产资料、iCloud 与真实 AI 凭证用于测试。

AGPL-3.0。原项目 https://github.com/quboliu/noonmark，基线 43e6eefaae3a04be89cab3e234d470111afd80ef。MyGo 保留其 MIT 许可。

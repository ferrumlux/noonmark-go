# 延期／延续 Service 验证记录

当前状态：实现与冻结 v3 完成，作者集中 race/vet、scripts/check 全集均通过；独立 focused 与附加日期／有效 catalog 拒绝测试通过，独立 aggregate 最终结论待其单独确认。尚未正式整合、提交本 Service 批次或发行。

## 精确候选与环境

- 独立 candidate：trace-carryover-service-work，基线 ddee25c8fc6f8e7ae7ee1251277654f8095cbd12。
- core 两文件是独立验收冻结版：carryover.go 8e18447e5db674a4a0922ef14834c0c8a5b576f0d32424310c77dbee3ded7dd1；carryover_test.go c1bef356a8b5f042cbfe182de20c810b16b7f0a74d1f53a1dcc4076757d03df6。
- Service carryover.go：81576b3feeffc2ab850b5a3f79d980a1ff0e9f50cc41bd1fb56baf69625facaa。
- history.go：2b4d9e8b189fcbc178be022415766ba92e773776349ac6de749efab1a660cc46。
- schedule_audit.go：1cf056caf72c9f5311fa06419c806e1c2170ff78ea9311784c45868dbb822a05。
- source/tests/scripts/gates 冻结清单：外置 evidence/service-frozen-v3-source.sha256，清单 SHA256 7b7fda8a7fc1227015ec0027957a46ea6dddcbc0dcff592cc2c295f9effa89ee。
- NOONMARK_MYGO_BUILD_ROOT=/workspace/shared/noonmark-carryover-service-build-20261105；Go 为 noonmark-mygo-work/go/bin/go（1.27.3）；GOPATH=/tmp/noonmark-gopath；GOCACHE=/workspace/shared/noonmark-go-build-cache-20261105。所有执行先 source scripts/environment，资料仅本轮 development/e2e 合成 session。

## 已运行证据

- 原有 taskapp baseline 已通过，恢复后初版实现既有测试 7.418s。
- 原始 generic stripped continuation 红：stripped-continuation-red.txt。独立 child stripped、old-pool double schedule、historical definition、new-chain stripped 红来自 carryover-audit-build/evidence，对应永久独立测试原断言保留。
- full-batch-first.txt：完整 witness 初版误拒合法新 chain 多链 batch；原因是相同时刻按随机 ID 重排数组。调整为稳定真实插入顺序，未剥除 canonical 字段。
- full-batch-second.txt：全部 taskapp 通过 10.012s。
- frozen-v3-taskapp-race.txt：最终冻结版全 taskapp race 通过 54.768s。frozen-v3-taskapp-vet.txt：vet 退出0。
- frozen-v3-hash-check.txt：冻结清单全部匹配。
- frozen-v3-aggregate.txt：scripts/check 退出0。主模块 race 207.076s、taskapp 52.679s、core 10.782s；所有模块 race/vet、canonical16 离线验证、51项依赖门禁、MyGo native UI/host popup 门禁通过。

旧 service-frozen-source.sha256 对应资格-only 中间原型，已作废。v2 保持相同生产代码，但复制的独立 audit 测试未 gofmt，aggregate 在格式门禁立即停止；v3 只格式化该测试并完整重跑，不能将旧结果冒充最终全集。

## 覆盖与明确限制

真实 Service 流程覆盖 defer/continue 的新旧目标 day、开放／已完成 child、lineage 和新身份、备注／描述／manual progress、defer→settle→continue 链、reopen、重复／错误 owner／错误 source state／过去与无效目标、closed/blocked、倒退 wall clock、极值全局 frontier、两个 handle 竞争、历史 COMMITTED prefix 损坏。

四阶段写入故障对两个命令均检查空返回 ID、graph/snapshot/Today/revision 不发布。after-persist 保留不确定结果并封锁句柄，reopen 读取真实已落盘候选，不重试伪造成功。审计覆盖 exact before、完整输入／命令种类错误、全五个任务区段篡改、缺／多事件、坏时钟／状态、deferred 终态冻结、缺失审计不得重建、unsupported supplement 与非空 catalog 拒绝。

Generic 批次新增 trace 时必须能重建合法 Create/Schedule/Complete/Undo/Settle source 命令见证并整图 canonical 相等。支持 existing-pool 多链 Schedule+Settle、empty-before 三链 Create+Schedule+Complete+Settle、planned child materialization、真实 Return 后 Schedule、Complete→Undo 及之后 Settle。不是恢复唯一真实历史或证明当时用户授权，真实 Today 仍由 Service 绑定。无新 trace 的 generic 规则不声称在本批全部改成命令日志审计。

明确未支持的混合 generic batch：新 chain 同批 planned-subtask 编辑、多条 initial notes／历史 definitions；新 schedule 同批 child 专用编辑再完成父项；最终时钟无法按固定有界排序重建的复杂修改序列。已有 Service 的这些专用命令各自使用独立事务和专用完整 replay，不能借 generic 绕行。

Reader 新增严格 deferred 终端事实支持，旧 reader 不兼容这些新开发会话；回滚只回滚源码，不删除、改写或降级 session。没有 UI 接线、生产替换、网络／Keychain／iCloud／真实 AI 访问。Swift oracle、原生 macOS/Windows、断电耐久性、本地化文案对等均未验收。

## 365日合成负载性能探针

使用已有 manualServiceYearFixture：365日真实 core Create/Schedule/Complete/Settle 合成 365 chain/trace，364个历史日已保存完整空分类审计，snapshot 为 895,504 bytes。fixture 构造与关闭不计时，测量只包住一次真实 Service 命令，保留所有审计、规范化、prefix CAS、持久化和确认步骤。通过外置 Go overlay 加入独立 benchmark，不更改冻结源码。

Linux amd64、GOMAXPROCS=2、-benchtime=1x -benchmem：

- DeferCurrentTrace：1,589,181,214 ns/op（约1.589秒）；627,921,816 B/op；3,247,531 allocs/op。
- ContinueUnfinishedTrace：2,033,084,006 ns/op（约2.033秒）；627,296,704 B/op；3,244,822 allocs/op。

证据 evidence/frozen-v3-year-performance.txt；探针源码与 overlay 位于外置 performance/。单次 probe 会受并发构建与 GC 影响，不是统计基准或硬阈值。与先前约2秒 manual 写入同级，后续 UI 必须异步；本批没有接线或拿短 fixture 冒充一年负载。分配量为累计 B/op，不是峰值常驻内存。


## 独立审查补证

独立 reviewer 的 frozen-v3-focused-race.log 通过17.440s，所有原始红断言转绿。另在独立 trace-carryover-audit-v3-review 中增加、单独冻结与运行两个测试文件：carryover_independent_date_review_test.go 证明 actual Today 延续成功、Today 延期拒绝、locked target 拒绝；carryover_independent_catalog_review_test.go 用 schema-valid category/label 证明两个真实 Service 命令拒绝非空 catalog，不仅是 malformed sentinel 拒绝。证据分别为 carryover-audit-build/evidence/frozen-v3-dates-race.log 与 frozen-v3-valid-catalog-race.log。

这些附加 reviewer 测试没有改作者冻结 v3；正式整合时应一并纳入再冻结和跑全集。GO-RUNTIME-014 仍为处理中，须正式修复 commit 与最终独立验收后才可更新状态。没有将作者通过误写成正式仓库已接线或已发行。

## 正式副本最终整合

整合基线为 8c68c3b581762c5ea49f21b5f8300005f23c5d50。16份作者文件与两份独立 reviewer 测试全部先落正式副本并冻结；完整文件清单与哈希保存于外置 noonmark-carryover-service-integration-20261105/integrated-source.json。原核心独立测试没有被作者旧副本覆盖。

2026-11-05 UTC：

- scripts/check 退出0：root race 202.020秒、taskapp race54.285秒、titlesession1.425秒；其余已命中相同内容的Go测试缓存如实保留标记。四模块race/vet/格式、Unicode16全量、28项合成Swift比较工具回归、51项依赖门禁与native UI/popup边界均完成。
- scripts/cross-build 退出0：linux、windows、darwin各amd64/arm64六目标编译通过；不代表六平台实机验收。
- scripts/test-entrypoints 退出0：实际Linux隔离development身份创建、重开及拒绝重用／错误身份通过；Day/Pool各渲染2400×1536像素；没有桌面原生窗口或IME验收。
- 测试结束后18份整合文件再次按事前manifest核对一致。此后仅更新验证与设计文档以记录结果，未改Go代码、测试或门禁脚本。

独立审查结论对应报告SHA256 5746bd0f8c5237ba007e62a45cee65a8887ffd27793dc3ab4ab858eeb150c4b3；作者完整check与独立完整check各自通过，正式整合包含后加的两份独立测试再跑全集，不依赖审查者运行中发现测试包的时序。

固定有界命令见证不保证任意合法core batch完备性，尤其跨链非单调clock可能改变数组插入顺序而fail-closed。真实Service单命令的单调frontier路径有回归证据。所有结果仅限空catalog普通任务开发范围。GitHub写入仍待用户确认，未执行重试或其他通道绕行，没有本批对应的新hosted CI。

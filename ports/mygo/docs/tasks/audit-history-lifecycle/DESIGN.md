# 冻结实施合同

冻结设计 SHA-256 `487f317195ed105f308dd9da6f71def2a094b9336e2901af81669f1293b7005e`；测试 oracle `16bec4700f9685b7be9a1ff6e09218177596044445237e5925e1949207eeff77`；独立 Accepted（仅准隔离实施）复核 `e38c0b89488d2fbf7561e441ffc4ed0055406f71865f8ac20f877ea8c5664f6f`。完整冻结文件另存本任务外置 evidence，不以本摘要替代。

1. 两端 graph metadata 预算及所有输入 ID count/length gate，必须在 bind、validation、clone、index、UUID 内容扫描之前；复用现有预算，不引入第二套成本模型。
2. Abandon 先核对请求 trace 的 owner，然后独立从 owned prior 找唯一 pending（否则请求 trace）。内部恰一 A@Now，绑定实际 target，proof 同时保留请求与有效身份。
3. Reactivate 不重定向；open current/future 产生 P，零 EventIDs，清 settledAt；past 或 locked 产生 U，恰一 U@Now，保留旧 A settledAt 的 exact bits。锁优先于日期比较。
4. 复用原 bind/match/finish：exact before+round-trip、原 supplement、core typed replay、whole-graph match、append→完整 inspector→prefix→固定补充段/count fences。days/definitions/subtasks及无关 chain/trace冻结。
5. 旧三个 command bodies、管道、r1 inspector/builder/machine、core、profile、runtime、v2 不改。只允许旧结果的两个 kind、两个 scalar 字段及显式 valid switch，另扩充 AST/预算测试。
6. 同链另一 A 不是拒绝理由，不增 latest-A 规则。两个独立 multi-A 参数例注明结构合成、profile-consistent，而非 Service 可达历史。

成功结果仅包含不可变 scalar 与 owned sealed endpoints；不保留 caller EventIDs slice。错误均返回零结果、输入不变。不修正时钟、不读墙钟、不分配任务身份。不承诺 caller 在调用期间并发写安全。

风险：错误 pending 路由、漏 U 或重复 capture、after 驱动 capture、旧 supplement／prefix 改写、预算后置与 alias 污染。测试必须覆盖这些边界。灰度只限合成隔离测试；回滚为撤下本批未接线新增文件及最小结果扩充，旧资料及 core/profile 不变。

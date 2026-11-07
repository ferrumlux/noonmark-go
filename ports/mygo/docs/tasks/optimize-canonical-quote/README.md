# 精简 canonical JSON 无转义字符串写入

状态（2026-11-07 UTC）：已合入 main，提交 `f7566df50eeb66c1bfbfc1c7e7d1d912c661a923`。此精确提交的常规 hosted 门禁与六 host CLI/headless 门禁通过；原生窗口生命周期门禁因 Intel macOS Create 超时失败，其余五 host 的十阶段通过。Task-Id：optimize-canonical-quote。

## 合入后 hosted 复核

- 常规：[42338467418](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467418) completed/success。root race339.348s，未改默认600s，余量260.652s；taskapp93.139s。此单次host测量不是受控性能倍数或未来runner保证。
- 六 host CLI/headless：[42338467571](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467571) completed/success。list/create/reopen exit0、reuse/implicit exit1，12张2400×1536 PNG仅证明合成开发会话headless渲染。
- 窗口：[42338467402](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467402) completed/failure。[Intel job128301753997](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467402/job/128301753997) Create在READY一次后software deadline exceeded，driver12.75036s，exit=-9且reaped/group_empty；stage1报告为空，Open未运行。[失败原件13560133136](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467402/artifacts/13560133136)保留，不宣称十二阶段全过。
- 三run均为精确f7566df、attempt1；保留lifecycle-only、系统像素/IME/完整产品未验。当前证据不足以把Intel在READY后的超时归因于quote改动。

完整公开复核见 [f7566df hosted CI 事实摘要](HOSTED-f7566df.md)，逐项身份与到期时间见 [evidence-f7566df.json](evidence-f7566df.json)。

driver 的 12.75036s 是包括启动及清理的总历时，不是 READY 后等候时长；原合同是 READY 后 10 秒完成。上述 artifacts 保留 7 天，记录的到期日为 2026-11-14 UTC；仓库只保存摘要与身份，不复制原 ZIP。

以下各节保留合入前的候选实验、测量、当时待办与失败记录原文。其中“独立候选”“正式源码保持不变”“下一步需复核”等属于历史时点，不代表本节所述当前状态。

## 来源与定位

基线 `824e96614a3c0196f9086505d201aae23b7c6de2`，tree `5509119fc599aec2cb2df4a80640a588f13c6cde`。正式源码保持不变；独立候选 `noonmark-root-margin-candidate`，缓存、二进制和原始运行证据全部在外置 `root-margin-build`。资料仅为隔离 development/e2e 合成资料。

已核 GO-PERF-DAY-001 及后续 DayTodo 窄读取，不重复认领它们的改进。当前未改源码的真实年度 root race 单项通过，测试体146.79s、包148.040s；带 CPU/内存 profile。累计分配采样10467MB，canonical.Marshal 累计2992.35MB、符号化 CPU12.90s，quote 累计6.59s。这里的累计分配不是峰值 RSS；符号化 CPU 不是测试 wall，未归因的外部/race 开销不被计入特定函数。

先考虑过 ProjectDay 的另一份冗余复制，但本轮 profile 显示整个 ProjectDay 符号化 CPU5.38s；未实施该分支。实际目标是当前 quote 对每个字符调用一次有界 buffer 写入，UUID、字段名和大多数普通文字均无需转义。

## 精确改动与安全边界

唯一产品差异：`domain-codec/canonical/canonical.go` 的 `writer.quote` 内新增15行。所有既有行、其他函数、公开 API、数字规则、Unicode 规则、schema/领域校验、Snapshot、缓存策略及测试资源配置不变。

1. 先执行原 UTF-8 校验，仍在任何输出之前拒绝非法编码。
2. 原 opening quote 成功后，只在整段 payload 能放进剩余 byte budget 时扫描。
3. 若字节中没有 ASCII 控制字元、双引号或反斜杠，经既有 `put` 一次写入整段，再经既有 `put` 写 closing quote。合法 UTF-8 中高于 ASCII 的字节不需 JSON 转义。
4. 若存在转义或 payload 放不下，完整使用原 rune loop。没有提前写入部分新分支内容，错误类型/文字、失败点和 partial buffer 都与旧实现一致。payload 恰好占满时，closing quote 仍按原处失败。检查使用原 `MaxBytes - Len` 形式，不使用会溢位的长度相加。
5. 不借用外部 mutable buffer、不引入全局状态或缓存；输出仍由每个 writer 自己拥有。输入 map/slice 不得并发被调用方修改，原合同不变。

完整原函数冻结在 `quote_before_test.go`。新 source-scope 门禁移除精确新增片段后必须还原旧整文件 SHA256 `6076f80103b6ea549df109a45722138918a42dd11c6c69757adbfbcb5cc100e8`，并逐字对账旧 oracle 函数体。新产品 SHA256 为 `165f1be6668310a35899652ddb364649db5c257f06cb4e60c7d3fd19fcfb3d4a`。

旧 frozen manifest 和全部旧 overlay 原字节不变。新单路径 overlay 接在既有 Service → Day detail → Day snapshot → Day read → Native window 链之后，仅接受上述精确 old/new/path。缺项、重复项、额外项、路径别名、错误 old/new、已前进基线均拒绝；成功或失败不得变异输入。旧 consumer-chain 断言只追加此新阶段。

## 定向验证与后续

新测试覆盖原函数字节/错误/partial buffer 差分、全部单字节类别、UTF-8 边界与非法编码、控制字元/组合字元/emoji、随机固定种子字符串、空与既有输出前缀、精确 byte 边界、双输出隔离和并发只读。完整 codec race 保留原 adversarial JSON、exact numbers、union、depth/node/byte guard、cycle、golden、schema 和 fuzz seed 回归。

性能对照使用同一进程、相同输入及冻结旧函数，记录 key、UUID、Unicode、长文字，以及首字元转义、末尾才转义、payload 超限和 closing quote 超限。快路并不保证所有输入更快；末尾转义会承担额外扫描，必须列出其样本。大段普通文字的 fresh-buffer 分配门禁不设 wall 阈值。

当前禁止用单项或微基准替代默认全量、四模块 race/vet、Unicode16、依赖/native、六目标和 CLI 门禁。下一步需独立复核、相同真实年度路径重测，再决定完整默认聚合验证。默认600s、race、GC/内存/CPU参数、fixture、旧业务断言及截图/命中检查一律不削弱。没有新 Swift、桌面/IME、完整产品或任意 runner 裕度保证。

回滚仅撤回此产品片段、新测试及专属 overlay 接入，不改用户资料，不重写旧证据，不影响原历史 overlay。

## 本地测量收口（2026-11-07 UTC）

同一外置环境的原年度 race 测试前后均无新增 timeout：测试体146.79→126.43s，包148.040→127.674s；保留真实 Tester、SerialLane、Service、365日/762→764任务、revision2→4及 Close/reopen 全部旧断言。quote 的符号化 CPU6.59→2.18s，canonical.Marshal12.90→8.61s；总采样分配10467→10409MB，仅小幅变化，主要成果为 CPU/调用减少。

这些是各一次带 profile 的运行，不是统计保证。fixture 编译区间8.62→7.64s、fixture生成10.42→9.95s，cache 热度不同；不把完整 wall 差称为净收益。未拿它推算 hosted root 的600s保证。候选年度运行前后逐文件 SHA 一致；其后只修正文档及一个更精确的路径别名负例，产品字节未变，该 gate 另行复测。

三次微基准样本的中位数：字段名240→100ns、UUID471→115ns、Unicode标题353→146ns、5KB普通文字48.14→7.63µs（16400→5520B，9→3alloc）。6.4KB首字元转义59.61→62.05µs、末字元转义58.69→64.75µs，展示预扫描的代价；payload超限33.89→33.31µs、closing quote超限60.15→10.01µs，原错误与partial输出相同。不是所有字串都更快。

完整 domain-codec race、相关 vet、新旧所有 frozen overlay focused race、gofmt/diff检查通过。独立复核与完整默认入口状态由后续精确候选回执决定，不在此预先宣称。原失败启动（环境无/usr/bin/time，实际测试未开始）、冻结门禁预期红、原始命令、profile、源清单、基准样本和退出码完整保留在外置 root-margin-build；未改旧证据。

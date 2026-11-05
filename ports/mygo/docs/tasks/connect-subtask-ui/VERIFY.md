# 验证记录

日期：2026-11-05。Swift 静态基线：43e6eefaae3a04be89cab3e234d470111afd80ef。所有 fixture 通过隔离 development/e2e 的真实领域 Service 合成；没有生产、iCloud、Keychain、真实 AI 网络或远端 push。

## 环境与命令

- Go：/workspace/scratch/586e77bd59a6/noonmark-mygo-work/go/bin/go（1.27.3）。
- 外置 NOONMARK_MYGO_BUILD_ROOT：/workspace/shared/noonmark-subtask-ui-build-20261105。
- GOPATH：/tmp/noonmark-gopath；GOCACHE：/workspace/shared/noonmark-go-build-cache-20261105。
- 每次 Go 调用先 source scripts/environment。该脚本验证只读模块选择、冻结archive/patch/hash，设置独立 GOWORK、GOTOOLCHAIN=local、外置tmp/cache；没有裸 Go 或仓内缓存。
- MyGo：v0.7.5-0.20261105040656-977ea57e0baf，准备后的依赖根标识 024e8ff5c4014cfb30c1cfee5ac9849b7b8b7ad35353bf4d2da3391f8806081a。原 MyGo 仓库和官方module cache未修改。

最终专项范围：24个本任务TestSubtaskUI顶层门禁，加14个独立审查顶层门禁，共38个；另执行既有TestFailureGateRegistry。子测试不额外计数。

复跑命令：`"$GO" test -p=1 -race -buildvcs=false . -run '^TestSubtaskUI|^TestReview(Subtask|FailedComposer|RetainedDeleted|Recovery|ForkedPlanned|ClearingOneFailedComposer|UnsubmittedComposer)|^TestFailureGateRegistry$' -count=1`。

最终源码冻结：2026-11-05 20:32:26 UTC，9个Go文件 SHA-256 在外置 evidence/frozen-go-manifest-r5.json。此后仅文档变动。r4专项已绿但全check发现上述Label回归，修正后以r5最终结果为准。最终scripts/check、六目标compile和LinuxCLI终态记录在本文件末尾；未完成项目不能据此声明通过。

## 专项覆盖

- 新增真实 Day 子项，标题699ms不写/700ms写，trim ack，Return，难度菜单，完成/撤销完成，父项不自动完成，删除保留取消墓碑，新项不复用位置，关闭与重开完整graph一致。
- Day future 可新增/标题/难度/删除但完成禁用；跨日 settlement 后不展示可写入口。
- 真实 native composition 不写半成品；导航和关闭 finalization 保存已送达 marked text；空白恢复 acknowledged title。
- Pool planned 新增、标题、难度、物化到 actual 新ID/保留lineage；排期前flush和旧callback拒绝。没有Pool完成按钮。
- 同child Day list/detail 两owner并存，各自同时dirty；FIFO导航flush；某surface存储失败时另一个成功，失败草稿仍保留且可重试。
- 两窗口同对象pending命令仅一项入writer；另一窗口删除时隐藏native输入仍被保留、原表面仍可见、关闭仍被拒绝。
- 父标题保存不确认child；已有日历史后合法父标题分叉，planned旧definition owner锁定且不写新definition。
- before-reserve、after-reserve、before-commit、after-persist 四阶段失败不发布ack、不清草稿、不关窗。重开区分旧事实、未确认但已持久新事实和不可打开的不完整提交。
- composer重复Return、pending关闭、成功后更新输入、失败保留、离屏提交ack后回到原对象清理。
- 两表面checkbox28×28、difficulty12×28、标题至删除4pt、文本32–120pt高度、960×720最小窗口和长标题无控件重叠。
- 25项长滚动详情中离屏dirty editor仍可关闭flush；三个父项同计数不共享原生测量key。
- native difficulty菜单跨导航、取消、Service Close/Open重挂窗口不复用旧callback；旧Add确认不吞掉新输入的一次Return。
- 失败Add离屏Close/Quit、重试成功和fresh native明确清空；多owner中只清空一个，不释放另一个失败草稿；普通未提交composer不自动Add也不扩大为关闭拦截。
- failed composer在parent完成、回池、真正跨到次日历史、planned definition fork后，原owner草稿可达，marked text仍在原native handle；原失效target的Return被Service拒绝，不写替代对象，明确清空不改变持久graph。
- old planned dirty editor即使child ID被新definition沿用，也明确标为原对象未保存草稿；被删child ghost保留但不计入确认子项数。
- 短与长父标题的披露胶囊按真实intrinsic内容宽度+12pt排布，标题与胶囊不重叠、不越行右界；chevron由9pt SVG渲染。

## 渲染证据

外置 evidence/subtask-ui/：

- day-list-detail.png：同一实际child的list/detail独立表面，2400×1536。
- day-long-2x.png：较长中英混合标题在list/detail不同宽度换行，2400×1536。
- recovery-original-2x.png：失败新增后父任务已回池，原日期恢复输入区和未确认文案，保留帧间marked文字，2400×1536。此图只证明端口失败恢复界面，不冒称Swift正常界面。
- pool-planned-2x.png：回池后的planned多行标题、不可完成图形、xmark删除、圆角7 composer，2400×1536。

这些是 MyGo Tester 执行真实UI函数和Service路径所得 native renderer PNG。已实际查看图片；没有使用CSS/HTML mockup或将参考PNG嵌入画面。没有同机Swift截图，因此不声称逐像素差分通过；系统字体、SF Symbol、真实窗口外壳与OS输入法仍需平台验收。

## 开发期间捕获的失败

已记录GO-UI-SUBTASK-001至009，均有红证据、修正、fast/symptom门禁及回滚边界，候选待正式整合commit后才关闭：重复测量key、旧菜单会话穿透、late ack吞Return、失败Add普通关闭丢稿、失效原对象不可达、胶囊固定靠右、ghost误计数量、旧planned草稿缺标识、几何测试辅助Label覆盖父标题文本。

测试自身的两处问题也保留说明：自写history分支起初硬编码早于fixture的日期，已改为liveTestDay.AddDays(1)、推进fixture clock并断言真实历史；独立ghost门禁起初substring误匹配以1开头UUID的accessibility label，独立审查者改exact text后在r2旧实现重新证红、r3重复10次绿，保留原版测试和诊断日志。没有删除草稿保留或事实数量断言。

红日志位于外置evidence/和 /workspace/shared/noonmark-subtask-audit/ui-independent-review/；完整独立结论由父任务维护，正式整合不能只使用本候选自检结论。

## r5 最终终态

- 专项38项＋registry：race通过，27.674s，evidence/r5-all-38-focused.log。
- 五个既有标题文本路径：全部原断言在最终完整scripts/check与独立r5复审通过，未修改既有测试。
- 六目标交叉编译通过：linux/amd64、linux/arm64、windows/amd64、windows/arm64、darwin/amd64、darwin/arm64，evidence/cross-build-r5.log。只证明compile，不证明各OS运行。
- Linux CLI隔离development创建、重开、重复创建拒绝、无身份拒绝通过，evidence/cli-r5.log；仅headless入口，不代表Linux桌面窗口已验证。
- 完整scripts/check：exit0通过，evidence/final-check-r5.log；包含根模块、domain-core、domain-extensions、domain-codec的gofmt/race/vet，51项依赖准备测试及MyGo native边界测试。
- 独立r5复审：38子任务/review＋registry＋5个既有受害路径，共44个顶层门禁race绿29.555s，/workspace/shared/noonmark-subtask-ui-review-r5-build/evidence/r5-final-review.log；9个Go文件在候选与独立副本逐一符合r5冻结hash，没有漂移。
- 截至20:35 UTC，本批已知阻断项均修正并验证。正式整合/commit和OS或Swift验收仍由父任务另行确认，不将候选通过当作已发布。

整合清单在外置evidence/owned-files-to-copy.txt，精确21个文件；gates只追加evidence/gates-to-append.tsv九行，不能用本候选整份gates覆盖父任务后续记录。evidence/unchanged-core-service-proof.json记录30个相关实现文件与core/Service候选字节一致。没有改依赖、三个领域module或正式树。

## 正式树整合

2026-11-05 20:40 UTC，已在正式64af1667（分类显示名no-op修复之后）按冻结清单逐文件核验SHA整合，保留既有registry并只追加9条子任务门禁。重新运行完整scripts/check，exit0：应用main race78.346s，native ui2.499s/root1.045s；四模块race/vet及51项依赖门禁通过。六目标编译及Linux隔离CLI创建／重开／拒绝覆盖也重新通过。整合日志外置 evidence/integrated-{check,cross,entry}.log。此处记录的是本地整合结果，远端新head的CI须另行核验；没有运行或声明真实OS/IME或Swift oracle。

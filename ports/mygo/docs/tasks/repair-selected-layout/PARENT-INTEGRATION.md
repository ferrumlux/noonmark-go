# Selected 布局门禁整合复核

基线293b22cc；候选feab6d94。独审R2确认Selected只影响绘制/可访问性及focus group，不是几何输入，已执行五个脚本/零匹配绕过反例。第一版静态wrapper不够强的原红保留；现在真实子进程检查七个精确症状run/pass。

本整合逐文件读取最终候选及日志，确认新patch仅改布局测试登记、新增Selected缓存/access对账测试；原0001–0006及应用/依赖生产源码未改。原2b身份完整check已过，R3仅unified=0重序列化，2197依赖文件逐字不变，新manifest准备器/native/根Selected复验通过。没有忽略diff whitespace；原红及新绿都保留。

整合保留Day详情、Service结果和状态范围修正，gates按ID仅追加Selected一行，不重写其它案例状态。新组合完整scripts/check已于2026-11-06 12:53:21 UTC退出0；root/UI race591.960秒、taskapp267.106秒、默认native72.748秒、host popup1.047秒。其余模块、vet、Unicode16及依赖51项通过；完整日志哈希见INTEGRATION-VALIDATION.json。代码冻结期间未改源码，之后只收束文档；新head的hosted CI另核，不把旧身份全集当新组合全集。完整MyGo UI仍有既有TestDiffFrameAllocs失败，不提高阈值、不声称全框架或实机通过。

文档收束后的Selected/Day状态范围/案例登记专项race3.299秒、root vet及六目标编译均退出0，完整回执已记录日志字节数与SHA256；没有将COMPILED改称实机运行。

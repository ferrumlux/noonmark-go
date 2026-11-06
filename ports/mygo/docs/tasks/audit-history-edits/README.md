# 未接线标题与手动进度历史审计

Task-Id：audit-history-edits。风险 P。日期：2026-11-06。

状态：作者与独立实现审查，以及正式整合全项目门禁均已通过。本批不接 Service/store/UI/v2，不新增 writer、allocator、迁移或已启用的产品功能。来源基线为 `2a808a3c4fe788c69d8f47cf21e1d686860a19ff`。

## 实施与保全

两个封闭入口重放原核心的标题 Rename/Input 与手动进度命令。标题保留 chain/trace owner、版本分离、输入复用与 canonical no-op 字节；手动进度保留 clamp、历史 floor、全链实际子项守卫及100不自动完成。unused Now/新定义ID不被提前消费，±0相等保留旧chain时钟位。

seal、两个caller图与全部输入字符串元数据预算均先于绑定、验证、复制和字符串扫描。零capture仍经过完整replay、inspector、prefix/count及独立固定字段fence。新结果只有私有标量请求身份与owned endpoints，不代表当前磁盘head或提交权限。

除共享结果的三个kind、元数据、注释和valid注册外，旧production函数冻结。三个旧测试函数只增加入口覆盖或调整未知kind哨兵；没有删除旧拒绝断言。旧command的auditSettlement到EOF及两个accessor身体逐字节不变。完整设计、矩阵和阶段结果见同目录归档。

## 已有独立证据

作者最终冻结源完整taskapp 29.533秒、未过滤race 182.575秒及vet均退出0。独立方没有读取或复制作者新/改测试，仅正式基线加3份production，自写12个顶层测试：首版10项的完整taskapp 30.268秒、race179.145秒、vet通过；追加kind0..255闭集及多事件大小写/字典pair排列保全后，12项focused0.357秒通过，追加版本未在独立树重跑race。

正式整合已将作者与最终独立测试一起跑完整门禁，没有以此前单方测试替代。失败fixture及registry红测均保留，未放宽production迎合测试。同日较小continuationSeq混合group未新增独立fixture；详细覆盖边界见 INDEPENDENT-REVIEW。

## 正式整合验证

2026-11-06 06:13:33–06:29:12 UTC，532个源码文件及模式全程冻结不变。`scripts/check`、`scripts/cross-build`、`scripts/build`、`scripts/test-entrypoints` 均退出0；其中UI包race 466.689秒、taskapp race 182.892秒，另含全部模块vet、Unicode固定来源及MyGo原生边界检查。未改包明确保留日志中的缓存状态；未使用超时hack、skip或降级。Linux/Windows/macOS各amd64与arm64六目标均编译，未将交叉编译当成目标系统实机验收。

整合focused先行7.409秒通过。最终正式race包括独立方最后追加的kind闭集和多事件历史保全两项测试，补齐其独立树未重跑race的边界。门禁完成后只更新本README，Go源码和测试仍与原冻结清单完全一致。

## 未完成

全消费者能力对等、live-session/head与实际磁盘prefix绑定、未来开发格式切换、真实UI与重开接线仍待后续。没有新Swift oracle、多OS实机、正式签名、Hosted CI或发版结论。原Git工具写审批结果未明，保持暂停，不通过替代写路线绕行。

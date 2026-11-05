# 接通有历史保护的任务标题编辑

task-id: connect-title-edit。级别 P（定义版本与历史审计）。2026-11-05。

## 需求、现状与设计

完整移植原版标题语义，但不能破坏已结算历史、在保存失败时发布候选或丢弃输入。实施前已研究上游 renameTaskTitle/saveTaskTitleInput、官方测试及详情 autosave，详见 RESEARCH.md。原有 CaptureEmptyHistory 冻结整个历史定义，不能直接放宽它以容许任意历史修改。

第一阶段为两个独立领域入口：无日事实可复用定义；显式 rename 有日事实时分叉；saveInput 在已隔离当前定义上复用。历史标题、详情、计划子任务和 trace 不改，只推进合法后继元数据。空标题核心允许，而详情 UI 不保存空白，两者不能混同。

第二阶段为专用命令重放审计及同锁聚合事务，详见 TRANSACTION.md。保留严格旧审计；完整候选验证后仅做一次 commit，成功才发布内存。未知 scope、缺失 audit、损坏、冲突及不确定写入均 fail-closed。无操作也检查 stale/blocked 状态。

## 当前实现与风险

领域层与 Service 两个入口已实现，未暴露 UI 编辑。独立审查发现 Swift String canonical equivalence 与 Go 字节比较的差异，已按 GO-RUNTIME-006 修复并加入两个层面的回归；NFC 只用于比较，不改变持久化字节。依赖 x/text v0.43.0 使三个领域模块最低 Go 提至 1.26.0，主程序仍为 1.27.3。

不会读取生产资料，不执行迁移、iCloud 或真实 AI。当前只接通普通任务及严格空分类历史；合成的复杂历史 fixture 不代表 UI 已能建立对应生命周期。Swift oracle、原生平台与 IME 均未验证。

## 验证、回滚与下一阶段

全合成验证覆盖两个入口、规范等价、历史分叉/复用、完整 metadata 与内容冻结、攻击矩阵、四阶段写入故障、重开、并发及自然日边界；随后执行 scripts/check、六目标交叉编译与 CLI 安全入口回归，结果单列 VERIFY.md。

没有 formatVersion 改动与旧库迁移；本次新 API 尚未暴露 UI，回滚源码不会启动资料清理。隔离测试会话不作 production cutover。after-persist 故障保留旧内存并冻结句柄，只能明确重新打开核实，不宣称完全回滚。

下一阶段仍需实现 trace 级路由、700ms autosave、IME composition、空白失焦恢复、切换/退出 flush、错误保留及实际交互测试。编辑 UI 未满足这些条件前不开放，不能以核心测试代替用户路径验收。

# 原生窗口第1层验证与冻结来源

本批只加入显式 development 窗口生命周期探针及独立 CI，不表示真实窗口、系统像素、IME 或正式产品已经验收。driver、平台限制与固定报告合同见 [native-window-smoke](../../../scripts/native-window-smoke/README.md)。

历史审计的原冻结 manifest 与此前全部 overlay 原样保留。新增 v1 overlay 只允许 main.go 从 a3e2a407 的 SHA256 a678fc92… 切换到已审窗口钩子 SHA256 23bd5f30…，固定完整旧/新 hash 和精确路径，不允许任意新 hash、路径别名、重复、缺行或二次应用。真实 consumer 必须顺序执行原四层后再执行此层，并继续计算每个原冻结文件的实际字节 hash；历史 command suffix 检查不变。

这不是历史审计或生产 Service/UI 接线授权。main.go 的新增路径仅在 -native-window-smoke 显式受限参数下启用；正常入口继续保留原 Service、writer/dispatcher drain 与首次 Close 顺序。窗口代码独审只证明该限定代码范围，六 host 实证与常规 CI 另行记录。

新增冻结绑定修正的初始完整门禁失败及永久回归见 [GO-NATIVE-WINDOW-FREEZE-001](../../engineering/failure-cases/GO-NATIVE-WINDOW-FREEZE-001.md)。

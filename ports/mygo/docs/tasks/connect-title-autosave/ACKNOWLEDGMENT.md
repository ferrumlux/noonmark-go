# 返回同事务确认的标题字节

设计背景：UI-DESIGN.md 已要求异步成功只能确认真实已提交内容，不能把 normalized 输入假作储存字节。NFC 等价 no-op 刻意保留原值，保存后另读 Snapshot 也可能已经看见另一请求的新值。

新增 `SaveTraceTitleInputAcknowledged(traceID,title) (string,error)`，在与路由/候选审计/commit 相同的 Service 锁内返回本次已验证的实际 current definition.Title。原 error-only SaveTraceTitleInput 保持兼容包装，两个链级 error-only 入口也保持接口不变。

确认值从已规范化的完整候选中、在任何磁盘写入之前解析。普通成功必须等 commit/fsync/readback 完整通过才返回；no-op 必须等现有 prefix 校验通过才返回。所有错误返回空确认值，含 after-persist 不确定结果；不能把空确认值单独当失败标记，因为核心允许合法空标题，应始终检查 error。

测试涵盖 canonical 两种字节方向、CRLF/trim 后实际值、重开一致、四阶段故障没有确认、stale canonical no-op 没有确认及同锁并发请求各自获得自己的确认值。接口证明本次保存成功时的字节，不保证返回后没有其他合法窗口继续修改。

这不解决 native draft 最终化、IME 或 UI 退出时序，不开放可见编辑器，也不改变会话格式或锁定历史保护。

最终验证：本轮 scripts/check 全部通过，主包 race 39.057s、taskapp race 13.122s，其余包 race/vet/gofmt 通过。六目标交叉编译和 CLI development 新建/重开/拒绝复用全部通过。证据位于外置 build root 的 evidence/title-ack-{check,cross,entrypoints}-final.log；仍不代表真实桌面或 Swift oracle。

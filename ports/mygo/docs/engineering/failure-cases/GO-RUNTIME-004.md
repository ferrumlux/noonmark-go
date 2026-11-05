# GO-RUNTIME-004：月历漏报边界失败、裁掉隐藏数量及键盘焦点不可见

状态：开发阶段复现并修复，独立复查通过，真实桌面门禁待补。

- 日期：2026-11-05。引入于 connect-calendar-runtime 未提交候选，不是已发布 b7ab86f 的月历行为；修复与案例同一提交。
- 独立症状：refreshDay 报错但 Calendar 早退路径不显示 runtimeError；960×720 六行月、8项任务时第6项部分裁切且 +2 标签高度0；Tab将焦点移至次日，但整帧像素不变，Enter却操作不可见焦点。
- 根因：页专属视图漏接公共错误区域；把固定6条及源点尺寸当作所有可用格高/字体都能容纳的证明；关闭 MyGo focus ring 后仅绘制 selected 状态。
- 修复：错误作为日历副标题警告并保留空间，成功恢复仅清除此边界警告；按实际格高最多6条并预留可见 +N（N=全部行减实际显示行），完整行仍在详情；以匹配字号的Themed测量域取得真实Text高度，节距至少14并为真实计数高度预留空间，不以FixedLineHeight或默认主题MeasureText冒充实际字体尺寸；保留原生键盘focus ring，箭头从实际focus日期移动。
- 验证：TestLiveCalendarBoundaryFailureIsVisibleAndRecovers、TestCalendarPreviewFit、TestLiveCalendarDenseSixRowsShowsActualRemainder、TestLiveCalendarTabFocusIsVisibleAndArrowStartsAtFocus。密集测试检查Find所得标签高度正且完整位于cell内，不使用HasText冒充可见证明。
- 范围与差异：密集小高窗减少预览条数是明确防裁切适配，原始Swift始终prefix(6)；普通5行足高窗口仍6条及+N。MyGo键盘focus样式与Swift专用焦点桥不同，未宣称native/SF像素一致。
- 数据边界：不新增存储或写命令，不重建历史；错误与只读导航保留草稿、任务图及磁盘内容。资料均为新合成development会话。

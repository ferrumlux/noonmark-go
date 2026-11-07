# 运行期准入与 utility 草稿保全

公开源码基线为 [c441d3b331dd3b3358d171f3c2d7e420bb9c2099](https://github.com/ferrumlux/noonmark-go/tree/c441d3b331dd3b3358d171f3c2d7e420bb9c2099)，tree c3658b612621a45705faf12c3f611e070186ddee。该基线与设计依据的八个源码文件逐字节相同，具体路径、SHA256、公开链接和33个原投递入口见 [SOURCES.json](SOURCES.json)。本切片只修改八个既有生产文件并新增 runtime_admission.go；不改变 Service、domain、codec、MyGo 或界面外观。

实现 producer/completion 分准入、lane 同锁 TryEnqueue、Run 返回终止新工作、writer 后 dispatcher drain、active UI 安全收尾，以及 utility Close veto 保留原草稿。lane 仍无界，不新增 FIFO/容量策略、Health、RecoveryClose、导出或异常进程退出恢复能力。

## 合同与验证范围

- [DESIGN.md](DESIGN.md)：准入状态、锁序、结果投递和关闭顺序。
- [PRODUCERS.md](PRODUCERS.md)：各入口、ticket 所有权及拒绝处理。
- [OBSERVE-DAY-CONTRACT.md](OBSERVE-DAY-CONTRACT.md)：准备保存先于自然日 Advance 的顺序，及两跳拒绝的准确边界。
- [TESTS.md](TESTS.md)：确定性验证矩阵及不能由有限测试代替的门禁。

异常 Run 的 worker 完成与 UI 未消费分开报告，不后台操作 native 或清草稿。有限 focused/race 验证不替代完整 scripts/check、六目标编译、真实原生窗口生命周期/utility 验证或同源码 CI；本文件不宣称这些门禁已经完成。

## 来源保护与摘要验证

[PRODUCT-CHANGES-v4.tsv](PRODUCT-CHANGES-v4.tsv) 明确列出8改1增；[SOURCE-GUARDS-v4.md](SOURCE-GUARDS-v4.md) 说明完整来源链。v4 全集与精确 overlay 必须先独立通过，再反向重建原 v3/v2/v1 输入。原122产品、73历史及38脚本合同不被直接刷新或绕过。main 新完整字节与生命周期 AST 先验证，再重建原 main 运行既有所有权保护。

在本目录执行 sha256sum -c DOCUMENT-SHA256SUMS 可验证这里实际存在的设计、合同与来源说明文件；sha256sum -c GUARD-EVIDENCE-SHA256SUMS 验证源码守卫的实际证据文件。SOURCES.json 中的八个 SHA256 对应公开基线文件字节，不是本切片修改后的产品摘要；修改后的完整产品摘要见 PRODUCT-SOURCES-v4.tsv。

# 普通开发入口 v1 精确合同

本合同固定 2b0704b2fac3c860cc24a4a16c5037856783a69e 的入口产品代码，不代表正式整合、生产或平台验收。

历史八层 manifest、overlay 和原 command suffix 均保留。第九层 ordinaryEntryFrozenOverlay 只授权 main.go 的精确前后 SHA256；完整73路径前序集合绑定 integrate-category-ui 的 AFTER-v1。三个既有 consumer chain 断言明确追加第九项，未放宽名称筛选或删除历史阶段。原 production hash 循环保持不变。

PRODUCT-SOURCES-BEFORE-v1.tsv 固定入口前 96665d5 的119件非测试 Go 源文件及 go.mod/go.sum，PRODUCT-SOURCES-v1.tsv 固定入口后120件完整同类产品路径集合。ENTRY-SOURCE-CHANGES-v1.tsv 只声明 main.go、native_window_preflight.go 两件精确转换和 runtime_startup.go 的 absent→新增。测试核对固定清单字节摘要、准确转换及磁盘完整路径/哈希集合；文件缺失、额外产品源、路径别名、旧新哈希变化、重复行、每字节 mutation 皆拒绝。ENTRY-SOURCE-v1.diff 保存这三件源码实际差异。

这里的完整产品集合范围是 ports/mygo 内全部非测试 .go 与 go.mod/go.sum，不声称覆盖平台依赖补丁、资源、工具脚本或所有仓库文件；这些仍由其原合同负责。未来 UI budget 修订会有意触发本 v1 产品源拒绝，须依据独审准确身份另增明确版本合同，不得改写 v1 或将 UI hash 混入第九层 main overlay。

旧 native probe guard、真实 Content UI、preflight 路径/链接/排他报告以及非 smoke wrapper 合同均未修改。scripts/test-entrypoints 保留20页面及原 legacy create/render/reopen/reuse/no-identity检查，只追加九个真实 CLI 非法参数全零写场景。

## 本轮验证

仅 taskapp 内八项确切名称 focused tests，非 race，count=1：历史 production、day-read/ordinary-service/category-UI consumer chain，以及本批 scope拒绝、consumer链、逐字节/逐路径mutation和完整产品集合。全部通过，最终0.485秒。第九层拒绝149个overlay字节mutation及219个全基线mutation；产品集合绑定120路径。

bash -n、git diff --check 通过。仅提取新增九个CLI拒绝场景，以已有外置实际Linux binary执行通过，未运行脚本的20页面/渲染部分。本次没有全包、完整check、race、cross-build或native host实机验证，不能借此宣称其通过。

外置证据：category-entry-build/entry-contract-focused.log、entry-contract-cli-location.txt。首次新增源码合同测试因生成脚本误替换清单文件名失败，修正后复跑通过；原失败保留 entry-contract-focused-initial-red.log。既有入口主文件冻结初始红例在 freeze-initial.log，未覆盖或重写。

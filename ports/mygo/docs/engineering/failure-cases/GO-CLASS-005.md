# GO-CLASS-005：离线对账测试不得依赖机器的相邻仓库布局

状态：已修复（本地工具回归；未推送、未取得真实Swift capture或发行）。修复commit：1dd93a2809eb3479eada197e1ff99e253cc66e57。

首次发现：2026-11-06（新加坡）。缺陷来自未提交的离线比较器候选，未进入正式远端；无引入commit，Git author／committer不适用，实际自然人操作者未知。旧test_compare.py SHA-256为7e121e51acc5557c14bf5bf9e43b81025169619d58cc9178ccf94f73bdb81439。

症状：test_bad_paths_no_write硬编码ROOT.parent/noonmark-go/outside-mygo并期待拒绝。在原工作机相邻正式库存在时通过；独立checkout没有该库，生产守卫正确允许外置路径，26测试变成25过、1失败。独立审查direct.stderr和新red.stderr保留原失败。排除生产路径守卫故障、Go工具链和Swift输入因素；根因是测试把机器布局当成fixture。

修复：只改变测试与门禁，不放宽生产路径守卫。外置合成布局patch ROOT和MODULE，Git／port两种标识各用任意名字，负例必须在mkdir／mkdtemp之前拒绝且fixture清单及bytes不变。正例验证没有标识的noonmark-go同名路径可作为外置输出且run独立。新增helper verified-byte绑定测试禁止prepare_run重新read_file。原五项独立回归逐字节保留。

fast／symptom均绑定TestSwiftNameComparisonRegressionGate，经scripts/check强制执行run-tests.py。每次创建全新独立最小源码checkout，其父目录只有checkout，无noonmark-go相邻库，运行全部test*.py；保留源码hash清单、真实exit和stdout／stderr，核对源未变及未新建相邻库。用旧测试回放自动入口判红、修复后全28项转绿，不能用人工这次搬目录代替永久门禁。

这是合成测试工具可靠性回归，不代表真实Swift执行或互通，没有修改production／iCloud／Keychain／AI资料。工具未发行；回滚可一并移除比较器与聚合入口，不改变oracle、runtime或用户资料。关闭案例需回填正式修复commit和聚合测试证据。

正式闭环：独立复审通过，正式scripts/check退出0（root race198.074秒），其中新独立checkout完整28项26.883秒、源hash不变且无noonmark-go相邻库；六目标编译及Linux隔离入口通过。原始红证据和五项独立回归保留，最新整合证据见docs/tasks/verify-swift-name-comparison/README.md。原“待commit”描述属于早期候选状态；本地修复不代表Swift语义对账已执行。

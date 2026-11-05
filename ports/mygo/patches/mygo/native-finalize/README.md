# MyGo downstream native title boundary

基线：github.com/egoist/mygo v0.7.5-0.20261105040656-977ea57e0baf（977ea57e0baf78da62698ba7371420d192260466）。保留原 MIT LICENSE；应用维持 AGPL-3.0-only。0001 修改七个源文件，0002 新增两个合成回归测试文件，manifest 固定官方归档、patch及每个文件前后hash。

本bundle由统一scripts/environment准备到外置副本。不能直接覆盖Go module cache，不能只改hash消除不明差异。旧根目录0001-preserve-preedit-before-focus-clear.patch是保留的历史最小候选，不再次叠加。

契约与边界见 docs/tasks/connect-title-autosave/DEPENDENCY.md。修复原生已经交付但尚未更新string binding的最后字符，并提供帧间最终化；不代表真实OS IME或应用退出接线已完成。

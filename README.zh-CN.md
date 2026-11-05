[English](README.md)

# Noonmark Go

这是 [Noonmark](https://github.com/quboliu/noonmark) 的原生 Go + MyGo 移植研发仓库，采用 AGPL-3.0，当前不是完整或可投入生产的替代品。

首批备份包含 ports/mygo 下的 20 场景原生 UI 原型、三批已独立测试的领域模块、迁移审计、合成 fixture 与验证说明。普通任务的创建、排期、完成/撤销已接通隔离开发文件存储；SQLite、同步、AI 和完整领域/UI 接线仍未完成。Swift v7 互通尚未验证，非空分类 audit 默认拒绝。

上游参考提交：43e6eefaae3a04be89cab3e234d470111afd80ef。没有上传工具链、缓存、二进制、凭证或生产资料。

这是独立仓库，不属于上游 fork 网络。日后向上游提出 PR 时，需要在获得授权的上游 fork 中从上述基线建立分支，仅移入审查后的 ports/mygo 改动，通过上游与移植测试后再建立 draft PR。目前未建立上游 PR。

当前能力、缺口与复现方法见 [移植说明](ports/mygo/README.md)。

[简体中文](README.zh-CN.md)

# Noonmark Go

An in-progress native Go + MyGo port of [Noonmark](https://github.com/quboliu/noonmark), licensed under AGPL-3.0. This repository is a source backup and development workspace, **not a complete or production-ready replacement**.

The first checkpoint contains the native 20-scene UI prototype, three independently tested domain modules, migration audit, synthetic fixtures, and verification notes under [ports/mygo](ports/mygo). A bounded development runtime now connects ordinary task creation, scheduling and completion to isolated, verified file persistence. SQLite, sync, AI and full domain/UI coverage remain unfinished. Swift v7 interoperability has not been verified, and nonempty classification audit fails closed by default.

Upstream reference: `43e6eefaae3a04be89cab3e234d470111afd80ef`. Toolchains, build caches, binaries, credentials, and production data are excluded.

This is an independent repository, not a GitHub fork. To propose the port upstream later, create a branch in an authorized upstream fork from the recorded upstream baseline, transplant only the reviewed `ports/mygo` changes, run upstream and port checks, and open a draft PR. No upstream PR has been opened.

See [the implementation status and checks](ports/mygo/README.md) for current limitations and reproduction commands.

# GitHub 开发版 CI

## 范围与设计

一个公开 `ubuntu-24.04` hosted job，响应 push、pull_request 和手动 workflow_dispatch。只授予 contents:read，checkout 不保留凭证，不使用 secrets、pull_request_target、自托管 runner 或 cache restore。无需调整仓库计费、安全设置。固定 Go 1.27.3，Python 唯一第三方依赖固定 PyYAML 6.0.3；原有 dependency prepare 使用 Python 标准库。

所有构建、module cache、编译缓存、Python venv、合成 session 和证据在 runner.temp 外置目录。`scripts/ci/run` 拒绝非空 build root，强制公共 Go proxy、checksum database 和全新缓存，不依赖本地预热资料。官方 MyGo zip 经既有 manifest 的 zip、go.sum、license、逐文件和补丁 hash 校验，在外置根生成 workspace replacement；不改官方 module cache，不从旁边开发 checkout 取依赖。

执行顺序：

1. 校验 workflow 安全契约、9 个拒绝样例和 shell 语法。
2. 原有 `scripts/check`：四个模块 gofmt、race tests、vet；50 项依赖准备测试；37 项 MyGo 原生输入定向测试及 vet。
3. 原有 `scripts/cross-build`：Linux、Windows、macOS 的 amd64/arm64，共六目标 CGO_ENABLED=0 编译。
4. 用实际 Linux amd64 产物运行 `scripts/test-entrypoints`：20 页面枚举、合成 development session 创建、渲染、reopen、拒绝覆盖和拒绝无身份启动。

Linux 安装 Noto Sans CJK、Fontconfig、Pango/Cairo、GTK3、Epoxy、EGL/Mesa；race 检查需 GCC/libc headers。headless rendering 使用软件 EGL surfaceless，不启动桌面窗口，不需要 Xvfb。字体缓存也外置。Ubuntu 包更新可变化，以上属于可重跑的固定源码/工具链 CI，不声称逐字节可复现的 OS 镜像。

## 证据、权限与发行边界

artifact 只包含 `ci-evidence/*.txt`：每阶段末尾最多 256 KiB 日志、工具版本/字体、六个编译产物 SHA256 和成功标记，保存 7 天。没有将二进制、整个 build root、缓存、session、截图、用户资料上传。结果标记仅在全部门禁通过后创建。完整 job 控制台仍可见；失败不会 continue-on-error。后续如需分发二进制，必须先完成许可清单、依赖许可与 source metadata 打包审查。

这是开发版 CI，不创建 tag、GitHub Release 或正式发行包。六目标编译不是六平台运行验证；Linux CLI 的无窗口渲染也不是桌面交互、真实 IME、签名、安装包或 Swift oracle 验收。完整 MyGo 框架 `TestDiffFrameAllocs` 有既存 allocation 失败（见 `tasks/connect-title-autosave/DEPENDENCY.md`），本 workflow 保留既有定向 suite，不调用 `--all-ui`，不调阈值，不声称框架全绿。正式发行须在完整验收后接通，不在此开发 CI 中自动发布。

## 本地复跑

先安装上述系统库与字体；从可信官方来源安装 Go 1.27.3。源码之外建立全新空目录，设置 `NOONMARK_MYGO_BUILD_ROOT`。如 Go 不在 PATH，设置 `NOONMARK_GO` 为其绝对路径。

```sh
python3 -m venv /outside/source/ci-python
/outside/source/ci-python/bin/pip install --no-cache-dir -r ports/mygo/scripts/ci/requirements.txt
/outside/source/ci-python/bin/python -B ports/mygo/scripts/ci/validate.py
mkdir /outside/source/ci-build-new
NOONMARK_MYGO_BUILD_ROOT=/outside/source/ci-build-new bash ports/mygo/scripts/ci/run
```

重新执行必须用新的空 build root；不要为通过门禁复制预热 cache。

## 公开来源核实（2026-11-05 UTC）

- https://go.dev/dl/?mode=json 列出 go1.27.3。Linux amd64 archive SHA256：`65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40`。setup-go 从官方分发渠道安装固定版本，不降级语言版本。
- https://proxy.golang.org/github.com/egoist/mygo/@v/v0.7.5-0.20261105040656-977ea57e0baf.info 返回同版本及 origin `977ea57e0baf78da62698ba7371420d192260466`。zip checksum 仍由既有 manifest 独立验证。
- 官方仓库 `git ls-remote ... refs/tags/v6`（checkout/setup-go）和 `refs/tags/v4`（upload-artifact）核实：checkout `d23441a48e516b6c34aea4fa41551a30e30af803`；setup-go `924ae3a1cded613372ab5595356fb5720e22ba16`；upload-artifact `ea165f8d65b6e75b540449e92b4886f43607fa02`。workflow 使用确切 SHA，不使用浮动 tag。
- https://github.com/actions/checkout/tree/d23441a48e516b6c34aea4fa41551a30e30af803
- https://github.com/actions/setup-go/tree/924ae3a1cded613372ab5595356fb5720e22ba16
- https://github.com/actions/upload-artifact/tree/ea165f8d65b6e75b540449e92b4886f43607fa02

Workflow contract validator 是离线契约检查，不冒充 GitHub schema validator 或实际 hosted run。远端提交后的 workflow run、job conclusions 和对应 commit 必须另外核实。

## Workflow lint 与本地审查

固定 actionlint v1.7.7 来源：https://github.com/rhysd/actionlint/tree/v1.7.7 。本地经 Go module checksum 验证并安装到独立外置工具根；执行下列检查 exit 0（2026-11-05 UTC）：

```sh
GOBIN=/outside/source/ci-tools/bin GOPATH=/outside/source/ci-tools/gopath GOCACHE=/outside/source/ci-tools/cache TMPDIR=/outside/source/ci-tools/tmp GOTOOLCHAIN=local go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.7
/outside/source/ci-tools/bin/actionlint -shellcheck= -pyflakes= .github/workflows/mygo.yml
```

先建立上述外置目录。此次关闭 actionlint 的可选 shellcheck/pyflakes 集成；shell 语法另由 validator 的 `bash -n` 检查。actionlint 曾发现 job-level env 不能使用 runner.temp，已改成步骤内 RUNNER_TEMP 加 GITHUB_ENV。有界日志由同步前台 Python 管道写入，门禁既等待实际命令，也等待日志落盘；原始命令失败和 logger 写入失败都会让 CI 失败。validator 额外验证超过 512 KiB 的输出不会截断控制台、证据末尾固定 256 KiB，以及写入失败会返回非零。所有本地静态检查仍不能代替远端实际 Actions run。

故障与永久门禁登记见 [GO-CI-001](engineering/failure-cases/GO-CI-001.md)。实际强制链为 `.github/workflows/mygo.yml` → `scripts/ci/validate.py` → `scripts/ci/test_runner.py`；含 9 项安全拒绝、同步有界 logger 及 8 项 runner 合成控制流 fixture，不替代真实构建。

## 首次托管运行

2026-11-05，[main commit f2f81df](https://github.com/ferrumlux/noonmark-go/commit/f2f81df2defa7d2d71ca1035371d7089bdc73564) 的真实 push 触发 [Actions run 42083897005](https://github.com/ferrumlux/noonmark-go/actions/runs/42083897005)，18:33 UTC completed/success，全部步骤通过。证据 artifact ID 13442358332，4029 bytes，保留至 2026-11-12。具体 job、hash 与验收边界见 GO-CI-001。这一绿灯只覆盖对应 commit，新业务提交会另起 CI，必须重新核实。正式 tag／release 流程仍须在完整验收与许可包准备后接通。

# 发行准备与正式发布门禁

目前 `ports/mygo/release/approval.json` 为 **未批准**。菜单批次、development CI 和六目标编译通过，均不能解除完整功能对等与六目标原生验收的门禁。本批没有创建 tag 或 Release。

## 验证候选

在干净已提交的 checkout，准备空的源码外目录，指定官方 Go 1.27.3：

```sh
mkdir /absolute/external/noonmark-release-check
export NOONMARK_MYGO_BUILD_ROOT=/absolute/external/noonmark-release-check
export NOONMARK_GO=/absolute/path/to/go
bash ports/mygo/scripts/release/check
```

本地须安装 Python 3 与 `ports/mygo/scripts/ci/requirements.txt` 中固定版本的 PyYAML。可复用可信外置 `GOPATH` 下载缓存；源码归档重建始终使用空缓存，关闭代理和校验服务器。默认版本 `v0.0.0-dev.<commit前12位>`，不会发布。发布流程相关文件推到 main 时自动执行只读候选验证，也可从 GitHub 手动运行 “Reproducible release packages” workflow，只上传短期 Actions 产物。

检查内容：发行逻辑单测、workflow 安全门禁、六目标二进制格式、许可证与源码摘要、完整 SHA256SUMS、两个不同绝对根目录的全部产物逐字节比较，以及从**已归档**对应源码包离线重建六目标并比较二进制。候选包目前为便携程序，没有发布者签名、安装器或 macOS `.app`。

默认输出在 `first/package/`，可分发文件仅为六份平台归档、一份 source.tar.gz、manifest.json 和 SHA256SUMS。`source/`、`build/`、Go 缓存和测试目录不是上传资产。`scripts/release/package` 不接受脏源码、未知版本、仓库内输出或覆盖既有输出。

## 正式批准

1. 完成所有功能与兼容性工作，运行项目完整验收。不得以单测、Linux headless、交叉编译或静态报告替代原生桌面验收。
2. 在最终源码提交执行 `python3 -B ports/mygo/scripts/release/release.py source-digest`。该摘要覆盖所有已跟踪文件及权限，只排除批准清单和其验收报告目录，避免证据自引用。后续任何应用、依赖、脚本、workflow 或普通文档变化都会使批准失效。
3. 将真实、可审核且不含用户资料的验收摘要保存为 `ports/mygo/release/acceptance/*.json`。九项 requirement 固定为 `feature-parity`、`storage-compatibility`、`privacy-security` 与六个 `native-<os>-<arch>`。每份报告须有 schema=1、对应 requirement、result=`passed`、source_sha256、非空 commands 和 observations；六平台报告还须 execution=`native-desktop`。测试环境、日期、工具版本与详细证据位置也应记录。commands 必须是**实际执行过**的命令，不是待运行计划。
4. 更新 approval.json，保留固定 schema，明确 `approved: true`、唯一稳定版本 `vMAJOR.MINOR.PATCH`、同一 source_sha256、各 requirement 的仓库内报告 path 和报告 SHA256。reason 说明验收范围。提交批准及报告。不得为通过门禁伪造报告或把尚未验收的平台标记通过。
5. 执行 `release.py gate vX.Y.Z` 以及 `scripts/release/check --version vX.Y.Z`。推送用户已授权的 main，并等待该**精确 commit** 的 MyGo development workflow 成功。报告元数据是维护者的明确验收声明，不是自动证明业务完整性。
6. 只有用户授权的正式发布时才创建并推送对应稳定 tag。tag 必须在 main 历史中，版本须与批准一致。流水线再次验证批准和精确 main commit 的成功 CI，然后全量构建两次并从源码包重建。

## 发布与失败处理

main候选和手动触发不传稳定版本、没有发布路径。只有 refs/tags/ 下、依赖验证 job 成功的 tag 发布 job 获得 contents:write。手动 workflow 没有发布路径，也没有绕过批准的输入。平台包、源码、校验文件全量上传到新 draft 后，检查 GitHub 返回的资产 SHA-256、大小和远端 tag commit，最后才公开为正式 Release。

既有 Release 不被覆盖，不使用 `--clobber`，不自动重试写请求。上传或远端摘要检查失败时，可能留下未公开 draft；检查实际状态后由维护者决定恢复方式，不得盲目重跑或覆盖。只有重跑 publish job 时，run_attempt 对应的产物不存在会安全失败；应先确认状态，再决定是否完整重跑。默认阻断清单使未完成的迁移无法误发正式版。

现有 `.github/workflows/mygo.yml` 与其门禁保持独立，不被此流程替代。发布准备不会改变应用 development 身份，不读取生产资料、iCloud 或凭证。

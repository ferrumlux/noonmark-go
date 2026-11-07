# f7566df hosted CI 事实摘要

提交 [f7566df50eeb66c1bfbfc1c7e7d1d912c661a923](https://github.com/ferrumlux/noonmark-go/commit/f7566df50eeb66c1bfbfc1c7e7d1d912c661a923)，tree `2b929173ce542a8a7bb8c1ce0459a7001df49e02`，parent `824e96614a3c0196f9086505d201aae23b7c6de2`。以下均为2026-11-07 UTC、main push、attempt1的新证据；没有重跑。

| 工作流 | Run | 终态 |
|---|---|---|
| MyGo development | [42338467418](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467418) | success，1/1job |
| Native CLI/headless | [42338467571](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467571) | success，7/7jobs、6/6hosts |
| Native window lifecycle | [42338467402](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467402) | failure，6/7jobs、5/6hosts |

## 常规门禁

四module race/vet、Unicode16、依赖及supported native tests、六目标compile-only与Linux synthetic CLI/headless gates通过。root race **339.348s**，默认600s未改，余量**260.652s**；taskapp **93.139s**。主门禁step904s，job完成04:42:02Z。单次host测量不是受控性能倍数，也不是任意runner裕度保证。

[artifact13560448727](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467418/artifacts/13560448727)：6415bytes，SHA256 `ba829cd2963a951e284efb1ab82a423144d649fe2eddfaf40bb08883de214e18`。正好六个有界文本成员，CRC与逐成员hash通过，四个streamed输出与完整日志对应。此artifact不含executables，不能重算它自身的binary bytes。

## 六 host CLI/headless

linux/windows/darwin × amd64/arm64均在对应真实runner执行。各target的list/create/reopen exit0；reuse/implicit exit1且拒绝诊断匹配。12张Day/Pool PNG均2400×1536，CRC、完整解码通过。只是合成开发会话headless图片，不是原生系统像素或IME验收。

## 窗口失败原件

[Intel macOS job128301753997](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467402/job/128301753997)，runner label macos-15-intel。第6步lifecycle执行和第9步required gate失败，采集/上传成功。其余五host共**10个stage成功**；不是12个stage全部通过。

- Create stage在READY输出一次后发生`software deadline exceeded`。driver12.75035987s；result12.752162778s。应用exit=-9，reaped=true、group_empty=true。
- stage1.json实际0bytes，stderr空。Open stage未运行。不能把READY当完成关闭和drain，也不能从当前证据判断具体卡点或归因quote改动。
- [失败artifact13560133136](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467402/artifacts/13560133136)：2326bytes，SHA256 `bdbb8ee9d58f4bdeb84545766fee9a8e143afaafee7129a9b9559b516eb12102`。原失败记录不改写为通过。
- [binary artifact13560826816](https://github.com/ferrumlux/noonmark-go/actions/runs/42338467402/artifacts/13560826816)：实际Mach-O darwin/amd64，18072736bytes，SHA256 `1243ab8b847950cb29ff65f96c78c92299b17400a5c5955cbb1934ce3f005f90`；manifest与runtime hash一致。

## 完整性与界限

27个真实ZIP的GitHub digest、大小、CRC与每个成员hash均验证。两条native workflow各六个真实binary均核ELF/Mach-O/PE架构和manifest；对应target的binary bytes hash一致。全部run/job/artifact ID、UTC时间和hash在`evidence-f7566df.json`。

窗口结论仅lifecycle-only；native system pixels=false、IME=false、完整产品/生产资料/发行验收均未完成。旧实验与失败证据保留，不被此次新结果覆盖。

# Codei18n Desktop

CodeI18n 的桌面端：基于 [mygo](https://github.com/egoist/mygo) 的**原生 UI**（GPU 自绘，不使用 webview），在进程内直接调用 [Codei18n core](https://github.com/CairnPack/Codei18n) 库，管理多个项目的注释提取、翻译与浏览。

## 功能

- **工作区与多项目**：用系统目录对话框添加项目，或把文件夹拖进窗口；最近项目列表持久化在用户配置目录；侧栏显示各项目状态（未初始化 / 已初始化 / 有待翻译 / 扫描中 / 翻译中 / 出错）。启动时只读取 mappings、index 与缓存（快速载入），不触发全量扫描。
- **项目初始化向导**：源语言、本地语言、排除规则（预填 `node_modules`、`build`、`.gradle`、`bin`、`obj`、`.dart_tool` 等）、provider 配置档；可选安装 git hook、写入 `.gitignore`。
- **翻译 API 配置与令牌**
  - 全局设置页管理 provider 配置档（新建、复制、重命名、删除），每个配置档可以保存自己的令牌。
  - 项目设置页选择配置档并覆盖单个字段，也可以保存项目专用令牌；每个字段旁标明值来自"默认 / 全局 / 项目"，可一键恢复继承（恢复全部时询问是否同时删除项目令牌）。
  - 令牌输入框默认遮罩，可临时显示正在输入的内容；保存后只显示"已保存 ••••abcd"与来源层，不再回显明文；替换与删除（删除需二次确认）。
  - 令牌只存系统凭据库（Windows 凭据管理器），全部经由 core 的令牌 API：在这里保存后，`codei18n translate` 与 git hook 立即可用，无需任何设置。
  - 检测到配置文件中的明文令牌时显示警告条和"迁移到系统凭据库"按钮；"测试连接"用当前生效的令牌发一次短请求，显示延迟或错误（错误信息不含令牌）。
- **提取与翻译**：扫描 / 提取注释、翻译缺失条目、清理孤儿条目、重试失败条目；实时进度（阶段、计数、当前文件），可取消，可调并发数。多项目批量：勾选多个项目加入队列，顺序执行或在全局并发上限内并行。
- **注释浏览器**：虚拟化列表（数万条不卡）；按文件、编程语言、识别出的原语言、是否已翻译、是否失败筛选，支持全文搜索；并排显示原文与译文，可手动编辑译文（原子写回 mappings.json）、纠正识别出的原语言；顶部统计总数、各原语言占比与翻译覆盖率。
- **文件监视**（可选）：项目文件变化后在后台做增量扫描并刷新统计，不会自动翻译（翻译会产生费用）。
- **界面语言**：English、简体中文、繁體中文、日本語，默认跟随系统，可在设置中即时切换。界面语言与项目的"翻译目标语言"是两个独立的设置。

## 构建

前置条件：

- **Go 1.27.1+**（未安装时 `GOTOOLCHAIN=go1.27.1` 会自动下载对应工具链）
- **MinGW-w64 gcc**：core 使用 tree-sitter（C 代码），必须启用 cgo，例如 `winget install BrechtSanders.WinLibs.POSIX.UCRT`
- `CGO_ENABLED=1`（mygo 自身默认 `CGO_ENABLED=0`，构建脚本会设置）

```powershell
.\scripts\build.ps1          # 产出 build/windows-amd64/Codei18n.exe 与安装包 "Codei18n Setup <版本> amd64.exe"
.\scripts\build.ps1 -Test    # 先运行全部测试
```

构建脚本还会让 MinGW 跳过它的默认 manifest（否则与 mygo 生成的 manifest 冲突，链接器报 `.rsrc merge failure`）。

本次只交付 Windows 构建。

## 开发

```powershell
$env:GOTOOLCHAIN = 'go1.27.1'; $env:CGO_ENABLED = '1'
go work init . ..\Codei18n     # 可选：使用本地 core 源码开发（go.work 不入库）
go tool mygo dev               # 实时重载运行（应用名为 "Codei18n Dev"，配置目录与安装版分开）
go test -race ./...            # 包括无窗口视图测试
go tool mygo vet .             # 检查是否把构建期的值存进了状态或 goroutine
go tool mygo generate          # 修改 locales/*.json 后重新生成 locales/catalog_gen.go
go tool mygo i18n check        # 检查各语言目录是否完整
```

发布时 `go.mod` 引用 `github.com/CairnPack/codei18n` 的提交（当前为 core 的 feat/a5-langdetect 分支头）；core 合并并打 tag 后更新为 tag。

## 架构

```
main.go                  窗口与依赖接线
shell.go                 系统目录对话框、UI 语言切换（views.Shell 的实现）
watcher.go               基于 mygo.WatchFiles 的项目文件监视
internal/app/            应用状态与业务逻辑（纯 Go，可单测）
  workspace.go           工作区、最近项目持久化、项目状态
  jobs.go                任务队列：顺序 / 并行、取消、进度节流
  wizard.go              初始化向导
  configform.go          项目 / 全局设置表单（字段来源层、覆盖、恢复继承、配置档管理）
  tokens.go              令牌面板（只经由 core 的令牌 API）
  browser.go             注释浏览器模型（筛选、搜索、统计、编辑、纠正语言）
internal/views/          mygo ui 视图函数，只调用 app 层，不直接做 I/O
locales/                 ARB 目录（en 为源语言，zh-Hans、zh-Hant、ja）
```

- 应用状态只在主线程读写；长任务在后台 goroutine 中运行，结果通过 `Window.Update` 送回主线程，界面不会卡顿。
- core 对同一项目的写任务互斥：任务运行时编辑译文会提示"项目忙"。
- 测试：`internal/app` 用真实 core（临时目录、内存凭据库、mock 翻译后端）测试任务队列、配置合并与取消；`internal/views` 用 `ui.NewTester` 做无窗口视图测试；`secrets_interop_test.go` 验证 mygo `Secrets.Service("codei18n")` 与 core 的令牌格式互通。

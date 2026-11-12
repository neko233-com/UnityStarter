

## Codely Structured Memories

### User

### Feedback

### Project
- [2026-11-05 14:13:53] UnityStarter（D:\Code\neko233-Projects\UnityStarter）是 Unity 国际版 + 团结引擎 Hub 的无广告替代品，用 Go + github.com/egoist/mygo 原生 UI（非 Web 前端）实现；2026-11-05 与用户确认首版范围：只做项目管理、已安装编辑器检测、设置、中英双语，在线下载/安装引擎留到下一版。
- [2026-11-05 14:30:25] Windows 上 Hub 数据位置（UnityStarter 扫描/同步依据）：Unity Hub 配置在 %APPDATA%\UnityHub（projects-v1.json 存项目列表/上次打开时间且可写，secondaryInstallPath.json 存额外编辑器根目录，projectDir.json 存项目扫描目录）；两个 Hub 的 favoriteProjects.json 都是「JSON 字符串里再套 JSON 数组」的双重编码格式。团结 Hub 1.4.7 的 app.asar 中 0 处引用 projects-v1，没有可写的项目清单文件（只有 projectsArchitecture.json 存名称→架构），所以团结项目只能靠磁盘扫描，收藏/目录可双向。编辑器布局统一为 <root>\<version>\Editor\Unity.exe 或 Tuanjie.exe，团结版本号带 t 后缀（如 2022.3.62t17）。
- [2026-11-05 14:30:30] UnityStarter 的公开仓库是 https://github.com/neko233-com/UnityStarter（gh 账号 neko233-com，默认分支 main），2026-11-05 已向 GitHub 用户 3cheung 发出 write 权限协作者邀请。
- [2026-11-05 16:59:07] 交付物与产品命名要求（2026-11-05 用户明确）：所有名字都不要有空格 —— exe、安装包、安装目录、窗口标题、产品名统一为 UnityStarter（mygo 的 `name`、`windows.displayName`、locales 的 appTitle/cliVersion/issueTitle 等都要一致），数据目录为 %APPDATA%\UnityStarter。
- [2026-11-05 18:32:26] UnityStarter 产品约束（2026-11-05 用户明确）：它只是 Hub，体积要小（exe 约 12 MB），引擎安装包（约 3.8 GB）不能算作应用体积；验证安装流程必须用「模拟安装」，不要真的下载 GB 级安装包。必须提供国际版 Unity 下载且不得跳转/换源到国内镜像或团结，Unity 与团结两套并行管理；项目行大字体并显示引擎、渲染管线、Git/SVN/Lore 状态；日志自动淘汰（1MB 轮转、5 份、14 天）。当前发布版本 1.3.0。
- [2026-11-05 18:32:31] Unity/团结编辑器静默安装参数（从 Unity Hub app.asar 的 installFromExe 确认）：Unity 为 `UnitySetup64-<ver>.exe /S /D=<目录>\<ver>`（/D= 不带引号），团结为 `/S /D="<目录>\<ver>"`（带引号）；/D 必须最后。UnityStarter 的模拟安装用 <安装目录>\<版本>\Editor\{Unity.exe|Tuanjie.exe} + .unitystarter-simulated 标记，扫描据此识别。CI 注意：Windows runner 的 core.autocrlf 会把 LF 换行变 CRLF 导致 gofmt -l 全量失败，仓库需保留 .gitattributes 的 `* text=auto eol=lf`。

### Reference
- [2026-11-05 17:27:09] Unity 官方版本接口（UnityStarter 下载页使用）：GET https://services.api.unity.com/unity/editor/release/v1/releases?limit=25&stream=LTS —— limit 上限 25（写 40 返回 400）；releaseDate 是带毫秒的 ISO8601（Go 用 time.RFC3339Nano 解析）；Windows 安装包条目 platform=WINDOWS、architecture=X86_64、type=EXE，URL 指向 download.unity3d.com/download_unity/<changeset>/Windows64EditorInstaller/UnitySetup64-<version>.exe。团结引擎版本来自 %APPDATA%\TuanjieHub\releases.json 的 official[]（downloadUrl 指向 download.tuanjie.cn）。

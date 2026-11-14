# kos-appstore

KOS 系统应用项目：应用中心 — 目录来源、安装参数、安装与升级入口。

- 设计：`tsic/kos-design` 仓的 `kos-appstore/`（范围、契约、验收、决策记录）
- 交付形态：独立扩展制品，经 `kos-sdk` 打包成带摘要的扩展包，装成系统组件；不随控制面编译

## 模块

| 目录 | 模块 | 职责 | 契约 |
|---|---|---|---|
| `catalogue/` | 目录来源 | 随包应用定义、条目投影（显示信息、版本、依赖、参数结构、摘要）、同步方式 | `APPSTORE-C-24`，引用 `KOSD-C-32` |
| `appcenter/` | 应用中心 | 页面与动作声明、窗口应用清单、页面 readings（条目与升级入口） | `APPSTORE-C-31`，引用 `DASH-C-22`、`KOSD-C-73` |
| `declaration/` | 扩展声明 | 打包清单与宿主声明的同一份产品数据 | `KOSD-C-18`，引用 `KOSD-C-17`、`KOSD-C-16` |
| `artifact/` | 打包 | 经 `kos-sdk` 的打包路径产出包与摘要，覆盖清单与全部 UI 资产 | `SDK-C-4`，引用 `KOSD-C-17` |
| `assets/` | 窗口资产 | 应用窗口的 HTML/CSS/JS/图标，随包分发 | `KOSD-C-73` |
| `cmd/kos-appstore/` | 构建入口 | 写出归档与摘要（`make package`） | `KOSD-C-17` |
| `tests/acceptance/<TC-id>/` | 验收用例，目录名取设计编号；`*.go` 为控制面侧，`shell.test.js` 为窗口侧 | | `03-acceptance.md` 的对应条目 |
| `tests/shell/` | 窗口侧的浏览器替身（假文档、假窗口、假 fetch、假时钟），无运行时依赖 | | |

## 目录来源

包贡献一个目录来源（`APPSTORE-C-24`）：来源标识 `kos-appstore`、同步方式 `pull`、每个随包定义一个条目。
条目的标识取定义名，条目携带定义 spec（含包摘要）与显示信息，因此目录同步写入的内容就是摘要覆盖的内容。

随包定义在 `catalogue/definitions/`：每个定义在文件里固定自己的包摘要，加载时按内容重算并比对，
不一致即拒绝加载（`KOSD-C-32`）。条目的不可安装判定由控制面目录给出：摘要不匹配或缺依赖时条目仍可见，
但带有原因，且同步不会改写已写入的定义。

## 应用中心

应用中心同时贡献页面与窗口应用（`KOSD-C-18` 允许两者并存，二者不互相替代）：

- 页面 `app-center`：两个计算源条目（可安装条目、可升级应用），各带安装与升级动作；动作是控制面 API 上的
  写请求，页面不持有集群凭据（`DASH-C-22`）。
- 窗口应用清单：必填字段按 `KOSD-C-73`；UI-only，无后端、无深链、无权限；入口是本包资产所在的自有 origin。
- readings：条目项按配置结构生成表单字段（类型、默认值、必填、取值范围、枚举、敏感标记），不硬编码
  应用字段；同一值带上包声明的权限集合（`APPSTORE-C-77`），详情与安装确认两处都渲染它；升级项给出
  升级路径状态（`Available` / `SourceMissing`）。
- 条目引用以包标识为前缀（`kos-appstore/entries`、`kos-appstore/upgrades`）：宿主按 `pages.Router`
  把条目分发给本产品的读者（`KOSD-C-22`）。未接线的条目在页面上被拒绝，不会给出没有权限集合的值。

窗口资产在 `assets/`，是一个不打包的 ES 模块加上同一目录的 HTML/CSS：控制台来源与窗口实例从框架地址的
`kos-console` / `kos-window` 取得（入口页设为 `no-referrer`，不靠 referrer），握手只接受该 origin 且
`source` 为嵌入方的 `init`，令牌只存在闭包里；随后读取应用中心页的条目项并报告存活性。列表与详情把每个
条目呈现为安装来源——显示信息、版本、依赖、参数结构（控件由结构生成，不硬编码应用字段）、包声明的
权限集合与包摘要——不呈现已安装应用状态：条目本身不是应用（`APPSTORE-C-31`）。不可安装的条目列出
缺失能力，不给安装入口。

安装向导由条目值生成：名称与命名空间是每一次安装相同的应用封装，其余字段按配置结构的类型、默认值、
必填、范围、枚举与敏感标记落成控件；界面只呈现结构，不预先判定值，缺失必填、越界与枚举不符由写路径
拒绝并在界面上指出字段与约束（`KOSD-C-32`）。敏感参数是只写控件，界面只显示是否已设置，不回显值
（`KOSD-C-14`）。确认预览取控制面计划请求（`KOSD-C-62`）的返回，确认后经页面动作声明的控制面 API 提交；
应用所有者取发起安装的会话账号，界面没有指定所有者的入口（`APPSTORE-C-76`）。详情与确认两处列出同一份
包声明权限，未声明的权限名不出现，也没有授予或撤销入口（`APPSTORE-C-77`）。

## 打包

`make package` 经 `kos-sdk` 的 `packaging.Pack` 写出 `dist/kos-appstore-<version>.tar.gz` 与
`dist/kos-appstore-<version>.digest`。包结构固定为 `manifest.json`、`assets/<path>` 与 `digest`，
摘要覆盖清单与全部 UI 资产：任一资产或清单变化都改变摘要，同一输入产出同一字节。第一方与第三方共用
同一条打包路径（`SDK-C-4`）。

## 构建与测试

```sh
make lint        # gofmt 检查 + go vet + 窗口模块语法检查
make test        # 单元测试、窗口侧用例与验收测试（自动下载 envtest 二进制）
make js-test     # 只跑窗口侧的浏览器用例（需要 node）
make acceptance  # 只跑 tests/acceptance 的 Go 用例，输出用例名
make package     # 写出扩展包与摘要到 dist/
```

验收测试使用 envtest 启动真实 kube-apiserver 与 etcd，通过控制面 API 与目录同步观察外部行为；
页面断言取 `GET /api/v1/pages/app-center` 的返回值：`tests/acceptance/support` 按宿主的接线方式把本产品
的读者接入页面壳，接线断开或读者错位时页面读取失败；
环境变量 `KUBEBUILDER_ASSETS` 指向 envtest 二进制目录，`make test` 会下载并设置。同一目录下的
`shell.test.js` 是窗口侧的用例部分：加载 `assets/app.js` 导出的窗口模块，用 `tests/shell/` 的替身
驱动握手、目录读取与条目呈现，由 `node --test` 运行。

当前验收用例：`tests/acceptance/APPSTORE-TC-9/`（P1）、`APPSTORE-TC-10/`（P1）、`APPSTORE-TC-57/`（P1）、
`APPSTORE-TC-58/`（P1）、`APPSTORE-TC-162/`（P1）、`APPSTORE-TC-163/`（P1）。

## 边界

- 不托管包仓库，不自造分发协议，不绕过摘要校验（`KOSD-NG-3`）。
- 不拥有应用生命周期、卷与入口；安装与升级由控制面执行，状态从应用对象读取。
- 不提供专用安装通道；第一方与第三方同一条路径。
- 不提供平台与组件的升级入口；那部分归 kos-updater。
- 权限的确认与撤销归框架（`KOSD-C-75`）；本项目只展示声明。

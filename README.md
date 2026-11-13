# kos-taskmanager

KOS 系统应用项目：运行视图 — 概览、应用、应用详情与运行中的 Pod。

- 设计：`tsic/kos-design` 仓的 `kos-taskmanager/`（范围、契约、验收、决策记录）
- 设计依据：`tsic/kos-design@19f6b81`

## 边界

- 只读运行状态与发起控制面动作；不拥有应用生命周期、卷与入口，不保存日志或事件。
- 浏览器不持有集群凭据：页面只经控制面 API 与自身应用令牌读写（`KOSD-F-9`、`KOSD-C-12`）。
- 本仓库交付一个独立扩展制品：装成系统组件后，其窗口应用出现在桌面，卸载或停用后消失（`KOSD-C-17`、`KOSD-AC-72`、`KOSD-AC-54`）。

## 现有实现

| 路径 | 职责 |
|---|---|
| `app/` | `App` 与扩展声明：唯一清单来源，字段集合按 `KOSD-C-73`、`KOSD-C-18` |
| `app/overview.go` | 概览页声明：七个条目及其来源类型与条目引用（`TASK-C-1`、`KOSD-C-53`） |
| `app/readings.go` | 概览读模型：按来源类型读取节点、异常对象、待处理升级、最近事件与维护归属（`KOSD-C-53`、`KOSD-C-64`、`KOSD-C-65`、`KOSD-C-82`、`KOSD-C-13`、`KOSD-C-59`） |
| `internal/artifact/` | 打包：读 `assets/`，经 kos-sdk 的 `packaging` 产出工件并用安装路径的检查自验（`SDK-C-3`、`SDK-C-4`） |
| `cmd/package/` | 构建入口：写 `dist/kos-taskmanager-<版本>.tar.gz` 与 `.digest` |
| `assets/` | 窗口文档与 UI 资产：入口文档、脚本、样式、图标与四个页面壳 |
| `assets/app.js` | 窗口引导：握手、按页面清单决定导航入口、读取投影并渲染（`DASH-C-22`） |
| `assets/api.js` | 控制面读路径：页面清单与单页投影，只发 GET、只带应用令牌（`KOSD-C-12`、`KOSD-C-75`） |
| `assets/router.js` | 四页路由：概览、应用、应用详情、Pod 运行态 |
| `assets/handshake.js` | 应用侧会话通道：`ready` / `init`、存活与未保存上报（`KOSD-C-75`、`KOSD-C-81`） |
| `assets/pages/` | 四个页面壳；概览按控制面投影逐项渲染（`TASK-C-1`），其余页面壳待接入 |
| `tests/acceptance/fixtures/` | 装配：把本仓打出的包装进一次性集群对象集、审计桩与会话，供概览用例读取页面 |
| `tests/acceptance/TASK-TC-1/` | 概览逐项读取：来源类型、观测时间、聚合规则、事件窗口与顺序、维护归属、四种身份的可见性 |
| `tests/acceptance/TASK-TC-2/` | 概览只读：写方法被拒、页面无修改入口、请求前后对象与投影不变 |
| `tests/acceptance/KOSD-TC-*/` | 装载、卸载、停用与打包契约的验收用例 |

## 清单

`app.Declaration()` 是打包与用例共用的唯一声明：

- 扩展 `kos-taskmanager`，贡献一个窗口 `App` 与一张只读的 `overview` 页面（七个条目）；无对象类、控制器与设置项。
- `Page` 是服务端数据投影：条目按来源类型读取，页面不声明动作，条目不带修改入口（`KOSD-C-53`、`KOSD-C-18`）。
- 概览的五个展示组落在七个条目上——节点就绪、异常对象、待处理升级、最近事件、维护中的节点、维护归属与维护受影响的应用；维护组拆成三项，因为节点事实、审计归属与受影响应用分别是三种来源类型。
- `App` 必填项齐全；`entry` 是独立 origin 上的 HTTPS 文档地址，与控制台不同源；`permissions` 为空数组，`backend` 与 `deepLinks` 缺席（UI-only）。
- 卸载保留语义：`apps` 与 `pages` 贡献删除，无外部资源；包不拥有任何数据。

入口 origin 由部署映射到本包资产（`https://taskmanager.apps.kos.tsic.top`）；控制台按扩展、应用、版本与包摘要签发短期资产地址（`KOSD-C-73`）。

概览页的数据面在控制面：窗口按 `GET /api/v1/pages/overview` 读取该页投影，每项带来源类型、来源与观测时间；写方法对该读接口返回 405。窗口不直连集群 API，也不在浏览器内重算聚合规则（`KOSD-C-12`、`KOSD-C-53`）。

窗口按 `GET /api/v1/pages` 读取当前会话可见的页面清单：清单里没有的页面，导航入口与正文都不出现，窗口转到第一个可见页面；投影读不出来时页面给出原因，不以空结果冒充没有数据（`KOSD-C-13`、`KOSD-C-59`）。概览逐项显示来源类型、观测时间与控制面给出的陈旧标记，聚合项显示判定规则，页面明示只读及其原因且不提供任何修改入口（`TASK-C-1`、`KOSD-C-53`）。

## 构建与运行

```sh
make lint        # gofmt 检查、go vet、ES 模块语法检查
make test        # Go 单元与验收用例，以及 Node 页面壳用例
make acceptance  # 只跑 tests/acceptance/，-v
make package     # 产出 dist/ 下的包与摘要
```

本地查看页面壳：把仓库根目录用静态文件服务打开 `assets/index.html`。未带控制台来源时窗口按独立预览运行，只渲染页壳与路由，不读取任何数据。

## 依赖

- kos-sdk：清单结构、打包与安装路径检查（`SDK-C-1`、`SDK-C-3`、`SDK-C-4`）。
- kos-dashboard：窗口与页面宿主；验收用例经它的应用注册表与资产存储判断装载、卸载与停用后的可见性。
- kosd：控制面 API、Pod 投影与动作（`KOSD-C-12`、`KOSD-C-76`、`KOSD-C-77`）。

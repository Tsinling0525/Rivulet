# wfengine — C++17 工作流流程引擎

按照仓库根目录的 `PRD.md`（企业级工作流流程引擎设计方案）实现的**单进程流程引擎**：C++17 + 标准库，无第三方依赖（JSON、表达式、脚本、状态机全部自研），自带 CLI、单元测试和示例流程。

```bash
cd cpp
cmake -S . -B build -G Ninja          # 需要 CMake >= 3.16 与 C++17 编译器
cmake --build build
ctest --test-dir build --output-on-failure   # 78 个测试用例
./build/wf-cli demo examples/expense_approval.json --demo-org
```

> 说明：Rivulet 根仓库（Go）的 `AGENTS.md` 明确规定该仓库内不再有流程引擎。本目录是按用户要求落地的 C++ 实验实现，独立自成一体：只依赖 C++ 标准库，不引入任何 Go 侧代码，也不改动 Go CLI 的行为。

---

## 1. 目录结构

```
cpp/
├── CMakeLists.txt
├── include/wf/          # 公共头文件（按职责分层）
│   ├── json.hpp         自研 JSON（值/解析/序列化/路径访问/深合并）
│   ├── expression.hpp   表达式引擎（${...}，沙箱）
│   ├── script.hpp       脚本节点语言 wfscript（沙箱）
│   ├── types.hpp        枚举、状态机、时长/时间工具
│   ├── model.hpp        流程定义 DSL（节点/连线/审批人/超时/监听器）+ 发布前校验
│   ├── entity.hpp       实例/节点实例/任务/轨迹/事件/定时任务/并行分支
│   ├── repository.hpp   仓储接口 + 内存实现 + JSON 快照持久化
│   ├── services.hpp     时钟、组织、服务调用器、消息通知、事件发布器
│   ├── engine.hpp       引擎核心
│   └── api.hpp          API 门面（对应 PRD §18 的接口语义）
├── src/                 实现（engine.cpp / engine_tasks.cpp / engine_scheduler.cpp 分文件）
├── tests/               单元测试（自研微框架 + 7 个测试文件）
└── examples/            JSON DSL 示例（含组织数据）
```

## 2. 能力清单（对应 PRD 章节）

| 能力 | 说明 | PRD |
|---|---|---|
| 流程定义管理 | JSON DSL 解析、草稿/发布/停用、版本管理、发布前校验 | §2.2 §4.2 §8 §34.2 |
| 定义-实例分离 | 实例绑定具体定义版本运行 | §3.1 |
| 状态机 | 流程/节点/任务三套状态与合法流转校验，非法流转被拒绝 | §6 §20 |
| 节点类型 | START / END / USER_TASK / SERVICE_TASK / SCRIPT_TASK / EXCLUSIVE_GATEWAY / PARALLEL_GATEWAY / SUB_PROCESS / WAIT_TASK / TIMER_TASK / MESSAGE_TASK | §4.3 |
| 人工审批操作 | 同意、拒绝、退回（任意历史节点）、转办、委派、加签（前/后加签）、撤回、认领 | §10 |
| 会签/或签/比例/依次 | `ALL / ANY / RATIO / SEQUENCE` 四种节点完成策略 | §11 |
| 审批人规则 | 发起人、指定用户、角色、岗位、主管（N 级）、表达式、上一节点处理人、组合规则 | §9 |
| 表达式引擎 | `${amount > 10000}`、逻辑/比较/算术、三元、路径与下标、内置函数与组织函数注入 | §13 |
| 脚本节点 | 沙箱化 wfscript：赋值、if/else、return、注释，无 IO | §16.3 §30.3 |
| 条件分支 | 条件网关按优先级求值，支持默认分支 | §7.4 |
| 并行分支汇聚 | 令牌 + 分支到达计数，全部到齐才继续 | §7.5 §24 |
| 子流程 | 同步/异步、变量入参出参回写 | §4.3.7 |
| 等待与定时 | 等待外部事件（`signalEvent`）、定时节点、定时触发扫描 | §4.3.8 §4.3.9 |
| 超时策略 | 节点/任务超时，动作：催办、升级、自动通过、自动拒绝、转办、终止 | §12 |
| 表单权限 | 字段可见/可编辑/必填，写入时过滤 + 必填校验 | §14.3 |
| 事件引擎 | 事件表（事务性 outbox）、监听器（HTTP/MQ）、幂等投递、退避重试阶梯 | §15 §21.2 §29.3 |
| 幂等 | businessKey 防重复发起、任务乐观锁版本、事件 eventId 消费 | §22 |
| 并发控制 | 实例级串行化 + 实例/任务版本号乐观锁 | §23 |
| 可追溯 | 流程轨迹（谁/何时/哪个节点/什么动作/输入输出/流转到哪） | §3.3 §17.5 §18.4 |
| 可观测 | 实例/任务/超时统计、JSONL 结构化事件 | §27 §33.5 |
| 查询服务 | 待办、已办、候选人任务、实例、轨迹、节点实例、事件 | §5.2.8 §18.3 |
| 运维干预 | 挂起/恢复/取消/终止、失败节点重试、管理员跳转 | §33.4 |
| 持久化 | 仓储接口 + 内存实现 + 全量 JSON 快照（可换 MySQL/Redis） | §17 §29.1 |

## 3. 快速开始

### 3.1 CLI

```bash
# 校验 + 发布
./build/wf-cli validate examples/expense_approval.json
./build/wf-cli publish  examples/expense_approval.json --repo /tmp/wf

# 发起（--var k=v，值可以是 JSON 字面量）
./build/wf-cli start --code expense_approval --key EXP-1 --initiator U10086 \
    --var amount=12000 --var type=travel --repo /tmp/wf --demo-org

# 待办 → 办理 → 轨迹
./build/wf-cli todo    --user U10086 --repo /tmp/wf --demo-org
./build/wf-cli approve --task 1010 --user U10086 --comment 同意 --repo /tmp/wf --demo-org
./build/wf-cli history 1001 --repo /tmp/wf --demo-org

# 定时/超时扫描（把时钟往前推）
./build/wf-cli tick --advance 24h --repo /tmp/wf --demo-org

# 事件 outbox 重投、统计、演示
./build/wf-cli dispatch-events --repo /tmp/wf
./build/wf-cli stats --repo /tmp/wf
./build/wf-cli demo examples/expense_approval.json --demo-org
```

所有子命令都打印与 HTTP 层一致的 JSON 信封 `{"code","message","data"}`（日志走 stderr），所以 CLI 本身就是一份可执行的 API 参考。`--help` 列出全部命令。

### 3.2 作为库使用

```cpp
#include "wf/engine.hpp"

wf::InMemoryRepository repository;
wf::SystemClock clock;                 // 测试里换 wf::ManualClock 就能控制时间
wf::InMemoryOrganization org;
wf::MockServiceInvoker invoker;        // 换成真实 HTTP/RPC 客户端
wf::LogNotifier notifier;
wf::LogEventPublisher publisher;

wf::ProcessEngine engine(repository, clock);
engine.setOrganization(&org);
engine.setServiceInvoker(&invoker);
engine.setNotifier(&notifier);
engine.setEventPublisher(&publisher);

engine.deployDefinition(wf::ProcessDefinition::fromJson(wf::json::Value::parse(dsl)), true);

wf::StartRequest request;
request.processCode = "expense_approval";
request.businessKey = "EXP-1";
request.initiator   = "U10086";
request.variables   = wf::json::Value::parse(R"({"amount": 12000})");
wf::OperationResult started = engine.startProcess(request);

wf::TaskOperation operation;
operation.action = wf::TaskAction::Approve;
operation.operatorUser = "U10086";
operation.comment = "同意";
engine.completeTask(started.createdTasks.front().id, operation);

engine.tick();            // 定时节点 / 超时策略
engine.dispatchEvents();  // 事件 outbox 投递
```

`wf::WorkflowApi`（`api.hpp` 中的三个服务类）把上面的调用包装成 PRD §18 的接口语义，返回 JSON。

## 4. DSL 速查

```jsonc
{
  "processCode": "expense_approval",
  "processName": "报销审批流程",
  "version": 1,
  "variables": { "amount": 0 },              // 默认变量
  "config": { "events": [                     // 流程级事件监听器
    { "type": "PROCESS_COMPLETED",
      "listener": { "type": "HTTP", "url": "http://business/callback" } }
  ]},
  "nodes": [
    { "id": "start", "type": "START" },
    { "id": "submit", "type": "USER_TASK", "name": "提交申请",
      "assignee": { "type": "INITIATOR" },
      "formPermissions": [{ "field": "amount", "editable": true, "required": true }],
      "taskTimeout": { "duration": "PT48H",
                       "actions": [{ "type": "REMIND", "channel": "EMAIL" }] } },
    { "id": "gate", "type": "EXCLUSIVE_GATEWAY" },
    { "id": "boss", "type": "USER_TASK", "assignee": { "type": "LEADER", "level": 2 },
      "completeStrategy": "ALL",              // 会签
      "rejectPolicy": "INITIATOR" },          // 拒绝后回到发起人
    { "id": "call", "type": "SERVICE_TASK",
      "service": { "type": "HTTP", "method": "POST", "url": "http://risk/check",
                   "body": { "userId": "${initiator}" },
                   "retry": { "maxAttempts": 3, "backoff": 1000 } } },
    { "id": "score", "type": "SCRIPT_TASK",
      "script": { "language": "wfscript",
                  "content": "level = amount > 10000 ? 'HIGH' : 'LOW';" } },
    { "id": "wait", "type": "WAIT_TASK", "eventKey": "order.paid" },
    { "id": "delay", "type": "TIMER_TASK", "duration": "PT1H" },
    { "id": "msg", "type": "MESSAGE_TASK", "channel": "EMAIL",
      "target": "${initiator}", "template": "APPROVED" },
    { "id": "sub", "type": "SUB_PROCESS", "processCode": "shipment",
      "async": false,
      "subProcessInput": { "orderId": "${orderId}" },
      "subProcessOutputs": ["trackingNo"] },
    { "id": "end", "type": "END" }
  ],
  "edges": [
    { "id": "e1", "source": "start", "target": "submit" },
    { "id": "e2", "source": "submit", "target": "gate" },
    { "id": "e3", "source": "gate", "target": "boss", "condition": "${amount > 10000}" },
    { "id": "e4", "source": "gate", "target": "call", "default": true }
  ]
}
```

审批人规则 `assignee`：

| type | 取值 | 语义 |
|---|---|---|
| `INITIATOR` | — | 发起人 |
| `USER` / `USERS` | `value` / `users` | 指定用户 |
| `ROLE` / `ROLES` | `value` / `users` | 角色（可多人 → 会签成员） |
| `POSITION` | `value` | 岗位 |
| `LEADER` / `INITIATOR_LEADER` | `level`（1=直属主管） | 主管链，`value` 可指定基准人 |
| `EXPRESSION` | `value` | 表达式求值，支持返回数组 |
| `PREV_ASSIGNEE` | — | 上一节点处理人 |
| `COMBINATION` | `rules` + `op: AND/OR` | 组合规则 |

简写形式也支持：`"assignee": "ROLE:FINANCE_MANAGER"`、`"assignee": "LEADER:2"`。

超时动作：`REMIND`（催办）、`ESCALATE`（升级到 N 级主管）、`AUTO_APPROVE`、`AUTO_REJECT`、`TRANSFER`（转办到指定人）、`TERMINATE`（终止流程）。

表达式：`${...}` 包裹；支持 `&& || ! == != > >= < <= + - * / % ?:`、路径 `form.type`、下标 `items[0]`、函数 `len/upper/lower/trim/contains/startsWith/endsWith/number/string/boolean/abs/min/max/round/now`，以及组织函数 `hasRole(user,'ROLE') / leader(user,2) / department(user) / usersByRole('ROLE') / userExists(user) / initiator()`。表达式沙箱内没有 IO、反射或任意代码执行。

脚本（`wfscript`，`SCRIPT_TASK`）：

```text
riskLevel = amount > 10000 ? 'HIGH' : 'LOW';   // 写流程变量
variables.score = round(amount / 1000, 2);     // 兼容 variables. 前缀
if (riskLevel == 'HIGH') { needManual = true; } else { needManual = false; }
return riskLevel;                              // 作为节点输出
```

## 5. 关键语义约定

这些是引擎里做出明确取舍的地方，改 DSL 之前先读：

1. **并行汇聚**：`PARALLEL_GATEWAY` 出度 > 1 为分叉（写 `wf_parallel_branch` 分支记录），入度 > 1 为汇聚（等所有入线到达才继续）；普通节点想当汇聚点用 `"joinAll": true`。令牌计数保证"分支未全部完成，流程不结束"。
2. **流程结束判定**：队列中无令牌且没有等待中的节点实例时才判定完成；任何一个分支的令牌挂起（用户任务/等待/定时/汇聚等待）都会让实例保持 RUNNING。
3. **会签统计口径**：`total` 只统计"有效投票"任务——被转办/委派子任务取代的父任务、委派子任务本身都不计票；转办子任务算新处理人的一票；前置加签（BEFORE）不参与通过统计。
4. **加签**：前置加签未完成时原处理人不能办理（前置条件）；后加签任务完成前节点不结束（`WAIT_AFTER_SIGN`）。
5. **委派**：子任务办完回到原处理人（`TASK_RETURN`），票仍属于原处理人；**转办**由新处理人直接投票并推进节点。
6. **拒绝策略** `rejectPolicy`：`END`（直接结束，result=REJECTED）、`INITIATOR`（回到发起人节点）、`PREV`（回到上一个用户任务节点）、`NODE`（回到 `rejectTarget`）、`EXCEPTION`（跳 `rejectTarget` 异常分支，缺失则 ERROR）。
7. **回溯（退回/撤回/驳回/跳转）**：清理未完成任务与节点实例、清空并行分支记录、取消未触发的定时任务，然后从目标节点重新执行；历史轨迹保留，用于审计。
8. **撤回条件**：发起人本人、流程处于 RUNNING、且没有其他审批人已处理过。
9. **事件投递**：主流程只往事件表写记录（SKIPPED / PENDING），`dispatchEvents()` 负责投递；失败按 `retryBackoffMillis`（默认 0s/10s/1m/5m/30m）退避重试，超过 `eventMaxRetries` 记为 FAILED 等人工处理。
10. **外部调用**：`SERVICE_TASK` 通过 `ServiceInvoker` 接口调用；重试次数与退避由节点 `service.retry` 控制，失败节点置 FAILED 并让实例进入 ERROR，可用 `retryNode` 恢复。
11. **字段权限**：节点配置了 `formPermissions` 时，办理人提交的变量补丁会被过滤（不可编辑/不可见字段写入被丢弃），必填字段缺失直接报错。
12. **幂等**：`processCode + businessKey` 重复发起返回既有实例（`idempotentReplay`）；任务已处理时重复提交返回错误且不改数据；`completeTask` 支持 `version` 乐观锁。

## 6. 扩展点

| 接口 | 用途 | 生产实现建议 |
|---|---|---|
| `Repository` | 数据存储 | MySQL/PostgreSQL（PRD §17 表结构），Redis 缓存待办 |
| `Clock` | 时间源 | 生产用 `SystemClock`，测试用 `ManualClock` 精确控制超时 |
| `OrganizationService` | 组织/角色/主管链/岗位 | 对接 HR 或组织中心，替换 `InMemoryOrganization` |
| `ServiceInvoker` | 自动节点外部调用 | libcurl / RPC 客户端（当前为 `MockServiceInvoker`） |
| `Notifier` | 消息节点与催办 | 邮件/短信/企业微信/钉钉/飞书 |
| `EventPublisher` | 事件投递 | HTTP webhook、MQ（Kafka/RocketMQ）、日志 |

HTTP 层、可视化设计器、多租户字段隔离、ES 查询、分布式调度属于 PRD 第三/第四阶段，本实现通过上面的接口预留，不含具体实现（见 `docs/PRD-mapping.md` 的"未实现"一节）。

## 7. 测试

```bash
ctest --test-dir build --output-on-failure
./build/wf-tests          # 直接运行，打印每个用例
```

覆盖：JSON 解析/序列化/路径/合并、表达式与函数、脚本语言、状态机流转、时长解析、DSL 解析与发布前校验（缺开始节点/孤立节点/循环/条件缺失/并行不汇聚/时长非法等）、仓储 CRUD 与乐观锁、快照往返、PRD 报销流程端到端、会签/或签/比例/依次、转办/委派/前加签/后加签、拒绝三种策略、退回、撤回、并行分叉汇聚、子流程变量回写、等待事件、定时节点、超时催办与升级、服务重试与失败重试、脚本驱动网关、表单权限、事件 outbox 退避重试、挂起/恢复、幂等与乐观锁、取消/终止/跳转、统计与查询。

## 8. 当前实现的边界

- **单进程**：没有分布式调度、没有跨进程锁；`InstanceLock` 只保证同一实例在进程内串行推进（PRD §23 的乐观锁字段已就位，可直接用于多实例部署）。
- **无 HTTP 层**：`api.hpp` 提供与 PRD §18 一致的调用与响应体，HTTP 适配器（httplib/libevent 等）未包含。
- **脚本语言是自定义子集**，不是 Groovy/JS；需要复杂脚本时把脚本节点换成 `SERVICE_TASK` 调外部脚本服务。
- **持久化是全量 JSON 快照**，没有按表建模的增量写入；换 MySQL 时实现 `Repository` 即可，引擎无需改动。

# PRD → 代码映射

`PRD.md`（2559 行）是本实现的需求来源。下表把 PRD 章节映射到具体文件/符号/测试，便于评审和后续迭代。状态说明：**已实现**（可用且被测试覆盖）、**部分实现**（核心语义在，外围能力简化）、**未实现**（需要基础设施或属于第三/四阶段）。

## 1. 设计目标与原则

| PRD | 状态 | 落点 |
|---|---|---|
| §1 设计目标 | 部分实现 | 定义/执行/任务/规则/集成/可观测/可靠性 7 类能力见 `README.md` §2；分布式与多租户未实现 |
| §2.2 业务编排 | 已实现 | `ServiceTask` + `SubProcess` + `WaitTask` + 重试（`src/engine.cpp:executeNode`） |
| §2.3 混合流程 | 已实现 | `wf-cli demo` 走通"人工 + 自动 + 外部系统" |
| §3.1 定义与实例分离 | 已实现 | `ProcessDefinition` / `ProcessInstance`（`include/wf/model.hpp`、`entity.hpp`），实例绑定 `definitionVersion` |
| §3.2 状态驱动 | 已实现 | `types.hpp` 的 `canTransit()` × 3 套状态机，`finishInstance/completeNode/updateTask` 全部走校验 |
| §3.3 执行可追溯 | 已实现 | `ProcessEngine::recordHistory()` + `HistoryRecord`（谁/何时/节点/动作/输入输出/traceId） |
| §3.4 幂等 | 已实现 | businessKey 去重（`startProcess`）、任务乐观锁（`Repository::updateTask`）、事件 eventId 状态（`dispatchEvents`） |
| §3.5 异步化 | 已实现 | 事件 outbox（`publishEvent` → `dispatchEvents`），外部调用不在主流程内重试阻塞 |
| §3.6 可扩展节点 | 已实现 | `NodeType` 枚举 + `executeNode` 分发；新增类型只需加 case |

## 2. 核心模型与节点

| PRD | 状态 | 落点 |
|---|---|---|
| §4.1/§4.2 概念与定义模型 | 已实现 | `ProcessDefinition`、`NodeDefinition`、`EdgeDefinition`（`src/model.cpp:fromJson`） |
| §4.3.1/4.3.2 开始/结束节点 | 已实现 | `NodeType::Start` / `End` |
| §4.3.3 人工任务节点 | 已实现 | `NodeType::UserTask` + `makeTask` + `completeNodeAfterTask` |
| §4.3.4 自动任务节点 | 已实现 | `NodeType::ServiceTask`（`ServiceInvoker` 接口） |
| §4.3.5 条件网关 | 已实现 | `nextNodes()` 按优先级求值，`default` 兜底，无匹配报错 |
| §4.3.6 并行网关 | 已实现 | `PARALLEL_GATEWAY` + `wf_parallel_branch` 到达计数（`allBranchesArrived`） |
| §4.3.7 子流程节点 | 已实现 | `SUB_PROCESS` 同步/异步 + `notifyParent` + 输出变量回写 |
| §4.3.8 等待节点 | 已实现 | `WAIT_TASK` + `signalEvent(eventKey, payload)` |
| §4.3.9 定时节点 | 已实现 | `TIMER_TASK` + `wf_timer_job`（`fireTimer`） |
| §4.3.10 消息节点 | 部分实现 | `MESSAGE_TASK` + `Notifier` 接口；渠道实现（邮件/短信/IM）留待接入 |

## 3. 执行模型

| PRD | 状态 | 落点 |
|---|---|---|
| §6 流程/节点/任务状态 | 已实现 | `types.hpp` 枚举 + `toString/parse*` |
| §7.1 流程启动 | 已实现 | `ProcessEngine::startProcess` |
| §7.2 节点执行 | 已实现 | `executeNode` / `completeNode` / `failNode` / `resumeNodeInstance` |
| §7.3 人工审批 | 已实现 | `makeTask` + `completeTask` |
| §7.4 条件分支 | 已实现 | `nextNodes` + `expr::evaluateCondition` |
| §7.5 并行分支 | 已实现 | 令牌队列 `drainQueue` + 分支到达记录 |
| §8 JSON DSL | 已实现 | `ProcessDefinition::fromJson`；示例 `examples/*.json` |
| §9 审批人规则 | 已实现 | `ApprovalRule` + `resolveRule()`（10 种规则 + 组合） |
| §10 审批操作 | 已实现 | `claimTask/completeTask/transferTask/delegateTask/addSignTask/returnTask/withdrawInstance` |
| §11 会签/或签/比例/依次 | 已实现 | `CompleteStrategy` + `completeNodeAfterTask` 统计口径 |
| §12 超时策略 | 已实现 | `TimeoutPolicy` + `scheduleTaskTimeout/scheduleNodeTimeout/applyTimeoutSteps` |
| §13 表达式引擎 | 已实现 | `include/wf/expression.hpp`（自研解析器，沙箱）；技术选型改为内置实现 |
| §13.3 技术选型 | 有意偏离 | PRD 建议 Aviator/SpEL/Groovy；这里自研以保证零依赖与沙箱安全 |
| §14 表单与变量 | 部分实现 | 变量 `json::Value`、字段权限 `FieldPermission` + `filterAndValidateForm`；表单定义/版本/字典未建模 |
| §15 事件与监听器 | 已实现 | `publishEvent` + `ListenerSpec`（HTTP/MQ/LOG）+ `dispatchEvents` |
| §16 自动任务节点 | 部分实现 | HTTP/RPC 走 `ServiceInvoker` 接口（内置 mock）；脚本节点为自研 wfscript |

## 4. 数据模型与接口

| PRD | 状态 | 落点 |
|---|---|---|
| §17.1 流程定义表 | 已实现（内存/JSON） | `ProcessDefinition` + `Repository::saveDefinition` |
| §17.2 流程实例表 | 已实现 | `ProcessInstance`（含 `version` 乐观锁字段） |
| §17.3 节点实例表 | 已实现 | `NodeInstance`（input/output/error/attempt） |
| §17.4 任务表 | 已实现 | `Task`（candidate/version/dueTime/parentTaskId） |
| §17.5 流程轨迹表 | 已实现 | `HistoryRecord` |
| §17.6 事件记录表 | 已实现 | `EventRecord`（retryCount/nextRetryTime/status） |
| §17.7 定时任务表 | 已实现 | `TimerJob`（NODE_TIMER/NODE_TIMEOUT/TASK_TIMEOUT） |
| §18.1 流程定义 API | 已实现（进程内） | `ProcessDefinitionService`（validate/create/publish/list/disable） |
| §18.2 流程实例 API | 已实现（进程内） | `ProcessInstanceService`（start/get/cancel/suspend/resume/terminate） |
| §18.3 任务 API | 已实现（进程内） | `TaskService`（todo/done/claim/complete/transfer/delegate/add-sign/return） |
| §18.4 轨迹 API | 已实现 | `ProcessInstanceService::histories` + `wf-cli history <id>` |
| §19 节点执行器设计 | 已实现 | `executeNode` 的 switch 即执行器注册表（C++ 静态分发） |

## 5. 可靠性、并发、可观测

| PRD | 状态 | 落点 |
|---|---|---|
| §20 状态机 | 已实现 | `canTransit` + 每次状态写入前的校验 |
| §21.1 本地事务边界 | 部分实现 | 单进程内一次流转的顺序写入（实例/节点/任务/轨迹/事件）；无真正 DB 事务 |
| §21.2 外部调用异步化 | 已实现 | 事件表 + 重试；`SERVICE_RETRY` 定时任务记录退避 |
| §22.1 启动幂等 | 已实现 | `findInstanceByBusinessKey` |
| §22.2 任务办理幂等 | 已实现 | `updateTask(..., expectedVersion)` |
| §22.3 事件消费幂等 | 已实现 | `EventRecord::eventId` + `status` 状态机 |
| §23 并发控制 | 部分实现 | 实例级串行锁 + 乐观锁版本；Redis 分布式锁未实现 |
| §24 并行汇聚 | 已实现 | `ParallelBranch` 到达/完成状态 + 令牌计数 |
| §25 多租户 | 部分实现 | 实体与查询均带 `tenantId`（字段隔离），Schema/独立库未实现 |
| §26 权限 | 部分实现 | 任务办理人校验、候选人/角色校验、字段权限；引擎级 RBAC 与数据权限未实现 |
| §27 可观测性 | 部分实现 | `statistics()` 指标 + 结构化事件 JSON；Prometheus/OTel 接入点未提供 |
| §28 性能容量 | 未实现 | 无索引/分片/ES/时间轮优化（内存实现，逻辑正确性优先） |
| §29.1 服务无状态化 | 部分实现 | 引擎无内部状态，状态全在 `Repository`；可替换为外部存储 |
| §29.2 分布式调度 | 未实现 | 单进程 `tick()`；需要 XXL-JOB/Redis 调度时由外部驱动 |
| §29.3 重试机制 | 已实现 | `EngineOptions::retryBackoffMillis`（0/10s/1m/5m/30m）+ 超限转 FAILED |
| §30 安全设计 | 已实现 | 表达式无 IO/反射；脚本沙箱无 IO/无循环/限制语句集；无危险调用 |
| §31 技术选型 | 有意偏离 | 按用户要求用 C++17 标准库实现，替代 Java/Spring 栈 |
| §32.1/32.2 与业务系统集成 | 已实现 | `startProcess` 入参 + `PROCESS_COMPLETED` 回调（HTTP/MQ 监听器） |
| §33 管理后台 | 部分实现 | CLI 提供实例/任务/异常/统计能力；无 Web 界面 |

## 6. 明确未实现（需基础设施或属于第三/四阶段）

- **HTTP 服务层**：`api.hpp` 已按 §18 语义封装，未引入 httplib/Boost 等依赖；接一层 adaptor 即可暴露 REST。
- **数据库/缓存/消息/搜索**：MySQL、Redis、Kafka/RocketMQ、Elasticsearch 全部通过接口预留（`Repository` / `EventPublisher` / `ServiceInvoker`），无具体实现。
- **分布式能力**：多实例部署、分布式锁、分布式调度、分库分表。
- **流程设计器**：可视化拖拽、流程图高亮、导入导出（PRD §34）。
- **流程模板市场、报表体系、审计合规**：PRD 第四阶段。
- **会签比例的高级形态**（如一票否决、加签层级嵌套）与 **表单引擎**（表单定义/版本/校验规则 DSL）。

## 7. 与 PRD 伪代码的对应

PRD 用 Java 伪代码描述了三处关键行为，本实现的对应物：

| PRD 伪代码 | 本实现 |
|---|---|
| §7.1 `startProcess(...)` | `ProcessEngine::startProcess(const StartRequest&)` |
| §11.4 `enum CompleteStrategy` | `wf::CompleteStrategy` + `completeNodeAfterTask` 中的统计与判定 |
| §19.1 `interface NodeExecutor` | `executeNode()` 中按 `NodeType` 分发的 switch（编译期注册，避免虚函数表开销） |
| §19.2 `NodeExecutionContext` | `expressionContext()`（PRD §13.2 的上下文变量） |
| §19.3 `NodeExecutionResult` | `completeNode` / `failNode` / park（等待）三条返回路径 + `NodeInstance.status` |
| §22.2 `UPDATE ... WHERE version = ?` | `Repository::updateTask(task, expectedVersion)` |

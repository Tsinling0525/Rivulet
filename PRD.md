下面给出一份**企业级工作流流程引擎设计方案**，可用于审批流、业务编排、任务调度、跨系统流程协同等场景。方案按“业务目标、核心模型、引擎架构、执行机制、数据模型、接口设计、可靠性、扩展性、落地路线”展开，可直接作为技术评审初稿。

---

# 工作流流程引擎设计方案

## 1. 设计目标

构建一个通用的、可扩展的工作流流程引擎，支持：

1. **流程定义管理**
   - 可视化设计流程
   - 支持流程版本管理
   - 支持流程发布、草稿、停用

2. **流程实例执行**
   - 支持发起流程
   - 支持流程推进、驳回、转办、委派、加签、撤回
   - 支持自动节点、人工节点、条件分支、并行分支、子流程

3. **任务管理**
   - 待办任务
   - 已办任务
   - 任务认领、办理、转交、代理、加签

4. **规则与条件**
   - 条件表达式
   - 审批人规则
   - 表单权限控制
   - 超时策略

5. **集成能力**
   - 调用外部服务
   - 消息事件
   - Webhook
   - MQ 异步任务

6. **可观测性**
   - 流程轨迹
   - 节点执行日志
   - 异常告警
   - 指标监控

7. **高可靠与可扩展**
   - 支持分布式部署
   - 支持幂等执行
   - 支持高并发流程推进
   - 支持多租户隔离

---

# 2. 适用场景

该引擎可覆盖以下典型场景：

## 2.1 审批流场景

例如：

- 请假审批
- 报销审批
- 采购审批
- 合同审批
- 用章审批
- 权限申请审批

特点：

- 人工任务多
- 审批规则复杂
- 存在加签、转办、退回、撤回等操作
- 对流程轨迹和权限要求高

## 2.2 业务编排场景

例如：

- 订单履约流程
- 开户流程
- 风控审核流程
- 会员开通流程
- 资源交付流程

特点：

- 自动节点多
- 需要调用外部服务
- 需要异步等待
- 需要补偿、重试、超时控制

## 2.3 混合流程

例如：

1. 用户提交申请
2. 系统自动校验
3. 主管审批
4. 风控系统审核
5. 财务复核
6. 自动开通权限
7. 发送通知

这种“人工 + 自动 + 外部系统”的混合流程是流程引擎的核心目标。

---

# 3. 核心设计原则

## 3.1 流程定义与流程实例分离

- 流程定义：描述流程“应该怎么跑”
- 流程实例：描述某一次流程“实际怎么跑”

流程定义可以发布多个版本，不同流程实例绑定具体版本运行。

## 3.2 状态驱动

流程引擎本质上是一个状态机系统：

- 流程实例有状态
- 节点实例有状态
- 任务有状态
- 流转动作触发状态变化

## 3.3 执行可追溯

每一次流程变化都必须记录：

- 谁操作
- 什么时间
- 在哪个节点
- 做了什么动作
- 输入输出是什么
- 流转到哪里

## 3.4 幂等设计

流程推进、任务办理、事件回调、定时任务都必须支持幂等，避免重复执行。

## 3.5 异步化设计

对于耗时操作、外部调用、消息通知等，优先异步执行，提高吞吐量和稳定性。

## 3.6 可扩展节点类型

节点类型应可插拔扩展，例如：

- 人工审批节点
- 自动服务节点
- 脚本节点
- 消息节点
- 等待节点
- 子流程节点
- 条件网关
- 并行网关

---

# 4. 核心概念模型

## 4.1 基本术语

| 概念 | 说明 |
|---|---|
| 流程定义 | 一个流程的模板 |
| 流程版本 | 同一流程定义的不同版本 |
| 流程实例 | 一次具体执行的流程 |
| 节点定义 | 流程模板中的一个环节 |
| 节点实例 | 流程执行过程中某个节点的实际执行记录 |
| 连线 | 节点之间的流转关系 |
| 任务 | 人工节点产生的待办事项 |
| 表单 | 流程流转中携带的数据 |
| 表达式 | 条件判断、审批人计算等规则 |
| 事件 | 节点开始、结束、超时、异常等触发点 |

---

## 4.2 流程定义模型

流程定义由以下部分组成：

```text
ProcessDefinition
├── id
├── code
├── name
├── version
├── status
├── nodes
│   ├── StartNode
│   ├── UserTaskNode
│   ├── ServiceTaskNode
│   ├── GatewayNode
│   ├── EndNode
├── edges
│   ├── sourceNodeId
│   ├── targetNodeId
│   └── condition
├── variables
└── config
```

---

## 4.3 节点类型设计

### 4.3.1 开始节点

流程入口，一个流程通常只有一个开始节点。

### 4.3.2 结束节点

流程结束节点，可以有多个。

### 4.3.3 人工任务节点

需要用户处理，例如：

- 审批
- 填写信息
- 确认
- 复核

支持操作：

- 同意
- 拒绝
- 退回
- 转办
- 委派
- 加签
- 撤回

### 4.3.4 自动任务节点

系统自动执行，例如：

- 调用 HTTP 接口
- 调用 RPC 服务
- 执行脚本
- 更新业务状态
- 发送消息

### 4.3.5 条件网关节点

根据表达式选择分支。

例如：

```text
金额 <= 10000 -> 直属主管审批
金额 > 10000 -> 部门经理审批
```

### 4.3.6 并行网关节点

支持多个分支同时执行。

例如：

```text
提交后同时进入：
- 财务审批
- 法务审批
- 风控审批
```

所有分支完成后，通过汇聚节点继续向下执行。

### 4.3.7 子流程节点

调用另一个流程。

支持：

- 同步子流程
- 异步子流程
- 父子流程变量传递
- 子流程结束后回调父流程

### 4.3.8 等待节点

等待外部事件或定时触发。

例如：

- 等待支付回调
- 等待用户确认
- 等待第三方审核结果
- 等待指定时间

### 4.3.9 定时节点

支持：

- 固定延迟
- Cron 表达式
- 指定日期时间
- 节点超时策略

### 4.3.10 消息节点

发送通知：

- 站内信
- 邮件
- 短信
- 企业微信
- 钉钉
- 飞书
- Webhook

---

# 5. 引擎总体架构

## 5.1 架构分层

```text
┌────────────────────────────────────────────┐
│                  接入层                     │
│  流程设计器 / 管理后台 / 业务系统 / OpenAPI   |
└────────────────────────────────────────────┘
                     │
┌────────────────────────────────────────────┐
│                 API 层                      │
│  流程定义服务 / 流程实例服务 / 任务服务 / 查询服务 |
└────────────────────────────────────────────┘
                     │
┌────────────────────────────────────────────┐
│                流程引擎核心                   │
│  解析器 / 状态机 / 执行器 / 规则引擎 / 事件引擎  |
└────────────────────────────────────────────┘
                     │
┌────────────────────────────────────────────┐
│                 支撑服务                     │
│  用户组织服务 / 消息服务 / 定时调度 / 文件服务    |
└────────────────────────────────────────────┘
                     │
┌────────────────────────────────────────────┐
│                 基础设施                     │
│  MySQL / Redis / MQ / ES / 对象存储 / 监控    |
└────────────────────────────────────────────┘
```

---

## 5.2 核心模块

### 5.2.1 流程设计器

提供可视化拖拽能力：

- 拖拽节点
- 配置连线条件
- 配置审批人
- 配置表单权限
- 配置超时策略
- 配置消息通知
- 校验流程合法性

### 5.2.2 流程定义服务

负责：

- 创建流程
- 编辑流程
- 发布流程
- 版本管理
- 启停流程
- 校验流程

### 5.2.3 流程执行引擎

核心模块，负责：

- 启动流程实例
- 节点流转
- 条件判断
- 节点执行
- 状态维护
- 异常处理
- 事件触发

### 5.2.4 任务服务

负责：

- 创建任务
- 分配任务
- 办理任务
- 转办任务
- 加签任务
- 任务查询
- 待办/已办查询

### 5.2.5 规则引擎

负责：

- 条件分支表达式
- 审批人计算
- 表单权限控制
- 节点跳过规则
- 自动完成规则

### 5.2.6 事件引擎

负责：

- 节点开始事件
- 节点结束事件
- 流程开始事件
- 流程结束事件
- 任务创建事件
- 任务完成事件
- 超时事件
- 异常事件

### 5.2.7 调度服务

负责：

- 定时任务
- 节点超时检测
- 延时任务
- 异步任务重试

### 5.2.8 查询服务

负责：

- 流程实例查询
- 任务查询
- 流程轨迹查询
- 操作日志查询
- 统计报表

---

# 6. 流程执行模型设计

## 6.1 流程状态

流程实例状态：

```text
RUNNING      运行中
SUSPENDED    挂起
COMPLETED    已完成
TERMINATED   已终止
CANCELLED    已取消
ERROR        异常
```

## 6.2 节点状态

节点实例状态：

```text
PENDING      待执行
RUNNING      执行中
COMPLETED    已完成
SKIPPED      已跳过
FAILED       执行失败
CANCELLED    已取消
WAITING      等待中
```

## 6.3 任务状态

人工任务状态：

```text
PENDING      待处理
CLAIMED      已认领
COMPLETED    已办理
TRANSFERRED  已转办
DELEGATED    已委派
CANCELLED    已取消
```

---

# 7. 核心执行流程

## 7.1 流程启动流程

```text
业务系统发起流程
    ↓
校验流程定义是否有效
    ↓
创建流程实例
    ↓
初始化流程变量
    ↓
进入开始节点
    ↓
根据连线找到下一节点
    ↓
执行下一节点
    ↓
记录流程轨迹
```

伪代码：

```java
public ProcessInstance startProcess(String processCode, Map<String, Object> variables) {

    ProcessDefinition definition = processDefinitionService.getActiveVersion(processCode);

    ProcessInstance instance = new ProcessInstance();
    instance.setProcessDefinitionId(definition.getId());
    instance.setBusinessKey(variables.get("businessKey"));
    instance.setStatus(ProcessStatus.RUNNING);
    instance.setVariables(variables);

    processInstanceRepository.save(instance);

    eventPublisher.publish(new ProcessStartedEvent(instance));

    executeNextNodes(instance, definition.getStartNodeId());

    return instance;
}
```

---

## 7.2 节点执行流程

```text
获取节点定义
    ↓
创建节点实例
    ↓
判断节点类型
    ↓
如果是人工节点：
    创建任务
    等待用户处理
    ↓
如果是自动节点：
    执行服务调用/脚本
    判断成功失败
    ↓
如果是网关节点：
    计算条件
    选择分支
    ↓
节点完成
    ↓
根据连线继续流转
```

---

## 7.3 人工审批流程

```text
进入人工节点
    ↓
计算审批人
    ↓
创建任务
    ↓
发送待办通知
    ↓
用户办理任务
    ↓
记录审批意见
    ↓
更新节点状态
    ↓
流程继续流转
```

---

## 7.4 条件分支执行流程

```text
获取当前节点所有出线
    ↓
依次计算条件表达式
    ↓
选择满足条件的分支
    ↓
如果没有满足条件：
    走默认分支或抛异常
```

示例：

```text
${amount > 10000} -> 总监审批
${amount <= 10000} -> 经理审批
```

---

## 7.5 并行分支执行流程

```text
进入并行分支节点
    ↓
同时创建多个分支节点实例
    ↓
各分支独立执行
    ↓
每个分支完成后记录完成状态
    ↓
汇聚节点判断所有分支是否完成
    ↓
全部完成后继续向后执行
```

需要记录：

- 并行分支数量
- 已完成分支数量
- 分支执行上下文
- 汇聚条件

---

# 8. 流程定义 DSL 设计

为了便于存储和解析，流程定义可以使用 JSON 描述。

## 8.1 示例流程

假设流程如下：

```text
开始
  ↓
提交申请
  ↓
金额判断
  ├── 金额 <= 10000 -> 主管审批
  └── 金额 > 10000 -> 总监审批
  ↓
自动通知
  ↓
结束
```

## 8.2 JSON DSL 示例

```json
{
  "processCode": "expense_approval",
  "processName": "报销审批流程",
  "version": 1,
  "nodes": [
    {
      "id": "start",
      "type": "START",
      "name": "开始"
    },
    {
      "id": "submit",
      "type": "USER_TASK",
      "name": "提交申请",
      "assignee": {
        "type": "INITIATOR"
      }
    },
    {
      "id": "amountGateway",
      "type": "EXCLUSIVE_GATEWAY",
      "name": "金额判断"
    },
    {
      "id": "managerApproval",
      "type": "USER_TASK",
      "name": "主管审批",
      "assignee": {
        "type": "ROLE",
        "value": "MANAGER"
      }
    },
    {
      "id": "directorApproval",
      "type": "USER_TASK",
      "name": "总监审批",
      "assignee": {
        "type": "ROLE",
        "value": "DIRECTOR"
      }
    },
    {
      "id": "notify",
      "type": "SERVICE_TASK",
      "name": "发送通知",
      "service": {
        "type": "HTTP",
        "url": "http://message-service/send"
      }
    },
    {
      "id": "end",
      "type": "END",
      "name": "结束"
    }
  ],
  "edges": [
    {
      "id": "edge-1",
      "source": "start",
      "target": "submit"
    },
    {
      "id": "edge-2",
      "source": "submit",
      "target": "amountGateway"
    },
    {
      "id": "edge-3",
      "source": "amountGateway",
      "target": "managerApproval",
      "condition": "${amount <= 10000}"
    },
    {
      "id": "edge-4",
      "source": "amountGateway",
      "target": "directorApproval",
      "condition": "${amount > 10000}"
    },
    {
      "id": "edge-5",
      "source": "managerApproval",
      "target": "notify"
    },
    {
      "id": "edge-6",
      "source": "directorApproval",
      "target": "notify"
    },
    {
      "id": "edge-7",
      "source": "notify",
      "target": "end"
    }
  ]
}
```

---

# 9. 审批人规则设计

审批人规则是流程引擎的关键能力之一。

## 9.1 常见审批人类型

| 类型 | 说明 |
|---|---|
| 发起人 | 当前流程发起人 |
| 指定用户 | 固定用户 |
| 指定角色 | 拥有某角色的用户 |
| 部门主管 | 发起人或上一节点处理人的主管 |
| 岗位 | 某个岗位的人 |
| 表达式 | 根据变量动态计算 |
| 上一节点处理人 | 继承上一节点处理人 |
| 候选人 | 多人可处理 |
| 组合规则 | 多种规则组合 |

## 9.2 审批人模型

```json
{
  "type": "LEADER",
  "level": 2
}
```

表示：发起人二级主管。

示例：

```json
{
  "type": "ROLE",
  "value": "FINANCE_MANAGER"
}
```

表示：财务经理角色。

示例：

```json
{
  "type": "EXPRESSION",
  "value": "${deptManager}"
}
```

表示：从流程变量中取 `deptManager`。

---

# 10. 常见审批操作设计

## 10.1 同意

当前审批人同意，流程继续向下流转。

## 10.2 拒绝

当前审批人拒绝，流程可能：

- 直接结束
- 退回发起人
- 退回上一节点
- 跳到指定节点
- 进入异常分支

## 10.3 退回

支持：

- 退回到发起人
- 退回到上一节点
- 退回到任意历史节点
- 退回后重新提交

## 10.4 转办

当前任务转给其他人处理。

转办后：

- 原审批人任务结束
- 新审批人任务生成
- 轨迹记录转办

## 10.5 委派

当前任务交给其他人处理，但责任仍属于原审批人。

例如：

```text
A 委派给 B
B 处理后任务回到 A
A 再最终审批
```

## 10.6 加签

在当前节点增加审批人。

支持：

- 前加签
- 后加签
- 并签
- 或签
- 依次审批

## 10.7 撤回

流程发起人在允许条件下撤回流程。

撤回条件：

- 下一节点未处理
- 未进入不可逆阶段
- 配置允许撤回

---

# 11. 会签/或签设计

## 11.1 会签

多个审批人都同意，节点才通过。

例如：

```text
A、B、C 三人都同意，才进入下一节点。
```

## 11.2 或签

任意一个审批人处理，节点即完成。

例如：

```text
A、B、C 中任意一人同意，即可进入下一节点。
```

## 11.3 比例通过

支持规则：

```text
超过 2/3 审批人同意则通过
```

## 11.4 设计方式

节点实例下创建多个任务：

```text
NodeInstance
├── Task A
├── Task B
└── Task C
```

节点完成策略：

```java
public enum CompleteStrategy {
    ALL,          // 所有人完成
    ANY,          // 任意一人完成
    RATIO,        // 按比例完成
    SEQUENCE      // 依次完成
}
```

---

# 12. 超时策略设计

## 12.1 节点超时

节点停留超过指定时间，可以触发：

- 提醒
- 自动转办
- 自动通过
- 自动拒绝
- 自动委派
- 进入异常分支
- 终止流程

## 12.2 任务超时

任务分配后超过时间未处理：

- 催办
- 升级审批
- 转交上级
- 自动完成

## 12.3 超时配置示例

```json
{
  "timeout": {
    "duration": "PT24H",
    "actions": [
      {
        "type": "REMIND",
        "channel": "EMAIL"
      },
      {
        "type": "ESCALATE",
        "target": "LEADER"
      }
    ]
  }
}
```

---

# 13. 表达式引擎设计

流程引擎需要表达式能力，用于：

- 条件分支
- 审批人计算
- 表单权限
- 自动任务参数映射
- 节点完成条件

## 13.1 表达式示例

```text
${amount > 10000}
```

```text
${form.type == 'travel' && form.days > 3}
```

```text
${user.departmentId == 'FINANCE'}
```

## 13.2 表达式上下文

表达式执行时可注入：

```text
processInstance
variables
initiator
currentNode
form
businessData
organizationService
```

## 13.3 技术选型

可选：

- SpEL
- Aviator
- Groovy
- JUEL
- MVEL

建议：

- 条件表达式使用 Aviator / SpEL
- 复杂脚本节点使用 Groovy
- 严格限制脚本权限，避免执行危险操作

---

# 14. 表单与变量设计

流程运行过程中会携带变量和表单数据。

## 14.1 流程变量

流程变量用于：

- 条件判断
- 审批人计算
- 服务调用参数
- 节点行为控制

示例：

```json
{
  "amount": 12000,
  "type": "travel",
  "deptId": "D1001",
  "initiator": "U10086"
}
```

## 14.2 表单数据

表单数据通常包括：

- 表单定义
- 表单版本
- 表单字段
- 字段权限
- 字段校验规则

## 14.3 节点表单权限

不同节点字段权限不同：

| 节点 | 字段权限 |
|---|---|
| 发起人提交 | 可编辑 |
| 主管审批 | 只读 |
| 财务审批 | 金额可编辑 |
| 抄送节点 | 只读 |

字段权限模型：

```json
{
  "field": "amount",
  "visible": true,
  "editable": false,
  "required": false
}
```

---

# 15. 事件与监听器设计

流程引擎应在关键节点发布事件。

## 15.1 事件类型

```text
PROCESS_STARTED
PROCESS_COMPLETED
PROCESS_CANCELLED
NODE_STARTED
NODE_COMPLETED
NODE_FAILED
TASK_CREATED
TASK_ASSIGNED
TASK_COMPLETED
TASK_TRANSFERRED
TASK_TIMEOUT
```

## 15.2 监听器配置

```json
{
  "events": [
    {
      "type": "TASK_CREATED",
      "listener": {
        "type": "HTTP",
        "url": "http://notification-service/task-created"
      }
    },
    {
      "type": "PROCESS_COMPLETED",
      "listener": {
        "type": "MQ",
        "topic": "process.completed"
      }
    }
  ]
}
```

## 15.3 事件用途

- 发送消息通知
- 更新业务状态
- 同步待办
- 触发下游系统
- 写审计日志
- 数据统计

---

# 16. 自动任务节点设计

自动任务节点用于执行系统逻辑。

## 16.1 HTTP 节点

```json
{
  "type": "SERVICE_TASK",
  "name": "调用风控服务",
  "service": {
    "type": "HTTP",
    "method": "POST",
    "url": "http://risk-service/check",
    "headers": {
      "Content-Type": "application/json"
    },
    "body": {
      "userId": "${initiator}",
      "amount": "${amount}"
    },
    "timeout": 3000,
    "retry": {
      "maxAttempts": 3,
      "backoff": 1000
    }
  }
}
```

## 16.2 RPC 节点

适用于内部微服务：

```json
{
  "type": "SERVICE_TASK",
  "service": {
    "type": "RPC",
    "serviceName": "com.example.OrderService",
    "methodName": "confirmOrder",
    "params": [
      "${orderId}"
    ]
  }
}
```

## 16.3 脚本节点

```json
{
  "type": "SCRIPT_TASK",
  "script": {
    "language": "groovy",
    "content": "if (variables.amount > 10000) { variables.level = 'HIGH' }"
  }
}
```

---

# 17. 数据模型设计

以下是核心数据库表设计。

---

## 17.1 流程定义表

```sql
CREATE TABLE wf_process_definition (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    process_code VARCHAR(64) NOT NULL,
    process_name VARCHAR(128) NOT NULL,
    version INT NOT NULL,
    status VARCHAR(32) NOT NULL,
    definition_json TEXT NOT NULL,
    description VARCHAR(512),
    created_by VARCHAR(64),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_process_version (process_code, version)
);
```

---

## 17.2 流程实例表

```sql
CREATE TABLE wf_process_instance (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    process_definition_id BIGINT NOT NULL,
    process_code VARCHAR(64) NOT NULL,
    business_key VARCHAR(128),
    status VARCHAR(32) NOT NULL,
    initiator VARCHAR(64) NOT NULL,
    current_node_id VARCHAR(64),
    variables TEXT,
    start_time DATETIME NOT NULL,
    end_time DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_business_key (business_key),
    INDEX idx_status (status),
    INDEX idx_initiator (initiator)
);
```

---

## 17.3 节点实例表

```sql
CREATE TABLE wf_node_instance (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    process_instance_id BIGINT NOT NULL,
    node_id VARCHAR(64) NOT NULL,
    node_name VARCHAR(128),
    node_type VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    input_data TEXT,
    output_data TEXT,
    error_message TEXT,
    start_time DATETIME,
    end_time DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_process_instance (process_instance_id)
);
```

---

## 17.4 任务表

```sql
CREATE TABLE wf_task (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    process_instance_id BIGINT NOT NULL,
    node_instance_id BIGINT NOT NULL,
    task_name VARCHAR(128),
    assignee VARCHAR(64),
    candidate_users TEXT,
    candidate_roles TEXT,
    status VARCHAR(32) NOT NULL,
    priority INT DEFAULT 0,
    due_time DATETIME,
    claim_time DATETIME,
    complete_time DATETIME,
    comment TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_assignee_status (assignee, status),
    INDEX idx_process_instance (process_instance_id)
);
```

---

## 17.5 流程轨迹表

```sql
CREATE TABLE wf_process_history (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    process_instance_id BIGINT NOT NULL,
    node_id VARCHAR(64),
    node_name VARCHAR(128),
    action VARCHAR(64),
    operator VARCHAR(64),
    comment TEXT,
    input_data TEXT,
    output_data TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_process_instance (process_instance_id)
);
```

---

## 17.6 事件记录表

```sql
CREATE TABLE wf_event_record (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    process_instance_id BIGINT,
    node_instance_id BIGINT,
    task_id BIGINT,
    event_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    payload TEXT,
    retry_count INT DEFAULT 0,
    next_retry_time DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_status_retry (status, next_retry_time)
);
```

---

## 17.7 定时任务表

```sql
CREATE TABLE wf_timer_job (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    process_instance_id BIGINT,
    node_instance_id BIGINT,
    task_id BIGINT,
    job_type VARCHAR(64) NOT NULL,
    trigger_time DATETIME NOT NULL,
    status VARCHAR(32) NOT NULL,
    payload TEXT,
    retry_count INT DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_trigger_time_status (trigger_time, status)
);
```

---

# 18. API 设计

## 18.1 流程定义 API

### 创建流程定义

```http
POST /api/process-definitions
```

请求体：

```json
{
  "processCode": "expense_approval",
  "processName": "报销审批流程",
  "definition": {}
}
```

### 发布流程定义

```http
POST /api/process-definitions/{id}/publish
```

### 查询流程定义列表

```http
GET /api/process-definitions?processCode=expense_approval
```

---

## 18.2 流程实例 API

### 发起流程

```http
POST /api/process-instances
```

请求体：

```json
{
  "processCode": "expense_approval",
  "businessKey": "EXPENSE-20260917-0001",
  "variables": {
    "amount": 12000,
    "type": "travel",
    "initiator": "U10086"
  }
}
```

### 查询流程实例

```http
GET /api/process-instances/{id}
```

### 取消流程

```http
POST /api/process-instances/{id}/cancel
```

### 挂起流程

```http
POST /api/process-instances/{id}/suspend
```

### 恢复流程

```http
POST /api/process-instances/{id}/resume
```

---

## 18.3 任务 API

### 查询我的待办

```http
GET /api/tasks/todo?assignee=U10086&page=1&size=20
```

### 查询我的已办

```http
GET /api/tasks/done?assignee=U10086&page=1&size=20
```

### 办理任务

```http
POST /api/tasks/{taskId}/complete
```

请求体：

```json
{
  "action": "APPROVE",
  "comment": "同意",
  "variables": {
    "financeCode": "F2026"
  }
}
```

### 转办任务

```http
POST /api/tasks/{taskId}/transfer
```

请求体：

```json
{
  "targetUser": "U10010",
  "comment": "转交财务处理"
}
```

### 委派任务

```http
POST /api/tasks/{taskId}/delegate
```

### 加签

```http
POST /api/tasks/{taskId}/add-sign
```

请求体：

```json
{
  "users": ["U10011", "U10012"],
  "type": "BEFORE",
  "mode": "ALL"
}
```

---

## 18.4 流程轨迹 API

```http
GET /api/process-instances/{id}/histories
```

返回：

```json
[
  {
    "nodeName": "开始",
    "action": "START",
    "operator": "U10086",
    "time": "2026-09-17 10:00:00"
  },
  {
    "nodeName": "主管审批",
    "action": "APPROVE",
    "operator": "U20001",
    "comment": "同意",
    "time": "2026-09-17 10:30:00"
  }
]
```

---

# 19. 流程引擎执行器设计

## 19.1 节点执行器接口

```java
public interface NodeExecutor {

    NodeType supportType();

    NodeExecutionResult execute(NodeExecutionContext context);
}
```

## 19.2 节点执行上下文

```java
public class NodeExecutionContext {
    private ProcessInstance processInstance;
    private NodeInstance nodeInstance;
    private ProcessDefinition processDefinition;
    private NodeDefinition nodeDefinition;
    private Map<String, Object> variables;
}
```

## 19.3 节点执行结果

```java
public class NodeExecutionResult {
    private boolean completed;
    private boolean waiting;
    private boolean failed;
    private Map<String, Object> output;
    private String errorMessage;
}
```

---

# 20. 状态机设计

流程引擎的核心是状态机。

## 20.1 流程实例状态流转

```text
RUNNING -> COMPLETED
RUNNING -> CANCELLED
RUNNING -> TERMINATED
RUNNING -> SUSPENDED
RUNNING -> ERROR
SUSPENDED -> RUNNING
ERROR -> RUNNING
```

## 20.2 节点实例状态流转

```text
PENDING -> RUNNING
RUNNING -> COMPLETED
RUNNING -> FAILED
RUNNING -> WAITING
WAITING -> RUNNING
RUNNING -> CANCELLED
PENDING -> SKIPPED
```

## 20.3 任务状态流转

```text
PENDING -> CLAIMED
CLAIMED -> COMPLETED
PENDING -> COMPLETED
PENDING -> TRANSFERRED
TRANSFERRED -> PENDING
PENDING -> CANCELLED
```

---

# 21. 流程事务与一致性设计

流程引擎涉及多个操作：

- 更新流程实例
- 更新节点实例
- 创建任务
- 发送事件
- 调用外部服务

必须保证一致性。

## 21.1 本地事务边界

一次节点流转尽量在同一个本地事务内完成：

```text
更新流程实例状态
更新节点实例状态
创建下一节点实例
创建任务
写流程轨迹
写事件记录
```

外部调用不放在主事务中。

## 21.2 外部调用异步化

外部调用采用：

```text
事件表 + MQ + 重试
```

流程：

```text
主事务写事件记录
    ↓
提交事务
    ↓
发送 MQ
    ↓
消费者执行外部调用
    ↓
成功则标记完成
失败则重试
```

---

# 22. 幂等设计

## 22.1 启动流程幂等

使用 `businessKey` 防止重复发起：

```text
processCode + businessKey
```

唯一索引：

```sql
UNIQUE KEY uk_process_business (process_code, business_key)
```

## 22.2 任务办理幂等

办理任务时使用版本号：

```sql
UPDATE wf_task
SET status = 'COMPLETED', version = version + 1
WHERE id = #{taskId}
AND version = #{version};
```

如果影响行数为 0，说明已被处理。

## 22.3 事件消费幂等

使用：

```text
event_id
```

作为幂等键。

---

# 23. 并发控制设计

## 23.1 流程实例锁

同一流程实例同时被多个请求推进时，需要加锁。

方案一：数据库乐观锁

```sql
UPDATE wf_process_instance
SET status = #{status}, version = version + 1
WHERE id = #{id}
AND version = #{version};
```

方案二：Redis 分布式锁

```text
lock:process-instance:{instanceId}
```

方案三：数据库行锁

```sql
SELECT * FROM wf_process_instance WHERE id = ? FOR UPDATE;
```

建议：

- 一般场景：乐观锁
- 高并发节点办理：分布式锁 + 乐观锁
- 并行分支汇聚：使用计数字段 + 原子更新

---

# 24. 并行分支汇聚设计

并行分支完成时需要判断是否所有分支都完成。

## 24.1 分支执行表

```sql
CREATE TABLE wf_parallel_branch (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    process_instance_id BIGINT NOT NULL,
    gateway_node_id VARCHAR(64) NOT NULL,
    branch_id VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    completed_at DATETIME,
    INDEX idx_process_gateway (process_instance_id, gateway_node_id)
);
```

## 24.2 汇聚逻辑

```text
分支完成
    ↓
更新分支状态为 COMPLETED
    ↓
统计该并行网关已完成分支数
    ↓
如果完成数等于总分支数
    ↓
触发汇聚节点继续执行
```

原子更新示例：

```sql
UPDATE wf_parallel_branch
SET status = 'COMPLETED'
WHERE id = #{id}
AND status = 'RUNNING';
```

---

# 25. 多租户设计

如果平台支持多租户，需要在核心表中增加租户字段。

## 25.1 表字段

```sql
tenant_id VARCHAR(64) NOT NULL
```

涉及表：

```text
wf_process_definition
wf_process_instance
wf_node_instance
wf_task
wf_process_history
```

## 25.2 隔离方式

三种方案：

| 方案 | 隔离级别 | 成本 | 适用场景 |
|---|---:|---:|---|
| 字段隔离 | 低 | 低 | 通用 SaaS |
| Schema 隔离 | 中 | 中 | 中型客户隔离 |
| 独立数据库 | 高 | 高 | 大客户/私有化 |

建议初期使用：

```text
tenant_id 字段隔离
```

---

# 26. 权限设计

## 26.1 引擎权限

| 权限 | 说明 |
|---|---|
| 流程定义管理 | 创建、编辑、发布流程 |
| 流程实例管理 | 查询、取消、挂起流程 |
| 任务办理 | 处理自己的任务 |
| 流程监控 | 管理员查看流程实例 |
| 流程干预 | 管理员强制跳转、终止 |
| 日志审计 | 查看流程轨迹和操作日志 |

## 26.2 数据权限

例如：

- 普通用户只能查看自己发起的流程
- 部门主管可查看本部门流程
- 管理员可查看全部流程
- 审计人员只读查看所有流程

---

# 27. 可观测性设计

## 27.1 指标

核心指标：

```text
流程启动数
流程完成数
流程失败数
节点执行耗时
任务平均处理时长
待办任务积压数
超时任务数
异常节点数
外部调用成功率
```

## 27.2 日志

每次节点执行记录：

```json
{
  "traceId": "a1b2c3",
  "processInstanceId": 1001,
  "nodeId": "managerApproval",
  "nodeType": "USER_TASK",
  "action": "TASK_CREATED",
  "operator": "U20001",
  "timestamp": "2026-09-17T10:00:00Z"
}
```

## 27.3 链路追踪

接入：

```text
traceId
spanId
processInstanceId
nodeInstanceId
taskId
```

便于定位：

```text
业务请求 -> 流程启动 -> 节点执行 -> 外部服务调用 -> 消息通知
```

---

# 28. 性能与容量设计

## 28.1 性能目标

可根据业务规模设定：

| 指标 | 目标 |
|---|---:|
| 流程启动响应时间 | < 300ms |
| 任务办理响应时间 | < 300ms |
| 待办查询响应时间 | < 500ms |
| 每日流程启动量 | 10 万 ~ 100 万 |
| 并发任务办理 | 1000 QPS 起 |
| 节点执行成功率 | > 99.9% |

## 28.2 性能瓶颈

常见瓶颈：

1. 流程实例表写入
2. 节点实例表写入
3. 任务表查询
4. 轨迹表写入
5. 外部服务调用
6. 超时任务扫描

## 28.3 优化策略

### 查询优化

- 待办任务建立联合索引
- 历史数据归档
- 冷热数据分离
- 列表查询使用 ES
- 大字段拆分

### 写入优化

- 异步写轨迹
- 批量写入历史
- 分库分表
- 事件异步消费

### 调度优化

- 时间轮处理短延迟任务
- 分布式调度处理超时任务
- 分片扫描任务表

---

# 29. 高可用设计

## 29.1 服务无状态化

流程引擎服务应无状态部署：

```text
engine-service-1
engine-service-2
engine-service-3
```

状态保存在：

```text
MySQL
Redis
MQ
```

## 29.2 分布式调度

超时任务、重试任务需要分布式调度，避免重复执行。

可用：

- XXL-JOB
- ElasticJob
- Quartz Cluster
- 自研基于 Redis 的分布式调度

## 29.3 重试机制

外部调用失败后重试：

```text
第 1 次：立即重试
第 2 次：10 秒后
第 3 次：1 分钟后
第 4 次：5 分钟后
第 5 次：30 分钟后
```

超过最大次数进入异常任务。

---

# 30. 安全设计

## 30.1 接口安全

- 认证
- 鉴权
- 限流
- 防重放
- 参数校验

## 30.2 表达式安全

禁止危险操作：

```text
Runtime.exec
System.exit
反射访问
文件操作
任意类加载
```

## 30.3 脚本安全

脚本节点建议使用沙箱：

- 限制执行时间
- 限制内存
- 限制类白名单
- 禁止 IO
- 禁止网络调用，除非显式授权

---

# 31. 技术选型建议

## 31.1 推荐技术栈

| 模块 | 推荐 |
|---|---|
| 后端服务 | Java / Spring Boot |
| 流程定义 | JSON DSL 或 BPMN |
| 数据库 | MySQL / PostgreSQL |
| 缓存 | Redis |
| 消息队列 | Kafka / RocketMQ |
| 搜索查询 | Elasticsearch |
| 分布式调度 | XXL-JOB / ElasticJob |
| 表达式 | Aviator / SpEL |
| 脚本 | Groovy |
| 前端设计器 | LogicFlow / bpmn.js / 自研 Canvas |
| 监控 | Prometheus + Grafana |
| 日志 | ELK / Loki |
| 链路追踪 | OpenTelemetry / SkyWalking |

---

## 31.2 是否使用 Activiti / Flowable / Camunda？

### 方案一：基于 Flowable / Camunda 二次开发

优点：

- 成熟稳定
- 支持 BPMN
- 支持复杂流程
- 生态完善
- 减少研发成本

缺点：

- 模型较重
- 学习成本高
- 定制审批操作需要较多改造
- 与业务系统集成可能需要适配

适用场景：

- 标准审批流
- BPMN 流程
- 复杂企业流程
- 希望快速落地

### 方案二：自研轻量流程引擎

优点：

- 更贴合业务
- 模型更轻
- 易于扩展审批操作
- 易于与组织架构、表单、权限深度集成

缺点：

- 初期研发成本高
- 需要自己保证稳定性
- 复杂流程能力需逐步完善

适用场景：

- 互联网审批流
- 业务编排
- 需要深度定制
- 对性能和扩展性有较高要求

### 建议

如果团队资源有限，建议：

```text
初期基于 Flowable 或 Camunda 做二次开发
```

如果业务高度定制，且长期投入，建议：

```text
自研轻量引擎 + 可视化设计器
```

---

# 32. 与业务系统集成设计

流程引擎不应直接承担所有业务逻辑。

推荐边界：

```text
业务系统负责：
- 业务数据
- 业务规则
- 业务状态

流程引擎负责：
- 流程流转
- 任务分配
- 节点调度
- 审批轨迹
- 超时策略
```

## 32.1 业务系统发起流程

```java
processEngineClient.startProcess(
    "expense_approval",
    businessKey,
    variables
);
```

## 32.2 流程完成后回调业务系统

方式一：HTTP 回调

```text
POST /business/callback/process-completed
```

方式二：MQ 消息

```text
topic: process.completed
```

消息体：

```json
{
  "processCode": "expense_approval",
  "businessKey": "EXPENSE-20260917-0001",
  "status": "COMPLETED",
  "result": "APPROVED"
}
```

---

# 33. 管理后台设计

管理后台应包含：

## 33.1 流程管理

- 流程定义列表
- 流程版本管理
- 流程发布
- 流程停用
- 流程导入导出

## 33.2 流程监控

- 运行中流程
- 异常流程
- 挂起流程
- 已完成流程
- 节点执行详情
- 流程图高亮

## 33.3 任务管理

- 待办任务
- 已办任务
- 任务转办
- 任务重分配
- 任务代理

## 33.4 异常处理

- 失败节点重试
- 手动跳过
- 手动跳转
- 强制结束
- 补偿执行

## 33.5 报表统计

- 流程发起量
- 流程完成率
- 平均审批时长
- 节点阻塞分析
- 超时任务统计
- 人员处理效率

---

# 34. 流程设计器设计

## 34.1 基础能力

- 拖拽节点
- 连线
- 节点属性配置
- 条件配置
- 审批人配置
- 表单配置
- 超时配置
- 消息配置

## 34.2 校验能力

发布前校验：

```text
是否存在开始节点
是否存在结束节点
是否存在孤立节点
是否存在循环引用
条件分支是否完整
审批人是否配置
自动节点是否配置服务
并行分支是否可汇聚
```

## 34.3 前端技术

可选：

- LogicFlow
- AntV X6
- bpmn.js
- React Flow
- Vue Flow
- 自研 Canvas / SVG

建议：

如果做国产审批流，推荐：

```text
LogicFlow / AntV X6 + 自研属性面板
```

如果做标准 BPMN，推荐：

```text
bpmn.js + Flowable / Camunda
```

---

# 35. 实施路线图

## 第一阶段：MVP 最小可用版本

目标：支撑基本审批流。

包含：

- 流程定义管理
- 简单流程发布
- 流程发起
- 人工审批
- 条件分支
- 待办/已办
- 流程轨迹
- 同意/拒绝

周期建议：

```text
6 ~ 8 周
```

---

## 第二阶段：复杂审批能力

目标：支撑企业审批场景。

包含：

- 会签/或签
- 加签
- 转办
- 委派
- 退回
- 撤回
- 超时提醒
- 审批人规则
- 表单权限
- 消息通知

周期建议：

```text
8 ~ 12 周
```

---

## 第三阶段：业务编排能力

目标：支撑自动流程。

包含：

- 自动任务节点
- HTTP 节点
- 脚本节点
- MQ 节点
- 子流程
- 等待事件
- 异步任务
- 重试机制
- 异常补偿

周期建议：

```text
10 ~ 14 周
```

---

## 第四阶段：平台化能力

目标：成为企业流程中台。

包含：

- 多租户
- 可视化设计器
- 流程监控
- 报表统计
- 流程干预
- 权限审计
- 开放 API
- 流程模板市场

周期建议：

```text
12 周以上
```

---

# 36. 关键风险与应对

## 36.1 流程模型复杂度失控

风险：

- 节点类型越来越多
- 条件逻辑越来越复杂
- 特殊审批需求不断膨胀

应对：

- 明确引擎边界
- 优先抽象通用能力
- 对特殊场景通过扩展点支持
- 避免把业务逻辑全部塞进流程引擎

## 36.2 状态一致性风险

风险：

- 节点已完成但任务未关闭
- 流程已结束但任务仍在待办
- 并行分支重复汇聚

应对：

- 状态机统一管理
- 事务内更新核心状态
- 使用乐观锁/分布式锁
- 并行分支原子计数
- 事件表幂等消费

## 36.3 性能风险

风险：

- 待办查询慢
- 流程轨迹写入压力大
- 超时任务扫描慢

应对：

- 索引优化
- 历史数据归档
- 查询走 ES
- 写入异步化
- 调度分片处理

## 36.4 运维风险

风险：

- 异常流程无法恢复
- 外部调用失败堆积
- 手动干预困难

应对：

- 提供管理后台
- 支持节点重试
- 支持人工跳转
- 支持异常告警
- 支持操作审计

---

# 37. 推荐最终架构

如果是企业级长期建设，推荐如下架构：

```text
┌─────────────────────────────┐
│        可视化流程设计器         │
└─────────────────────────────┘
              ↓
┌─────────────────────────────┐
│         流程管理服务           │
│  流程定义 / 版本 / 发布 / 模板   |
└─────────────────────────────┘
              ↓
┌─────────────────────────────┐
│         流程执行引擎           │
│  状态机 / 节点执行器 / 网关       │
│  任务管理 / 规则引擎 / 事件引擎   │
└─────────────────────────────┘
              ↓
┌─────────────────────────────┐
│         异步执行层            │
│  MQ / 事件表 / 重试 / 补偿      │
└─────────────────────────────┘
              ↓
┌─────────────────────────────┐
│         基础支撑层            │
│  组织用户 / 消息中心 / 定时调度   │
└─────────────────────────────┘
              ↓
┌─────────────────────────────┐
│         数据存储层            │
│  MySQL / Redis / ES / 日志存储  │
└─────────────────────────────┘
```

---

# 38. 总结

一个成熟的工作流流程引擎应至少具备以下能力：

1. **流程定义能力**
   - 可视化设计
   - 版本管理
   - 发布管理

2. **流程执行能力**
   - 启动流程
   - 节点流转
   - 条件分支
   - 并行分支
   - 子流程

3. **任务管理能力**
   - 待办
   - 已办
   - 转办
   - 委派
   - 加签
   - 会签/或签

4. **规则能力**
   - 条件表达式
   - 审批人规则
   - 表单权限
   - 超时策略

5. **集成能力**
   - 服务调用
   - 消息通知
   - Webhook
   - MQ

6. **可靠性能力**
   - 幂等
   - 重试
   - 补偿
   - 事务一致性

7. **可观测能力**
   - 流程轨迹
   - 节点日志
   - 监控指标
   - 异常告警

8. **平台化能力**
   - 多租户
   - 权限控制
   - 管理后台
   - 统计报表

---

如果你希望，我还可以继续帮你输出以下任一项内容：

1. **更详细的数据库表结构设计**
2. **基于 Spring Boot 的流程引擎代码工程结构**
3. **流程引擎 JSON DSL 完整规范**
4. **类似钉钉/飞书审批流的详细设计**
5. **基于 Flowable / Camunda 的落地方案**
6. **自研轻量流程引擎的核心代码示例**
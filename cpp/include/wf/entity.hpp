// Runtime entities: process instance / node instance / task / history /
// event record / timer job / parallel branch (PRD §17 data model, kept in
// memory instead of MySQL for this single-process engine).
#pragma once

#include <string>
#include <vector>

#include "wf/json.hpp"
#include "wf/model.hpp"
#include "wf/types.hpp"

namespace wf {

// PRD §17.2
struct ProcessInstance {
  long long id = 0;
  long long definitionId = 0;
  std::string processCode;
  int definitionVersion = 1;
  std::string tenantId = "default";
  std::string businessKey;
  ProcessStatus status = ProcessStatus::Running;
  std::string initiator;
  std::string currentNodeId;
  json::Value variables;  // object: 流程变量 + 表单数据
  long long startTime = 0;
  long long endTime = 0;
  long long version = 0;  // optimistic lock (PRD §22.2 / §23.1)
  long long parentInstanceId = 0;      // 子流程
  long long parentNodeInstanceId = 0;  // 子流程返回时恢复的父节点实例
  int activeTokens = 0;                // 未消费的流转令牌（并行分支汇聚用）
  std::string result;                  // APPROVED / REJECTED / CANCELLED
  std::string errorMessage;

  json::Value toJson() const;
  static ProcessInstance fromJson(const json::Value& value);
};

// PRD §17.3
struct NodeInstance {
  long long id = 0;
  long long processInstanceId = 0;
  std::string nodeId;
  std::string nodeName;
  NodeType nodeType = NodeType::Unknown;
  NodeInstanceStatus status = NodeInstanceStatus::Pending;
  json::Value input;
  json::Value output;
  std::string errorMessage;
  std::string assignee;
  int attempt = 0;
  std::string waitKey;  // WAIT_TASK / TIMER_TASK 的等待键
  long long startTime = 0;
  long long endTime = 0;

  json::Value toJson() const;
  static NodeInstance fromJson(const json::Value& value);
};

// PRD §17.4
struct Task {
  long long id = 0;
  long long processInstanceId = 0;
  long long nodeInstanceId = 0;
  std::string nodeId;
  std::string name;
  std::string assignee;
  std::vector<std::string> candidateUsers;
  std::vector<std::string> candidateRoles;
  TaskStatus status = TaskStatus::Pending;
  int priority = 0;
  long long createTime = 0;
  long long claimTime = 0;
  long long completeTime = 0;
  long long dueTime = 0;
  std::string comment;
  long long version = 1;  // 乐观锁版本号
  long long parentTaskId = 0;
  std::string signType = "NORMAL";   // NORMAL | BEFORE | AFTER
  std::string signMode = "ANY";      // ALL | ANY
  CompleteStrategy strategy = CompleteStrategy::Any;
  std::string action;  // APPROVE / REJECT / ...
  std::string tenantId = "default";
  bool countersignMember = false;

  json::Value toJson() const;
  static Task fromJson(const json::Value& value);
};

// PRD §17.5 流程轨迹
struct HistoryRecord {
  long long id = 0;
  long long processInstanceId = 0;
  long long nodeInstanceId = 0;
  long long taskId = 0;
  std::string nodeId;
  std::string nodeName;
  std::string action;
  std::string operatorUser;
  std::string comment;
  json::Value input;
  json::Value output;
  std::string traceId;
  long long createdAt = 0;

  json::Value toJson() const;
  static HistoryRecord fromJson(const json::Value& value);
};

// PRD §17.6 事件记录（事务性 outbox + 幂等消费）
struct EventRecord {
  long long id = 0;
  std::string eventId;
  long long processInstanceId = 0;
  long long nodeInstanceId = 0;
  long long taskId = 0;
  EventType eventType = EventType::NodeStarted;
  std::string status = "PENDING";  // PENDING | SUCCESS | FAILED | SKIPPED
  json::Value payload;
  json::Value listeners;
  int retryCount = 0;
  long long nextRetryTime = 0;
  long long createdAt = 0;
  long long updatedAt = 0;
  std::string error;

  json::Value toJson() const;
  static EventRecord fromJson(const json::Value& value);
};

// PRD §17.7 定时任务
struct TimerJob {
  long long id = 0;
  long long processInstanceId = 0;
  long long nodeInstanceId = 0;
  long long taskId = 0;
  std::string jobType;  // NODE_TIMER | NODE_TIMEOUT | TASK_TIMEOUT
  long long triggerTime = 0;
  std::string status = "PENDING";  // PENDING | DONE | CANCELLED | FAILED
  json::Value payload;
  int retryCount = 0;
  long long createdAt = 0;

  json::Value toJson() const;
  static TimerJob fromJson(const json::Value& value);
};

// PRD §24.1 并行分支表
struct ParallelBranch {
  long long id = 0;
  long long processInstanceId = 0;
  std::string gatewayNodeId;  // fork 时为 fork 节点，join 时为汇聚节点
  std::string branchId;       // 出线 id / 入线 id
  std::string status = "RUNNING";  // RUNNING | ARRIVED | COMPLETED | CANCELLED
  long long completedAt = 0;

  json::Value toJson() const;
  static ParallelBranch fromJson(const json::Value& value);
};

}  // namespace wf

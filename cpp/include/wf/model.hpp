// Process definition model: JSON DSL (PRD §8), node/edge/assignee/timeout
// configuration (PRD §4, §9, §12) and the pre-publish validation checks
// (PRD §34.2).
#pragma once

#include <string>
#include <vector>

#include "wf/json.hpp"
#include "wf/types.hpp"

namespace wf {

// PRD §9 approval assignee rule.
struct ApprovalRule {
  // INITIATOR | USER | USERS | ROLE | ROLES | LEADER | POSITION | EXPRESSION
  // | PREV_ASSIGNEE | INITIATOR_LEADER | CANDIDATE | COMBINATION
  std::string type = "INITIATOR";
  std::string value;                       // role/user/position/expression
  int level = 1;                           // LEADER level (2 = 二级主管)
  std::vector<std::string> users;
  std::vector<ApprovalRule> rules;          // COMBINATION children
  std::string op = "AND";                   // COMBINATION: AND | OR
  std::string mode = "ANY";                 // candidate mode: ANY | ALL
  bool specified = false;                   // DSL 中是否显式配置了 assignee

  static ApprovalRule fromJson(const json::Value& value);
  json::Value toJson() const;
  bool empty() const;
};

// PRD §14.3 node level form field permissions.
struct FieldPermission {
  std::string field;
  bool visible = true;
  bool editable = false;
  bool required = false;

  static FieldPermission fromJson(const json::Value& value);
  json::Value toJson() const;
};

struct RetryPolicy {
  int maxAttempts = 1;
  long long backoffMillis = 1000;

  static RetryPolicy fromJson(const json::Value& value);
  json::Value toJson() const;
};

// PRD §16 service task configuration.
struct ServiceSpec {
  std::string type = "HTTP";  // HTTP | RPC | MOCK
  std::string method = "POST";
  std::string url;
  std::string serviceName;
  std::string methodName;
  json::Value headers;
  json::Value body;
  json::Value params;
  long long timeoutMillis = 3000;
  RetryPolicy retry;
  std::string resultVariable;  // where to store the response

  static ServiceSpec fromJson(const json::Value& value);
  json::Value toJson() const;
  bool configured() const { return !url.empty() || !serviceName.empty(); }
};

struct ScriptSpec {
  std::string language = "wfscript";
  std::string content;

  static ScriptSpec fromJson(const json::Value& value);
  json::Value toJson() const;
};

// PRD §12 timeout steps.
struct TimeoutStep {
  TimeoutAction action = TimeoutAction::Remind;
  std::string target;   // user / role / LEADER
  std::string channel;  // EMAIL / SMS / INNER / WEBHOOK
  std::string message;

  static TimeoutStep fromJson(const json::Value& value);
  json::Value toJson() const;
};

struct TimeoutPolicy {
  std::string duration;  // "PT24H", "30m", ...
  std::vector<TimeoutStep> steps;

  static TimeoutPolicy fromJson(const json::Value& value);
  json::Value toJson() const;
  bool configured() const { return !duration.empty() && !steps.empty(); }
  bool durationMillis(long long* out) const;
};

// PRD §15.2 listeners bound to events.
struct ListenerSpec {
  EventType event = EventType::ProcessCompleted;
  std::string type = "HTTP";  // HTTP | MQ | LOG
  std::string url;
  std::string topic;
  json::Value payload;

  static ListenerSpec fromJson(const json::Value& value);
  json::Value toJson() const;
};

// PRD §4.3 node definition.
struct NodeDefinition {
  std::string id;
  std::string name;
  NodeType type = NodeType::Unknown;

  ApprovalRule assignee;  // USER_TASK
  CompleteStrategy strategy = CompleteStrategy::Any;  // 会签/或签/比例/依次
  double ratio = 0.5;                                 // RATIO threshold
  std::vector<FieldPermission> fieldPermissions;

  TimeoutPolicy nodeTimeout;  // 节点超时
  TimeoutPolicy taskTimeout;  // 任务超时（催办/升级）

  // 拒绝策略（PRD §10.2）
  std::string rejectPolicy = "END";  // END | INITIATOR | PREV | NODE | EXCEPTION
  std::string rejectTarget;
  bool allowReturn = true;
  bool allowWithdraw = true;

  ServiceSpec service;  // SERVICE_TASK
  ScriptSpec script;    // SCRIPT_TASK

  std::string subProcessCode;  // SUB_PROCESS
  bool subProcessAsync = false;
  json::Value subProcessInput;                 // variable mapping into child
  std::vector<std::string> subProcessOutputs;  // variables copied back

  std::string eventKey;       // WAIT_TASK event name
  std::string duration;       // TIMER_TASK delay ("PT5M")
  std::string messageChannel;  // MESSAGE_TASK
  std::string messageTarget;
  std::string messageTemplate;

  bool joinAll = false;  // any node can act as an AND-join

  std::vector<ListenerSpec> listeners;
  json::Value properties;  // raw node JSON for forward compatibility

  bool durationMillis(long long* out) const;
  static NodeDefinition fromJson(const json::Value& value);
  json::Value toJson() const;
};

struct EdgeDefinition {
  std::string id;
  std::string source;
  std::string target;
  std::string condition;      // "${amount > 10000}"
  bool isDefault = false;
  int priority = 0;

  static EdgeDefinition fromJson(const json::Value& value);
  json::Value toJson() const;
};

// PRD §4.2 process definition.
struct ProcessDefinition {
  long long id = 0;
  std::string code;
  std::string name;
  int version = 1;
  DefinitionStatus status = DefinitionStatus::Draft;
  std::string tenantId = "default";
  std::vector<NodeDefinition> nodes;
  std::vector<EdgeDefinition> edges;
  json::Value variables;  // default variables
  json::Value config;
  std::string description;
  std::string createdBy;
  long long createdAt = 0;
  long long updatedAt = 0;

  const NodeDefinition* findNode(const std::string& nodeId) const;
  NodeDefinition* findNode(const std::string& nodeId);
  const NodeDefinition* startNode() const;
  const NodeDefinition* endNode() const;
  std::vector<const EdgeDefinition*> outEdges(const std::string& nodeId) const;
  std::vector<const EdgeDefinition*> inEdges(const std::string& nodeId) const;
  const EdgeDefinition* findEdge(const std::string& edgeId) const;
  bool hasCycle(std::string* cyclePath) const;
  std::vector<std::string> reachableFrom(const std::string& nodeId) const;
  int fanOut(const std::string& nodeId) const { return static_cast<int>(outEdges(nodeId).size()); }
  int fanIn(const std::string& nodeId) const { return static_cast<int>(inEdges(nodeId).size()); }

  static ProcessDefinition fromJson(const json::Value& value);
  json::Value toJson() const;
};

struct ValidationIssue {
  std::string code;
  std::string nodeId;
  std::string message;
};

struct ValidationResult {
  std::vector<ValidationIssue> errors;
  std::vector<ValidationIssue> warnings;

  bool ok() const { return errors.empty(); }
  bool hasCode(const std::string& code) const;
  std::string summary() const;  // multi-line Chinese report
};

// PRD §34.2 publish-time validation.
ValidationResult validateDefinition(const ProcessDefinition& definition);

}  // namespace wf

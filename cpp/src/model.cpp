#include "wf/model.hpp"

#include <algorithm>
#include <deque>
#include <map>
#include <set>
#include <sstream>

namespace wf {
namespace {

using json::Value;

std::vector<std::string> stringList(const Value& value) {
  std::vector<std::string> out;
  if (value.isArray()) {
    for (const auto& item : value.items()) {
      const std::string text = item.toString();
      if (!text.empty()) out.push_back(text);
    }
  } else if (value.isString()) {
    out = splitList(value.asString());
  }
  return out;
}

Value listToJson(const std::vector<std::string>& items) {
  Value array = Value::array();
  for (const auto& item : items) array.push_back(Value(item));
  return array;
}

// Accepts `"assignee": {"type": "ROLE", "value": "M"}`, the shorthand
// `"assignee": "ROLE:M"` and nested combination rules — all handled by
// ApprovalRule::fromJson.
ApprovalRule parseAssignee(const Value& value) { return ApprovalRule::fromJson(value); }

}  // namespace

bool ApprovalRule::empty() const {
  if (type.empty()) return true;
  const std::string kind = toLower(type);
  // 这些规则只靠类型（或 level）就能计算审批人，不需要额外取值
  if (kind == "initiator" || kind == "prev_assignee" || kind == "last_assignee" ||
      kind == "initiator_leader" || kind == "leader") {
    return false;
  }
  if (kind == "combination") return rules.empty();
  return value.empty() && users.empty();
}

ApprovalRule ApprovalRule::fromJson(const Value& value) {
  ApprovalRule rule;
  rule.specified = true;
  if (value.isString()) {
    // Shorthand: "ROLE:FINANCE_MANAGER", "INITIATOR", "LEADER:2"
    const std::string text = value.asString();
    const std::size_t colon = text.find(':');
    if (colon == std::string::npos) {
      rule.type = toLower(trimCopy(text));
      return rule;
    }
    rule.type = toLower(trimCopy(text.substr(0, colon)));
    const std::string rest = trimCopy(text.substr(colon + 1));
    if (rule.type == "leader") {
      try {
        rule.level = std::stoi(rest);
      } catch (...) {
        rule.value = rest;
      }
    } else {
      rule.value = rest;
    }
    return rule;
  }
  if (!value.isObject()) {
    return rule;
  }
  rule.type = toLower(value.stringOr("type", value.stringOr("kind", "initiator")));
  rule.value = value.stringOr("value", value.stringOr("role", value.stringOr("user", "")));
  if (rule.value.empty() && value.contains("expression")) {
    rule.value = value.at("expression").toString();
    if (rule.type == "initiator") rule.type = "expression";
  }
  rule.level = static_cast<int>(value.intOr("level", 1));
  rule.op = value.stringOr("op", value.stringOr("operator", "AND"));
  rule.mode = value.stringOr("mode", "ANY");
  if (value.contains("users")) rule.users = stringList(value.at("users"));
  if (rule.users.empty() && value.contains("user")) rule.users = stringList(value.at("user"));
  if (rule.users.empty() && value.contains("roles")) {
    const std::vector<std::string> roles = stringList(value.at("roles"));
    if (!roles.empty()) {
      rule.type = "roles";
      rule.users = roles;
    }
  }
  if (value.contains("rules") && value.at("rules").isArray()) {
    for (const auto& child : value.at("rules").items()) {
      rule.rules.push_back(ApprovalRule::fromJson(child));
    }
    if (rule.type == "initiator" || rule.type.empty()) rule.type = "combination";
  }
  // Normalise a few aliases used in hand written DSL.
  if (rule.type == "currentuser" || rule.type == "start_user") rule.type = "initiator";
  return rule;
}

Value ApprovalRule::toJson() const {
  Value out = Value::object();
  out.set("type", Value(type));
  if (!value.empty()) out.set("value", Value(value));
  if (level != 1) out.set("level", Value(level));
  if (!users.empty()) out.set("users", listToJson(users));
  if (!rules.empty()) {
    Value children = Value::array();
    for (const auto& rule : rules) children.push_back(rule.toJson());
    out.set("rules", children);
    out.set("op", Value(op));
  }
  if (mode != "ANY") out.set("mode", Value(mode));
  return out;
}

FieldPermission FieldPermission::fromJson(const Value& value) {
  FieldPermission permission;
  if (value.isString()) {
    permission.field = value.asString();
    return permission;
  }
  permission.field = value.stringOr("field", value.stringOr("name", ""));
  permission.visible = value.boolOr("visible", true);
  permission.editable = value.boolOr("editable", value.boolOr("writeable", false));
  permission.required = value.boolOr("required", false);
  return permission;
}

Value FieldPermission::toJson() const {
  Value out = Value::object();
  out.set("field", Value(field));
  out.set("visible", Value(visible));
  out.set("editable", Value(editable));
  out.set("required", Value(required));
  return out;
}

RetryPolicy RetryPolicy::fromJson(const Value& value) {
  RetryPolicy policy;
  if (!value.isObject()) return policy;
  policy.maxAttempts = static_cast<int>(value.intOr("maxAttempts", value.intOr("max_attempts", 1)));
  const Value& backoff = value.at("backoff");
  if (backoff.isNumber()) {
    policy.backoffMillis = backoff.asInt();
  } else if (backoff.isString()) {
    long long millis = 0;
    if (parseDuration(backoff.asString(), &millis)) policy.backoffMillis = millis;
  }
  return policy;
}

Value RetryPolicy::toJson() const {
  Value out = Value::object();
  out.set("maxAttempts", Value(maxAttempts));
  out.set("backoff", Value(backoffMillis));
  return out;
}

ServiceSpec ServiceSpec::fromJson(const Value& value) {
  ServiceSpec spec;
  if (!value.isObject()) return spec;
  spec.type = toLower(value.stringOr("type", "http"));
  spec.method = value.stringOr("method", "POST");
  spec.url = value.stringOr("url", "");
  spec.serviceName = value.stringOr("serviceName", "");
  spec.methodName = value.stringOr("methodName", "");
  spec.headers = value.at("headers");
  spec.body = value.at("body");
  spec.params = value.at("params");
  spec.timeoutMillis = value.intOr("timeout", value.intOr("timeoutMillis", 3000));
  spec.retry = RetryPolicy::fromJson(value.at("retry"));
  spec.resultVariable = value.stringOr("resultVariable", "");
  return spec;
}

Value ServiceSpec::toJson() const {
  Value out = Value::object();
  out.set("type", Value(type));
  out.set("method", Value(method));
  if (!url.empty()) out.set("url", Value(url));
  if (!serviceName.empty()) out.set("serviceName", Value(serviceName));
  if (!methodName.empty()) out.set("methodName", Value(methodName));
  if (!headers.isNull()) out.set("headers", headers);
  if (!body.isNull()) out.set("body", body);
  if (!params.isNull()) out.set("params", params);
  out.set("timeout", Value(timeoutMillis));
  out.set("retry", retry.toJson());
  if (!resultVariable.empty()) out.set("resultVariable", Value(resultVariable));
  return out;
}

ScriptSpec ScriptSpec::fromJson(const Value& value) {
  ScriptSpec spec;
  if (value.isString()) {
    spec.content = value.asString();
    return spec;
  }
  if (!value.isObject()) return spec;
  spec.language = toLower(value.stringOr("language", "wfscript"));
  spec.content = value.stringOr("content", value.stringOr("script", ""));
  return spec;
}

Value ScriptSpec::toJson() const {
  Value out = Value::object();
  out.set("language", Value(language));
  out.set("content", Value(content));
  return out;
}

TimeoutStep TimeoutStep::fromJson(const Value& value) {
  TimeoutStep step;
  if (value.isString()) {
    step.action = parseTimeoutAction(value.asString());
    return step;
  }
  if (!value.isObject()) return step;
  step.action = parseTimeoutAction(value.stringOr("type", value.stringOr("action", "REMIND")));
  step.target = value.stringOr("target", "");
  step.channel = value.stringOr("channel", "");
  step.message = value.stringOr("message", "");
  return step;
}

Value TimeoutStep::toJson() const {
  Value out = Value::object();
  out.set("type", Value(toString(action)));
  if (!target.empty()) out.set("target", Value(target));
  if (!channel.empty()) out.set("channel", Value(channel));
  if (!message.empty()) out.set("message", Value(message));
  return out;
}

TimeoutPolicy TimeoutPolicy::fromJson(const Value& value) {
  TimeoutPolicy policy;
  if (value.isString()) {
    policy.duration = value.asString();
    return policy;
  }
  if (!value.isObject()) return policy;
  policy.duration = value.stringOr("duration", value.stringOr("delay", ""));
  if (value.contains("actions")) {
    for (const auto& item : value.at("actions").items()) {
      policy.steps.push_back(TimeoutStep::fromJson(item));
    }
  } else if (value.contains("action")) {
    policy.steps.push_back(TimeoutStep::fromJson(value.at("action")));
  }
  return policy;
}

Value TimeoutPolicy::toJson() const {
  Value out = Value::object();
  if (!duration.empty()) out.set("duration", Value(duration));
  if (!steps.empty()) {
    Value actions = Value::array();
    for (const auto& step : steps) actions.push_back(step.toJson());
    out.set("actions", actions);
  }
  return out;
}

bool TimeoutPolicy::durationMillis(long long* out) const {
  if (duration.empty()) return false;
  return parseDuration(duration, out);
}

ListenerSpec ListenerSpec::fromJson(const Value& value) {
  ListenerSpec listener;
  if (!value.isObject()) return listener;
  listener.event = parseEventType(value.stringOr("type", value.stringOr("event", "")));
  const Value& target = value.at("listener");
  const Value& source = target.isObject() ? target : value;
  listener.type = toLower(source.stringOr("type", source.stringOr("channel", "http")));
  if (listener.type == "http" || listener.type == "mq") {
    // keep
  }
  listener.url = source.stringOr("url", "");
  listener.topic = source.stringOr("topic", "");
  listener.payload = source.at("payload");
  if (listener.type == "webhook" || listener.type == "rest") listener.type = "http";
  if (listener.type == "kafka" || listener.type == "rocketmq" || listener.type == "event") {
    listener.type = "mq";
  }
  return listener;
}

Value ListenerSpec::toJson() const {
  Value out = Value::object();
  out.set("type", Value(toString(event)));
  Value listener = Value::object();
  listener.set("type", Value(type));
  if (!url.empty()) listener.set("url", Value(url));
  if (!topic.empty()) listener.set("topic", Value(topic));
  if (!payload.isNull()) listener.set("payload", payload);
  out.set("listener", listener);
  return out;
}

bool NodeDefinition::durationMillis(long long* out) const {
  if (duration.empty()) return false;
  return parseDuration(duration, out);
}

NodeDefinition NodeDefinition::fromJson(const Value& value) {
  NodeDefinition node;
  node.properties = value;
  node.id = value.stringOr("id", value.stringOr("nodeId", ""));
  node.name = value.stringOr("name", node.id);
  node.type = parseNodeType(value.stringOr("type", value.stringOr("nodeType", "")));
  if (value.contains("assignee")) node.assignee = parseAssignee(value.at("assignee"));
  node.strategy = parseCompleteStrategy(value.stringOr("completeStrategy", value.stringOr("strategy", "")));
  node.ratio = value.numberOr("ratio", value.numberOr("passRatio", 0.5));
  if (value.contains("formPermissions") || value.contains("fieldPermissions")) {
    const Value& permissions = value.contains("formPermissions") ? value.at("formPermissions")
                                                                 : value.at("fieldPermissions");
    for (const auto& item : permissions.items()) {
      node.fieldPermissions.push_back(FieldPermission::fromJson(item));
    }
  }
  if (value.contains("timeout")) node.nodeTimeout = TimeoutPolicy::fromJson(value.at("timeout"));
  if (value.contains("taskTimeout")) node.taskTimeout = TimeoutPolicy::fromJson(value.at("taskTimeout"));
  node.rejectPolicy = value.stringOr("rejectPolicy", "END");
  node.rejectTarget = value.stringOr("rejectTarget", "");
  node.allowReturn = value.boolOr("allowReturn", true);
  node.allowWithdraw = value.boolOr("allowWithdraw", true);
  if (value.contains("service")) node.service = ServiceSpec::fromJson(value.at("service"));
  if (value.contains("script")) node.script = ScriptSpec::fromJson(value.at("script"));
  node.subProcessCode = value.stringOr("processCode", value.stringOr("subProcessCode", ""));
  node.subProcessAsync = value.boolOr("async", false);
  if (value.contains("subProcessInput") || value.contains("input")) {
    node.subProcessInput = value.contains("subProcessInput") ? value.at("subProcessInput") : value.at("input");
  }
  if (value.contains("subProcessOutputs") || value.contains("outputs")) {
    node.subProcessOutputs = stringList(value.contains("subProcessOutputs") ? value.at("subProcessOutputs")
                                                                          : value.at("outputs"));
  }
  node.eventKey = value.stringOr("eventKey", value.stringOr("signal", ""));
  node.duration = value.stringOr("duration", value.stringOr("delay", ""));
  node.messageChannel = value.stringOr("channel", "");
  node.messageTarget = value.stringOr("target", "");
  node.messageTemplate = value.stringOr("template", "");
  node.joinAll = value.boolOr("joinAll", value.boolOr("waitAll", false));
  if (value.contains("listeners")) {
    for (const auto& item : value.at("listeners").items()) {
      node.listeners.push_back(ListenerSpec::fromJson(item));
    }
  }
  if (value.contains("events")) {
    for (const auto& item : value.at("events").items()) {
      node.listeners.push_back(ListenerSpec::fromJson(item));
    }
  }
  return node;
}

Value NodeDefinition::toJson() const {
  Value out = Value::object();
  out.set("id", Value(id));
  out.set("name", Value(name));
  out.set("type", Value(toString(type)));
  switch (type) {
    case NodeType::UserTask:
      out.set("assignee", assignee.toJson());
      out.set("completeStrategy", Value(toString(strategy)));
      if (strategy == CompleteStrategy::Ratio) out.set("ratio", Value(ratio));
      if (!fieldPermissions.empty()) {
        Value permissions = Value::array();
        for (const auto& permission : fieldPermissions) permissions.push_back(permission.toJson());
        out.set("formPermissions", permissions);
      }
      if (nodeTimeout.configured()) out.set("timeout", nodeTimeout.toJson());
      if (taskTimeout.configured()) out.set("taskTimeout", taskTimeout.toJson());
      if (rejectPolicy != "END") out.set("rejectPolicy", Value(rejectPolicy));
      if (!rejectTarget.empty()) out.set("rejectTarget", Value(rejectTarget));
      break;
    case NodeType::ServiceTask:
      out.set("service", service.toJson());
      break;
    case NodeType::ScriptTask:
      out.set("script", script.toJson());
      break;
    case NodeType::SubProcess:
      out.set("processCode", Value(subProcessCode));
      out.set("async", Value(subProcessAsync));
      if (!subProcessInput.isNull()) out.set("subProcessInput", subProcessInput);
      if (!subProcessOutputs.empty()) out.set("subProcessOutputs", listToJson(subProcessOutputs));
      break;
    case NodeType::WaitTask:
      out.set("eventKey", Value(eventKey));
      break;
    case NodeType::TimerTask:
      out.set("duration", Value(duration));
      break;
    case NodeType::MessageTask:
      out.set("channel", Value(messageChannel));
      out.set("target", Value(messageTarget));
      out.set("template", Value(messageTemplate));
      break;
    default:
      break;
  }
  if (!listeners.empty()) {
    Value listeners = Value::array();
    for (const auto& listener : this->listeners) listeners.push_back(listener.toJson());
    out.set("listeners", listeners);
  }
  return out;
}

EdgeDefinition EdgeDefinition::fromJson(const Value& value) {
  EdgeDefinition edge;
  edge.id = value.stringOr("id", "");
  edge.source = value.stringOr("source", value.stringOr("sourceNodeId", ""));
  edge.target = value.stringOr("target", value.stringOr("targetNodeId", ""));
  edge.condition = value.stringOr("condition", value.stringOr("expression", ""));
  edge.isDefault = value.boolOr("default", false);
  edge.priority = static_cast<int>(value.intOr("priority", 0));
  if (edge.id.empty() && !edge.source.empty() && !edge.target.empty()) {
    edge.id = edge.source + "->" + edge.target;
  }
  return edge;
}

Value EdgeDefinition::toJson() const {
  Value out = Value::object();
  out.set("id", Value(id));
  out.set("source", Value(source));
  out.set("target", Value(target));
  if (!condition.empty()) out.set("condition", Value(condition));
  if (isDefault) out.set("default", Value(true));
  if (priority != 0) out.set("priority", Value(priority));
  return out;
}

const NodeDefinition* ProcessDefinition::findNode(const std::string& nodeId) const {
  for (const auto& node : nodes) {
    if (node.id == nodeId) return &node;
  }
  return nullptr;
}

NodeDefinition* ProcessDefinition::findNode(const std::string& nodeId) {
  for (auto& node : nodes) {
    if (node.id == nodeId) return &node;
  }
  return nullptr;
}

const NodeDefinition* ProcessDefinition::startNode() const {
  for (const auto& node : nodes) {
    if (node.type == NodeType::Start) return &node;
  }
  return nullptr;
}

const NodeDefinition* ProcessDefinition::endNode() const {
  for (const auto& node : nodes) {
    if (node.type == NodeType::End) return &node;
  }
  return nullptr;
}

std::vector<const EdgeDefinition*> ProcessDefinition::outEdges(const std::string& nodeId) const {
  std::vector<const EdgeDefinition*> out;
  for (const auto& edge : edges) {
    if (edge.source == nodeId) out.push_back(&edge);
  }
  std::sort(out.begin(), out.end(), [](const EdgeDefinition* a, const EdgeDefinition* b) {
    if (a->priority != b->priority) return a->priority > b->priority;
    return a->id < b->id;
  });
  return out;
}

std::vector<const EdgeDefinition*> ProcessDefinition::inEdges(const std::string& nodeId) const {
  std::vector<const EdgeDefinition*> in;
  for (const auto& edge : edges) {
    if (edge.target == nodeId) in.push_back(&edge);
  }
  return in;
}

const EdgeDefinition* ProcessDefinition::findEdge(const std::string& edgeId) const {
  for (const auto& edge : edges) {
    if (edge.id == edgeId) return &edge;
  }
  return nullptr;
}

std::vector<std::string> ProcessDefinition::reachableFrom(const std::string& nodeId) const {
  std::vector<std::string> visited;
  std::set<std::string> seen;
  std::deque<std::string> queue;
  queue.push_back(nodeId);
  while (!queue.empty()) {
    const std::string current = queue.front();
    queue.pop_front();
    if (!seen.insert(current).second) continue;
    visited.push_back(current);
    for (const EdgeDefinition* edge : outEdges(current)) {
      queue.push_back(edge->target);
    }
  }
  return visited;
}

bool ProcessDefinition::hasCycle(std::string* cyclePath) const {
  std::map<std::string, int> state;  // 0 unvisited, 1 in-stack, 2 done
  std::vector<std::string> stack;
  bool found = false;
  std::string path;

  std::function<void(const std::string&)> visit = [&](const std::string& nodeId) {
    if (found) return;
    state[nodeId] = 1;
    stack.push_back(nodeId);
    for (const EdgeDefinition* edge : outEdges(nodeId)) {
      if (found) break;
      const std::string& next = edge->target;
      if (state[next] == 1) {
        found = true;
        std::ostringstream out;
        bool started = false;
        for (const auto& id : stack) {
          if (!started && id == next) started = true;
          if (started) out << id << " -> ";
        }
        out << next;
        path = out.str();
        break;
      }
      if (state[next] == 0) visit(next);
    }
    stack.pop_back();
    state[nodeId] = 2;
  };

  for (const auto& node : nodes) {
    if (state[node.id] == 0) visit(node.id);
    if (found) break;
  }
  if (found && cyclePath != nullptr) *cyclePath = path;
  return found;
}

ProcessDefinition ProcessDefinition::fromJson(const Value& value) {
  ProcessDefinition definition;
  Value source = value;
  if (value.contains("definition") && value.at("definition").isObject()) {
    const Value& nested = value.at("definition");
    for (const auto& member : nested.members()) {
      source.set(member.first, member.second);
    }
  }
  definition.id = source.intOr("id", 0);
  definition.code = source.stringOr("processCode", source.stringOr("code", ""));
  definition.name = source.stringOr("processName", source.stringOr("name", definition.code));
  definition.version = static_cast<int>(source.intOr("version", 1));
  definition.status = parseDefinitionStatus(source.stringOr("status", "DRAFT"));
  definition.tenantId = source.stringOr("tenantId", "default");
  definition.description = source.stringOr("description", "");
  definition.createdBy = source.stringOr("createdBy", "");
  definition.createdAt = source.intOr("createdAt", 0);
  definition.updatedAt = source.intOr("updatedAt", 0);
  definition.variables = source.contains("variables") ? source.at("variables") : Value::object();
  definition.config = source.contains("config") ? source.at("config") : Value::object();
  if (source.contains("nodes")) {
    for (const auto& item : source.at("nodes").items()) {
      definition.nodes.push_back(NodeDefinition::fromJson(item));
    }
  }
  if (source.contains("edges")) {
    for (const auto& item : source.at("edges").items()) {
      definition.edges.push_back(EdgeDefinition::fromJson(item));
    }
  }
  return definition;
}

Value ProcessDefinition::toJson() const {
  Value out = Value::object();
  if (id != 0) out.set("id", Value(id));
  out.set("processCode", Value(code));
  out.set("processName", Value(name));
  out.set("version", Value(version));
  out.set("status", Value(toString(status)));
  out.set("tenantId", Value(tenantId));
  Value nodeArray = Value::array();
  for (const auto& node : nodes) nodeArray.push_back(node.toJson());
  out.set("nodes", nodeArray);
  Value edgeArray = Value::array();
  for (const auto& edge : edges) edgeArray.push_back(edge.toJson());
  out.set("edges", edgeArray);
  if (!variables.isNull() && !variables.empty()) out.set("variables", variables);
  if (!config.isNull() && !config.empty()) out.set("config", config);
  if (!description.empty()) out.set("description", Value(description));
  if (!createdBy.empty()) out.set("createdBy", Value(createdBy));
  if (createdAt != 0) out.set("createdAt", Value(createdAt));
  if (updatedAt != 0) out.set("updatedAt", Value(updatedAt));
  return out;
}

bool ValidationResult::hasCode(const std::string& code) const {
  for (const auto& issue : errors) {
    if (issue.code == code) return true;
  }
  for (const auto& issue : warnings) {
    if (issue.code == code) return true;
  }
  return false;
}

std::string ValidationResult::summary() const {
  std::ostringstream out;
  out << (errors.empty() ? "校验通过" : "校验失败") << "：错误 " << errors.size() << " 项，警告 "
      << warnings.size() << " 项";
  for (const auto& issue : errors) {
    out << "\n  [ERROR] " << issue.code;
    if (!issue.nodeId.empty()) out << " (节点 " << issue.nodeId << ")";
    out << ": " << issue.message;
  }
  for (const auto& issue : warnings) {
    out << "\n  [WARN ] " << issue.code;
    if (!issue.nodeId.empty()) out << " (节点 " << issue.nodeId << ")";
    out << ": " << issue.message;
  }
  return out.str();
}

ValidationResult validateDefinition(const ProcessDefinition& definition) {
  ValidationResult result;
  if (definition.code.empty()) {
    result.errors.push_back({"WF_NO_CODE", "", "流程 code 不能为空"});
  }
  if (definition.nodes.empty()) {
    result.errors.push_back({"WF_NO_NODES", "", "流程必须包含至少一个节点"});
    return result;
  }

  std::set<std::string> nodeIds;
  for (const auto& node : definition.nodes) {
    if (node.id.empty()) {
      result.errors.push_back({"WF_NODE_ID_MISSING", "", "存在没有 id 的节点"});
      continue;
    }
    if (!nodeIds.insert(node.id).second) {
      result.errors.push_back({"WF_NODE_DUPLICATE", node.id, "节点 id 重复"});
    }
    if (node.type == NodeType::Unknown) {
      result.errors.push_back(
          {"WF_UNKNOWN_NODE_TYPE", node.id, "无法识别的节点类型：" + node.properties.stringOr("type", "")});
    }
  }

  // 是否存在开始/结束节点（PRD §34.2）
  int startCount = 0;
  int endCount = 0;
  for (const auto& node : definition.nodes) {
    if (node.type == NodeType::Start) ++startCount;
    if (node.type == NodeType::End) ++endCount;
  }
  if (startCount == 0) {
    result.errors.push_back({"WF_NO_START", "", "流程缺少开始节点"});
  } else if (startCount > 1) {
    result.errors.push_back({"WF_MULTIPLE_START", "", "流程存在多个开始节点"});
  }
  if (endCount == 0) {
    result.errors.push_back({"WF_NO_END", "", "流程缺少结束节点"});
  }

  // 连线端点必须存在
  for (const auto& edge : definition.edges) {
    if (!nodeIds.count(edge.source)) {
      result.errors.push_back({"WF_EDGE_SOURCE_MISSING", edge.source,
                               "连线 " + edge.id + " 的源节点不存在"});
    }
    if (!nodeIds.count(edge.target)) {
      result.errors.push_back({"WF_EDGE_TARGET_MISSING", edge.target,
                               "连线 " + edge.id + " 的目标节点不存在"});
    }
    if (edge.condition.empty() && !edge.isDefault && definition.findNode(edge.source) != nullptr &&
        definition.findNode(edge.source)->type == NodeType::ExclusiveGateway) {
      // exclusive gateway edges without condition are only legal as default
    }
  }

  // 孤立节点 / 无人到达的节点（PRD §34.2）
  const NodeDefinition* start = definition.startNode();
  std::set<std::string> reachable;
  if (start != nullptr) {
    for (const auto& id : definition.reachableFrom(start->id)) reachable.insert(id);
  }
  for (const auto& node : definition.nodes) {
    if (definition.fanIn(node.id) == 0 && definition.fanOut(node.id) == 0 &&
        node.type != NodeType::Start) {
      result.errors.push_back({"WF_ORPHAN_NODE", node.id, "孤立节点：既没有入线也没有出线"});
      continue;
    }
    if (start != nullptr && node.type != NodeType::Start && !reachable.count(node.id)) {
      result.warnings.push_back({"WF_UNREACHABLE", node.id, "节点从开始节点不可达"});
    }
  }

  // 循环引用（PRD §34.2）
  std::string cycle;
  if (definition.hasCycle(&cycle)) {
    result.errors.push_back({"WF_CYCLE", "", "流程存在循环引用：" + cycle});
  }

  for (const auto& node : definition.nodes) {
    const std::vector<const EdgeDefinition*> out = definition.outEdges(node.id);
    const std::vector<const EdgeDefinition*> in = definition.inEdges(node.id);

    switch (node.type) {
      case NodeType::ExclusiveGateway: {
        if (out.size() < 2) {
          result.warnings.push_back({"WF_GATEWAY_SINGLE_OUT", node.id, "条件网关只有一条出线"});
        }
        bool hasDefault = false;
        bool hasCondition = false;
        for (const EdgeDefinition* edge : out) {
          if (edge->isDefault) hasDefault = true;
          if (!edge->condition.empty()) hasCondition = true;
        }
        if (!hasCondition) {
          result.errors.push_back({"WF_CONDITION_MISSING", node.id, "条件网关必须至少配置一条条件分支"});
        } else if (!hasDefault) {
          result.warnings.push_back(
              {"WF_CONDITION_INCOMPLETE", node.id, "条件分支没有配置默认分支，可能无法匹配"});
        }
        break;
      }
      case NodeType::ParallelGateway: {
        if (out.size() > 1) {
          // 并行分支是否可汇聚（PRD §34.2）
          const std::vector<std::string> first = definition.reachableFrom(out.front()->target);
          std::set<std::string> common(first.begin(), first.end());
          for (std::size_t i = 1; i < out.size(); ++i) {
            const std::vector<std::string> branch = definition.reachableFrom(out[i]->target);
            std::set<std::string> intersection;
            for (const auto& id : branch) {
              if (common.count(id)) intersection.insert(id);
            }
            common = intersection;
          }
          bool foundJoin = false;
          for (const auto& id : common) {
            const NodeDefinition* candidate = definition.findNode(id);
            if (candidate == nullptr) continue;
            if (candidate->type == NodeType::ParallelGateway && definition.fanIn(id) > 1) {
              foundJoin = true;
              break;
            }
            if (candidate->type == NodeType::End || candidate->joinAll) {
              foundJoin = true;
              break;
            }
          }
          if (!foundJoin) {
            result.errors.push_back({"WF_PARALLEL_NO_JOIN", node.id,
                                     "并行分支无法汇聚：各分支没有共同的可汇合节点"});
          }
        } else if (out.empty()) {
          result.errors.push_back({"WF_PARALLEL_NO_OUT", node.id, "并行网关没有出线"});
        }
        if (in.empty()) {
          result.errors.push_back({"WF_PARALLEL_NO_IN", node.id, "并行网关没有入线"});
        }
        break;
      }
      case NodeType::UserTask: {
        if (!node.assignee.specified) {
          result.errors.push_back({"WF_ASSIGNEE_MISSING", node.id, "人工任务节点必须配置审批人规则"});
        } else if (node.assignee.empty()) {
          result.errors.push_back({"WF_ASSIGNEE_MISSING", node.id,
                                   "审批人规则缺少必要参数：" + node.assignee.type});
        }
        if (node.strategy == CompleteStrategy::Ratio && (node.ratio <= 0 || node.ratio > 1)) {
          result.errors.push_back({"WF_RATIO_INVALID", node.id, "比例通过阈值必须在 (0,1] 区间"});
        }
        if (node.taskTimeout.configured()) {
          long long millis = 0;
          if (!node.taskTimeout.durationMillis(&millis)) {
            result.errors.push_back({"WF_DURATION_INVALID", node.id,
                                     "任务超时时长格式错误：" + node.taskTimeout.duration});
          }
        }
        if (node.nodeTimeout.configured()) {
          long long millis = 0;
          if (!node.nodeTimeout.durationMillis(&millis)) {
            result.errors.push_back({"WF_DURATION_INVALID", node.id,
                                     "节点超时时长格式错误：" + node.nodeTimeout.duration});
          }
        }
        break;
      }
      case NodeType::ServiceTask: {
        if (!node.service.configured()) {
          result.errors.push_back({"WF_SERVICE_MISSING", node.id, "自动任务节点必须配置服务地址"});
        }
        if (node.service.retry.maxAttempts < 1) {
          result.errors.push_back({"WF_RETRY_INVALID", node.id, "重试次数必须大于 0"});
        }
        break;
      }
      case NodeType::ScriptTask: {
        if (trimCopy(node.script.content).empty()) {
          result.errors.push_back({"WF_SCRIPT_MISSING", node.id, "脚本节点必须配置脚本内容"});
        }
        if (!node.script.language.empty() && node.script.language != "wfscript" &&
            node.script.language != "groovy" && node.script.language != "javascript" &&
            node.script.language != "expr") {
          result.warnings.push_back(
              {"WF_SCRIPT_LANGUAGE", node.id, "脚本语言将被按内置 wfscript 解析：" + node.script.language});
        }
        break;
      }
      case NodeType::SubProcess: {
        if (node.subProcessCode.empty()) {
          result.errors.push_back({"WF_SUBPROCESS_MISSING", node.id, "子流程节点必须指定 processCode"});
        } else if (node.subProcessCode == definition.code) {
          result.errors.push_back({"WF_SUBPROCESS_RECURSIVE", node.id, "子流程不能调用自身"});
        }
        break;
      }
      case NodeType::WaitTask: {
        if (node.eventKey.empty()) {
          result.errors.push_back({"WF_WAIT_EVENT_MISSING", node.id, "等待节点必须配置 eventKey"});
        }
        break;
      }
      case NodeType::TimerTask: {
        long long millis = 0;
        if (node.duration.empty() || !node.durationMillis(&millis)) {
          result.errors.push_back({"WF_DURATION_INVALID", node.id,
                                   "定时节点必须配置合法的 duration（如 PT5M / 30m）"});
        }
        break;
      }
      case NodeType::MessageTask: {
        if (node.messageChannel.empty()) {
          result.errors.push_back({"WF_MESSAGE_CHANNEL_MISSING", node.id, "消息节点必须配置发送渠道"});
        }
        break;
      }
      case NodeType::Start:
      case NodeType::End:
      case NodeType::Unknown:
        break;
    }

    if (node.type != NodeType::End && node.type != NodeType::Unknown && out.empty()) {
      result.errors.push_back({"WF_DEAD_END", node.id, "非结束节点必须至少有一条出线"});
    }

    for (const auto& permission : node.fieldPermissions) {
      if (permission.field.empty()) {
        result.errors.push_back({"WF_FIELD_PERMISSION", node.id, "表单字段权限缺少 field 名称"});
      }
    }
  }

  return result;
}

}  // namespace wf

#include "wf/engine.hpp"

#include <algorithm>
#include <cctype>
#include <sstream>

#include "wf/expression.hpp"
#include "wf/script.hpp"

namespace wf {
namespace {

using json::Value;

void collectListeners(const ProcessDefinition& definition, const NodeDefinition* node, EventType type,
                      Value* out) {
  auto append = [&](const ListenerSpec& listener) {
    if (listener.event != type) return;
    out->push_back(listener.toJson());
  };
  if (node != nullptr) {
    for (const auto& listener : node->listeners) append(listener);
  }
  if (definition.config.contains("events")) {
    for (const auto& item : definition.config.at("events").items()) {
      append(ListenerSpec::fromJson(item));
    }
  }
}

Value nodeRef(const NodeDefinition* node) {
  Value reference = Value::object();
  if (node == nullptr) return reference;
  reference.set("id", Value(node->id));
  reference.set("name", Value(node->name));
  reference.set("type", Value(toString(node->type)));
  return reference;
}

Value objectWith(const std::string& key, Value value) {
  Value out = Value::object();
  out.set(key, std::move(value));
  return out;
}

Value taskIdsAppend(const Value& current, long long taskId) {
  Value out = current.isArray() ? current : Value::array();
  out.push_back(Value(taskId));
  return out;
}

std::string upperCopy(const std::string& text) {
  std::string out = text;
  std::transform(out.begin(), out.end(), out.begin(),
                 [](unsigned char c) { return static_cast<char>(std::toupper(c)); });
  return out;
}

}  // namespace

// ---------------------------------------------------------------------------
// 实例锁
// ---------------------------------------------------------------------------
ProcessEngine::InstanceLock::InstanceLock(ProcessEngine& engine, long long instanceId) {
  {
    std::lock_guard<std::mutex> guard(engine.locksMutex_);
    auto it = engine.locks_.find(instanceId);
    if (it == engine.locks_.end()) {
      it = engine.locks_.emplace(instanceId, std::make_shared<std::recursive_mutex>()).first;
    }
    mutex_ = it->second;
  }
  guard_.reset(new std::unique_lock<std::recursive_mutex>(*mutex_));
}

ProcessEngine::InstanceLock::~InstanceLock() = default;

ProcessEngine::ProcessEngine(Repository& repository, Clock& clock, EngineOptions options)
    : repository_(repository), clock_(clock), options_(std::move(options)) {
  organization_ = &defaultOrganization_;
}

ProcessEngine::~ProcessEngine() = default;

// ---------------------------------------------------------------------------
// 流程定义
// ---------------------------------------------------------------------------
ValidationResult ProcessEngine::validateDefinition(const ProcessDefinition& definition) const {
  return wf::validateDefinition(definition);
}

ProcessDefinition ProcessEngine::deployDefinition(const ProcessDefinition& definition, bool publish) {
  ValidationResult validation = validateDefinition(definition);
  if (!validation.ok()) {
    throw std::runtime_error("流程定义校验失败：" + validation.summary());
  }
  ProcessDefinition stored = definition;
  stored.status = publish ? DefinitionStatus::Published : DefinitionStatus::Draft;
  stored.updatedAt = clock_.now();
  stored = repository_.saveDefinition(stored);
  if (publish) {
    repository_.setDefinitionStatus(stored.id, DefinitionStatus::Published);
    stored.status = DefinitionStatus::Published;
  }
  return stored;
}

const ProcessDefinition* ProcessEngine::definitionOf(const ProcessInstance& instance) const {
  Repository& repository = const_cast<Repository&>(repository_);
  return repository.findDefinition(instance.tenantId, instance.processCode,
                                   instance.definitionVersion);
}

const ProcessDefinition* ProcessEngine::definitionFor(const ProcessInstance& instance) const {
  Repository& repository = const_cast<Repository&>(repository_);
  const ProcessDefinition* definition = definitionOf(instance);
  if (definition == nullptr) {
    definition = repository.findDefinition(instance.tenantId, instance.processCode, 0);
  }
  return definition;
}

// ---------------------------------------------------------------------------
// 表达式上下文与函数
// ---------------------------------------------------------------------------
Value ProcessEngine::expressionContext(const ProcessInstance& instance, const NodeDefinition* node) const {
  const Value& variables = instance.variables.isObject() ? instance.variables : Value();
  Value root = variables.isObject() ? variables : Value::object();
  root.set("variables", variables.isObject() ? variables : Value::object());
  if (variables.contains("form")) {
    root.set("form", variables.at("form"));
  } else {
    root.set("form", variables.isObject() ? variables : Value::object());
  }
  if (!root.contains("initiator") || root.at("initiator").isNull()) {
    root.set("initiator", Value(instance.initiator));
  }
  Value process = Value::object();
  process.set("id", Value(instance.id));
  process.set("processCode", Value(instance.processCode));
  process.set("businessKey", Value(instance.businessKey));
  process.set("status", Value(toString(instance.status)));
  process.set("currentNodeId", Value(instance.currentNodeId));
  process.set("startTime", Value(instance.startTime));
  root.set("processInstance", process);
  root.set("currentNode", nodeRef(node));
  if (definitionOf(instance) != nullptr && definitionOf(instance)->config.contains("businessData")) {
    root.set("businessData", definitionOf(instance)->config.at("businessData"));
  } else {
    root.set("businessData", Value::object());
  }
  return root;
}

expr::Functions ProcessEngine::functionsFor(const ProcessInstance& instance) const {
  expr::Functions functions = expr::builtinFunctions();
  OrganizationService* organization = organization_;
  if (organization == nullptr) return functions;

  expr::registerFunction(functions, "hasRole", [organization](const std::vector<Value>& args) {
    if (args.size() < 2) return Value(false);
    return Value(organization->hasRole(args[0].toString(), args[1].toString()));
  });
  expr::registerFunction(functions, "leader", [organization](const std::vector<Value>& args) {
    if (args.empty()) return Value(std::string());
    const int level = args.size() > 1 ? static_cast<int>(args[1].asInt()) : 1;
    return Value(organization->leaderOf(args[0].toString(), level));
  });
  expr::registerFunction(functions, "department", [organization](const std::vector<Value>& args) {
    return Value(args.empty() ? std::string() : organization->departmentOf(args[0].toString()));
  });
  expr::registerFunction(functions, "usersByRole", [organization](const std::vector<Value>& args) {
    Value out = Value::array();
    if (args.empty()) return out;
    for (const auto& user : organization->usersByRole(args[0].toString())) out.push_back(Value(user));
    return out;
  });
  expr::registerFunction(functions, "userExists", [organization](const std::vector<Value>& args) {
    return Value(!args.empty() && organization->exists(args[0].toString()));
  });
  expr::registerFunction(functions, "initiator", [initiator = instance.initiator](const std::vector<Value>&) {
    return Value(initiator);
  });
  return functions;
}

// ---------------------------------------------------------------------------
// 基础工具
// ---------------------------------------------------------------------------
bool ProcessEngine::updateInstance(ProcessInstance& instance, const std::string& what) {
  const bool ok = repository_.updateInstance(instance, -1);
  if (ok) {
    if (ProcessInstance* stored = repository_.findInstance(instance.id)) {
      instance.version = stored->version;
    }
  } else {
    std::ostringstream message;
    message << "流程实例更新失败(" << what << "): id=" << instance.id;
    instance.errorMessage = message.str();
  }
  return ok;
}

void ProcessEngine::mergeVariables(ProcessInstance& instance, const json::Value& patch) {
  if (patch.isNull()) return;
  if (!instance.variables.isObject()) instance.variables = Value::object();
  Value::merge(instance.variables, patch);
}

HistoryRecord ProcessEngine::recordHistory(ProcessInstance& instance, const NodeDefinition* node,
                                           const NodeInstance* nodeInstance, const Task* task,
                                           const std::string& action, const std::string& user,
                                           const std::string& comment, Value input, Value output) {
  HistoryRecord record;
  record.processInstanceId = instance.id;
  record.nodeId = node != nullptr ? node->id : "process";
  record.nodeName = node != nullptr ? node->name : "流程";
  record.nodeInstanceId = nodeInstance != nullptr ? nodeInstance->id : 0;
  record.taskId = task != nullptr ? task->id : 0;
  record.action = action;
  record.operatorUser = user;
  record.comment = comment;
  record.input = std::move(input);
  record.output = std::move(output);
  record.createdAt = clock_.now();
  record.traceId = "trace-" + std::to_string(instance.id) + "-" + std::to_string(record.createdAt);
  return repository_.appendHistory(record);
}

EventRecord ProcessEngine::publishEvent(ProcessInstance& instance, const NodeDefinition* node,
                                        const NodeInstance* nodeInstance, const Task* task,
                                        EventType type, Value payload) {
  const ProcessDefinition* definition = definitionFor(instance);
  Value listeners = Value::array();
  if (definition != nullptr) {
    collectListeners(*definition, node, type, &listeners);
  }
  Value body = Value::object();
  body.set("eventType", Value(toString(type)));
  body.set("processInstanceId", Value(instance.id));
  body.set("processCode", Value(instance.processCode));
  body.set("businessKey", Value(instance.businessKey));
  body.set("tenantId", Value(instance.tenantId));
  body.set("status", Value(toString(instance.status)));
  if (node != nullptr) body.set("nodeId", Value(node->id));
  if (nodeInstance != nullptr) body.set("nodeInstanceId", Value(nodeInstance->id));
  if (task != nullptr) {
    body.set("taskId", Value(task->id));
    body.set("assignee", Value(task->assignee));
  }
  body.set("timestamp", Value(clock_.now()));
  if (!payload.isNull()) body.set("data", payload);

  EventRecord event;
  event.processInstanceId = instance.id;
  event.nodeInstanceId = nodeInstance != nullptr ? nodeInstance->id : 0;
  event.taskId = task != nullptr ? task->id : 0;
  event.eventType = type;
  event.payload = body;
  event.listeners = listeners;
  event.createdAt = clock_.now();
  event.updatedAt = event.createdAt;
  event.nextRetryTime = event.createdAt;
  event.status = listeners.empty() ? "SKIPPED" : "PENDING";
  return repository_.createEvent(event);
}

TimerJob ProcessEngine::scheduleTimer(ProcessInstance& instance, const NodeInstance* nodeInstance,
                                      const Task* task, const std::string& jobType,
                                      long long triggerTime, Value payload) {
  TimerJob job;
  job.processInstanceId = instance.id;
  job.nodeInstanceId = nodeInstance != nullptr ? nodeInstance->id : 0;
  job.taskId = task != nullptr ? task->id : 0;
  job.jobType = jobType;
  job.triggerTime = triggerTime;
  job.payload = std::move(payload);
  job.createdAt = clock_.now();
  return repository_.createTimer(job);
}

Value ProcessEngine::mergeTimeoutPayload(const Value& current, const TimeoutPolicy& policy) const {
  Value payload = current.isObject() ? current : Value::object();
  payload.set("duration", Value(policy.duration));
  Value steps = Value::array();
  for (const auto& step : policy.steps) steps.push_back(step.toJson());
  payload.set("actions", steps);
  return payload;
}

void ProcessEngine::cancelTask(Task& task, const std::string& reason) {
  if (!isOpen(task.status)) return;
  Task updated = task;
  updated.status = TaskStatus::Cancelled;
  updated.comment = reason;
  updated.completeTime = clock_.now();
  repository_.updateTask(updated, -1);
  task = *repository_.findTask(task.id);
}

void ProcessEngine::cancelOpenTasks(long long instanceId, const NodeDefinition* node,
                                    const std::string& reason) {
  for (Task task : repository_.tasksByInstance(instanceId)) {
    if (node != nullptr && task.nodeId != node->id) continue;
    if (!isOpen(task.status)) continue;
    cancelTask(task, reason);
  }
  ProcessInstance* instance = repository_.findInstance(instanceId);
  if (instance == nullptr) return;
  for (NodeInstance nodeInstance : repository_.openNodeInstances(instanceId)) {
    if (node != nullptr && nodeInstance.nodeId != node->id) continue;
    NodeInstance updated = nodeInstance;
    updated.status = NodeInstanceStatus::Cancelled;
    updated.endTime = clock_.now();
    updated.errorMessage = reason;
    repository_.updateNodeInstance(updated);
    (void)instance;
  }
}

// ---------------------------------------------------------------------------
// 审批人解析（PRD §9）
// ---------------------------------------------------------------------------
std::vector<std::string> ProcessEngine::resolveRule(const ApprovalRule& rule,
                                                    const ProcessInstance& instance,
                                                    const NodeDefinition& node) {
  (void)node;
  std::vector<std::string> out;
  const Value context = expressionContext(instance);
  const expr::Functions functions = functionsFor(instance);
  const std::string type = toLower(rule.type);

  auto push = [&out](const std::string& user) {
    if (user.empty()) return;
    if (std::find(out.begin(), out.end(), user) != out.end()) return;
    out.push_back(user);
  };

  if (type == "initiator") {
    push(instance.initiator);
  } else if (type == "user") {
    push(rule.value);
  } else if (type == "users") {
    for (const auto& user : rule.users) push(user);
  } else if (type == "role") {
    if (organization_ != nullptr) {
      for (const auto& user : organization_->usersByRole(rule.value)) push(user);
    }
  } else if (type == "roles") {
    if (organization_ != nullptr) {
      for (const auto& role : rule.users) {
        for (const auto& user : organization_->usersByRole(role)) push(user);
      }
    }
  } else if (type == "position") {
    if (organization_ != nullptr) {
      for (const auto& user : organization_->usersByPosition(rule.value)) push(user);
    }
  } else if (type == "leader" || type == "initiator_leader") {
    // value 为空或显式写成 LEADER/INITIATOR/SELF 时都以发起人为基准
    const std::string upper = upperCopy(rule.value);
    std::string base = rule.value;
    if (base.empty() || upper == "LEADER" || upper == "INITIATOR" || upper == "SELF") {
      base = instance.initiator;
    }
    const std::string resolved =
        organization_ != nullptr ? organization_->leaderOf(base, std::max(1, rule.level)) : std::string();
    push(resolved);
  } else if (type == "expression") {
    const Value value = expr::evaluateValue(rule.value, context, functions);
    if (value.isArray()) {
      for (const auto& item : value.items()) push(item.toString());
    } else {
      push(value.toString());
    }
  } else if (type == "prev_assignee" || type == "last_assignee") {
    const std::vector<HistoryRecord> history = repository_.histories(instance.id);
    for (auto it = history.rbegin(); it != history.rend(); ++it) {
      if (it->action == "APPROVE" && !it->operatorUser.empty()) {
        push(it->operatorUser);
        break;
      }
    }
  } else if (type == "combination") {
    std::vector<std::vector<std::string>> groups;
    for (const auto& child : rule.rules) {
      groups.push_back(resolveRule(child, instance, node));
    }
    const bool anyMode = upperCopy(rule.op) == "OR";
    if (anyMode) {
      for (const auto& group : groups) {
        for (const auto& user : group) push(user);
      }
    } else {
      // AND: 各组结果取交集；交集为空时退化为并集，避免流程卡死
      std::vector<std::string> intersection;
      bool first = true;
      for (const auto& group : groups) {
        if (first) {
          intersection = group;
          first = false;
          continue;
        }
        std::vector<std::string> next;
        for (const auto& user : group) {
          if (std::find(intersection.begin(), intersection.end(), user) != intersection.end()) {
            next.push_back(user);
          }
        }
        intersection = next;
      }
      if (intersection.empty()) {
        for (const auto& group : groups) {
          for (const auto& user : group) push(user);
        }
      } else {
        for (const auto& user : intersection) push(user);
      }
    }
  }
  return out;
}

std::vector<std::string> ProcessEngine::resolveAssignees(const ProcessInstance& instance,
                                                         const NodeDefinition& node,
                                                         const NodeInstance* previous) {
  if (node.assignee.empty()) return {};
  std::vector<std::string> out = resolveRule(node.assignee, instance, node);
  if (out.empty() && node.assignee.type == "initiator" && !instance.initiator.empty()) {
    out.push_back(instance.initiator);
  }
  if (out.empty() && previous != nullptr && !previous->assignee.empty()) {
    // 兜底：沿用上一节点处理人，避免出现无人可办的死任务
    out.push_back(previous->assignee);
  }
  return out;
}

// ---------------------------------------------------------------------------
// 连线选择（PRD §7.4）
// ---------------------------------------------------------------------------
std::vector<std::string> ProcessEngine::nextNodes(ProcessInstance& instance,
                                                  const ProcessDefinition& definition,
                                                  const NodeDefinition& node,
                                                  const EdgeDefinition** chosen,
                                                  std::string* error) {
  const std::vector<const EdgeDefinition*> edges = definition.outEdges(node.id);
  std::vector<std::string> out;
  if (edges.empty()) return out;

  if (edges.size() == 1 && edges.front()->condition.empty()) {
    if (chosen != nullptr) *chosen = edges.front();
    out.push_back(edges.front()->target);
    return out;
  }

  const Value context = expressionContext(instance, &node);
  const expr::Functions functions = functionsFor(instance);
  const EdgeDefinition* fallback = nullptr;

  for (const EdgeDefinition* edge : edges) {
    if (edge->isDefault) {
      if (fallback == nullptr) fallback = edge;
      continue;
    }
    if (edge->condition.empty()) {
      if (node.type == NodeType::ExclusiveGateway) {
        // 无条件分支在条件网关上按默认分支处理
        if (fallback == nullptr) fallback = edge;
        continue;
      }
      out.push_back(edge->target);
      continue;
    }
    std::string evaluationError;
    const bool matched = expr::evaluateCondition(edge->condition, context, functions, false,
                                                &evaluationError);
    if (!evaluationError.empty() && error != nullptr && error->empty()) {
      *error = "表达式求值失败：" + edge->condition + " (" + evaluationError + ")";
    }
    if (matched) {
      if (chosen != nullptr) *chosen = edge;
      if (node.type == NodeType::ExclusiveGateway) {
        return {edge->target};
      }
      out.push_back(edge->target);
    }
  }

  if (out.empty() && fallback != nullptr) {
    if (chosen != nullptr) *chosen = fallback;
    out.push_back(fallback->target);
    return out;
  }
  if (out.empty() && node.type == NodeType::ExclusiveGateway && error != nullptr) {
    *error = "条件网关没有匹配的分支，且未配置默认分支";
  }
  return out;
}

// ---------------------------------------------------------------------------
// 并行汇聚（PRD §24）
// ---------------------------------------------------------------------------
bool ProcessEngine::hasJoinSemantics(const ProcessDefinition& definition, const NodeDefinition& node) const {
  if (node.joinAll) return true;
  return node.type == NodeType::ParallelGateway && definition.fanIn(node.id) > 1;
}

bool ProcessEngine::allBranchesArrived(ProcessInstance& instance, const ProcessDefinition& definition,
                                       const NodeDefinition& node) {
  const std::vector<const EdgeDefinition*> incoming = definition.inEdges(node.id);
  if (incoming.size() <= 1) return true;
  const std::vector<ParallelBranch> branches = repository_.branches(instance.id, node.id);
  for (const EdgeDefinition* edge : incoming) {
    bool arrived = false;
    for (const auto& branch : branches) {
      if (branch.branchId == edge->id && branch.status == "ARRIVED") {
        arrived = true;
        break;
      }
    }
    if (!arrived) return false;
  }
  return true;
}

// ---------------------------------------------------------------------------
// 执行循环
// ---------------------------------------------------------------------------
bool ProcessEngine::drainQueue(ProcessInstance& instance, const ProcessDefinition& definition,
                               std::vector<Token> queue, RunContext& context) {
  // 令牌计数由压栈方维护：初始队列由调用者计数，流转中新产生的令牌在 completeNode 里计数
  while (!queue.empty()) {
    if (++context.steps > options_.maxStepsPerRun) {
      instance.errorMessage = "流程步数超过上限（可能存在死循环）";
      publishEvent(instance, nullptr, nullptr, nullptr, EventType::ProcessTerminated,
                   Value(instance.errorMessage));
      return finishInstance(instance, definition, ProcessStatus::Error, "ERROR", context);
    }
    const Token token = queue.back();
    queue.pop_back();
    instance.activeTokens -= 1;
    const NodeDefinition* node = definition.findNode(token.nodeId);
    if (node == nullptr) {
      instance.errorMessage = "节点不存在：" + token.nodeId;
      return finishInstance(instance, definition, ProcessStatus::Error, "ERROR", context);
    }
    if (!executeNode(instance, definition, *node, token, queue, context)) return false;
    if (isTerminal(instance.status)) {
      instance.activeTokens = 0;
      updateInstance(instance, "terminal");
      return true;
    }
  }

  if (instance.status == ProcessStatus::Running) {
    const std::vector<NodeInstance> open = repository_.openNodeInstances(instance.id);
    if (open.empty() && instance.activeTokens <= 0) {
      const std::string result = instance.result.empty() ? "COMPLETED" : instance.result;
      return finishInstance(instance, definition, ProcessStatus::Completed, result, context);
    }
    updateInstance(instance, "wait");
  }
  return true;
}

bool ProcessEngine::executeNode(ProcessInstance& instance, const ProcessDefinition& definition,
                                const NodeDefinition& node, const Token& token,
                                std::vector<Token>& queue, RunContext& context) {
  const long long now = clock_.now();
  instance.currentNodeId = node.id;

  // 汇聚等待：所有入线到齐后才执行（PRD §24）
  if (hasJoinSemantics(definition, node) && definition.fanIn(node.id) > 1) {
    bool recorded = false;
    for (ParallelBranch branch : repository_.branches(instance.id, node.id)) {
      if (branch.branchId != token.edgeId) continue;
      branch.status = "ARRIVED";
      branch.completedAt = 0;
      repository_.updateBranch(branch);
      recorded = true;
      break;
    }
    if (!recorded) {
      ParallelBranch branch;
      branch.processInstanceId = instance.id;
      branch.gatewayNodeId = node.id;
      branch.branchId = token.edgeId;
      branch.status = "ARRIVED";
      repository_.createBranch(branch);
    }
    if (!allBranchesArrived(instance, definition, node)) {
      instance.activeTokens += 1;  // 该令牌在汇聚点挂起
      publishEvent(instance, &node, nullptr, nullptr, EventType::NodeWaiting,
                   objectWith("reason", Value("等待并行分支汇聚")));
      updateInstance(instance, "join-wait");
      return true;
    }
    // 分支到齐：释放之前挂起的令牌
    const int held = static_cast<int>(definition.inEdges(node.id).size()) - 1;
    instance.activeTokens = std::max(0, instance.activeTokens - held);
    for (ParallelBranch branch : repository_.branches(instance.id, node.id)) {
      branch.status = "COMPLETED";
      branch.completedAt = now;
      repository_.updateBranch(branch);
    }
  }

  NodeInstance nodeInstance;
  nodeInstance.processInstanceId = instance.id;
  nodeInstance.nodeId = node.id;
  nodeInstance.nodeName = node.name;
  nodeInstance.nodeType = node.type;
  nodeInstance.status = NodeInstanceStatus::Running;
  nodeInstance.startTime = now;
  nodeInstance.input = instance.variables;
  nodeInstance = repository_.createNodeInstance(nodeInstance);
  publishEvent(instance, &node, &nodeInstance, nullptr, EventType::NodeStarted, instance.variables);

  const Value evalContext = expressionContext(instance, &node);
  const expr::Functions functions = functionsFor(instance);

  switch (node.type) {
    case NodeType::Start: {
      Value output = Value::object();
      output.set("startedBy", Value(instance.initiator));
      return completeNode(instance, definition, nodeInstance, node, output, queue, context);
    }

    case NodeType::End: {
      Value output = Value::object();
      output.set("endedAt", Value(now));
      recordHistory(instance, &node, &nodeInstance, nullptr, "END", options_.systemUser, "",
                    instance.variables, Value());
      return completeNode(instance, definition, nodeInstance, node, output, queue, context);
    }

    case NodeType::UserTask: {
      std::vector<std::string> assignees = resolveAssignees(instance, node, nullptr);
      std::string error;
      Value output = Value::object();
      if (node.strategy == CompleteStrategy::Sequence) {
        Value sequenceQueue = Value::array();
        for (std::size_t i = 1; i < assignees.size(); ++i) sequenceQueue.push_back(Value(assignees[i]));
        output.set("sequenceQueue", sequenceQueue);
        if (!assignees.empty()) assignees.resize(1);
      }
      if (assignees.empty()) {
        // 候选人为空：生成一个无人认领的任务（候选池）
        assignees.push_back("");
      }
      for (const auto& assignee : assignees) {
        Task task = makeTask(instance, nodeInstance, node, assignee,
                             node.strategy == CompleteStrategy::Sequence ? "SEQUENCE" : "NORMAL", 0,
                             node.strategy);
        scheduleTaskTimeout(instance, node, nodeInstance, task);
        context.createdTasks.push_back(task);
        output.set("taskIds", taskIdsAppend(output.at("taskIds"), task.id));
      }
      scheduleNodeTimeout(instance, node, nodeInstance);
      NodeInstance waiting = nodeInstance;
      waiting.status = NodeInstanceStatus::Waiting;
      waiting.output = output;
      waiting.waitKey = "tasks";
      repository_.updateNodeInstance(waiting);
      publishEvent(instance, &node, &waiting, nullptr, EventType::NodeWaiting, output);
      updateInstance(instance, "user-task");
      (void)error;
      return true;
    }

    case NodeType::ServiceTask:
    case NodeType::MessageTask: {
      if (node.type == NodeType::MessageTask) {
        Value payload = expr::renderValue(
            node.properties.contains("payload") ? node.properties.at("payload") : Value::object(),
            evalContext, functions);
        if (notifier_ != nullptr) {
          notifier_->send(node.messageChannel.empty() ? "INNER" : node.messageChannel,
                          expr::interpolate(node.messageTarget, evalContext, functions).toString(),
                          node.messageTemplate, payload);
        }
        Value output = Value::object();
        output.set("channel", Value(node.messageChannel));
        output.set("target", Value(node.messageTarget));
        return completeNode(instance, definition, nodeInstance, node, output, queue, context);
      }

      if (invoker_ == nullptr) {
        return failNode(instance, definition, node, nodeInstance, "未配置服务调用器", context);
      }
      const int maxAttempts = std::max(1, node.service.retry.maxAttempts);
      ServiceResponse response;
      int attempt = 0;
      for (; attempt < maxAttempts; ++attempt) {
        response = invoker_->invoke(node.service, evalContext);
        if (response.ok) break;
        if (attempt + 1 < maxAttempts) {
          publishEvent(instance, &node, &nodeInstance, nullptr, EventType::ServiceRetried,
                       objectWith("attempt", Value(attempt + 1)));
          recordHistory(instance, &node, &nodeInstance, nullptr, "SERVICE_RETRIED",
                        options_.systemUser,
                        "第 " + std::to_string(attempt + 1) + " 次调用失败：" + response.error,
                        Value(), Value());
          if (node.service.retry.backoffMillis > 0) {
            scheduleTimer(instance, &nodeInstance, nullptr, "SERVICE_RETRY",
                          clock_.now() + node.service.retry.backoffMillis * (attempt + 1),
                          Value(Value::object()));
          }
        }
      }
      if (!response.ok) {
        std::string message = "服务调用失败：" + node.service.url + " (" + response.error + ")";
        return failNode(instance, definition, node, nodeInstance, message, context);
      }
      Value output = Value::object();
      output.set("status", Value(response.status));
      output.set("response", response.body);
      output.set("attempts", Value(attempt + 1));
      if (!node.service.resultVariable.empty()) {
        Value patch = Value::object();
        patch.set(node.service.resultVariable, response.body);
        mergeVariables(instance, patch);
      }
      return completeNode(instance, definition, nodeInstance, node, output, queue, context);
    }

    case NodeType::ScriptTask: {
      Value working = instance.variables.isObject() ? instance.variables : Value::object();
      const script::ScriptOutcome outcome = script::run(node.script.content, working, functions);
      if (!outcome.ok) {
        return failNode(instance, definition, node, nodeInstance, "脚本执行失败：" + outcome.error,
                        context);
      }
      instance.variables = working;
      Value output = Value::object();
      if (outcome.hasReturn) output.set("return", outcome.returnValue);
      return completeNode(instance, definition, nodeInstance, node, output, queue, context);
    }

    case NodeType::ExclusiveGateway: {
      std::string error;
      const EdgeDefinition* chosen = nullptr;
      const std::vector<std::string> targets = nextNodes(instance, definition, node, &chosen, &error);
      if (!error.empty()) {
        return failNode(instance, definition, node, nodeInstance, error, context);
      }
      Value output = Value::object();
      if (chosen != nullptr) {
        output.set("branch", Value(chosen->id));
        output.set("target", Value(chosen->target));
      }
      return completeNode(instance, definition, nodeInstance, node, output, queue, context);
    }

    case NodeType::ParallelGateway: {
      const std::vector<const EdgeDefinition*> edges = definition.outEdges(node.id);
      Value branches = Value::array();
      for (const EdgeDefinition* edge : edges) {
        ParallelBranch branch;
        branch.processInstanceId = instance.id;
        branch.gatewayNodeId = node.id;
        branch.branchId = edge->id;
        branch.status = "RUNNING";
        repository_.createBranch(branch);
        branches.push_back(Value(edge->id));
      }
      Value output = Value::object();
      output.set("branchCount", Value(static_cast<long long>(edges.size())));
      output.set("branches", branches);
      return completeNode(instance, definition, nodeInstance, node, output, queue, context);
    }

    case NodeType::SubProcess: {
      StartRequest child;
      child.tenantId = instance.tenantId;
      child.processCode = node.subProcessCode;
      child.businessKey = instance.businessKey.empty()
                              ? ("child-" + std::to_string(instance.id) + "-" + node.id)
                              : (instance.businessKey + "::" + node.id);
      child.initiator = instance.initiator;
      child.variables = node.subProcessInput.isNull()
                            ? Value::object()
                            : expr::renderValue(node.subProcessInput, evalContext, functions);
      child.parentInstanceId = instance.id;
      child.parentNodeInstanceId = nodeInstance.id;
      OperationResult childResult = startProcess(child);
      if (!childResult.ok) {
        return failNode(instance, definition, node, nodeInstance,
                        "子流程启动失败：" + childResult.error, context);
      }
      Value output = Value::object();
      output.set("childInstanceId", Value(childResult.instance.id));
      output.set("childProcessCode", Value(node.subProcessCode));
      output.set("async", Value(node.subProcessAsync));
      // 同步子流程产生的待办要一起返回给调用方，否则业务侧看不到子流程任务
      for (const Task& childTask : childResult.createdTasks) {
        context.createdTasks.push_back(childTask);
      }
      if (node.subProcessAsync) {
        return completeNode(instance, definition, nodeInstance, node, output, queue, context);
      }
      NodeInstance waiting = nodeInstance;
      waiting.status = NodeInstanceStatus::Waiting;
      waiting.output = output;
      waiting.waitKey = "subprocess";
      repository_.updateNodeInstance(waiting);
      publishEvent(instance, &node, &waiting, nullptr, EventType::NodeWaiting, output);
      updateInstance(instance, "subprocess");
      return true;
    }

    case NodeType::WaitTask: {
      NodeInstance waiting = nodeInstance;
      waiting.status = NodeInstanceStatus::Waiting;
      waiting.waitKey = node.eventKey;
      repository_.updateNodeInstance(waiting);
      publishEvent(instance, &node, &waiting, nullptr, EventType::NodeWaiting,
                   objectWith("eventKey", Value(node.eventKey)));
      updateInstance(instance, "wait-event");
      return true;
    }

    case NodeType::TimerTask: {
      long long millis = 0;
      if (!node.durationMillis(&millis)) {
        return failNode(instance, definition, node, nodeInstance,
                        "定时节点时长非法：" + node.duration, context);
      }
      Value payload = Value::object();
      payload.set("nodeId", Value(node.id));
      payload.set("duration", Value(node.duration));
      scheduleTimer(instance, &nodeInstance, nullptr, "NODE_TIMER", clock_.now() + millis, payload);
      NodeInstance waiting = nodeInstance;
      waiting.status = NodeInstanceStatus::Waiting;
      waiting.waitKey = "timer";
      repository_.updateNodeInstance(waiting);
      publishEvent(instance, &node, &waiting, nullptr, EventType::NodeWaiting,
                   objectWith("delay", Value(node.duration)));
      updateInstance(instance, "timer");
      return true;
    }

    case NodeType::Unknown:
      return failNode(instance, definition, node, nodeInstance, "未知节点类型", context);
  }
  return failNode(instance, definition, node, nodeInstance, "未支持的节点类型", context);
}

bool ProcessEngine::completeNode(ProcessInstance& instance, const ProcessDefinition& definition,
                                 NodeInstance nodeInstance, const NodeDefinition& node,
                                 const json::Value& output, std::vector<Token>& queue,
                                 RunContext& context) {
  const long long now = clock_.now();
  if (options_.enforceStateMachine && !canTransit(nodeInstance.status, NodeInstanceStatus::Completed)) {
    return failNode(instance, definition, node, nodeInstance, "节点状态不允许完成", context);
  }
  NodeInstance updated = nodeInstance;
  updated.status = NodeInstanceStatus::Completed;
  updated.endTime = now;
  if (!output.isNull()) updated.output = output;
  repository_.updateNodeInstance(updated);
  // 用户任务的输出只用于节点实例留痕（taskIds / sequenceQueue），不污染流程变量
  if (node.type != NodeType::UserTask) mergeVariables(instance, updated.output);
  recordHistory(instance, &node, &updated, nullptr, "NODE_COMPLETE", options_.systemUser, "",
                updated.input, updated.output);
  publishEvent(instance, &node, &updated, nullptr, EventType::NodeCompleted, updated.output);

  std::string error;
  const EdgeDefinition* chosen = nullptr;
  const std::vector<std::string> targets = nextNodes(instance, definition, node, &chosen, &error);
  if (!error.empty()) {
    return failNode(instance, definition, node, updated, error, context);
  }
  for (const std::string& target : targets) {
    instance.activeTokens += 1;
    Token next;
    next.nodeId = target;
    next.edgeId = chosen != nullptr ? chosen->id : std::string();
    if (node.type != NodeType::ExclusiveGateway) {
      for (const EdgeDefinition* edge : definition.outEdges(node.id)) {
        if (edge->target == target) {
          next.edgeId = edge->id;
          break;
        }
      }
    }
    queue.push_back(next);
  }
  updateInstance(instance, "node-complete");
  return true;
}

bool ProcessEngine::failNode(ProcessInstance& instance, const ProcessDefinition& definition,
                             const NodeDefinition& node, NodeInstance nodeInstance,
                             const std::string& message, RunContext& context) {
  NodeInstance failed = nodeInstance;
  failed.status = NodeInstanceStatus::Failed;
  failed.endTime = clock_.now();
  failed.errorMessage = message;
  failed.attempt += 1;
  repository_.updateNodeInstance(failed);
  recordHistory(instance, &node, &failed, nullptr, "NODE_FAILED", options_.systemUser, message,
                failed.input, failed.output);
  publishEvent(instance, &node, &failed, nullptr, EventType::NodeFailed,
               objectWith("error", Value(message)));
  cancelOpenTasks(instance.id, &node, "节点执行失败");
  return finishInstance(instance, definition, ProcessStatus::Error, "ERROR", context);
}

bool ProcessEngine::resumeNodeInstance(ProcessInstance& instance, const ProcessDefinition& definition,
                                       NodeInstance& nodeInstance, const json::Value& output,
                                       RunContext& context) {
  const NodeDefinition* node = definition.findNode(nodeInstance.nodeId);
  if (node == nullptr) {
    instance.errorMessage = "节点不存在：" + nodeInstance.nodeId;
    return finishInstance(instance, definition, ProcessStatus::Error, "ERROR", context);
  }
  NodeInstance resumed = nodeInstance;
  resumed.status = NodeInstanceStatus::Running;
  if (!output.isNull()) {
    Value merged = resumed.output.isObject() ? resumed.output : Value::object();
    Value::merge(merged, output);
    resumed.output = merged;
  }
  repository_.updateNodeInstance(resumed);
  recordHistory(instance, node, &resumed, nullptr, "NODE_RESUME", options_.systemUser, "",
                instance.variables, output);

  // 子流程返回：回写声明的输出变量
  if (node->type == NodeType::SubProcess && !node->subProcessOutputs.empty()) {
    if (ProcessInstance* child = repository_.findInstance(output.intOr("childInstanceId", 0))) {
      Value patch = Value::object();
      for (const auto& name : node->subProcessOutputs) {
        const json::Value* value = child->variables.find(name);
        if (value != nullptr) patch.set(name, *value);
      }
      mergeVariables(instance, patch);
    }
  }

  std::vector<Token> queue;
  if (!completeNode(instance, definition, resumed, *node, Value::object(), queue, context)) return false;
  return drainQueue(instance, definition, queue, context);
}

bool ProcessEngine::finishInstance(ProcessInstance& instance, const ProcessDefinition& definition,
                                   ProcessStatus status, const std::string& result,
                                   RunContext& context) {
  (void)definition;
  (void)context;
  if (options_.enforceStateMachine && !canTransit(instance.status, status)) {
    instance.errorMessage = std::string("非法的流程状态流转：") + toString(instance.status) + " -> " +
                            toString(status);
    status = ProcessStatus::Error;
  }
  instance.status = status;
  if (!result.empty()) instance.result = result;
  instance.endTime = clock_.now();
  instance.activeTokens = 0;
  updateInstance(instance, "finish");
  cancelOpenTasks(instance.id, nullptr, std::string("流程已结束：") + toString(status));

  EventType event = EventType::ProcessCompleted;
  if (status == ProcessStatus::Cancelled) event = EventType::ProcessCancelled;
  if (status == ProcessStatus::Terminated) event = EventType::ProcessTerminated;
  Value payload = Value::object();
  payload.set("result", Value(instance.result));
  publishEvent(instance, nullptr, nullptr, nullptr, event, payload);
  recordHistory(instance, nullptr, nullptr, nullptr, toString(status), options_.systemUser,
                instance.errorMessage, Value(), payload);

  if (instance.parentInstanceId != 0) {
    return notifyParent(instance, context);
  }
  return true;
}

bool ProcessEngine::notifyParent(ProcessInstance& child, RunContext& context) {
  ProcessInstance* parent = repository_.findInstance(child.parentInstanceId);
  if (parent == nullptr) return true;
  const ProcessDefinition* definition = definitionFor(*parent);
  if (definition == nullptr) return true;
  NodeInstance* parentNode = repository_.findNodeInstance(child.parentNodeInstanceId);
  if (parentNode == nullptr) return true;
  NodeInstance waiting = *parentNode;
  if (waiting.status != NodeInstanceStatus::Waiting) return true;

  Value output = Value::object();
  output.set("childInstanceId", Value(child.id));
  output.set("childStatus", Value(toString(child.status)));
  output.set("childResult", Value(child.result));

  InstanceLock lock = lockInstance(parent->id);
  ProcessInstance snapshot = *repository_.findInstance(parent->id);
  if (isTerminal(snapshot.status)) return true;
  if (!resumeNodeInstance(snapshot, *definition, waiting, output, context)) return false;
  if (options_.autoDispatchEvents) dispatchEvents();
  return true;
}

// ---------------------------------------------------------------------------
// 流程实例生命周期
// ---------------------------------------------------------------------------
OperationResult ProcessEngine::startProcess(const StartRequest& request) {
  OperationResult result;
  if (request.processCode.empty()) {
    result.error = "processCode 不能为空";
    return result;
  }
  const ProcessDefinition* definition =
      repository_.findDefinition(request.tenantId, request.processCode, request.version);
  if (definition == nullptr) {
    result.error = "未找到可用的流程定义：" + request.processCode;
    return result;
  }

  // 幂等：processCode + businessKey（PRD §22.1）
  if (ProcessInstance* existing = repository_.findInstanceByBusinessKey(
          request.tenantId, request.processCode, request.businessKey)) {
    result.ok = true;
    result.idempotentReplay = true;
    result.instance = *existing;
    for (const Task& task : repository_.tasksByInstance(existing->id)) {
      if (isOpen(task.status)) result.createdTasks.push_back(task);
    }
    return result;
  }

  ProcessInstance instance;
  instance.definitionId = definition->id;
  instance.processCode = definition->code;
  instance.definitionVersion = definition->version;
  instance.tenantId = request.tenantId;
  instance.businessKey = request.businessKey;
  instance.status = ProcessStatus::Running;
  instance.initiator = request.initiator;
  instance.startTime = clock_.now();
  instance.variables = Value::object();
  mergeVariables(instance, definition->variables);
  mergeVariables(instance, request.variables);
  if (!instance.variables.contains("initiator")) {
    instance.variables.set("initiator", Value(request.initiator));
  }
  instance.parentInstanceId = request.parentInstanceId;
  instance.parentNodeInstanceId = request.parentNodeInstanceId;
  instance = repository_.createInstance(instance);

  InstanceLock lock = lockInstance(instance.id);
  RunContext context;
  recordHistory(instance, nullptr, nullptr, nullptr, "START", request.initiator, "",
                instance.variables, Value());
  Value startPayload = Value::object();
  startPayload.set("initiator", Value(request.initiator));
  startPayload.set("businessKey", Value(request.businessKey));
  publishEvent(instance, nullptr, nullptr, nullptr, EventType::ProcessStarted, startPayload);

  const NodeDefinition* start = definition->startNode();
  if (start == nullptr) {
    result.error = "流程定义缺少开始节点";
    finishInstance(instance, *definition, ProcessStatus::Error, "ERROR", context);
    return result;
  }
  std::vector<Token> queue;
  Token token;
  token.nodeId = start->id;
  queue.push_back(token);
  instance.activeTokens += 1;
  if (!drainQueue(instance, *definition, queue, context)) {
    result.error = "流程启动失败：" + instance.errorMessage;
  }
  if (options_.autoDispatchEvents) dispatchEvents();

  if (ProcessInstance* stored = repository_.findInstance(instance.id)) {
    result.instance = *stored;
  }
  result.createdTasks = context.createdTasks;
  result.ok = result.error.empty();
  return result;
}

OperationResult ProcessEngine::cancelInstance(long long instanceId, const std::string& user,
                                              const std::string& comment) {
  OperationResult result;
  InstanceLock lock = lockInstance(instanceId);
  ProcessInstance* stored = repository_.findInstance(instanceId);
  if (stored == nullptr) {
    result.error = "流程实例不存在：" + std::to_string(instanceId);
    return result;
  }
  ProcessInstance instance = *stored;
  if (isTerminal(instance.status)) {
    result.error = std::string("流程已结束（") + toString(instance.status) + "），无法取消";
    return result;
  }
  RunContext context;
  finishInstance(instance, ProcessDefinition(), ProcessStatus::Cancelled, "CANCELLED", context);
  result.instance = *repository_.findInstance(instanceId);
  result.ok = true;
  (void)user;
  (void)comment;
  return result;
}

OperationResult ProcessEngine::suspendInstance(long long instanceId, const std::string& user) {
  OperationResult result;
  InstanceLock lock = lockInstance(instanceId);
  ProcessInstance* stored = repository_.findInstance(instanceId);
  if (stored == nullptr) {
    result.error = "流程实例不存在";
    return result;
  }
  ProcessInstance instance = *stored;
  if (!canTransit(instance.status, ProcessStatus::Suspended)) {
    result.error = std::string("当前状态不允许挂起：") + toString(instance.status);
    return result;
  }
  instance.status = ProcessStatus::Suspended;
  updateInstance(instance, "suspend");
  recordHistory(instance, nullptr, nullptr, nullptr, "SUSPEND", user, "", Value(), Value());
  publishEvent(instance, nullptr, nullptr, nullptr, EventType::ProcessSuspended, Value(Value::object()));
  result.instance = *repository_.findInstance(instanceId);
  result.ok = true;
  if (options_.autoDispatchEvents) dispatchEvents();
  return result;
}

OperationResult ProcessEngine::resumeInstance(long long instanceId, const std::string& user) {
  OperationResult result;
  InstanceLock lock = lockInstance(instanceId);
  ProcessInstance* stored = repository_.findInstance(instanceId);
  if (stored == nullptr) {
    result.error = "流程实例不存在";
    return result;
  }
  ProcessInstance instance = *stored;
  if (!canTransit(instance.status, ProcessStatus::Running)) {
    result.error = std::string("当前状态不允许恢复：") + toString(instance.status);
    return result;
  }
  instance.status = ProcessStatus::Running;
  updateInstance(instance, "resume");
  recordHistory(instance, nullptr, nullptr, nullptr, "RESUME", user, "", Value(), Value());
  publishEvent(instance, nullptr, nullptr, nullptr, EventType::ProcessResumed, Value(Value::object()));
  result.instance = *repository_.findInstance(instanceId);
  result.ok = true;
  if (options_.autoDispatchEvents) dispatchEvents();
  return result;
}

OperationResult ProcessEngine::terminateInstance(long long instanceId, const std::string& user,
                                                 const std::string& comment) {
  OperationResult result;
  InstanceLock lock = lockInstance(instanceId);
  ProcessInstance* stored = repository_.findInstance(instanceId);
  if (stored == nullptr) {
    result.error = "流程实例不存在";
    return result;
  }
  ProcessInstance instance = *stored;
  if (isTerminal(instance.status)) {
    result.error = "流程已结束，无法终止";
    return result;
  }
  RunContext context;
  finishInstance(instance, ProcessDefinition(), ProcessStatus::Terminated, "TERMINATED", context);
  recordHistory(instance, nullptr, nullptr, nullptr, "TERMINATE", user, comment, Value(), Value());
  result.instance = *repository_.findInstance(instanceId);
  result.ok = true;
  if (options_.autoDispatchEvents) dispatchEvents();
  return result;
}

OperationResult ProcessEngine::retryNode(long long instanceId, const std::string& nodeId,
                                         const std::string& user) {
  OperationResult result;
  InstanceLock lock = lockInstance(instanceId);
  ProcessInstance* stored = repository_.findInstance(instanceId);
  if (stored == nullptr) {
    result.error = "流程实例不存在";
    return result;
  }
  ProcessInstance instance = *stored;
  const ProcessDefinition* definition = definitionFor(instance);
  if (definition == nullptr) {
    result.error = "流程定义已不存在";
    return result;
  }
  NodeInstance* nodeInstance = repository_.latestNodeInstance(instanceId, nodeId);
  if (nodeInstance == nullptr) {
    result.error = "节点实例不存在：" + nodeId;
    return result;
  }
  if (nodeInstance->status != NodeInstanceStatus::Failed &&
      nodeInstance->status != NodeInstanceStatus::Cancelled) {
    result.error = "只有失败或已取消的节点可以重试";
    return result;
  }
  NodeInstance retry = *nodeInstance;
  retry.status = NodeInstanceStatus::Cancelled;
  retry.endTime = clock_.now();
  retry.errorMessage = "管理员重试";
  repository_.updateNodeInstance(retry);

  instance.status = ProcessStatus::Running;
  instance.errorMessage.clear();
  instance.result.clear();
  updateInstance(instance, "retry");
  recordHistory(instance, definition->findNode(nodeId), &retry, nullptr, "RETRY", user, "", Value(),
                Value());

  RunContext context;
  std::vector<Token> queue;
  Token token;
  token.nodeId = nodeId;
  queue.push_back(token);
  instance.activeTokens += 1;
  if (!drainQueue(instance, *definition, queue, context)) {
    result.error = "节点重试失败：" + instance.errorMessage;
  }
  if (options_.autoDispatchEvents) dispatchEvents();
  result.instance = *repository_.findInstance(instanceId);
  result.createdTasks = context.createdTasks;
  result.ok = result.error.empty();
  return result;
}

OperationResult ProcessEngine::jumpToNode(long long instanceId, const std::string& nodeId,
                                          const std::string& user, const std::string& comment) {
  OperationResult result;
  InstanceLock lock = lockInstance(instanceId);
  ProcessInstance* stored = repository_.findInstance(instanceId);
  if (stored == nullptr) {
    result.error = "流程实例不存在";
    return result;
  }
  ProcessInstance instance = *stored;
  const ProcessDefinition* definition = definitionFor(instance);
  if (definition == nullptr || definition->findNode(nodeId) == nullptr) {
    result.error = "目标节点不存在：" + nodeId;
    return result;
  }
  RunContext context;
  if (!rewindTo(instance, *definition, nodeId, user, "JUMP", comment, context)) {
    result.error = "跳转失败";
    return result;
  }
  if (options_.autoDispatchEvents) dispatchEvents();
  result.instance = *repository_.findInstance(instanceId);
  result.createdTasks = context.createdTasks;
  result.ok = true;
  return result;
}

OperationResult ProcessEngine::signalEvent(const std::string& eventKey, const json::Value& payload,
                                           long long instanceId) {
  OperationResult result;
  if (eventKey.empty()) {
    result.error = "eventKey 不能为空";
    return result;
  }
  std::vector<std::pair<long long, long long>> matched;  // instanceId, nodeInstanceId
  for (const auto& instance : repository_.listInstances("", "", 100000)) {
    if (instanceId != 0 && instance.id != instanceId) continue;
    if (instance.status != ProcessStatus::Running) continue;
    for (const NodeInstance& nodeInstance : repository_.openNodeInstances(instance.id)) {
      if (nodeInstance.status == NodeInstanceStatus::Waiting && nodeInstance.waitKey == eventKey) {
        matched.emplace_back(instance.id, nodeInstance.id);
      }
    }
  }
  if (matched.empty()) {
    result.error = "没有等待该事件的节点：" + eventKey;
    return result;
  }
  for (const auto& entry : matched) {
    InstanceLock lock = lockInstance(entry.first);
    ProcessInstance* stored = repository_.findInstance(entry.first);
    if (stored == nullptr) continue;
    ProcessInstance instance = *stored;
    const ProcessDefinition* definition = definitionFor(instance);
    if (definition == nullptr) continue;
    NodeInstance* nodePtr = repository_.findNodeInstance(entry.second);
    if (nodePtr == nullptr || nodePtr->status != NodeInstanceStatus::Waiting) continue;
    NodeInstance nodeInstance = *nodePtr;

    mergeVariables(instance, payload);
    Value patch = Value::object();
    patch.set("event", payload);
    mergeVariables(instance, patch);
    recordHistory(instance, definition->findNode(nodeInstance.nodeId), &nodeInstance, nullptr,
                  "EVENT_RECEIVED", options_.systemUser, eventKey, payload, Value());
    RunContext context;
    if (!resumeNodeInstance(instance, *definition, nodeInstance, payload, context)) {
      result.error = "事件恢复节点失败";
      return result;
    }
    result.createdTasks = context.createdTasks;
  }
  if (options_.autoDispatchEvents) dispatchEvents();
  result.instance = *repository_.findInstance(matched.front().first);
  result.ok = true;
  return result;
}

// ---------------------------------------------------------------------------
// 查询
// ---------------------------------------------------------------------------
std::vector<Task> ProcessEngine::todoTasks(const TodoQuery& query) {
  return repository_.todoTasks(query.tenantId, query.assignee, query.page, query.size);
}

std::vector<Task> ProcessEngine::doneTasks(const TodoQuery& query) {
  return repository_.doneTasks(query.tenantId, query.assignee, query.page, query.size);
}

std::vector<Task> ProcessEngine::candidateTasks(const std::string& tenantId, const std::string& user) {
  std::vector<std::string> roles;
  if (organization_ != nullptr) roles = organization_->rolesOf(user);
  return repository_.candidateTasks(tenantId, user, roles);
}

std::vector<Task> ProcessEngine::tasksByInstance(long long instanceId) {
  return repository_.tasksByInstance(instanceId);
}

std::vector<HistoryRecord> ProcessEngine::history(long long instanceId) {
  return repository_.histories(instanceId);
}

std::vector<NodeInstance> ProcessEngine::nodeInstances(long long instanceId) {
  return repository_.nodeInstances(instanceId);
}

std::vector<EventRecord> ProcessEngine::events(long long instanceId) {
  return repository_.events(instanceId);
}

std::vector<ParallelBranch> ProcessEngine::branches(long long instanceId) {
  return repository_.branches(instanceId, "");
}

std::vector<ProcessInstance> ProcessEngine::instances(const std::string& tenantId,
                                                      const std::string& status, std::size_t limit) {
  return repository_.listInstances(tenantId, status, limit);
}

std::vector<FieldPermission> ProcessEngine::fieldPermissions(long long instanceId,
                                                             const std::string& nodeId) {
  std::vector<FieldPermission> permissions;
  ProcessInstance* instance = repository_.findInstance(instanceId);
  if (instance == nullptr) return permissions;
  const ProcessDefinition* definition = definitionFor(*instance);
  if (definition == nullptr) return permissions;
  const NodeDefinition* node = definition->findNode(nodeId);
  if (node == nullptr) return permissions;
  return node->fieldPermissions;
}

Value ProcessEngine::statistics(const std::string& tenantId) {
  Value out = Value::object();
  long long running = 0;
  long long completed = 0;
  long long cancelled = 0;
  long long terminated = 0;
  long long errors = 0;
  long long suspended = 0;
  long long totalDuration = 0;
  long long finished = 0;
  for (const auto& instance : repository_.listInstances(tenantId, "", 1000000)) {
    switch (instance.status) {
      case ProcessStatus::Running: ++running; break;
      case ProcessStatus::Suspended: ++suspended; break;
      case ProcessStatus::Completed: ++completed; break;
      case ProcessStatus::Cancelled: ++cancelled; break;
      case ProcessStatus::Terminated: ++terminated; break;
      case ProcessStatus::Error: ++errors; break;
    }
    if (isTerminal(instance.status) && instance.endTime > instance.startTime) {
      totalDuration += instance.endTime - instance.startTime;
      ++finished;
    }
  }
  out.set("总实例数", Value(running + completed + cancelled + terminated + errors + suspended));
  out.set("运行中", Value(running));
  out.set("挂起", Value(suspended));
  out.set("已完成", Value(completed));
  out.set("已取消", Value(cancelled));
  out.set("已终止", Value(terminated));
  out.set("异常", Value(errors));
  out.set("平均耗时(秒)", Value(finished == 0 ? 0.0 : static_cast<double>(totalDuration) / 1000.0 / finished));

  long long pending = 0;
  long long done = 0;
  long long overdue = 0;
  const long long now = clock_.now();
  for (const auto& task : repository_.todoTasks(tenantId, "", 1, 1000000)) {
    ++pending;
    if (task.dueTime > 0 && task.dueTime <= now) ++overdue;
  }
  for (const auto& task : repository_.doneTasks(tenantId, "", 1, 1000000)) {
    (void)task;
    ++done;
  }
  out.set("待办任务", Value(pending));
  out.set("已办任务", Value(done));
  out.set("超时任务", Value(overdue));
  return out;
}

}  // namespace wf

#include "wf/api.hpp"

#include "wf/types.hpp"

namespace wf {
namespace {

using json::Value;

std::string text(const Value& value, const std::string& key, const std::string& fallback = "") {
  return value.stringOr(key, fallback);
}

Value instancePayload(const ProcessInstance& instance) {
  return instance.toJson();
}

Value taskPayload(const Task& task) { return task.toJson(); }

Value taskListPayload(const std::vector<Task>& tasks) {
  Value out = Value::array();
  for (const auto& task : tasks) out.push_back(task.toJson());
  return out;
}

}  // namespace

json::Value okResponse(json::Value data) {
  Value out = Value::object();
  out.set("code", Value(0));
  out.set("message", Value("ok"));
  out.set("data", std::move(data));
  return out;
}

json::Value errorResponse(const std::string& message) {
  Value out = Value::object();
  out.set("code", Value(1));
  out.set("message", Value(message));
  out.set("data", Value());
  return out;
}

// ---------------------------------------------------------------------------
// 流程定义服务
// ---------------------------------------------------------------------------
ProcessDefinitionService::ProcessDefinitionService(ProcessEngine& engine, Repository& repository)
    : engine_(engine), repository_(repository) {}

json::Value ProcessDefinitionService::validate(const json::Value& body) {
  const ProcessDefinition definition = ProcessDefinition::fromJson(body);
  const ValidationResult result = engine_.validateDefinition(definition);
  Value data = Value::object();
  data.set("valid", Value(result.ok()));
  Value errors = Value::array();
  for (const auto& issue : result.errors) {
    Value item = Value::object();
    item.set("code", Value(issue.code));
    item.set("nodeId", Value(issue.nodeId));
    item.set("message", Value(issue.message));
    errors.push_back(item);
  }
  Value warnings = Value::array();
  for (const auto& issue : result.warnings) {
    Value item = Value::object();
    item.set("code", Value(issue.code));
    item.set("nodeId", Value(issue.nodeId));
    item.set("message", Value(issue.message));
    warnings.push_back(item);
  }
  data.set("errors", errors);
  data.set("warnings", warnings);
  data.set("summary", Value(result.summary()));
  return okResponse(data);
}

json::Value ProcessDefinitionService::create(const json::Value& body) {
  try {
    const ProcessDefinition definition = ProcessDefinition::fromJson(body);
    const ValidationResult validation = engine_.validateDefinition(definition);
    if (!validation.ok()) return errorResponse(validation.summary());
    const ProcessDefinition stored = engine_.deployDefinition(definition, false);
    return okResponse(stored.toJson());
  } catch (const std::exception& error) {
    return errorResponse(error.what());
  }
}

json::Value ProcessDefinitionService::publish(const json::Value& body) {
  try {
    const ProcessDefinition definition = ProcessDefinition::fromJson(body);
    const ProcessDefinition stored = engine_.deployDefinition(definition, true);
    return okResponse(stored.toJson());
  } catch (const std::exception& error) {
    return errorResponse(error.what());
  }
}

json::Value ProcessDefinitionService::publishByCode(const std::string& tenantId,
                                                    const std::string& code, int version) {
  for (ProcessDefinition definition : repository_.listDefinitions(tenantId, code)) {
    if (version > 0 && definition.version != version) continue;
    if (repository_.setDefinitionStatus(definition.id, DefinitionStatus::Published)) {
      definition.status = DefinitionStatus::Published;
      return okResponse(definition.toJson());
    }
  }
  return errorResponse("未找到流程定义：" + code + " v" + std::to_string(version));
}

json::Value ProcessDefinitionService::disable(const std::string& tenantId, const std::string& code,
                                             int version) {
  for (ProcessDefinition definition : repository_.listDefinitions(tenantId, code)) {
    if (definition.version != version) continue;
    repository_.setDefinitionStatus(definition.id, DefinitionStatus::Disabled);
    definition.status = DefinitionStatus::Disabled;
    return okResponse(definition.toJson());
  }
  return errorResponse("未找到流程定义：" + code + " v" + std::to_string(version));
}

json::Value ProcessDefinitionService::list(const std::string& tenantId, const std::string& code) const {
  // const_cast: Repository 的查询接口按 §17 表结构设计为可变访问
  Repository& repository = const_cast<Repository&>(repository_);
  Value out = Value::array();
  for (const auto& definition : repository.listDefinitions(tenantId, code)) {
    out.push_back(definition.toJson());
  }
  return okResponse(out);
}

const ProcessDefinition* ProcessDefinitionService::active(const std::string& tenantId,
                                                          const std::string& code) const {
  Repository& repository = const_cast<Repository&>(repository_);
  return repository.findDefinition(tenantId, code, 0);
}

// ---------------------------------------------------------------------------
// 流程实例服务
// ---------------------------------------------------------------------------
ProcessInstanceService::ProcessInstanceService(ProcessEngine& engine, Repository& repository)
    : engine_(engine), repository_(repository) {}

json::Value ProcessInstanceService::start(const json::Value& body) {
  StartRequest request;
  request.tenantId = text(body, "tenantId", "default");
  request.processCode = text(body, "processCode");
  request.businessKey = text(body, "businessKey");
  request.initiator = text(body, "initiator");
  request.variables = body.contains("variables") ? body.at("variables") : Value::object();
  request.version = static_cast<int>(body.intOr("version", 0));
  if (request.processCode.empty()) return errorResponse("processCode 不能为空");

  const OperationResult result = engine_.startProcess(request);
  if (!result.ok) return errorResponse(result.error);
  Value data = Value::object();
  data.set("instance", instancePayload(result.instance));
  data.set("tasks", taskListPayload(result.createdTasks));
  data.set("idempotentReplay", Value(result.idempotentReplay));
  return okResponse(data);
}

json::Value ProcessInstanceService::get(long long instanceId) {
  ProcessInstance* instance = repository_.findInstance(instanceId);
  if (instance == nullptr) return errorResponse("流程实例不存在");
  Value data = Value::object();
  data.set("instance", instance->toJson());
  data.set("nodeInstances", nodeInstances(instanceId).at("data"));
  data.set("tasks", taskListPayload(repository_.tasksByInstance(instanceId)));
  return okResponse(data);
}

json::Value ProcessInstanceService::list(const std::string& tenantId, const std::string& status,
                                         std::size_t limit) {
  Value out = Value::array();
  for (const auto& instance : repository_.listInstances(tenantId, status, limit)) {
    out.push_back(instance.toJson());
  }
  return okResponse(out);
}

json::Value ProcessInstanceService::cancel(long long instanceId, const std::string& user,
                                           const std::string& comment) {
  const OperationResult result = engine_.cancelInstance(instanceId, user, comment);
  if (!result.ok) return errorResponse(result.error);
  return okResponse(result.instance.toJson());
}

json::Value ProcessInstanceService::suspend(long long instanceId, const std::string& user) {
  const OperationResult result = engine_.suspendInstance(instanceId, user);
  if (!result.ok) return errorResponse(result.error);
  return okResponse(result.instance.toJson());
}

json::Value ProcessInstanceService::resume(long long instanceId, const std::string& user) {
  const OperationResult result = engine_.resumeInstance(instanceId, user);
  if (!result.ok) return errorResponse(result.error);
  return okResponse(result.instance.toJson());
}

json::Value ProcessInstanceService::terminate(long long instanceId, const std::string& user,
                                              const std::string& comment) {
  const OperationResult result = engine_.terminateInstance(instanceId, user, comment);
  if (!result.ok) return errorResponse(result.error);
  return okResponse(result.instance.toJson());
}

json::Value ProcessInstanceService::withdraw(long long instanceId, const std::string& user,
                                             const std::string& comment) {
  const OperationResult result = engine_.withdrawInstance(instanceId, user, comment);
  if (!result.ok) return errorResponse(result.error);
  Value data = Value::object();
  data.set("instance", instancePayload(result.instance));
  data.set("tasks", taskListPayload(result.createdTasks));
  return okResponse(data);
}

json::Value ProcessInstanceService::retryNode(long long instanceId, const std::string& nodeId,
                                              const std::string& user) {
  const OperationResult result = engine_.retryNode(instanceId, nodeId, user);
  if (!result.ok) return errorResponse(result.error);
  return okResponse(result.instance.toJson());
}

json::Value ProcessInstanceService::jumpToNode(long long instanceId, const std::string& nodeId,
                                               const std::string& user) {
  const OperationResult result = engine_.jumpToNode(instanceId, nodeId, user, "管理员跳转");
  if (!result.ok) return errorResponse(result.error);
  Value data = Value::object();
  data.set("instance", instancePayload(result.instance));
  data.set("tasks", taskListPayload(result.createdTasks));
  return okResponse(data);
}

json::Value ProcessInstanceService::signal(const std::string& eventKey, const json::Value& payload,
                                           long long instanceId) {
  const OperationResult result = engine_.signalEvent(eventKey, payload, instanceId);
  if (!result.ok) return errorResponse(result.error);
  Value data = Value::object();
  data.set("instance", instancePayload(result.instance));
  data.set("tasks", taskListPayload(result.createdTasks));
  return okResponse(data);
}

json::Value ProcessInstanceService::histories(long long instanceId) {
  Value out = Value::array();
  for (const auto& record : repository_.histories(instanceId)) out.push_back(record.toJson());
  return okResponse(out);
}

json::Value ProcessInstanceService::nodeInstances(long long instanceId) {
  Value out = Value::array();
  for (const auto& node : repository_.nodeInstances(instanceId)) out.push_back(node.toJson());
  return okResponse(out);
}

json::Value ProcessInstanceService::events(long long instanceId) {
  Value out = Value::array();
  for (const auto& event : repository_.events(instanceId)) out.push_back(event.toJson());
  return okResponse(out);
}

json::Value ProcessInstanceService::statistics(const std::string& tenantId) {
  return okResponse(engine_.statistics(tenantId));
}

// ---------------------------------------------------------------------------
// 任务服务
// ---------------------------------------------------------------------------
TaskService::TaskService(ProcessEngine& engine, Repository& repository)
    : engine_(engine), repository_(repository) {}

json::Value TaskService::todo(const std::string& tenantId, const std::string& assignee,
                              std::size_t page, std::size_t size) {
  TodoQuery query;
  query.tenantId = tenantId;
  query.assignee = assignee;
  query.page = page;
  query.size = size;
  return okResponse(taskListPayload(engine_.todoTasks(query)));
}

json::Value TaskService::done(const std::string& tenantId, const std::string& assignee,
                              std::size_t page, std::size_t size) {
  TodoQuery query;
  query.tenantId = tenantId;
  query.assignee = assignee;
  query.page = page;
  query.size = size;
  return okResponse(taskListPayload(engine_.doneTasks(query)));
}

json::Value TaskService::candidates(const std::string& tenantId, const std::string& user) {
  return okResponse(taskListPayload(engine_.candidateTasks(tenantId, user)));
}

json::Value TaskService::get(long long taskId) {
  Task* task = repository_.findTask(taskId);
  if (task == nullptr) return errorResponse("任务不存在");
  return okResponse(task->toJson());
}

json::Value TaskService::claim(long long taskId, const std::string& user) {
  const OperationResult result = engine_.claimTask(taskId, user);
  if (!result.ok) return errorResponse(result.error);
  return okResponse(result.task.toJson());
}

json::Value TaskService::complete(long long taskId, const json::Value& body) {
  TaskOperation operation;
  const std::string action = toLower(text(body, "action", "approve"));
  if (action == "reject" || action == "refuse" || action == "deny") {
    operation.action = TaskAction::Reject;
  } else {
    operation.action = TaskAction::Approve;
  }
  operation.operatorUser = text(body, "operator", text(body, "user"));
  operation.comment = text(body, "comment");
  operation.variables = body.contains("variables") ? body.at("variables") : Value::object();
  operation.expectedVersion = body.intOr("version", -1);
  const OperationResult result = engine_.completeTask(taskId, operation);
  if (!result.ok) return errorResponse(result.error);
  Value data = Value::object();
  data.set("task", taskPayload(result.task));
  data.set("instance", instancePayload(result.instance));
  data.set("tasks", taskListPayload(result.createdTasks));
  return okResponse(data);
}

json::Value TaskService::transfer(long long taskId, const json::Value& body) {
  const OperationResult result =
      engine_.transferTask(taskId, text(body, "operator"), text(body, "targetUser"),
                           text(body, "comment"));
  if (!result.ok) return errorResponse(result.error);
  Value data = Value::object();
  data.set("task", taskPayload(result.task));
  data.set("tasks", taskListPayload(result.createdTasks));
  return okResponse(data);
}

json::Value TaskService::delegate(long long taskId, const json::Value& body) {
  const OperationResult result =
      engine_.delegateTask(taskId, text(body, "operator"), text(body, "targetUser"),
                           text(body, "comment"));
  if (!result.ok) return errorResponse(result.error);
  Value data = Value::object();
  data.set("task", taskPayload(result.task));
  data.set("tasks", taskListPayload(result.createdTasks));
  return okResponse(data);
}

json::Value TaskService::addSign(long long taskId, const json::Value& body) {
  std::vector<std::string> users;
  if (body.contains("users") && body.at("users").isArray()) {
    for (const auto& user : body.at("users").items()) users.push_back(user.toString());
  } else if (body.contains("user")) {
    users = splitList(body.at("user").toString());
  }
  const OperationResult result =
      engine_.addSignTask(taskId, text(body, "operator"), users, text(body, "type", "AFTER"),
                          text(body, "mode", "ALL"), text(body, "comment"));
  if (!result.ok) return errorResponse(result.error);
  Value data = Value::object();
  data.set("task", taskPayload(result.task));
  data.set("tasks", taskListPayload(result.createdTasks));
  return okResponse(data);
}

json::Value TaskService::returnBack(long long taskId, const json::Value& body) {
  const OperationResult result =
      engine_.returnTask(taskId, text(body, "operator"), text(body, "targetNodeId"),
                         text(body, "comment"));
  if (!result.ok) return errorResponse(result.error);
  Value data = Value::object();
  data.set("task", taskPayload(result.task));
  data.set("instance", instancePayload(result.instance));
  data.set("tasks", taskListPayload(result.createdTasks));
  return okResponse(data);
}

}  // namespace wf

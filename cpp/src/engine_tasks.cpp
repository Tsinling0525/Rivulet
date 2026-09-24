// Task lifecycle: claim / approve / reject / return / transfer / delegate /
// add-sign / withdraw (PRD §10, §11, §18.3) plus countersign evaluation.
#include <algorithm>
#include <set>
#include <sstream>

#include "wf/engine.hpp"
#include "wf/expression.hpp"

namespace wf {
namespace {

using json::Value;

std::string upperCopy(const std::string& text) {
  std::string out = text;
  std::transform(out.begin(), out.end(), out.begin(),
                 [](unsigned char c) { return static_cast<char>(std::toupper(c)); });
  return out;
}

bool contains(const std::vector<std::string>& items, const std::string& value) {
  return std::find(items.begin(), items.end(), value) != items.end();
}

// Removes "a.b" style paths so field permission rules can strip writes.
void removePath(Value& target, const std::string& path) {
  const std::size_t dot = path.find('.');
  if (dot == std::string::npos) {
    target.erase(path);
    return;
  }
  const std::string head = path.substr(0, dot);
  Value* nested = target.find(head);
  if (nested == nullptr) return;
  removePath(*nested, path.substr(dot + 1));
}

}  // namespace

// ---------------------------------------------------------------------------
// 任务创建
// ---------------------------------------------------------------------------
Task ProcessEngine::makeTask(ProcessInstance& instance, const NodeInstance& nodeInstance,
                             const NodeDefinition& node, const std::string& assignee,
                             const std::string& signType, long long parentTaskId,
                             CompleteStrategy strategy) {
  Task task;
  task.processInstanceId = instance.id;
  task.nodeInstanceId = nodeInstance.id;
  task.nodeId = node.id;
  task.name = node.name;
  task.assignee = assignee;
  task.tenantId = instance.tenantId;
  task.strategy = strategy;
  task.signType = signType;
  task.parentTaskId = parentTaskId;
  task.createTime = clock_.now();
  task.status = TaskStatus::Pending;
  task.priority = static_cast<int>(node.properties.intOr("priority", 0));
  if (assignee.empty()) {
    const ApprovalRule& rule = node.assignee;
    const std::string type = toLower(rule.type);
    if (type == "role") {
      if (!rule.value.empty()) task.candidateRoles.push_back(rule.value);
    } else if (type == "roles") {
      for (const auto& role : rule.users) task.candidateRoles.push_back(role);
    } else if (!rule.users.empty()) {
      for (const auto& user : rule.users) task.candidateUsers.push_back(user);
    } else if (!rule.value.empty()) {
      task.candidateUsers.push_back(rule.value);
    }
  }
  task = repository_.createTask(task);
  recordHistory(instance, &node, &nodeInstance, &task, "TASK_CREATE", options_.systemUser, "",
                instance.variables, task.toJson());
  publishEvent(instance, &node, &nodeInstance, &task, EventType::TaskCreated, task.toJson());
  return task;
}

void ProcessEngine::scheduleTaskTimeout(ProcessInstance& instance, const NodeDefinition& node,
                                        NodeInstance& nodeInstance, Task& task) {
  if (!node.taskTimeout.configured()) return;
  long long millis = 0;
  if (!node.taskTimeout.durationMillis(&millis)) return;
  task.dueTime = clock_.now() + millis;
  repository_.updateTask(task, -1);
  Value payload = mergeTimeoutPayload(Value::object(), node.taskTimeout);
  payload.set("taskId", Value(task.id));
  payload.set("nodeId", Value(node.id));
  scheduleTimer(instance, &nodeInstance, &task, "TASK_TIMEOUT", task.dueTime, payload);
}

void ProcessEngine::scheduleNodeTimeout(ProcessInstance& instance, const NodeDefinition& node,
                                        NodeInstance& nodeInstance) {
  if (!node.nodeTimeout.configured()) return;
  long long millis = 0;
  if (!node.nodeTimeout.durationMillis(&millis)) return;
  Value payload = mergeTimeoutPayload(Value::object(), node.nodeTimeout);
  payload.set("nodeId", Value(node.id));
  scheduleTimer(instance, &nodeInstance, nullptr, "NODE_TIMEOUT", clock_.now() + millis, payload);
}

// ---------------------------------------------------------------------------
// 表单权限（PRD §14.3）
// ---------------------------------------------------------------------------
bool ProcessEngine::filterAndValidateForm(const NodeDefinition& node, const ProcessInstance& instance,
                                          json::Value* patch, std::string* error) const {
  if (patch == nullptr) return true;
  if (!options_.enforceFieldPermissions || node.fieldPermissions.empty()) return true;
  Value filtered = patch->isObject() ? *patch : Value::object();
  for (const auto& permission : node.fieldPermissions) {
    const Value* incoming = patch->isObject() ? patch->find(permission.field) : nullptr;
    const Value* existing = instance.variables.find(permission.field);
    if (permission.required && incoming == nullptr && existing == nullptr) {
      if (error != nullptr) *error = "字段必填：" + permission.field;
      return false;
    }
    if (incoming != nullptr && (!permission.editable || !permission.visible)) {
      removePath(filtered, permission.field);
    }
  }
  *patch = filtered;
  return true;
}

// ---------------------------------------------------------------------------
// 认领
// ---------------------------------------------------------------------------
OperationResult ProcessEngine::claimTask(long long taskId, const std::string& user) {
  OperationResult result;
  Task* found = repository_.findTask(taskId);
  if (found == nullptr) {
    result.error = "任务不存在：" + std::to_string(taskId);
    return result;
  }
  InstanceLock lock = lockInstance(found->processInstanceId);
  Task task = *repository_.findTask(taskId);
  ProcessInstance instance = *repository_.findInstance(task.processInstanceId);

  if (!isOpen(task.status)) {
    result.error = std::string("任务已结束：") + toString(task.status);
    result.task = task;
    return result;
  }
  if (!task.assignee.empty() && task.assignee != user) {
    result.error = "任务已分配给：" + task.assignee;
    return result;
  }
  if (!task.candidateUsers.empty() || !task.candidateRoles.empty()) {
    bool allowed = contains(task.candidateUsers, user);
    if (!allowed && organization_ != nullptr) {
      const std::vector<std::string> roles = organization_->rolesOf(user);
      for (const auto& role : roles) {
        if (contains(task.candidateRoles, role)) allowed = true;
      }
    }
    if (!allowed) {
      result.error = "用户不在候选人范围内：" + user;
      return result;
    }
  }
  task.assignee = user;
  task.status = TaskStatus::Claimed;
  task.claimTime = clock_.now();
  if (!repository_.updateTask(task, task.version)) {
    result.error = "任务已被其他人认领";
    return result;
  }
  const NodeDefinition* node = nullptr;
  if (const ProcessDefinition* definition = definitionFor(instance)) {
    node = definition->findNode(task.nodeId);
  }
  recordHistory(instance, node, nullptr, &task, "CLAIM", user, "", Value(), Value());
  publishEvent(instance, node, nullptr, &task, EventType::TaskAssigned, task.toJson());
  result.task = *repository_.findTask(taskId);
  result.instance = instance;
  result.ok = true;
  if (options_.autoDispatchEvents) dispatchEvents();
  return result;
}

// ---------------------------------------------------------------------------
// 办理（同意 / 拒绝）
// ---------------------------------------------------------------------------
OperationResult ProcessEngine::completeTask(long long taskId, const TaskOperation& operation) {
  OperationResult result;
  Task* found = repository_.findTask(taskId);
  if (found == nullptr) {
    result.error = "任务不存在：" + std::to_string(taskId);
    return result;
  }
  InstanceLock lock = lockInstance(found->processInstanceId);
  Task task = *repository_.findTask(taskId);
  ProcessInstance* instancePtr = repository_.findInstance(task.processInstanceId);
  if (instancePtr == nullptr) {
    result.error = "流程实例不存在";
    return result;
  }
  ProcessInstance instance = *instancePtr;
  const ProcessDefinition* definition = definitionFor(instance);
  if (definition == nullptr) {
    result.error = "流程定义不存在";
    return result;
  }
  result.instance = instance;
  result.task = task;

  if (!isOpen(task.status)) {
    // 幂等：重复提交不改变任何状态
    result.error = std::string("任务已处理（当前状态 ") + toString(task.status) + "）";
    return result;
  }
  if (instance.status == ProcessStatus::Suspended) {
    result.error = "流程已挂起，无法办理任务";
    return result;
  }
  if (isTerminal(instance.status)) {
    result.error = std::string("流程已结束（") + toString(instance.status) + "）";
    return result;
  }
  if (!task.assignee.empty() && !operation.operatorUser.empty() &&
      task.assignee != operation.operatorUser) {
    result.error = "任务当前处理人是 " + task.assignee + "，无权办理";
    return result;
  }
  if (operation.expectedVersion >= 0 && task.version != operation.expectedVersion) {
    result.error = "任务已被其他人处理（版本冲突）";
    return result;
  }
  for (const Task& sibling : repository_.tasksByNodeInstance(task.nodeInstanceId)) {
    if (sibling.id != task.id && sibling.signType == "BEFORE" && isOpen(sibling.status)) {
      result.error = "存在未处理的前置加签任务，无法办理";
      return result;
    }
  }

  NodeInstance* nodeInstancePtr = repository_.findNodeInstance(task.nodeInstanceId);
  if (nodeInstancePtr == nullptr) {
    result.error = "节点实例不存在";
    return result;
  }
  NodeInstance nodeInstance = *nodeInstancePtr;
  const NodeDefinition* node = definition->findNode(task.nodeId);
  if (node == nullptr) {
    result.error = "节点定义不存在：" + task.nodeId;
    return result;
  }

  json::Value patch = operation.variables.isObject() ? operation.variables : Value::object();
  std::string formError;
  if (!filterAndValidateForm(*node, instance, &patch, &formError)) {
    result.error = formError;
    return result;
  }
  mergeVariables(instance, patch);

  const std::string actor = operation.operatorUser.empty() ? task.assignee : operation.operatorUser;
  task.status = TaskStatus::Completed;
  task.completeTime = clock_.now();
  task.comment = operation.comment;
  task.action = toString(operation.action);
  if (task.assignee.empty()) task.assignee = actor;
  if (!repository_.updateTask(task, task.version)) {
    result.error = "任务已被其他人处理（版本冲突）";
    return result;
  }
  recordHistory(instance, node, &nodeInstance, &task, task.action, actor, operation.comment, patch,
                Value());
  publishEvent(instance, node, &nodeInstance, &task, EventType::TaskCompleted, task.toJson());

  RunContext context;

  // 委派产生的子任务：办完后交回原处理人（PRD §10.5）；转办（§10.4）由新处理人
  // 直接完成节点，不再回到原处理人。
  if (task.parentTaskId != 0 && task.signType == "DELEGATE") {
    Task* parent = repository_.findTask(task.parentTaskId);
    if (parent != nullptr && parent->status == TaskStatus::Delegated) {
      Task reopened = *parent;
      reopened.status = TaskStatus::Pending;
      repository_.updateTask(reopened, reopened.version);
      recordHistory(instance, node, &nodeInstance, &reopened, "TASK_RETURN", actor,
                    "委派处理完成，任务回到原处理人", Value(), Value());
      publishEvent(instance, node, &nodeInstance, &reopened, EventType::TaskAssigned,
                   reopened.toJson());
      context.createdTasks.push_back(*repository_.findTask(reopened.id));
    }
    updateInstance(instance, "child-task-complete");
    if (options_.autoDispatchEvents) dispatchEvents();
    result.task = *repository_.findTask(taskId);
    result.instance = *repository_.findInstance(instance.id);
    result.createdTasks = context.createdTasks;
    result.ok = true;
    return result;
  }

  // 前置加签任务完成：唤醒原任务，不参与节点通过判定（PRD §10.6）
  if (task.signType == "BEFORE") {
    bool remaining = false;
    for (const Task& sibling : repository_.tasksByNodeInstance(task.nodeInstanceId)) {
      if (sibling.id != task.id && sibling.signType == "BEFORE" && isOpen(sibling.status)) {
        remaining = true;
      }
    }
    if (!remaining) {
      recordHistory(instance, node, &nodeInstance, &task, "ADD_SIGN_DONE", actor,
                    "前置加签完成", Value(), Value());
    }
    updateInstance(instance, "before-sign-complete");
    if (options_.autoDispatchEvents) dispatchEvents();
    result.task = *repository_.findTask(taskId);
    result.instance = *repository_.findInstance(instance.id);
    result.ok = true;
    return result;
  }

  if (operation.action == TaskAction::Reject) {
    if (!applyRejection(instance, *definition, nodeInstance, *node, task, operation, context)) {
      result.error = "驳回处理失败";
      return result;
    }
  } else if (!completeNodeAfterTask(instance, *definition, nodeInstance, *node, task, operation,
                                    context)) {
    result.error = "节点办理失败";
    return result;
  }
  if (options_.autoDispatchEvents) dispatchEvents();
  result.task = *repository_.findTask(taskId);
  result.instance = *repository_.findInstance(instance.id);
  result.createdTasks = context.createdTasks;
  result.ok = true;
  return result;
}

// ---------------------------------------------------------------------------
// 会签 / 或签 / 比例 / 依次（PRD §11）
// ---------------------------------------------------------------------------
bool ProcessEngine::completeNodeAfterTask(ProcessInstance& instance, const ProcessDefinition& definition,
                                          NodeInstance& nodeInstance, const NodeDefinition& node,
                                          Task& task, const TaskOperation& operation,
                                          RunContext& context) {
  const std::vector<Task> all = repository_.tasksByNodeInstance(nodeInstance.id);

  int total = 0;
  int approved = 0;
  int rejected = 0;
  int pending = 0;
  int beforeOpen = 0;
  int afterOpen = 0;
  for (const auto& item : all) {
    if (item.status == TaskStatus::Cancelled) continue;
    if (item.signType == "BEFORE") {
      if (isOpen(item.status)) ++beforeOpen;
      continue;
    }
    // 委派任务只代表代理处理，票仍属于原处理人；转办任务则由新处理人投票。
    if (item.signType == "DELEGATE") continue;
    if (item.status == TaskStatus::Delegated || item.status == TaskStatus::Transferred) continue;
    if (item.signType == "AFTER" && isOpen(item.status)) ++afterOpen;
    ++total;
    if (item.status == TaskStatus::Completed) {
      if (item.action == "APPROVE") ++approved;
      if (item.action == "REJECT") ++rejected;
    } else if (isOpen(item.status)) {
      ++pending;
    }
  }

  // 后加签未完成前节点不结束
  if (afterOpen > 0 && task.signType != "AFTER") {
    recordHistory(instance, &node, &nodeInstance, &task, "WAIT_AFTER_SIGN", task.assignee, "",
                  Value(), Value());
    updateInstance(instance, "wait-after-sign");
    return true;
  }
  if (rejected > 0) {
    return applyRejection(instance, definition, nodeInstance, node, task, operation, context);
  }
  if (beforeOpen > 0) {
    updateInstance(instance, "wait-before-sign");
    return true;
  }

  bool nodeApproved = false;
  bool nodeRejected = false;
  switch (node.strategy) {
    case CompleteStrategy::Any:
      nodeApproved = approved > 0;
      break;
    case CompleteStrategy::All:
      nodeApproved = (approved == total && pending == 0 && total > 0);
      break;
    case CompleteStrategy::Ratio: {
      const double ratio = node.ratio > 0 ? node.ratio : 0.5;
      if (total == 0) break;
      if (static_cast<double>(approved) / total >= ratio) {
        nodeApproved = true;
      } else if (static_cast<double>(approved + pending) / total < ratio) {
        nodeRejected = true;
      }
      break;
    }
    case CompleteStrategy::Sequence: {
      Value queue = nodeInstance.output.at("sequenceQueue");
      if (queue.isArray() && !queue.empty()) {
        const std::string nextUser = queue.at(0).toString();
        Value rest = Value::array();
        for (std::size_t i = 1; i < queue.size(); ++i) rest.push_back(queue.at(i));
        NodeInstance updated = nodeInstance;
        Value output = updated.output.isObject() ? updated.output : Value::object();
        output.set("sequenceQueue", rest);
        updated.output = output;
        repository_.updateNodeInstance(updated);
        nodeInstance = updated;
        Task nextTask = makeTask(instance, nodeInstance, node, nextUser, "SEQUENCE", 0,
                                 CompleteStrategy::Sequence);
        context.createdTasks.push_back(nextTask);
        scheduleTaskTimeout(instance, node, nodeInstance, nextTask);
        recordHistory(instance, &node, &nodeInstance, &task, "SEQUENCE_NEXT", nextUser, "",
                      Value(), nextTask.toJson());
        updateInstance(instance, "sequence-next");
        return true;
      }
      nodeApproved = approved > 0;
      break;
    }
  }

  if (nodeRejected) {
    TaskOperation rejection = operation;
    rejection.action = TaskAction::Reject;
    return applyRejection(instance, definition, nodeInstance, node, task, rejection, context);
  }

  if (!nodeApproved) {
    recordHistory(instance, &node, &nodeInstance, &task, "WAIT_COUNTERSIGN", task.assignee, "",
                  Value(), Value());
    updateInstance(instance, "wait-countersign");
    return true;
  }

  cancelOpenTasks(instance.id, &node, "节点已通过");
  recordHistory(instance, &node, &nodeInstance, &task, "NODE_APPROVED", task.assignee, "", Value(),
                Value());
  Value output = Value::object();
  output.set("result", Value("APPROVED"));
  output.set("approved", Value(approved));
  output.set("total", Value(total));
  std::vector<Token> queue;
  if (!completeNode(instance, definition, nodeInstance, node, output, queue, context)) return false;
  return drainQueue(instance, definition, queue, context);
}

// ---------------------------------------------------------------------------
// 拒绝处理（PRD §10.2）
// ---------------------------------------------------------------------------
bool ProcessEngine::applyRejection(ProcessInstance& instance, const ProcessDefinition& definition,
                                   NodeInstance& nodeInstance, const NodeDefinition& node,
                                   const Task& task, const TaskOperation& operation,
                                   RunContext& context) {
  cancelOpenTasks(instance.id, &node, "节点被驳回");
  NodeInstance rejectedInstance = nodeInstance;
  rejectedInstance.status = NodeInstanceStatus::Completed;
  rejectedInstance.endTime = clock_.now();
  Value output = Value::object();
  output.set("result", Value("REJECTED"));
  output.set("comment", Value(operation.comment));
  rejectedInstance.output = output;
  repository_.updateNodeInstance(rejectedInstance);
  recordHistory(instance, &node, &rejectedInstance, &task, "REJECT", operation.operatorUser,
                operation.comment, Value(), output);
  publishEvent(instance, &node, &rejectedInstance, &task, EventType::TaskCompleted, output);

  const std::string policy = upperCopy(node.rejectPolicy);
  if (policy == "INITIATOR" || policy == "PREV" || policy == "NODE") {
    std::string target = node.rejectTarget;
    if (policy == "INITIATOR") {
      for (const auto& candidate : definition.nodes) {
        if (candidate.type != NodeType::UserTask) continue;
        const std::vector<std::string> assignees = resolveAssignees(instance, candidate, nullptr);
        if (contains(assignees, instance.initiator)) {
          target = candidate.id;
          break;
        }
      }
    } else if (policy == "PREV") {
      const std::vector<HistoryRecord> history = repository_.histories(instance.id);
      for (auto it = history.rbegin(); it != history.rend(); ++it) {
        if (it->nodeId == node.id) continue;
        if (it->action == "APPROVE" || it->action == "NODE_APPROVED" || it->action == "NODE_COMPLETE") {
          target = it->nodeId;
          break;
        }
      }
    }
    if (!target.empty() && definition.findNode(target) != nullptr) {
      return rewindTo(instance, definition, target, operation.operatorUser, "REJECT", operation.comment,
                      context);
    }
    if (policy != "NODE") {
      // 找不到目标节点时退化为直接结束
      return finishInstance(instance, definition, ProcessStatus::Completed, "REJECTED", context);
    }
  }
  if (policy == "EXCEPTION" && !node.rejectTarget.empty() &&
      definition.findNode(node.rejectTarget) != nullptr) {
    return rewindTo(instance, definition, node.rejectTarget, operation.operatorUser, "REJECT",
                    operation.comment, context);
  }
  instance.result = "REJECTED";
  return finishInstance(instance, definition, ProcessStatus::Completed, "REJECTED", context);
}

// ---------------------------------------------------------------------------
// 转办 / 委派 / 加签 / 退回 / 撤回
// ---------------------------------------------------------------------------
OperationResult ProcessEngine::transferTask(long long taskId, const std::string& by,
                                           const std::string& to, const std::string& comment) {
  OperationResult result;
  Task* found = repository_.findTask(taskId);
  if (found == nullptr) {
    result.error = "任务不存在";
    return result;
  }
  InstanceLock lock = lockInstance(found->processInstanceId);
  Task task = *repository_.findTask(taskId);
  ProcessInstance instance = *repository_.findInstance(task.processInstanceId);
  if (!isOpen(task.status)) {
    result.error = "任务已结束，无法转办";
    return result;
  }
  if (to.empty()) {
    result.error = "转办目标人不能为空";
    return result;
  }
  const ProcessDefinition* definition = definitionFor(instance);
  const NodeDefinition* node = definition != nullptr ? definition->findNode(task.nodeId) : nullptr;

  Task updated = task;
  updated.status = TaskStatus::Transferred;
  updated.comment = comment;
  repository_.updateTask(updated, updated.version);

  Task next = task;
  next.id = 0;
  next.assignee = to;
  next.status = TaskStatus::Pending;
  next.parentTaskId = task.id;
  next.signType = "TRANSFER";
  next.createTime = clock_.now();
  next.claimTime = 0;
  next.completeTime = 0;
  next.comment = comment;
  next.version = 1;
  next = repository_.createTask(next);

  recordHistory(instance, node, nullptr, &next, "TRANSFER", by, "转办给 " + to + "：" + comment,
                Value(), next.toJson());
  publishEvent(instance, node, nullptr, &next, EventType::TaskTransferred, next.toJson());
  if (options_.autoDispatchEvents) dispatchEvents();
  result.task = next;
  result.instance = instance;
  result.createdTasks.push_back(next);
  result.ok = true;
  return result;
}

OperationResult ProcessEngine::delegateTask(long long taskId, const std::string& by,
                                           const std::string& to, const std::string& comment) {
  OperationResult result;
  Task* found = repository_.findTask(taskId);
  if (found == nullptr) {
    result.error = "任务不存在";
    return result;
  }
  InstanceLock lock = lockInstance(found->processInstanceId);
  Task task = *repository_.findTask(taskId);
  ProcessInstance instance = *repository_.findInstance(task.processInstanceId);
  if (!isOpen(task.status)) {
    result.error = "任务已结束，无法委派";
    return result;
  }
  if (to.empty()) {
    result.error = "委派目标人不能为空";
    return result;
  }
  const ProcessDefinition* definition = definitionFor(instance);
  const NodeDefinition* node = definition != nullptr ? definition->findNode(task.nodeId) : nullptr;

  Task updated = task;
  updated.status = TaskStatus::Delegated;
  updated.comment = comment;
  repository_.updateTask(updated, updated.version);

  Task delegate = task;
  delegate.id = 0;
  delegate.assignee = to;
  delegate.status = TaskStatus::Pending;
  delegate.parentTaskId = task.id;
  delegate.signType = "DELEGATE";
  delegate.createTime = clock_.now();
  delegate.claimTime = 0;
  delegate.completeTime = 0;
  delegate.comment = comment;
  delegate.version = 1;
  delegate = repository_.createTask(delegate);

  recordHistory(instance, node, nullptr, &delegate, "DELEGATE", by, "委派给 " + to + "：" + comment,
                Value(), delegate.toJson());
  publishEvent(instance, node, nullptr, &delegate, EventType::TaskDelegated, delegate.toJson());
  if (options_.autoDispatchEvents) dispatchEvents();
  result.task = delegate;
  result.instance = instance;
  result.createdTasks.push_back(delegate);
  result.ok = true;
  return result;
}

OperationResult ProcessEngine::addSignTask(long long taskId, const std::string& by,
                                          const std::vector<std::string>& users,
                                          const std::string& type, const std::string& mode,
                                          const std::string& comment) {
  OperationResult result;
  Task* found = repository_.findTask(taskId);
  if (found == nullptr) {
    result.error = "任务不存在";
    return result;
  }
  if (users.empty()) {
    result.error = "加签人员不能为空";
    return result;
  }
  InstanceLock lock = lockInstance(found->processInstanceId);
  Task task = *repository_.findTask(taskId);
  ProcessInstance instance = *repository_.findInstance(task.processInstanceId);
  if (!isOpen(task.status)) {
    result.error = "任务已结束，无法加签";
    return result;
  }
  NodeInstance* nodeInstancePtr = repository_.findNodeInstance(task.nodeInstanceId);
  if (nodeInstancePtr == nullptr) {
    result.error = "节点实例不存在";
    return result;
  }
  NodeInstance nodeInstance = *nodeInstancePtr;
  const ProcessDefinition* definition = definitionFor(instance);
  const NodeDefinition* node = definition != nullptr ? definition->findNode(task.nodeId) : nullptr;
  const std::string signType = upperCopy(type) == "BEFORE" ? "BEFORE" : "AFTER";
  const std::string signMode = upperCopy(mode) == "ANY" ? "ANY" : "ALL";

  for (const auto& user : users) {
    Task added;
    added.processInstanceId = instance.id;
    added.nodeInstanceId = nodeInstance.id;
    added.nodeId = task.nodeId;
    added.name = task.name;
    added.assignee = user;
    added.tenantId = instance.tenantId;
    added.strategy = task.strategy;
    added.signType = signType;
    added.signMode = signMode;
    added.parentTaskId = task.id;
    added.createTime = clock_.now();
    added.status = TaskStatus::Pending;
    added = repository_.createTask(added);
    if (node != nullptr) scheduleTaskTimeout(instance, *node, nodeInstance, added);
    recordHistory(instance, node, &nodeInstance, &added, "ADD_SIGN",
                  by, (signType == "BEFORE" ? "前加签：" : "后加签：") + user + " " + comment,
                  Value(), added.toJson());
    publishEvent(instance, node, &nodeInstance, &added, EventType::TaskCreated, added.toJson());
    result.createdTasks.push_back(added);
  }
  updateInstance(instance, "add-sign");
  if (options_.autoDispatchEvents) dispatchEvents();
  result.task = task;
  result.instance = instance;
  result.ok = true;
  return result;
}

OperationResult ProcessEngine::returnTask(long long taskId, const std::string& by,
                                         const std::string& targetNodeId, const std::string& comment) {
  OperationResult result;
  Task* found = repository_.findTask(taskId);
  if (found == nullptr) {
    result.error = "任务不存在";
    return result;
  }
  InstanceLock lock = lockInstance(found->processInstanceId);
  Task task = *repository_.findTask(taskId);
  ProcessInstance instance = *repository_.findInstance(task.processInstanceId);
  if (!isOpen(task.status)) {
    result.error = "任务已结束，无法退回";
    return result;
  }
  const ProcessDefinition* definition = definitionFor(instance);
  if (definition == nullptr) {
    result.error = "流程定义不存在";
    return result;
  }
  const NodeDefinition* node = definition->findNode(task.nodeId);
  if (node != nullptr && !node->allowReturn) {
    result.error = "当前节点不允许退回";
    return result;
  }
  std::string target = targetNodeId;
  if (target.empty() && node != nullptr) {
    target = node->rejectTarget;
  }
  if (target.empty()) {
    const std::vector<HistoryRecord> history = repository_.histories(instance.id);
    for (auto it = history.rbegin(); it != history.rend(); ++it) {
      if (it->nodeId == task.nodeId) continue;
      if (it->action == "NODE_COMPLETE" || it->action == "APPROVE") {
        target = it->nodeId;
        break;
      }
    }
  }
  if (target.empty() || definition->findNode(target) == nullptr) {
    result.error = "找不到退回目标节点";
    return result;
  }
  Task updated = task;
  updated.status = TaskStatus::Cancelled;
  updated.comment = comment;
  repository_.updateTask(updated, updated.version);

  RunContext context;
  if (!rewindTo(instance, *definition, target, by, "RETURN", comment, context)) {
    result.error = "退回失败";
    return result;
  }
  if (options_.autoDispatchEvents) dispatchEvents();
  result.task = *repository_.findTask(taskId);
  result.instance = *repository_.findInstance(instance.id);
  result.createdTasks = context.createdTasks;
  result.ok = true;
  return result;
}

OperationResult ProcessEngine::withdrawInstance(long long instanceId, const std::string& user,
                                               const std::string& comment) {
  OperationResult result;
  InstanceLock lock = lockInstance(instanceId);
  ProcessInstance* stored = repository_.findInstance(instanceId);
  if (stored == nullptr) {
    result.error = "流程实例不存在";
    return result;
  }
  ProcessInstance instance = *stored;
  if (instance.status != ProcessStatus::Running) {
    result.error = std::string("当前状态不允许撤回：") + toString(instance.status);
    return result;
  }
  if (!instance.initiator.empty() && !user.empty() && instance.initiator != user) {
    result.error = "只有发起人可以撤回流程";
    return result;
  }
  const ProcessDefinition* definition = definitionFor(instance);
  if (definition == nullptr) {
    result.error = "流程定义不存在";
    return result;
  }
  // 撤回条件：后续节点尚未处理（PRD §10.7）
  for (const HistoryRecord& record : repository_.histories(instanceId)) {
    if (record.action != "APPROVE" || record.operatorUser.empty()) continue;
    if (record.operatorUser == instance.initiator) continue;
    result.error = "已有其他审批人处理过该流程，无法撤回";
    return result;
  }
  std::string target;
  for (const auto& node : definition->nodes) {
    if (node.type != NodeType::UserTask) continue;
    if (!node.allowWithdraw) continue;
    const std::vector<std::string> assignees = resolveAssignees(instance, node, nullptr);
    if (contains(assignees, instance.initiator)) {
      target = node.id;
      break;
    }
  }
  if (target.empty()) {
    const NodeDefinition* start = definition->startNode();
    target = start != nullptr ? definition->outEdges(start->id).empty()
                                    ? std::string()
                                    : definition->outEdges(start->id).front()->target
                              : std::string();
  }
  if (target.empty() || definition->findNode(target) == nullptr) {
    result.error = "找不到撤回目标节点";
    return result;
  }
  RunContext context;
  if (!rewindTo(instance, *definition, target, user, "WITHDRAW", comment, context)) {
    result.error = "撤回失败";
    return result;
  }
  if (options_.autoDispatchEvents) dispatchEvents();
  result.instance = *repository_.findInstance(instanceId);
  result.createdTasks = context.createdTasks;
  result.ok = true;
  return result;
}

// ---------------------------------------------------------------------------
// 回溯到指定节点（退回 / 撤回 / 驳回 / 管理员跳转）
// ---------------------------------------------------------------------------
bool ProcessEngine::rewindTo(ProcessInstance& instance, const ProcessDefinition& definition,
                             const std::string& targetNodeId, const std::string& user,
                             const std::string& action, const std::string& comment,
                             RunContext& context) {
  if (isTerminal(instance.status) && instance.status != ProcessStatus::Error) {
    instance.errorMessage = "流程已结束，无法回溯";
    return false;
  }
  instance.status = ProcessStatus::Running;
  instance.errorMessage.clear();
  instance.result.clear();
  cancelOpenTasks(instance.id, nullptr, "流程回溯：" + action);
  repository_.clearBranches(instance.id);
  for (TimerJob job : repository_.timersByInstance(instance.id)) {
    if (job.status != "PENDING") continue;
    job.status = "CANCELLED";
    repository_.updateTimer(job);
  }
  instance.activeTokens = 0;
  updateInstance(instance, "rewind");
  recordHistory(instance, definition.findNode(targetNodeId), nullptr, nullptr, action, user, comment,
                Value(), Value());

  std::vector<Token> queue;
  Token token;
  token.nodeId = targetNodeId;
  queue.push_back(token);
  instance.activeTokens += 1;
  return drainQueue(instance, definition, queue, context);
}

}  // namespace wf

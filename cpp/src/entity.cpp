#include "wf/entity.hpp"

namespace wf {
namespace {

using json::Value;

Value stringArray(const std::vector<std::string>& items) {
  Value array = Value::array();
  for (const auto& item : items) array.push_back(Value(item));
  return array;
}

std::vector<std::string> readStringArray(const Value& value) {
  std::vector<std::string> out;
  if (!value.isArray()) return out;
  for (const auto& item : value.items()) out.push_back(item.toString());
  return out;
}

}  // namespace

Value ProcessInstance::toJson() const {
  Value out = Value::object();
  out.set("id", Value(id));
  out.set("processDefinitionId", Value(definitionId));
  out.set("processCode", Value(processCode));
  out.set("definitionVersion", Value(definitionVersion));
  out.set("tenantId", Value(tenantId));
  out.set("businessKey", Value(businessKey));
  out.set("status", Value(toString(status)));
  out.set("initiator", Value(initiator));
  out.set("currentNodeId", Value(currentNodeId));
  out.set("variables", variables);
  out.set("startTime", Value(startTime));
  out.set("endTime", Value(endTime));
  out.set("version", Value(version));
  if (parentInstanceId != 0) out.set("parentInstanceId", Value(parentInstanceId));
  if (parentNodeInstanceId != 0) out.set("parentNodeInstanceId", Value(parentNodeInstanceId));
  out.set("activeTokens", Value(activeTokens));
  if (!result.empty()) out.set("result", Value(result));
  if (!errorMessage.empty()) out.set("errorMessage", Value(errorMessage));
  return out;
}

ProcessInstance ProcessInstance::fromJson(const Value& value) {
  ProcessInstance instance;
  instance.id = value.intOr("id", 0);
  instance.definitionId = value.intOr("processDefinitionId", value.intOr("definitionId", 0));
  instance.processCode = value.stringOr("processCode", "");
  instance.definitionVersion = static_cast<int>(value.intOr("definitionVersion", 1));
  instance.tenantId = value.stringOr("tenantId", "default");
  instance.businessKey = value.stringOr("businessKey", "");
  instance.status = parseProcessStatus(value.stringOr("status", "RUNNING"));
  instance.initiator = value.stringOr("initiator", "");
  instance.currentNodeId = value.stringOr("currentNodeId", "");
  instance.variables = value.contains("variables") ? value.at("variables") : Value::object();
  instance.startTime = value.intOr("startTime", 0);
  instance.endTime = value.intOr("endTime", 0);
  instance.version = value.intOr("version", 0);
  instance.parentInstanceId = value.intOr("parentInstanceId", 0);
  instance.parentNodeInstanceId = value.intOr("parentNodeInstanceId", 0);
  instance.activeTokens = static_cast<int>(value.intOr("activeTokens", 0));
  instance.result = value.stringOr("result", "");
  instance.errorMessage = value.stringOr("errorMessage", "");
  return instance;
}

Value NodeInstance::toJson() const {
  Value out = Value::object();
  out.set("id", Value(id));
  out.set("processInstanceId", Value(processInstanceId));
  out.set("nodeId", Value(nodeId));
  out.set("nodeName", Value(nodeName));
  out.set("nodeType", Value(toString(nodeType)));
  out.set("status", Value(toString(status)));
  if (!input.isNull()) out.set("inputData", input);
  if (!output.isNull()) out.set("outputData", output);
  if (!errorMessage.empty()) out.set("errorMessage", Value(errorMessage));
  if (!assignee.empty()) out.set("assignee", Value(assignee));
  out.set("attempt", Value(attempt));
  if (!waitKey.empty()) out.set("waitKey", Value(waitKey));
  out.set("startTime", Value(startTime));
  out.set("endTime", Value(endTime));
  return out;
}

NodeInstance NodeInstance::fromJson(const Value& value) {
  NodeInstance node;
  node.id = value.intOr("id", 0);
  node.processInstanceId = value.intOr("processInstanceId", 0);
  node.nodeId = value.stringOr("nodeId", "");
  node.nodeName = value.stringOr("nodeName", "");
  node.nodeType = parseNodeType(value.stringOr("nodeType", ""));
  node.status = parseNodeInstanceStatus(value.stringOr("status", "PENDING"));
  node.input = value.contains("inputData") ? value.at("inputData") : Value();
  node.output = value.contains("outputData") ? value.at("outputData") : Value();
  node.errorMessage = value.stringOr("errorMessage", "");
  node.assignee = value.stringOr("assignee", "");
  node.attempt = static_cast<int>(value.intOr("attempt", 0));
  node.waitKey = value.stringOr("waitKey", "");
  node.startTime = value.intOr("startTime", 0);
  node.endTime = value.intOr("endTime", 0);
  return node;
}

Value Task::toJson() const {
  Value out = Value::object();
  out.set("id", Value(id));
  out.set("processInstanceId", Value(processInstanceId));
  out.set("nodeInstanceId", Value(nodeInstanceId));
  out.set("nodeId", Value(nodeId));
  out.set("taskName", Value(name));
  out.set("assignee", Value(assignee));
  if (!candidateUsers.empty()) out.set("candidateUsers", stringArray(candidateUsers));
  if (!candidateRoles.empty()) out.set("candidateRoles", stringArray(candidateRoles));
  out.set("status", Value(toString(status)));
  out.set("priority", Value(priority));
  out.set("createTime", Value(createTime));
  out.set("claimTime", Value(claimTime));
  out.set("completeTime", Value(completeTime));
  out.set("dueTime", Value(dueTime));
  if (!comment.empty()) out.set("comment", Value(comment));
  out.set("version", Value(version));
  if (parentTaskId != 0) out.set("parentTaskId", Value(parentTaskId));
  out.set("signType", Value(signType));
  out.set("signMode", Value(signMode));
  out.set("strategy", Value(toString(strategy)));
  if (!action.empty()) out.set("action", Value(action));
  out.set("tenantId", Value(tenantId));
  out.set("countersignMember", Value(countersignMember));
  return out;
}

Task Task::fromJson(const Value& value) {
  Task task;
  task.id = value.intOr("id", 0);
  task.processInstanceId = value.intOr("processInstanceId", 0);
  task.nodeInstanceId = value.intOr("nodeInstanceId", 0);
  task.nodeId = value.stringOr("nodeId", "");
  task.name = value.stringOr("taskName", value.stringOr("name", ""));
  task.assignee = value.stringOr("assignee", "");
  task.candidateUsers = readStringArray(value.at("candidateUsers"));
  task.candidateRoles = readStringArray(value.at("candidateRoles"));
  task.status = parseTaskStatus(value.stringOr("status", "PENDING"));
  task.priority = static_cast<int>(value.intOr("priority", 0));
  task.createTime = value.intOr("createTime", 0);
  task.claimTime = value.intOr("claimTime", 0);
  task.completeTime = value.intOr("completeTime", 0);
  task.dueTime = value.intOr("dueTime", 0);
  task.comment = value.stringOr("comment", "");
  task.version = value.intOr("version", 1);
  task.parentTaskId = value.intOr("parentTaskId", 0);
  task.signType = value.stringOr("signType", "NORMAL");
  task.signMode = value.stringOr("signMode", "ANY");
  task.strategy = parseCompleteStrategy(value.stringOr("strategy", "ANY"));
  task.action = value.stringOr("action", "");
  task.tenantId = value.stringOr("tenantId", "default");
  task.countersignMember = value.boolOr("countersignMember", false);
  return task;
}

Value HistoryRecord::toJson() const {
  Value out = Value::object();
  out.set("id", Value(id));
  out.set("processInstanceId", Value(processInstanceId));
  out.set("nodeInstanceId", Value(nodeInstanceId));
  if (taskId != 0) out.set("taskId", Value(taskId));
  out.set("nodeId", Value(nodeId));
  out.set("nodeName", Value(nodeName));
  out.set("action", Value(action));
  out.set("operator", Value(operatorUser));
  if (!comment.empty()) out.set("comment", Value(comment));
  if (!input.isNull()) out.set("inputData", input);
  if (!output.isNull()) out.set("outputData", output);
  if (!traceId.empty()) out.set("traceId", Value(traceId));
  out.set("createdAt", Value(createdAt));
  return out;
}

HistoryRecord HistoryRecord::fromJson(const Value& value) {
  HistoryRecord record;
  record.id = value.intOr("id", 0);
  record.processInstanceId = value.intOr("processInstanceId", 0);
  record.nodeInstanceId = value.intOr("nodeInstanceId", 0);
  record.taskId = value.intOr("taskId", 0);
  record.nodeId = value.stringOr("nodeId", "");
  record.nodeName = value.stringOr("nodeName", "");
  record.action = value.stringOr("action", "");
  record.operatorUser = value.stringOr("operator", value.stringOr("operatorUser", ""));
  record.comment = value.stringOr("comment", "");
  record.input = value.contains("inputData") ? value.at("inputData") : Value();
  record.output = value.contains("outputData") ? value.at("outputData") : Value();
  record.traceId = value.stringOr("traceId", "");
  record.createdAt = value.intOr("createdAt", 0);
  return record;
}

Value EventRecord::toJson() const {
  Value out = Value::object();
  out.set("id", Value(id));
  out.set("eventId", Value(eventId));
  out.set("processInstanceId", Value(processInstanceId));
  if (nodeInstanceId != 0) out.set("nodeInstanceId", Value(nodeInstanceId));
  if (taskId != 0) out.set("taskId", Value(taskId));
  out.set("eventType", Value(toString(eventType)));
  out.set("status", Value(status));
  if (!payload.isNull()) out.set("payload", payload);
  if (!listeners.isNull()) out.set("listeners", listeners);
  out.set("retryCount", Value(retryCount));
  out.set("nextRetryTime", Value(nextRetryTime));
  out.set("createdAt", Value(createdAt));
  out.set("updatedAt", Value(updatedAt));
  if (!error.empty()) out.set("error", Value(error));
  return out;
}

EventRecord EventRecord::fromJson(const Value& value) {
  EventRecord event;
  event.id = value.intOr("id", 0);
  event.eventId = value.stringOr("eventId", "");
  event.processInstanceId = value.intOr("processInstanceId", 0);
  event.nodeInstanceId = value.intOr("nodeInstanceId", 0);
  event.taskId = value.intOr("taskId", 0);
  event.eventType = parseEventType(value.stringOr("eventType", "NODE_STARTED"));
  event.status = value.stringOr("status", "PENDING");
  event.payload = value.contains("payload") ? value.at("payload") : Value();
  event.listeners = value.contains("listeners") ? value.at("listeners") : Value();
  event.retryCount = static_cast<int>(value.intOr("retryCount", 0));
  event.nextRetryTime = value.intOr("nextRetryTime", 0);
  event.createdAt = value.intOr("createdAt", 0);
  event.updatedAt = value.intOr("updatedAt", 0);
  event.error = value.stringOr("error", "");
  return event;
}

Value TimerJob::toJson() const {
  Value out = Value::object();
  out.set("id", Value(id));
  out.set("processInstanceId", Value(processInstanceId));
  if (nodeInstanceId != 0) out.set("nodeInstanceId", Value(nodeInstanceId));
  if (taskId != 0) out.set("taskId", Value(taskId));
  out.set("jobType", Value(jobType));
  out.set("triggerTime", Value(triggerTime));
  out.set("status", Value(status));
  if (!payload.isNull()) out.set("payload", payload);
  out.set("retryCount", Value(retryCount));
  out.set("createdAt", Value(createdAt));
  return out;
}

TimerJob TimerJob::fromJson(const Value& value) {
  TimerJob job;
  job.id = value.intOr("id", 0);
  job.processInstanceId = value.intOr("processInstanceId", 0);
  job.nodeInstanceId = value.intOr("nodeInstanceId", 0);
  job.taskId = value.intOr("taskId", 0);
  job.jobType = value.stringOr("jobType", "");
  job.triggerTime = value.intOr("triggerTime", 0);
  job.status = value.stringOr("status", "PENDING");
  job.payload = value.contains("payload") ? value.at("payload") : Value();
  job.retryCount = static_cast<int>(value.intOr("retryCount", 0));
  job.createdAt = value.intOr("createdAt", 0);
  return job;
}

Value ParallelBranch::toJson() const {
  Value out = Value::object();
  out.set("id", Value(id));
  out.set("processInstanceId", Value(processInstanceId));
  out.set("gatewayNodeId", Value(gatewayNodeId));
  out.set("branchId", Value(branchId));
  out.set("status", Value(status));
  out.set("completedAt", Value(completedAt));
  return out;
}

ParallelBranch ParallelBranch::fromJson(const Value& value) {
  ParallelBranch branch;
  branch.id = value.intOr("id", 0);
  branch.processInstanceId = value.intOr("processInstanceId", 0);
  branch.gatewayNodeId = value.stringOr("gatewayNodeId", "");
  branch.branchId = value.stringOr("branchId", "");
  branch.status = value.stringOr("status", "RUNNING");
  branch.completedAt = value.intOr("completedAt", 0);
  return branch;
}

}  // namespace wf

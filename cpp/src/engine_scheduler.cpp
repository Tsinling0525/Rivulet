// Scheduler: timer nodes, node/task timeouts, escalations and outbox delivery
// with the PRD §29.3 retry ladder.
#include <algorithm>

#include "wf/engine.hpp"
#include "wf/expression.hpp"

namespace wf {
namespace {

using json::Value;

}  // namespace

int ProcessEngine::tick(int maxJobs) {
  int processed = 0;
  while (processed < maxJobs) {
    std::vector<TimerJob> due = repository_.dueTimers(clock_.now(), 1);
    if (due.empty()) break;
    TimerJob job = due.front();
    if (!fireTimer(job)) break;  // 例如流程挂起，稍后再处理
    ++processed;
  }
  if (options_.autoDispatchEvents) dispatchEvents();
  return processed;
}

bool ProcessEngine::fireTimer(TimerJob& job) {
  InstanceLock lock = lockInstance(job.processInstanceId);
  ProcessInstance* stored = repository_.findInstance(job.processInstanceId);
  if (stored == nullptr) {
    job.status = "FAILED";
    repository_.updateTimer(job);
    return true;
  }
  ProcessInstance instance = *stored;
  if (instance.status == ProcessStatus::Suspended) {
    return false;  // 挂起流程的定时器保留，恢复后再触发
  }
  const ProcessDefinition* definition = definitionFor(instance);
  if (definition == nullptr) {
    job.status = "FAILED";
    repository_.updateTimer(job);
    return true;
  }
  if (isTerminal(instance.status)) {
    job.status = "CANCELLED";
    repository_.updateTimer(job);
    return true;
  }
  job.status = "DONE";
  repository_.updateTimer(job);

  RunContext context;
  const std::string nodeId = job.payload.stringOr("nodeId", "");
  const NodeDefinition* node = nodeId.empty() ? nullptr : definition->findNode(nodeId);

  if (job.jobType == "NODE_TIMER") {
    NodeInstance* nodePtr = repository_.findNodeInstance(job.nodeInstanceId);
    if (nodePtr == nullptr || nodePtr->status != NodeInstanceStatus::Waiting) {
      return true;
    }
    NodeInstance nodeInstance = *nodePtr;
    recordHistory(instance, node, &nodeInstance, nullptr, "TIMER_FIRED", options_.systemUser,
                  job.payload.stringOr("duration", ""), Value(), Value());
    Value output = Value::object();
    output.set("firedAt", Value(clock_.now()));
    resumeNodeInstance(instance, *definition, nodeInstance, output, context);
    return true;
  }

  if (job.jobType == "TASK_TIMEOUT" || job.jobType == "NODE_TIMEOUT") {
    if (node == nullptr) return true;
    Task taskStorage;
    Task* task = nullptr;
    if (job.taskId != 0) {
      if (Task* found = repository_.findTask(job.taskId);
          found != nullptr && isOpen(found->status)) {
        taskStorage = *found;
        task = &taskStorage;
      }
    }
    if (task == nullptr && job.nodeInstanceId != 0) {
      for (Task candidate : repository_.tasksByNodeInstance(job.nodeInstanceId)) {
        if (isOpen(candidate.status)) {
          taskStorage = candidate;
          task = &taskStorage;
          break;
        }
      }
    }
    NodeInstance* nodePtr = repository_.findNodeInstance(job.nodeInstanceId);
    NodeInstance nodeStorage;
    if (nodePtr != nullptr) nodeStorage = *nodePtr;

    TimeoutAction lastAction = TimeoutAction::Remind;
    applyTimeoutSteps(instance, *definition, *node, task, nodePtr != nullptr ? &nodeStorage : nullptr,
                      job.payload.at("actions"), lastAction, context);
    return true;
  }
  return true;
}

bool ProcessEngine::applyTimeoutSteps(ProcessInstance& instance, const ProcessDefinition& definition,
                                      const NodeDefinition& node, Task* task,
                                      NodeInstance* nodeInstance, const json::Value& steps,
                                      TimeoutAction& lastAction, RunContext& context) {
  (void)definition;
  (void)context;
  if (!steps.isArray() || steps.empty()) return true;
  const std::string subject = task != nullptr ? task->assignee : std::string();
  Value payload = Value::object();
  payload.set("nodeId", Value(node.id));
  payload.set("processInstanceId", Value(instance.id));
  payload.set("duration", Value(node.taskTimeout.duration.empty() ? node.nodeTimeout.duration
                                                                 : node.taskTimeout.duration));
  if (task != nullptr) {
    payload.set("taskId", Value(task->id));
    payload.set("assignee", Value(task->assignee));
  }

  recordHistory(instance, &node, nodeInstance, task, "TASK_TIMEOUT", options_.systemUser,
                "触发超时策略", Value(), steps);
  publishEvent(instance, &node, nodeInstance, task, EventType::TaskTimeout, payload);

  for (const auto& item : steps.items()) {
    const TimeoutStep step = TimeoutStep::fromJson(item);
    lastAction = step.action;
    switch (step.action) {
      case TimeoutAction::Remind: {
        if (notifier_ != nullptr) {
          notifier_->send(step.channel.empty() ? "EMAIL" : step.channel,
                          step.target.empty() ? subject : step.target,
                          step.message.empty() ? "TASK_TIMEOUT" : step.message, payload);
        }
        recordHistory(instance, &node, nodeInstance, task, "TIMEOUT_REMIND", options_.systemUser,
                      step.channel, Value(), Value());
        break;
      }
      case TimeoutAction::Escalate: {
        if (task == nullptr || organization_ == nullptr) break;
        int level = 1;
        if (!step.target.empty()) {
          try {
            level = std::stoi(step.target);
          } catch (...) {
            level = 1;
          }
        }
        const std::string leader = organization_->leaderOf(task->assignee, std::max(1, level));
        if (leader.empty()) {
          recordHistory(instance, &node, nodeInstance, task, "TIMEOUT_ESCALATE_FAILED",
                        options_.systemUser, "找不到上级主管", Value(), Value());
          break;
        }
        OperationResult transferred =
            transferTask(task->id, options_.systemUser, leader, "超时升级至上级");
        recordHistory(instance, &node, nodeInstance, task, "TIMEOUT_ESCALATE", options_.systemUser,
                      "升级给 " + leader, Value(), Value());
        if (transferred.ok) {
          instance = *repository_.findInstance(instance.id);
          return true;  // 任务已转交给他人，后续动作交给新的处理人
        }
        break;
      }
      case TimeoutAction::Transfer: {
        if (task == nullptr || step.target.empty()) break;
        const OperationResult transferred =
            transferTask(task->id, options_.systemUser, step.target, "超时转办");
        if (transferred.ok) {
          instance = *repository_.findInstance(instance.id);
          return true;
        }
        break;
      }
      case TimeoutAction::AutoApprove:
      case TimeoutAction::AutoReject: {
        if (task == nullptr) break;
        TaskOperation operation;
        operation.action = step.action == TimeoutAction::AutoApprove ? TaskAction::Approve
                                                                    : TaskAction::Reject;
        operation.operatorUser = options_.systemUser;
        operation.comment = "超时自动处理";
        const OperationResult completed = completeTask(task->id, operation);
        if (!completed.ok) {
          recordHistory(instance, &node, nodeInstance, task, "TIMEOUT_AUTO_FAILED",
                        options_.systemUser, completed.error, Value(), Value());
        }
        instance = *repository_.findInstance(instance.id);
        return true;
      }
      case TimeoutAction::Terminate: {
        terminateInstance(instance.id, options_.systemUser, "超时终止流程");
        instance = *repository_.findInstance(instance.id);
        return true;
      }
    }
  }
  updateInstance(instance, "timeout");
  return true;
}

int ProcessEngine::dispatchEvents(int maxEvents) {
  int handled = 0;
  int guard = 0;
  const int guardLimit = std::max(1, maxEvents) * 4;
  while (handled < maxEvents && guard++ < guardLimit) {
    std::vector<EventRecord> pending = repository_.pendingEvents(clock_.now(), 1);
    if (pending.empty()) break;
    EventRecord event = pending.front();
    bool allDelivered = true;
    std::string lastError;
    const std::vector<json::Value> listeners = event.listeners.isArray()
                                                  ? std::vector<json::Value>(event.listeners.items().begin(),
                                                                             event.listeners.items().end())
                                                  : std::vector<json::Value>();
    for (const auto& item : listeners) {
      const ListenerSpec listener = ListenerSpec::fromJson(item);
      if (publisher_ == nullptr) {
        allDelivered = false;
        lastError = "未配置事件发布器";
        break;
      }
      const std::string destination = listener.type == "mq" ? listener.topic : listener.url;
      std::string error;
      if (!publisher_->publish(event.eventType, listener.type, destination, event.payload, &error)) {
        allDelivered = false;
        lastError = error.empty() ? "投递失败" : error;
      }
    }

    event.updatedAt = clock_.now();
    if (allDelivered) {
      event.status = "SUCCESS";
      event.error.clear();
    } else {
      event.retryCount += 1;
      event.error = lastError;
      if (event.retryCount > options_.eventMaxRetries) {
        event.status = "FAILED";  // 进入异常任务，等待人工处理（PRD §36.4）
      } else {
        const std::vector<long long>& backoff = options_.retryBackoffMillis;
        const std::size_t index = std::min<std::size_t>(
            static_cast<std::size_t>(event.retryCount), backoff.empty() ? 0 : backoff.size() - 1);
        const long long delay = backoff.empty() ? 0 : backoff[index];
        event.nextRetryTime = clock_.now() + delay;
      }
    }
    repository_.updateEvent(event);
    ++handled;
  }
  return handled;
}

}  // namespace wf

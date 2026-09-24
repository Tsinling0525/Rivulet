// Process engine core: state machine driven execution of a process
// definition, task management, gateways, sub-processes, timers, timeouts and
// the transactional event outbox.
//
// Design notes (mapped to the PRD):
//  * 定义-实例分离 (§3.1): instances bind to a concrete definition version.
//  * 状态驱动 (§3.2/§20): every status change goes through canTransit().
//  * 执行可追溯 (§3.3): every step writes wf_process_history + node instance IO.
//  * 幂等 (§3.4/§22): businessKey dedup, task version optimistic lock,
//    eventId based idempotent delivery.
//  * 异步化 (§3.5/§21.2): external calls/notifications are recorded in the
//    event table and delivered by dispatchEvents() with retry/backoff.
//  * 并行汇聚 (§7.5/§24): tokens + wf_parallel_branch arrival counting.
#pragma once

#include <map>
#include <memory>
#include <mutex>
#include <string>
#include <vector>

#include "wf/entity.hpp"
#include "wf/expression.hpp"
#include "wf/json.hpp"
#include "wf/model.hpp"
#include "wf/repository.hpp"
#include "wf/services.hpp"
#include "wf/types.hpp"

namespace wf {

struct EngineOptions {
  int maxStepsPerRun = 2000;  // 死循环保护
  int eventMaxRetries = 5;    // PRD §29.3
  std::vector<long long> retryBackoffMillis{0, 10000, 60000, 300000, 1800000};
  bool autoDispatchEvents = true;  // 操作结束后立即投递 outbox
  bool enforceFieldPermissions = true;
  bool enforceStateMachine = true;
  std::string systemUser = "SYSTEM";
};

struct StartRequest {
  std::string tenantId = "default";
  std::string processCode;
  std::string businessKey;
  std::string initiator;
  json::Value variables;
  int version = 0;  // 0 = 当前已发布版本
  long long parentInstanceId = 0;
  long long parentNodeInstanceId = 0;
};

struct OperationResult {
  bool ok = false;
  std::string error;
  ProcessInstance instance;
  Task task;
  std::vector<Task> createdTasks;
  bool idempotentReplay = false;
};

struct TaskOperation {
  TaskAction action = TaskAction::Approve;
  std::string operatorUser;
  std::string comment;
  json::Value variables;  // 表单/变量补丁
  std::string targetUser;
  std::vector<std::string> targetUsers;
  std::string signType = "AFTER";
  std::string signMode = "ALL";
  std::string targetNodeId;  // 退回目标节点
  long long expectedVersion = -1;
};

struct TodoQuery {
  std::string tenantId = "default";
  std::string assignee;
  std::size_t page = 1;
  std::size_t size = 20;
};

class ProcessEngine {
 public:
  ProcessEngine(Repository& repository, Clock& clock, EngineOptions options = EngineOptions());
  ~ProcessEngine();

  void setOrganization(OrganizationService* organization) { organization_ = organization; }
  void setServiceInvoker(ServiceInvoker* invoker) { invoker_ = invoker; }
  void setNotifier(Notifier* notifier) { notifier_ = notifier; }
  void setEventPublisher(EventPublisher* publisher) { publisher_ = publisher; }
  EngineOptions& options() { return options_; }

  // ---------------------------------------------------------------------
  // 流程定义（PRD §5.2.2）
  // ---------------------------------------------------------------------
  ProcessDefinition deployDefinition(const ProcessDefinition& definition, bool publish);
  ValidationResult validateDefinition(const ProcessDefinition& definition) const;

  // ---------------------------------------------------------------------
  // 流程实例（PRD §7.1、§18.2）
  // ---------------------------------------------------------------------
  OperationResult startProcess(const StartRequest& request);
  OperationResult cancelInstance(long long instanceId, const std::string& user,
                                 const std::string& comment = "");
  OperationResult suspendInstance(long long instanceId, const std::string& user);
  OperationResult resumeInstance(long long instanceId, const std::string& user);
  OperationResult terminateInstance(long long instanceId, const std::string& user,
                                    const std::string& comment = "");
  OperationResult retryNode(long long instanceId, const std::string& nodeId, const std::string& user);
  OperationResult jumpToNode(long long instanceId, const std::string& nodeId, const std::string& user,
                             const std::string& comment = "");
  // 等待节点 / 外部事件回调（PRD §4.3.8）
  OperationResult signalEvent(const std::string& eventKey, const json::Value& payload,
                              long long instanceId = 0);

  // ---------------------------------------------------------------------
  // 任务操作（PRD §10、§18.3）
  // ---------------------------------------------------------------------
  OperationResult claimTask(long long taskId, const std::string& user);
  OperationResult completeTask(long long taskId, const TaskOperation& operation);
  OperationResult transferTask(long long taskId, const std::string& by, const std::string& to,
                               const std::string& comment = "");
  OperationResult delegateTask(long long taskId, const std::string& by, const std::string& to,
                               const std::string& comment = "");
  OperationResult addSignTask(long long taskId, const std::string& by,
                              const std::vector<std::string>& users, const std::string& type,
                              const std::string& mode, const std::string& comment = "");
  OperationResult returnTask(long long taskId, const std::string& by,
                             const std::string& targetNodeId, const std::string& comment = "");
  OperationResult withdrawInstance(long long instanceId, const std::string& user,
                                   const std::string& comment = "");

  // ---------------------------------------------------------------------
  // 调度：定时节点、节点/任务超时、事件投递（PRD §5.2.7、§12、§21.2）
  // ---------------------------------------------------------------------
  int tick(int maxJobs = 100);
  int dispatchEvents(int maxEvents = 100);

  // ---------------------------------------------------------------------
  // 查询服务（PRD §5.2.8、§18.3、§18.4）
  // ---------------------------------------------------------------------
  std::vector<Task> todoTasks(const TodoQuery& query);
  std::vector<Task> doneTasks(const TodoQuery& query);
  std::vector<Task> candidateTasks(const std::string& tenantId, const std::string& user);
  std::vector<Task> tasksByInstance(long long instanceId);
  std::vector<HistoryRecord> history(long long instanceId);
  std::vector<NodeInstance> nodeInstances(long long instanceId);
  std::vector<EventRecord> events(long long instanceId);
  std::vector<ParallelBranch> branches(long long instanceId);
  std::vector<ProcessInstance> instances(const std::string& tenantId, const std::string& status,
                                         std::size_t limit = 100);
  std::vector<FieldPermission> fieldPermissions(long long instanceId, const std::string& nodeId);
  json::Value statistics(const std::string& tenantId);

  const ProcessDefinition* definitionOf(const ProcessInstance& instance) const;
  ProcessInstance* findInstance(long long instanceId) { return repository_.findInstance(instanceId); }
  // 表达式上下文（PRD §13.2）
  json::Value expressionContext(const ProcessInstance& instance,
                                const NodeDefinition* node = nullptr) const;
  expr::Functions functionsFor(const ProcessInstance& instance) const;

 private:
  // 流转令牌：nodeId 为要执行的节点，edgeId 为进入该节点所走的连线。
  struct Token {
    std::string nodeId;
    std::string edgeId;
  };

  struct RunContext {
    std::vector<Task> createdTasks;
    int steps = 0;
  };

  // 实例级串行化（PRD §23.1：同一实例同时只允许一个推进者）
  class InstanceLock {
   public:
    InstanceLock(ProcessEngine& engine, long long instanceId);
    ~InstanceLock();
    InstanceLock(const InstanceLock&) = delete;
    InstanceLock& operator=(const InstanceLock&) = delete;
    InstanceLock(InstanceLock&&) noexcept = default;
    InstanceLock& operator=(InstanceLock&&) noexcept = default;

   private:
    std::shared_ptr<std::recursive_mutex> mutex_;
    std::unique_ptr<std::unique_lock<std::recursive_mutex>> guard_;
  };

  // --- 执行核心 ---------------------------------------------------------
  bool drainQueue(ProcessInstance& instance, const ProcessDefinition& definition,
                  std::vector<Token> queue, RunContext& context);
  bool executeNode(ProcessInstance& instance, const ProcessDefinition& definition,
                   const NodeDefinition& node, const Token& token, std::vector<Token>& queue,
                   RunContext& context);
  bool completeNode(ProcessInstance& instance, const ProcessDefinition& definition,
                    NodeInstance nodeInstance, const NodeDefinition& node, const json::Value& output,
                    std::vector<Token>& queue, RunContext& context);
  bool failNode(ProcessInstance& instance, const ProcessDefinition& definition,
                const NodeDefinition& node, NodeInstance nodeInstance, const std::string& message,
                RunContext& context);
  bool resumeNodeInstance(ProcessInstance& instance, const ProcessDefinition& definition,
                          NodeInstance& nodeInstance, const json::Value& output, RunContext& context);
  bool finishInstance(ProcessInstance& instance, const ProcessDefinition& definition,
                      ProcessStatus status, const std::string& result, RunContext& context);
  bool rewindTo(ProcessInstance& instance, const ProcessDefinition& definition,
                const std::string& targetNodeId, const std::string& user, const std::string& action,
                const std::string& comment, RunContext& context);
  bool notifyParent(ProcessInstance& child, RunContext& context);
  bool hasJoinSemantics(const ProcessDefinition& definition, const NodeDefinition& node) const;
  bool allBranchesArrived(ProcessInstance& instance, const ProcessDefinition& definition,
                          const NodeDefinition& node);
  std::vector<std::string> nextNodes(ProcessInstance& instance, const ProcessDefinition& definition,
                                     const NodeDefinition& node, const EdgeDefinition** chosen = nullptr,
                                     std::string* error = nullptr);

  // --- 审批人 -----------------------------------------------------------
  std::vector<std::string> resolveAssignees(const ProcessInstance& instance, const NodeDefinition& node,
                                            const NodeInstance* previous);
  std::vector<std::string> resolveRule(const ApprovalRule& rule, const ProcessInstance& instance,
                                       const NodeDefinition& node);

  // --- 记录 -------------------------------------------------------------
  HistoryRecord recordHistory(ProcessInstance& instance, const NodeDefinition* node,
                              const NodeInstance* nodeInstance, const Task* task,
                              const std::string& action, const std::string& user,
                              const std::string& comment, json::Value input, json::Value output);
  EventRecord publishEvent(ProcessInstance& instance, const NodeDefinition* node,
                           const NodeInstance* nodeInstance, const Task* task, EventType type,
                           json::Value payload);
  TimerJob scheduleTimer(ProcessInstance& instance, const NodeInstance* nodeInstance, const Task* task,
                         const std::string& jobType, long long triggerTime, json::Value payload);
  void cancelOpenTasks(long long instanceId, const NodeDefinition* node, const std::string& reason);

  // --- 任务 -------------------------------------------------------------
  bool completeNodeAfterTask(ProcessInstance& instance, const ProcessDefinition& definition,
                             NodeInstance& nodeInstance, const NodeDefinition& node, Task& task,
                             const TaskOperation& operation, RunContext& context);
  bool applyRejection(ProcessInstance& instance, const ProcessDefinition& definition,
                      NodeInstance& nodeInstance, const NodeDefinition& node, const Task& task,
                      const TaskOperation& operation, RunContext& context);
  bool filterAndValidateForm(const NodeDefinition& node, const ProcessInstance& instance,
                             json::Value* patch, std::string* error) const;
  Task makeTask(ProcessInstance& instance, const NodeInstance& nodeInstance, const NodeDefinition& node,
                const std::string& assignee, const std::string& signType, long long parentTaskId,
                CompleteStrategy strategy);
  void cancelTask(Task& task, const std::string& reason);
  void scheduleTaskTimeout(ProcessInstance& instance, const NodeDefinition& node,
                           NodeInstance& nodeInstance, Task& task);
  void scheduleNodeTimeout(ProcessInstance& instance, const NodeDefinition& node,
                           NodeInstance& nodeInstance);

  // --- 调度 -------------------------------------------------------------
  bool fireTimer(TimerJob& job);
  bool applyTimeoutSteps(ProcessInstance& instance, const ProcessDefinition& definition,
                         const NodeDefinition& node, Task* task, NodeInstance* nodeInstance,
                         const json::Value& steps, TimeoutAction& lastAction, RunContext& context);

  // --- helpers ----------------------------------------------------------
  bool updateInstance(ProcessInstance& instance, const std::string& what);
  InstanceLock lockInstance(long long instanceId) { return InstanceLock(*this, instanceId); }
  void mergeVariables(ProcessInstance& instance, const json::Value& patch);
  const ProcessDefinition* definitionFor(const ProcessInstance& instance) const;
  json::Value mergeTimeoutPayload(const json::Value& current, const TimeoutPolicy& policy) const;

  Repository& repository_;
  Clock& clock_;
  EngineOptions options_;
  OrganizationService* organization_ = nullptr;
  ServiceInvoker* invoker_ = nullptr;
  Notifier* notifier_ = nullptr;
  EventPublisher* publisher_ = nullptr;
  EmptyOrganization defaultOrganization_;
  std::map<long long, std::shared_ptr<std::recursive_mutex>> locks_;
  std::mutex locksMutex_;
};

}  // namespace wf

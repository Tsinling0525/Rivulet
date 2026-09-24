// Persistence boundary. The engine only talks to this interface, so swapping
// the in-memory store for MySQL/Redis later (PRD §29.1) is a matter of adding
// one implementation.
#pragma once

#include <cstddef>
#include <memory>
#include <string>
#include <vector>

#include "wf/entity.hpp"
#include "wf/json.hpp"
#include "wf/model.hpp"

namespace wf {

class Repository {
 public:
  virtual ~Repository() = default;

  virtual long long nextId() = 0;

  // --- 流程定义 -----------------------------------------------------------
  virtual ProcessDefinition saveDefinition(const ProcessDefinition& definition) = 0;
  // version <= 0 selects the active (latest published) version.
  virtual const ProcessDefinition* findDefinition(const std::string& tenantId, const std::string& code,
                                                  int version) = 0;
  virtual std::vector<ProcessDefinition> listDefinitions(const std::string& tenantId,
                                                         const std::string& code) = 0;
  virtual bool setDefinitionStatus(long long id, DefinitionStatus status) = 0;

  // --- 流程实例 -----------------------------------------------------------
  virtual ProcessInstance createInstance(const ProcessInstance& instance) = 0;
  virtual ProcessInstance* findInstance(long long id) = 0;
  virtual ProcessInstance* findInstanceByBusinessKey(const std::string& tenantId,
                                                    const std::string& processCode,
                                                    const std::string& businessKey) = 0;
  virtual bool updateInstance(const ProcessInstance& instance, long long expectedVersion = -1) = 0;
  virtual std::vector<ProcessInstance> listInstances(const std::string& tenantId, const std::string& status,
                                                     std::size_t limit) = 0;

  // --- 节点实例 -----------------------------------------------------------
  virtual NodeInstance createNodeInstance(const NodeInstance& node) = 0;
  virtual NodeInstance* findNodeInstance(long long id) = 0;
  virtual bool updateNodeInstance(const NodeInstance& node) = 0;
  virtual std::vector<NodeInstance> nodeInstances(long long instanceId) = 0;
  virtual NodeInstance* latestNodeInstance(long long instanceId, const std::string& nodeId) = 0;
  virtual std::vector<NodeInstance> openNodeInstances(long long instanceId) = 0;

  // --- 任务 ---------------------------------------------------------------
  virtual Task createTask(const Task& task) = 0;
  virtual Task* findTask(long long id) = 0;
  virtual bool updateTask(const Task& task, long long expectedVersion = -1) = 0;
  virtual std::vector<Task> tasksByNodeInstance(long long nodeInstanceId) = 0;
  virtual std::vector<Task> tasksByInstance(long long instanceId) = 0;
  virtual std::vector<Task> todoTasks(const std::string& tenantId, const std::string& assignee,
                                      std::size_t page, std::size_t size) = 0;
  virtual std::vector<Task> doneTasks(const std::string& tenantId, const std::string& assignee,
                                      std::size_t page, std::size_t size) = 0;
  virtual std::vector<Task> candidateTasks(const std::string& tenantId, const std::string& user,
                                           const std::vector<std::string>& roles) = 0;

  // --- 轨迹 ---------------------------------------------------------------
  virtual HistoryRecord appendHistory(const HistoryRecord& record) = 0;
  virtual std::vector<HistoryRecord> histories(long long instanceId) = 0;

  // --- 事件（outbox） -----------------------------------------------------
  virtual EventRecord createEvent(const EventRecord& event) = 0;
  virtual bool updateEvent(const EventRecord& event) = 0;
  virtual EventRecord* findEvent(long long id) = 0;
  virtual std::vector<EventRecord> pendingEvents(long long now, std::size_t limit) = 0;
  virtual std::vector<EventRecord> events(long long instanceId) = 0;

  // --- 定时任务 -----------------------------------------------------------
  virtual TimerJob createTimer(const TimerJob& job) = 0;
  virtual bool updateTimer(const TimerJob& job) = 0;
  virtual std::vector<TimerJob> dueTimers(long long now, std::size_t limit) = 0;
  virtual std::vector<TimerJob> timersByInstance(long long instanceId) = 0;

  // --- 并行分支 -----------------------------------------------------------
  virtual ParallelBranch createBranch(const ParallelBranch& branch) = 0;
  virtual bool updateBranch(const ParallelBranch& branch) = 0;
  virtual std::vector<ParallelBranch> branches(long long instanceId,
                                               const std::string& gatewayNodeId) = 0;
  virtual bool clearBranches(long long instanceId) = 0;

  // --- 运维 / 诊断 --------------------------------------------------------
  virtual json::Value snapshot() = 0;
  virtual void restore(const json::Value& snapshot) = 0;
  virtual void clear() = 0;
  virtual std::size_t entityCount() = 0;
};

// PRD §17 的默认实现：进程内存储 + 全量 JSON 快照。
class InMemoryRepository : public Repository {
 public:
  long long nextId() override;

  ProcessDefinition saveDefinition(const ProcessDefinition& definition) override;
  const ProcessDefinition* findDefinition(const std::string& tenantId, const std::string& code,
                                          int version) override;
  std::vector<ProcessDefinition> listDefinitions(const std::string& tenantId,
                                                 const std::string& code) override;
  bool setDefinitionStatus(long long id, DefinitionStatus status) override;

  ProcessInstance createInstance(const ProcessInstance& instance) override;
  ProcessInstance* findInstance(long long id) override;
  ProcessInstance* findInstanceByBusinessKey(const std::string& tenantId,
                                             const std::string& processCode,
                                             const std::string& businessKey) override;
  bool updateInstance(const ProcessInstance& instance, long long expectedVersion = -1) override;
  std::vector<ProcessInstance> listInstances(const std::string& tenantId, const std::string& status,
                                             std::size_t limit) override;

  NodeInstance createNodeInstance(const NodeInstance& node) override;
  NodeInstance* findNodeInstance(long long id) override;
  bool updateNodeInstance(const NodeInstance& node) override;
  std::vector<NodeInstance> nodeInstances(long long instanceId) override;
  NodeInstance* latestNodeInstance(long long instanceId, const std::string& nodeId) override;
  std::vector<NodeInstance> openNodeInstances(long long instanceId) override;

  Task createTask(const Task& task) override;
  Task* findTask(long long id) override;
  bool updateTask(const Task& task, long long expectedVersion = -1) override;
  std::vector<Task> tasksByNodeInstance(long long nodeInstanceId) override;
  std::vector<Task> tasksByInstance(long long instanceId) override;
  std::vector<Task> todoTasks(const std::string& tenantId, const std::string& assignee,
                              std::size_t page, std::size_t size) override;
  std::vector<Task> doneTasks(const std::string& tenantId, const std::string& assignee,
                              std::size_t page, std::size_t size) override;
  std::vector<Task> candidateTasks(const std::string& tenantId, const std::string& user,
                                   const std::vector<std::string>& roles) override;

  HistoryRecord appendHistory(const HistoryRecord& record) override;
  std::vector<HistoryRecord> histories(long long instanceId) override;

  EventRecord createEvent(const EventRecord& event) override;
  bool updateEvent(const EventRecord& event) override;
  EventRecord* findEvent(long long id) override;
  std::vector<EventRecord> pendingEvents(long long now, std::size_t limit) override;
  std::vector<EventRecord> events(long long instanceId) override;

  TimerJob createTimer(const TimerJob& job) override;
  bool updateTimer(const TimerJob& job) override;
  std::vector<TimerJob> dueTimers(long long now, std::size_t limit) override;
  std::vector<TimerJob> timersByInstance(long long instanceId) override;

  ParallelBranch createBranch(const ParallelBranch& branch) override;
  bool updateBranch(const ParallelBranch& branch) override;
  std::vector<ParallelBranch> branches(long long instanceId, const std::string& gatewayNodeId) override;
  bool clearBranches(long long instanceId) override;

  json::Value snapshot() override;
  void restore(const json::Value& snapshot) override;
  void clear() override;
  std::size_t entityCount() override;

 private:
  long long nextId_ = 1000;
  std::vector<ProcessDefinition> definitions_;
  std::vector<std::unique_ptr<ProcessInstance>> instances_;
  std::vector<std::unique_ptr<NodeInstance>> nodeInstances_;
  std::vector<std::unique_ptr<Task>> tasks_;
  std::vector<HistoryRecord> histories_;
  std::vector<EventRecord> events_;
  std::vector<TimerJob> timers_;
  std::vector<ParallelBranch> branches_;
};

// 仓储快照（`wf-cli --repo <dir>` 的持久化方式）。
bool saveRepositoryToFile(Repository& repository, const std::string& path, std::string* error);
bool loadRepositoryFromFile(InMemoryRepository& repository, const std::string& path, std::string* error);

}  // namespace wf

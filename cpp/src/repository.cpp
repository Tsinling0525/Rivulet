#include "wf/repository.hpp"

#include <algorithm>
#include <fstream>
#include <sstream>

#include "wf/types.hpp"

namespace wf {
namespace {

using json::Value;

template <typename T>
T* findById(std::vector<std::unique_ptr<T>>& items, long long id) {
  for (auto& item : items) {
    if (item->id == id) return item.get();
  }
  return nullptr;
}

Value definitionArrayToJson(const std::vector<ProcessDefinition>& definitions) {
  Value array = Value::array();
  for (const auto& definition : definitions) array.push_back(definition.toJson());
  return array;
}

template <typename T>
Value entityArrayToJson(const std::vector<std::unique_ptr<T>>& items) {
  Value array = Value::array();
  for (const auto& item : items) array.push_back(item->toJson());
  return array;
}

template <typename T>
Value valueArrayToJson(const std::vector<T>& items) {
  Value array = Value::array();
  for (const auto& item : items) array.push_back(item.toJson());
  return array;
}

}  // namespace

long long InMemoryRepository::nextId() { return nextId_++; }

ProcessDefinition InMemoryRepository::saveDefinition(const ProcessDefinition& definition) {
  ProcessDefinition stored = definition;
  if (stored.id == 0) {
    stored.id = nextId();
    if (stored.version <= 0) {
      int maxVersion = 0;
      for (const auto& existing : definitions_) {
        if (existing.code == stored.code && existing.tenantId == stored.tenantId &&
            existing.version > maxVersion) {
          maxVersion = existing.version;
        }
      }
      stored.version = maxVersion + 1;
    }
    stored.createdAt = stored.createdAt != 0 ? stored.createdAt : 0;
    definitions_.push_back(stored);
    return stored;
  }
  for (auto& existing : definitions_) {
    if (existing.id == stored.id) {
      existing = stored;
      return stored;
    }
  }
  definitions_.push_back(stored);
  return stored;
}

const ProcessDefinition* InMemoryRepository::findDefinition(const std::string& tenantId,
                                                            const std::string& code, int version) {
  const ProcessDefinition* best = nullptr;
  for (const auto& definition : definitions_) {
    if (definition.code != code) continue;
    if (definition.tenantId != tenantId) continue;
    if (version > 0) {
      if (definition.version == version) return &definition;
      continue;
    }
    if (definition.status != DefinitionStatus::Published) continue;
    if (best == nullptr || definition.version > best->version) best = &definition;
  }
  return best;
}

std::vector<ProcessDefinition> InMemoryRepository::listDefinitions(const std::string& tenantId,
                                                                   const std::string& code) {
  std::vector<ProcessDefinition> out;
  for (const auto& definition : definitions_) {
    if (definition.tenantId != tenantId) continue;
    if (!code.empty() && definition.code != code) continue;
    out.push_back(definition);
  }
  std::sort(out.begin(), out.end(), [](const ProcessDefinition& a, const ProcessDefinition& b) {
    if (a.code != b.code) return a.code < b.code;
    return a.version < b.version;
  });
  return out;
}

bool InMemoryRepository::setDefinitionStatus(long long id, DefinitionStatus status) {
  for (auto& definition : definitions_) {
    if (definition.id == id) {
      // Only one published version per process code (PRD §2.2 发布/停用).
      if (status == DefinitionStatus::Published) {
        for (auto& other : definitions_) {
          if (other.id != id && other.code == definition.code && other.tenantId == definition.tenantId &&
              other.status == DefinitionStatus::Published) {
            other.status = DefinitionStatus::Disabled;
          }
        }
      }
      definition.status = status;
      return true;
    }
  }
  return false;
}

ProcessInstance InMemoryRepository::createInstance(const ProcessInstance& instance) {
  auto stored = std::unique_ptr<ProcessInstance>(new ProcessInstance(instance));
  if (stored->id == 0) stored->id = nextId();
  if (stored->version == 0) stored->version = 1;
  ProcessInstance copy = *stored;
  instances_.push_back(std::move(stored));
  return copy;
}

ProcessInstance* InMemoryRepository::findInstance(long long id) {
  return findById(instances_, id);
}

ProcessInstance* InMemoryRepository::findInstanceByBusinessKey(const std::string& tenantId,
                                                               const std::string& processCode,
                                                               const std::string& businessKey) {
  if (businessKey.empty()) return nullptr;
  for (auto& instance : instances_) {
    if (instance->tenantId == tenantId && instance->processCode == processCode &&
        instance->businessKey == businessKey && !isTerminal(instance->status)) {
      return instance.get();
    }
  }
  return nullptr;
}

bool InMemoryRepository::updateInstance(const ProcessInstance& instance, long long expectedVersion) {
  ProcessInstance* stored = findInstance(instance.id);
  if (stored == nullptr) return false;
  if (expectedVersion >= 0 && stored->version != expectedVersion) {
    return false;  // 乐观锁冲突（PRD §22.2）
  }
  const long long nextVersion = stored->version + 1;
  *stored = instance;
  stored->version = nextVersion;
  return true;
}

std::vector<ProcessInstance> InMemoryRepository::listInstances(const std::string& tenantId,
                                                              const std::string& status,
                                                              std::size_t limit) {
  std::vector<ProcessInstance> out;
  for (const auto& instance : instances_) {
    if (!tenantId.empty() && instance->tenantId != tenantId) continue;
    if (!status.empty() && toString(instance->status) != toLower(status) &&
        toLower(std::string(toString(instance->status))) != toLower(status)) {
      continue;
    }
    out.push_back(*instance);
    if (out.size() >= limit) break;
  }
  return out;
}

NodeInstance InMemoryRepository::createNodeInstance(const NodeInstance& node) {
  auto stored = std::unique_ptr<NodeInstance>(new NodeInstance(node));
  if (stored->id == 0) stored->id = nextId();
  NodeInstance copy = *stored;
  nodeInstances_.push_back(std::move(stored));
  return copy;
}

NodeInstance* InMemoryRepository::findNodeInstance(long long id) { return findById(nodeInstances_, id); }

bool InMemoryRepository::updateNodeInstance(const NodeInstance& node) {
  NodeInstance* stored = findNodeInstance(node.id);
  if (stored == nullptr) return false;
  *stored = node;
  return true;
}

std::vector<NodeInstance> InMemoryRepository::nodeInstances(long long instanceId) {
  std::vector<NodeInstance> out;
  for (const auto& node : nodeInstances_) {
    if (node->processInstanceId == instanceId) out.push_back(*node);
  }
  std::sort(out.begin(), out.end(),
            [](const NodeInstance& a, const NodeInstance& b) { return a.id < b.id; });
  return out;
}

NodeInstance* InMemoryRepository::latestNodeInstance(long long instanceId, const std::string& nodeId) {
  NodeInstance* best = nullptr;
  for (auto& node : nodeInstances_) {
    if (node->processInstanceId != instanceId || node->nodeId != nodeId) continue;
    if (best == nullptr || node->id > best->id) best = node.get();
  }
  return best;
}

std::vector<NodeInstance> InMemoryRepository::openNodeInstances(long long instanceId) {
  std::vector<NodeInstance> out;
  for (const auto& node : nodeInstances_) {
    if (node->processInstanceId == instanceId && isOpen(node->status)) out.push_back(*node);
  }
  return out;
}

Task InMemoryRepository::createTask(const Task& task) {
  auto stored = std::unique_ptr<Task>(new Task(task));
  if (stored->id == 0) stored->id = nextId();
  if (stored->version == 0) stored->version = 1;
  Task copy = *stored;
  tasks_.push_back(std::move(stored));
  return copy;
}

Task* InMemoryRepository::findTask(long long id) { return findById(tasks_, id); }

bool InMemoryRepository::updateTask(const Task& task, long long expectedVersion) {
  Task* stored = findTask(task.id);
  if (stored == nullptr) return false;
  if (expectedVersion >= 0 && stored->version != expectedVersion) return false;
  const long long nextVersion = stored->version + 1;
  *stored = task;
  stored->version = nextVersion;
  return true;
}

std::vector<Task> InMemoryRepository::tasksByNodeInstance(long long nodeInstanceId) {
  std::vector<Task> out;
  for (const auto& task : tasks_) {
    if (task->nodeInstanceId == nodeInstanceId) out.push_back(*task);
  }
  std::sort(out.begin(), out.end(), [](const Task& a, const Task& b) { return a.id < b.id; });
  return out;
}

std::vector<Task> InMemoryRepository::tasksByInstance(long long instanceId) {
  std::vector<Task> out;
  for (const auto& task : tasks_) {
    if (task->processInstanceId == instanceId) out.push_back(*task);
  }
  std::sort(out.begin(), out.end(), [](const Task& a, const Task& b) { return a.id < b.id; });
  return out;
}

std::vector<Task> InMemoryRepository::todoTasks(const std::string& tenantId, const std::string& assignee,
                                               std::size_t page, std::size_t size) {
  std::vector<Task> out;
  for (const auto& task : tasks_) {
    if (!assignee.empty() && task->assignee != assignee) continue;
    if (!tenantId.empty() && task->tenantId != tenantId) continue;
    if (task->status != TaskStatus::Pending && task->status != TaskStatus::Claimed) continue;
    out.push_back(*task);
  }
  std::sort(out.begin(), out.end(), [](const Task& a, const Task& b) { return a.id < b.id; });
  const std::size_t offset = page > 0 ? (page - 1) * size : 0;
  if (offset >= out.size()) return {};
  std::vector<Task> pageItems(out.begin() + static_cast<long>(offset),
                             out.begin() + static_cast<long>(std::min(out.size(), offset + size)));
  return pageItems;
}

std::vector<Task> InMemoryRepository::doneTasks(const std::string& tenantId, const std::string& assignee,
                                               std::size_t page, std::size_t size) {
  std::vector<Task> out;
  for (const auto& task : tasks_) {
    if (!assignee.empty() && task->assignee != assignee) continue;
    if (!tenantId.empty() && task->tenantId != tenantId) continue;
    if (task->status != TaskStatus::Completed && task->status != TaskStatus::Transferred &&
        task->status != TaskStatus::Delegated) {
      continue;
    }
    out.push_back(*task);
  }
  std::sort(out.begin(), out.end(), [](const Task& a, const Task& b) {
    return a.completeTime > b.completeTime;
  });
  const std::size_t offset = page > 0 ? (page - 1) * size : 0;
  if (offset >= out.size()) return {};
  return std::vector<Task>(out.begin() + static_cast<long>(offset),
                           out.begin() + static_cast<long>(std::min(out.size(), offset + size)));
}

std::vector<Task> InMemoryRepository::candidateTasks(const std::string& tenantId,
                                                     const std::string& user,
                                                     const std::vector<std::string>& roles) {
  std::vector<Task> out;
  for (const auto& task : tasks_) {
    if (task->status != TaskStatus::Pending) continue;
    if (!tenantId.empty() && task->tenantId != tenantId) continue;
    if (task->assignee == user) continue;
    bool match = false;
    for (const auto& candidate : task->candidateUsers) {
      if (candidate == user) match = true;
    }
    for (const auto& role : task->candidateRoles) {
      for (const auto& userRole : roles) {
        if (role == userRole) match = true;
      }
    }
    if (match || (task->assignee.empty() && task->candidateUsers.empty() && task->candidateRoles.empty())) {
      out.push_back(*task);
    }
  }
  return out;
}

HistoryRecord InMemoryRepository::appendHistory(const HistoryRecord& record) {
  HistoryRecord stored = record;
  if (stored.id == 0) stored.id = nextId();
  histories_.push_back(stored);
  return stored;
}

std::vector<HistoryRecord> InMemoryRepository::histories(long long instanceId) {
  std::vector<HistoryRecord> out;
  for (const auto& record : histories_) {
    if (record.processInstanceId == instanceId) out.push_back(record);
  }
  std::sort(out.begin(), out.end(), [](const HistoryRecord& a, const HistoryRecord& b) {
    return a.id < b.id;
  });
  return out;
}

EventRecord InMemoryRepository::createEvent(const EventRecord& event) {
  EventRecord stored = event;
  if (stored.id == 0) stored.id = nextId();
  if (stored.eventId.empty()) stored.eventId = "EVT-" + std::to_string(stored.id);
  events_.push_back(stored);
  return stored;
}

bool InMemoryRepository::updateEvent(const EventRecord& event) {
  for (auto& stored : events_) {
    if (stored.id == event.id) {
      stored = event;
      return true;
    }
  }
  return false;
}

EventRecord* InMemoryRepository::findEvent(long long id) {
  for (auto& event : events_) {
    if (event.id == id) return &event;
  }
  return nullptr;
}

std::vector<EventRecord> InMemoryRepository::pendingEvents(long long now, std::size_t limit) {
  std::vector<EventRecord> out;
  for (const auto& event : events_) {
    if (event.status != "PENDING") continue;
    if (event.nextRetryTime > now) continue;
    out.push_back(event);
    if (out.size() >= limit) break;
  }
  return out;
}

std::vector<EventRecord> InMemoryRepository::events(long long instanceId) {
  std::vector<EventRecord> out;
  for (const auto& event : events_) {
    if (event.processInstanceId == instanceId) out.push_back(event);
  }
  return out;
}

TimerJob InMemoryRepository::createTimer(const TimerJob& job) {
  TimerJob stored = job;
  if (stored.id == 0) stored.id = nextId();
  timers_.push_back(stored);
  return stored;
}

bool InMemoryRepository::updateTimer(const TimerJob& job) {
  for (auto& stored : timers_) {
    if (stored.id == job.id) {
      stored = job;
      return true;
    }
  }
  return false;
}

std::vector<TimerJob> InMemoryRepository::dueTimers(long long now, std::size_t limit) {
  std::vector<TimerJob> out;
  for (const auto& job : timers_) {
    if (job.status != "PENDING") continue;
    if (job.triggerTime > now) continue;
    out.push_back(job);
    if (out.size() >= limit) break;
  }
  return out;
}

std::vector<TimerJob> InMemoryRepository::timersByInstance(long long instanceId) {
  std::vector<TimerJob> out;
  for (const auto& job : timers_) {
    if (job.processInstanceId == instanceId) out.push_back(job);
  }
  return out;
}

ParallelBranch InMemoryRepository::createBranch(const ParallelBranch& branch) {
  ParallelBranch stored = branch;
  if (stored.id == 0) stored.id = nextId();
  branches_.push_back(stored);
  return stored;
}

bool InMemoryRepository::updateBranch(const ParallelBranch& branch) {
  for (auto& stored : branches_) {
    if (stored.id == branch.id) {
      stored = branch;
      return true;
    }
  }
  return false;
}

std::vector<ParallelBranch> InMemoryRepository::branches(long long instanceId,
                                                        const std::string& gatewayNodeId) {
  std::vector<ParallelBranch> out;
  for (const auto& branch : branches_) {
    if (branch.processInstanceId != instanceId) continue;
    if (!gatewayNodeId.empty() && branch.gatewayNodeId != gatewayNodeId) continue;
    out.push_back(branch);
  }
  return out;
}

bool InMemoryRepository::clearBranches(long long instanceId) {
  branches_.erase(std::remove_if(branches_.begin(), branches_.end(),
                                 [instanceId](const ParallelBranch& branch) {
                                   return branch.processInstanceId == instanceId;
                                 }),
                  branches_.end());
  return true;
}

json::Value InMemoryRepository::snapshot() {
  Value out = Value::object();
  out.set("sequence", Value(nextId_));
  out.set("definitions", definitionArrayToJson(definitions_));
  out.set("instances", entityArrayToJson(instances_));
  out.set("nodeInstances", entityArrayToJson(nodeInstances_));
  out.set("tasks", entityArrayToJson(tasks_));
  out.set("histories", valueArrayToJson(histories_));
  out.set("events", valueArrayToJson(events_));
  out.set("timers", valueArrayToJson(timers_));
  out.set("branches", valueArrayToJson(branches_));
  return out;
}

void InMemoryRepository::restore(const json::Value& snapshotValue) {
  clear();
  nextId_ = snapshotValue.intOr("sequence", 1000);
  if (snapshotValue.contains("definitions")) {
    for (const auto& item : snapshotValue.at("definitions").items()) {
      definitions_.push_back(ProcessDefinition::fromJson(item));
    }
  }
  if (snapshotValue.contains("instances")) {
    for (const auto& item : snapshotValue.at("instances").items()) {
      instances_.push_back(std::unique_ptr<ProcessInstance>(new ProcessInstance(ProcessInstance::fromJson(item))));
    }
  }
  if (snapshotValue.contains("nodeInstances")) {
    for (const auto& item : snapshotValue.at("nodeInstances").items()) {
      nodeInstances_.push_back(std::unique_ptr<NodeInstance>(new NodeInstance(NodeInstance::fromJson(item))));
    }
  }
  if (snapshotValue.contains("tasks")) {
    for (const auto& item : snapshotValue.at("tasks").items()) {
      tasks_.push_back(std::unique_ptr<Task>(new Task(Task::fromJson(item))));
    }
  }
  if (snapshotValue.contains("histories")) {
    for (const auto& item : snapshotValue.at("histories").items()) {
      histories_.push_back(HistoryRecord::fromJson(item));
    }
  }
  if (snapshotValue.contains("events")) {
    for (const auto& item : snapshotValue.at("events").items()) {
      events_.push_back(EventRecord::fromJson(item));
    }
  }
  if (snapshotValue.contains("timers")) {
    for (const auto& item : snapshotValue.at("timers").items()) {
      timers_.push_back(TimerJob::fromJson(item));
    }
  }
  if (snapshotValue.contains("branches")) {
    for (const auto& item : snapshotValue.at("branches").items()) {
      branches_.push_back(ParallelBranch::fromJson(item));
    }
  }
}

void InMemoryRepository::clear() {
  definitions_.clear();
  instances_.clear();
  nodeInstances_.clear();
  tasks_.clear();
  histories_.clear();
  events_.clear();
  timers_.clear();
  branches_.clear();
  nextId_ = 1000;
}

std::size_t InMemoryRepository::entityCount() {
  return definitions_.size() + instances_.size() + nodeInstances_.size() + tasks_.size() +
         histories_.size() + events_.size() + timers_.size() + branches_.size();
}

bool saveRepositoryToFile(Repository& repository, const std::string& path, std::string* error) {
  std::ofstream out(path, std::ios::binary | std::ios::trunc);
  if (!out) {
    if (error != nullptr) *error = "无法写入仓储文件：" + path;
    return false;
  }
  out << repository.snapshot().dump(2);
  out.close();
  return true;
}

bool loadRepositoryFromFile(InMemoryRepository& repository, const std::string& path, std::string* error) {
  std::ifstream in(path, std::ios::binary);
  if (!in) {
    if (error != nullptr) *error = "无法读取仓储文件：" + path;
    return false;
  }
  std::ostringstream buffer;
  buffer << in.rdbuf();
  try {
    repository.restore(json::Value::parse(buffer.str()));
  } catch (const std::exception& exception) {
    if (error != nullptr) *error = std::string("仓储文件解析失败：") + exception.what();
    return false;
  }
  return true;
}

}  // namespace wf

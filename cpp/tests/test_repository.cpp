#include "test.hpp"
#include "wf/repository.hpp"

using namespace wf;
using wf::json::Value;

namespace {

ProcessDefinition sampleDefinition(const std::string& code = "expense", int version = 1) {
  Value node = Value::object();
  node.set("id", Value("start"));
  node.set("type", Value("START"));
  ProcessDefinition definition;
  definition.code = code;
  definition.name = code;
  definition.version = version;
  definition.status = DefinitionStatus::Draft;
  definition.nodes.push_back(NodeDefinition::fromJson(node));
  return definition;
}

}  // namespace

WF_TEST(repository_definition_versioning) {
  InMemoryRepository repository;
  const ProcessDefinition v1 = repository.saveDefinition(sampleDefinition("expense", 0));
  CHECK(v1.id != 0);
  CHECK_EQ(v1.version, 1);
  CHECK(repository.findDefinition("default", "expense", 0) == nullptr);  // 未发布

  repository.setDefinitionStatus(v1.id, DefinitionStatus::Published);
  CHECK(repository.findDefinition("default", "expense", 0) != nullptr);
  CHECK_EQ(repository.findDefinition("default", "expense", 0)->version, 1);

  const ProcessDefinition v2 = repository.saveDefinition(sampleDefinition("expense", 0));
  CHECK_EQ(v2.version, 2);
  repository.setDefinitionStatus(v2.id, DefinitionStatus::Published);
  CHECK_EQ(repository.findDefinition("default", "expense", 0)->version, 2);
  // 发布新版本后旧版本自动停用
  CHECK_EQ(repository.findDefinition("default", "expense", 1)->status, DefinitionStatus::Disabled);
  CHECK_EQ(repository.listDefinitions("default", "expense").size(), std::size_t(2));
}

WF_TEST(repository_instance_crud_and_optimistic_lock) {
  InMemoryRepository repository;
  ProcessInstance instance;
  instance.processCode = "expense";
  instance.businessKey = "EXPENSE-1";
  instance.initiator = "U1";
  instance.variables = Value::parse(R"({"amount": 500})");
  const ProcessInstance created = repository.createInstance(instance);
  CHECK(created.id != 0);
  CHECK_EQ(created.version, 1);

  ProcessInstance* found = repository.findInstance(created.id);
  CHECK(found != nullptr);
  CHECK_EQ(found->businessKey, std::string("EXPENSE-1"));

  ProcessInstance updated = *found;
  updated.status = ProcessStatus::Completed;
  CHECK(repository.updateInstance(updated, created.version));
  CHECK(repository.findInstance(created.id)->status == ProcessStatus::Completed);
  CHECK_EQ(repository.findInstance(created.id)->version, 2);
  // 版本不匹配时更新被拒绝
  CHECK(!repository.updateInstance(updated, created.version));
}

WF_TEST(repository_business_key_ignores_finished_instances) {
  InMemoryRepository repository;
  ProcessInstance instance;
  instance.processCode = "expense";
  instance.businessKey = "BK-1";
  const ProcessInstance created = repository.createInstance(instance);
  CHECK(repository.findInstanceByBusinessKey("default", "expense", "BK-1") != nullptr);

  ProcessInstance done = *repository.findInstance(created.id);
  done.status = ProcessStatus::Completed;
  repository.updateInstance(done, -1);
  CHECK(repository.findInstanceByBusinessKey("default", "expense", "BK-1") == nullptr);
}

WF_TEST(repository_task_queries) {
  InMemoryRepository repository;
  Task pending;
  pending.processInstanceId = 1;
  pending.nodeId = "approve";
  pending.assignee = "U1";
  pending.tenantId = "default";
  const Task first = repository.createTask(pending);

  Task claimed = *repository.findTask(first.id);
  claimed.status = TaskStatus::Claimed;
  claimed.claimTime = 1000;
  CHECK(repository.updateTask(claimed, first.version));
  CHECK(repository.findTask(first.id)->status == TaskStatus::Claimed);
  // 用过期的版本号更新会被拒绝（PRD §22.2 幂等）
  CHECK(!repository.updateTask(claimed, first.version));

  Task done = *repository.findTask(first.id);
  done.status = TaskStatus::Completed;
  done.completeTime = 2000;
  CHECK(repository.updateTask(done, -1));
  CHECK_EQ(repository.todoTasks("default", "U1", 1, 20).size(), std::size_t(0));
  CHECK_EQ(repository.doneTasks("default", "U1", 1, 20).size(), std::size_t(1));

  Task candidate;
  candidate.processInstanceId = 1;
  candidate.nodeId = "review";
  candidate.candidateRoles.push_back("FINANCE_MANAGER");
  repository.createTask(candidate);
  const std::vector<Task> candidates =
      repository.candidateTasks("default", "U9", std::vector<std::string>{"FINANCE_MANAGER"});
  CHECK_EQ(candidates.size(), std::size_t(1));
}

WF_TEST(repository_snapshot_round_trip) {
  InMemoryRepository repository;
  const ProcessDefinition definition = repository.saveDefinition(sampleDefinition("expense", 0));
  repository.setDefinitionStatus(definition.id, DefinitionStatus::Published);

  ProcessInstance instance;
  instance.processCode = "expense";
  instance.businessKey = "BK-42";
  instance.variables = Value::parse(R"({"amount": 12000, "form": {"type": "travel"}})");
  const ProcessInstance stored = repository.createInstance(instance);

  NodeInstance node;
  node.processInstanceId = stored.id;
  node.nodeId = "approve";
  node.nodeType = NodeType::UserTask;
  node.status = NodeInstanceStatus::Waiting;
  const NodeInstance storedNode = repository.createNodeInstance(node);

  Task task;
  task.processInstanceId = stored.id;
  task.nodeInstanceId = storedNode.id;
  task.nodeId = "approve";
  task.assignee = "U20001";
  task.strategy = CompleteStrategy::All;
  repository.createTask(task);

  HistoryRecord history;
  history.processInstanceId = stored.id;
  history.action = "START";
  history.operatorUser = "U1";
  repository.appendHistory(history);

  ParallelBranch branch;
  branch.processInstanceId = stored.id;
  branch.gatewayNodeId = "fork";
  branch.branchId = "edge-a";
  repository.createBranch(branch);

  const std::string path = "/tmp/wfengine-repository-test.json";
  std::string error;
  CHECK(saveRepositoryToFile(repository, path, &error));
  CHECK(error.empty());

  InMemoryRepository restored;
  CHECK(loadRepositoryFromFile(restored, path, &error));
  CHECK(restored.findInstanceByBusinessKey("default", "expense", "BK-42") != nullptr);
  CHECK_EQ(restored.findInstance(stored.id)->variables.at("form").at("type").asString(),
           std::string("travel"));
  CHECK(restored.findDefinition("default", "expense", 0) != nullptr);
  // 定义 id 必须随快照往返，否则实例的 processDefinitionId 会丢失
  CHECK_EQ(restored.findDefinition("default", "expense", 0)->id, definition.id);
  CHECK_EQ(restored.tasksByInstance(stored.id).size(), std::size_t(1));
  CHECK_EQ(restored.tasksByInstance(stored.id).front().assignee, std::string("U20001"));
  CHECK(restored.tasksByInstance(stored.id).front().strategy == CompleteStrategy::All);
  CHECK_EQ(restored.histories(stored.id).size(), std::size_t(1));
  CHECK_EQ(restored.branches(stored.id, "fork").size(), std::size_t(1));
  CHECK_EQ(restored.entityCount(), repository.entityCount());
}

WF_TEST(repository_events_and_timers) {
  InMemoryRepository repository;
  EventRecord event;
  event.eventType = EventType::TaskCreated;
  event.processInstanceId = 7;
  event.payload = Value::parse(R"({"taskId": 5})");
  event.nextRetryTime = 100;
  const EventRecord stored = repository.createEvent(event);
  CHECK_EQ(stored.eventId, "EVT-" + std::to_string(stored.id));

  CHECK_EQ(repository.pendingEvents(50, 10).size(), std::size_t(0));   // 未到期
  CHECK_EQ(repository.pendingEvents(100, 10).size(), std::size_t(1));  // 到期

  EventRecord retried = *repository.findEvent(stored.id);
  retried.retryCount = 1;
  retried.nextRetryTime = 500;
  CHECK(repository.updateEvent(retried));
  CHECK_EQ(repository.pendingEvents(100, 10).size(), std::size_t(0));
  CHECK_EQ(repository.pendingEvents(500, 10).size(), std::size_t(1));
  CHECK_EQ(repository.events(7).size(), std::size_t(1));

  TimerJob job;
  job.processInstanceId = 7;
  job.jobType = "TASK_TIMEOUT";
  job.triggerTime = 1000;
  const TimerJob storedJob = repository.createTimer(job);
  CHECK_EQ(repository.dueTimers(999, 10).size(), std::size_t(0));
  CHECK_EQ(repository.dueTimers(1000, 10).size(), std::size_t(1));
  TimerJob fired = repository.dueTimers(1000, 10).front();
  fired.status = "DONE";
  CHECK(repository.updateTimer(fired));
  CHECK_EQ(repository.dueTimers(2000, 10).size(), std::size_t(0));
  CHECK_EQ(repository.timersByInstance(7).size(), std::size_t(1));
  CHECK_EQ(storedJob.id, fired.id);
}

WF_TEST(repository_clear_and_counts) {
  InMemoryRepository repository;
  repository.createInstance(ProcessInstance());
  CHECK(repository.entityCount() > 0);
  repository.clear();
  CHECK_EQ(repository.entityCount(), std::size_t(0));
  CHECK(repository.nextId() != 0);
}

#include <memory>
#include <sstream>

#include "test.hpp"
#include "wf/engine.hpp"

using namespace wf;
using wf::json::Value;

namespace {

const char* kExpenseDefinition = R"({
  "processCode": "expense_approval",
  "processName": "报销审批流程",
  "version": 1,
  "nodes": [
    {"id": "start", "type": "START", "name": "开始"},
    {"id": "submit", "type": "USER_TASK", "name": "提交申请", "assignee": {"type": "INITIATOR"}},
    {"id": "amountGateway", "type": "EXCLUSIVE_GATEWAY", "name": "金额判断"},
    {"id": "managerApproval", "type": "USER_TASK", "name": "主管审批",
     "assignee": {"type": "ROLE", "value": "MANAGER"}},
    {"id": "directorApproval", "type": "USER_TASK", "name": "总监审批",
     "assignee": {"type": "ROLE", "value": "DIRECTOR"}},
    {"id": "notify", "type": "SERVICE_TASK", "name": "发送通知",
     "service": {"type": "HTTP", "method": "POST", "url": "http://message-service/send",
                 "body": {"userId": "${initiator}", "amount": "${amount}"},
                 "retry": {"maxAttempts": 2}}},
    {"id": "end", "type": "END", "name": "结束"}
  ],
  "edges": [
    {"id": "edge-1", "source": "start", "target": "submit"},
    {"id": "edge-2", "source": "submit", "target": "amountGateway"},
    {"id": "edge-3", "source": "amountGateway", "target": "managerApproval",
     "condition": "${amount <= 10000}"},
    {"id": "edge-4", "source": "amountGateway", "target": "directorApproval",
     "condition": "${amount > 10000}"},
    {"id": "edge-5", "source": "managerApproval", "target": "notify"},
    {"id": "edge-6", "source": "directorApproval", "target": "notify"},
    {"id": "edge-7", "source": "notify", "target": "end"}
  ]
})";

struct Fixture {
  InMemoryRepository repository;
  ManualClock clock{1000000};
  InMemoryOrganization org;
  MockServiceInvoker invoker;
  RecordingNotifier notifier;
  RecordingEventPublisher publisher;
  std::unique_ptr<ProcessEngine> engine;

  Fixture() {
    engine.reset(new ProcessEngine(repository, clock));
    engine->setOrganization(&org);
    engine->setServiceInvoker(&invoker);
    engine->setNotifier(&notifier);
    engine->setEventPublisher(&publisher);
    // 组织：U10086(发起人, D1) -> U20001(主管) -> U30001(总监)；U40001 财务经理
    org.addUser("U10086", "张三", "D1", "U20001");
    org.addUser("U20001", "李四", "D1", "U30001");
    org.addUser("U30001", "王五", "D0", "");
    org.addUser("U40001", "赵六", "D2", "U30001");
    org.grantRole("U20001", "MANAGER");
    org.grantRole("U30001", "DIRECTOR");
    org.grantRole("U40001", "FINANCE_MANAGER");
    org.grantRole("U10086", "STAFF");
  }

  ProcessDefinition deploy(const std::string& json, bool publish = true) {
    return engine->deployDefinition(ProcessDefinition::fromJson(Value::parse(json)), publish);
  }

  StartRequest request(const std::string& code, const std::string& businessKey,
                       const std::string& initiator, const Value& variables) {
    StartRequest start;
    start.processCode = code;
    start.businessKey = businessKey;
    start.initiator = initiator;
    start.variables = variables;
    return start;
  }

  OperationResult approve(long long taskId, const std::string& user,
                          const std::string& comment = "同意", const Value& variables = Value()) {
    TaskOperation operation;
    operation.action = TaskAction::Approve;
    operation.operatorUser = user;
    operation.comment = comment;
    operation.variables = variables;
    return engine->completeTask(taskId, operation);
  }

  OperationResult reject(long long taskId, const std::string& user,
                         const std::string& comment = "不同意") {
    TaskOperation operation;
    operation.action = TaskAction::Reject;
    operation.operatorUser = user;
    operation.comment = comment;
    return engine->completeTask(taskId, operation);
  }

  Task taskFor(const std::string& user) {
    TodoQuery query;
    query.assignee = user;
    const std::vector<Task> tasks = engine->todoTasks(query);
    return tasks.empty() ? Task() : tasks.front();
  }

  std::vector<std::string> historyActions(long long instanceId) {
    std::vector<std::string> actions;
    for (const auto& record : engine->history(instanceId)) actions.push_back(record.action);
    return actions;
  }

  bool hasAction(long long instanceId, const std::string& action) {
    for (const auto& record : engine->history(instanceId)) {
      if (record.action == action) return true;
    }
    return false;
  }

  std::string approvalDefinition(const std::string& strategy, const std::string& users,
                                 const std::string& extraNode = "") {
    std::ostringstream out;
    out << "{\"processCode\": \"multi_approval\", \"version\": 1, \"nodes\": ["
        << "{\"id\": \"start\", \"type\": \"START\"},"
        << "{\"id\": \"approve\", \"type\": \"USER_TASK\", \"name\": \"会签\","
        << "\"assignee\": {\"type\": \"USERS\", \"users\": " << users << "},"
        << "\"completeStrategy\": \"" << strategy << "\"" << extraNode << "},"
        << "{\"id\": \"end\", \"type\": \"END\"}],"
        << "\"edges\": [{\"id\": \"e1\", \"source\": \"start\", \"target\": \"approve\"},"
        << "{\"id\": \"e2\", \"source\": \"approve\", \"target\": \"end\"}]}";
    return out.str();
  }
};

}  // namespace

WF_TEST(engine_runs_prd_expense_flow_end_to_end) {
  Fixture fixture;
  fixture.deploy(kExpenseDefinition);

  const OperationResult started = fixture.engine->startProcess(
      fixture.request("expense_approval", "EXPENSE-20260917-0001", "U10086",
                      Value::parse(R"({"amount": 12000, "type": "travel"})")));
  CHECK(started.ok);
  CHECK(started.instance.status == ProcessStatus::Running);
  CHECK_EQ(started.instance.currentNodeId, std::string("submit"));
  CHECK_EQ(started.createdTasks.size(), std::size_t(1));
  CHECK_EQ(started.createdTasks.front().assignee, std::string("U10086"));

  // 提交申请 -> 金额 12000 -> 走总监审批分支
  const OperationResult submitted = fixture.approve(started.createdTasks.front().id, "U10086", "提交");
  CHECK(submitted.ok);
  CHECK_EQ(submitted.instance.currentNodeId, std::string("directorApproval"));
  CHECK_EQ(submitted.createdTasks.size(), std::size_t(1));
  CHECK_EQ(submitted.createdTasks.front().assignee, std::string("U30001"));

  const OperationResult approved = fixture.approve(submitted.createdTasks.front().id, "U30001");
  CHECK(approved.ok);
  CHECK(approved.instance.status == ProcessStatus::Completed);
  CHECK_EQ(approved.instance.result, std::string("COMPLETED"));
  // 自动节点调用了外部服务，并把模板变量渲染进请求体
  CHECK_EQ(fixture.invoker.callCount("http://message-service/send"), 1);
  CHECK_EQ(fixture.invoker.calls().front().body.at("userId").asString(), std::string("U10086"));
  CHECK_EQ(fixture.invoker.calls().front().body.at("amount").asInt(), 12000);
  // 轨迹完整：START / TASK_CREATE / APPROVE / NODE_COMPLETE / COMPLETED
  CHECK(fixture.hasAction(approved.instance.id, "START"));
  CHECK(fixture.hasAction(approved.instance.id, "NODE_APPROVED"));
  CHECK(fixture.hasAction(approved.instance.id, "COMPLETED"));
}

WF_TEST(engine_routes_low_amount_to_manager) {
  Fixture fixture;
  fixture.deploy(kExpenseDefinition);
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("expense_approval", "EXP-2", "U10086", Value::parse(R"({"amount": 5000})")));
  const OperationResult submitted = fixture.approve(started.createdTasks.front().id, "U10086");
  CHECK_EQ(submitted.instance.currentNodeId, std::string("managerApproval"));
  CHECK_EQ(submitted.createdTasks.front().assignee, std::string("U20001"));
  const OperationResult finished = fixture.approve(submitted.createdTasks.front().id, "U20001");
  CHECK(finished.instance.status == ProcessStatus::Completed);
}

WF_TEST(engine_start_is_idempotent_by_business_key) {
  Fixture fixture;
  fixture.deploy(kExpenseDefinition);
  const StartRequest request =
      fixture.request("expense_approval", "BK-1", "U10086", Value::parse(R"({"amount": 100})"));
  const OperationResult first = fixture.engine->startProcess(request);
  const OperationResult second = fixture.engine->startProcess(request);
  CHECK(first.ok);
  CHECK(second.ok);
  CHECK(second.idempotentReplay);
  CHECK_EQ(second.instance.id, first.instance.id);
  CHECK_EQ(fixture.engine->instances("default", "", 100).size(), std::size_t(1));
}

WF_TEST(engine_countersign_all_requires_every_approval) {
  Fixture fixture;
  fixture.deploy(fixture.approvalDefinition("ALL", R"(["U20001", "U30001", "U40001"])"));
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("multi_approval", "BK-ALL", "U10086", Value::object()));
  CHECK_EQ(started.createdTasks.size(), std::size_t(3));

  const OperationResult first = fixture.approve(started.createdTasks[0].id, "U20001");
  CHECK(first.instance.status == ProcessStatus::Running);
  CHECK_EQ(first.instance.currentNodeId, std::string("approve"));
  const OperationResult second = fixture.approve(started.createdTasks[1].id, "U30001");
  CHECK(second.instance.status == ProcessStatus::Running);

  const OperationResult third = fixture.approve(started.createdTasks[2].id, "U40001");
  CHECK(third.instance.status == ProcessStatus::Completed);
  for (const auto& task : fixture.engine->tasksByInstance(started.instance.id)) {
    CHECK(task.status == TaskStatus::Completed);
  }
}

WF_TEST(engine_or_sign_completes_on_first_approval) {
  Fixture fixture;
  fixture.deploy(fixture.approvalDefinition("ANY", R"(["U20001", "U30001", "U40001"])"));
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("multi_approval", "BK-ANY", "U10086", Value::object()));
  const OperationResult approved = fixture.approve(started.createdTasks.front().id, "U20001");
  CHECK(approved.instance.status == ProcessStatus::Completed);
  // 其余或签人任务被取消
  int cancelled = 0;
  for (const auto& task : fixture.engine->tasksByInstance(started.instance.id)) {
    if (task.status == TaskStatus::Cancelled) ++cancelled;
  }
  CHECK_EQ(cancelled, 2);
}

WF_TEST(engine_ratio_strategy_and_impossible_branch) {
  Fixture fixture;
  fixture.deploy(fixture.approvalDefinition("RATIO", R"(["U20001", "U30001", "U40001"])",
                                            R"(, "ratio": 0.6)"));
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("multi_approval", "BK-RATIO", "U10086", Value::object()));
  const OperationResult first = fixture.approve(started.createdTasks[0].id, "U20001");
  CHECK(first.instance.status == ProcessStatus::Running);
  const OperationResult second = fixture.approve(started.createdTasks[1].id, "U30001");
  CHECK(second.instance.status == ProcessStatus::Completed);  // 2/3 >= 0.6

  Fixture other;
  other.deploy(other.approvalDefinition("RATIO", R"(["U20001", "U30001", "U40001"])",
                                        R"(, "ratio": 0.5)"));
  const OperationResult rejectedStart = other.engine->startProcess(
      other.request("multi_approval", "BK-RATIO2", "U10086", Value::object()));
  other.approve(rejectedStart.createdTasks[0].id, "U20001");
  const OperationResult rejected = other.reject(rejectedStart.createdTasks[1].id, "U30001");
  CHECK(rejected.instance.status == ProcessStatus::Completed);
  CHECK_EQ(rejected.instance.result, std::string("REJECTED"));
}

WF_TEST(engine_sequence_strategy_creates_tasks_one_by_one) {
  Fixture fixture;
  fixture.deploy(fixture.approvalDefinition("SEQUENCE", R"(["U20001", "U30001"])"));
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("multi_approval", "BK-SEQ", "U10086", Value::object()));
  CHECK_EQ(started.createdTasks.size(), std::size_t(1));
  CHECK_EQ(started.createdTasks.front().assignee, std::string("U20001"));

  const OperationResult first = fixture.approve(started.createdTasks.front().id, "U20001");
  CHECK(first.instance.status == ProcessStatus::Running);
  CHECK_EQ(first.createdTasks.size(), std::size_t(1));
  CHECK_EQ(first.createdTasks.front().assignee, std::string("U30001"));

  const OperationResult second = fixture.approve(first.createdTasks.front().id, "U30001");
  CHECK(second.instance.status == ProcessStatus::Completed);
}

WF_TEST(engine_transfer_moves_task_to_new_handler) {
  Fixture fixture;
  fixture.deploy(fixture.approvalDefinition("ANY", R"(["U20001"])"));
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("multi_approval", "BK-TR", "U10086", Value::object()));
  const long long taskId = started.createdTasks.front().id;
  const OperationResult transferred = fixture.engine->transferTask(taskId, "U20001", "U40001", "转交财务");
  CHECK(transferred.ok);
  CHECK_EQ(transferred.task.assignee, std::string("U40001"));
  CHECK(fixture.engine->findInstance(started.instance.id) != nullptr);
  CHECK(fixture.engine->findInstance(started.instance.id)->status == ProcessStatus::Running);

  const OperationResult done = fixture.approve(transferred.task.id, "U40001");
  CHECK(done.instance.status == ProcessStatus::Completed);
  CHECK(fixture.hasAction(started.instance.id, "TRANSFER"));
}

WF_TEST(engine_delegate_returns_task_to_owner) {
  Fixture fixture;
  fixture.deploy(fixture.approvalDefinition("ANY", R"(["U20001"])"));
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("multi_approval", "BK-DG", "U10086", Value::object()));
  const long long taskId = started.createdTasks.front().id;
  const OperationResult delegated = fixture.engine->delegateTask(taskId, "U20001", "U40001", "请协助");
  CHECK(delegated.ok);
  CHECK_EQ(delegated.task.assignee, std::string("U40001"));
  CHECK_EQ(delegated.task.signType, std::string("DELEGATE"));

  const OperationResult handled = fixture.approve(delegated.task.id, "U40001");
  CHECK(handled.ok);
  // 委派不产生审批意见，任务回到原处理人
  CHECK(handled.instance.status == ProcessStatus::Running);
  CHECK_EQ(handled.createdTasks.size(), std::size_t(1));
  CHECK_EQ(handled.createdTasks.front().assignee, std::string("U20001"));
  CHECK(fixture.hasAction(started.instance.id, "TASK_RETURN"));

  const OperationResult owner = fixture.approve(handled.createdTasks.front().id, "U20001");
  CHECK(owner.instance.status == ProcessStatus::Completed);
}

WF_TEST(engine_add_sign_before_and_after) {
  Fixture fixture;
  fixture.deploy(fixture.approvalDefinition("ALL", R"(["U20001"])"));
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("multi_approval", "BK-SIGN", "U10086", Value::object()));
  const long long taskId = started.createdTasks.front().id;

  const OperationResult before =
      fixture.engine->addSignTask(taskId, "U20001", {"U30001"}, "BEFORE", "ALL", "先请总监看");
  CHECK(before.ok);
  const OperationResult blocked = fixture.approve(taskId, "U20001");
  CHECK(!blocked.ok);  // 前加签未完成时原处理人不能办理

  const OperationResult signedResult = fixture.approve(before.createdTasks.front().id, "U30001");
  CHECK(signedResult.ok);
  CHECK(signedResult.instance.status == ProcessStatus::Running);
  const OperationResult after = fixture.approve(taskId, "U20001");
  CHECK(after.instance.status == ProcessStatus::Completed);
}

WF_TEST(engine_add_sign_after_blocks_node_completion) {
  Fixture fixture;
  fixture.deploy(fixture.approvalDefinition("ALL", R"(["U20001"])"));
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("multi_approval", "BK-SIGN2", "U10086", Value::object()));
  const long long taskId = started.createdTasks.front().id;
  const OperationResult added =
      fixture.engine->addSignTask(taskId, "U20001", {"U30001"}, "AFTER", "ALL", "请总监复核");
  CHECK(added.ok);

  const OperationResult first = fixture.approve(taskId, "U20001");
  CHECK(first.ok);
  CHECK(first.instance.status == ProcessStatus::Running);  // 后加签未完成，节点不结束
  CHECK(fixture.hasAction(started.instance.id, "WAIT_AFTER_SIGN"));

  const OperationResult second = fixture.approve(added.createdTasks.front().id, "U30001");
  CHECK(second.instance.status == ProcessStatus::Completed);
}

WF_TEST(engine_reject_policies) {
  // 直接结束
  Fixture endFixture;
  endFixture.deploy(endFixture.approvalDefinition("ANY", R"(["U20001"])"));
  const OperationResult started = endFixture.engine->startProcess(
      endFixture.request("multi_approval", "BK-R1", "U10086", Value::object()));
  const OperationResult rejected = endFixture.reject(started.createdTasks.front().id, "U20001");
  CHECK(rejected.ok);
  CHECK(rejected.instance.status == ProcessStatus::Completed);
  CHECK_EQ(rejected.instance.result, std::string("REJECTED"));

  // 退回发起人
  Fixture initiatorFixture;
  initiatorFixture.deploy(R"({
    "processCode": "reject_to_initiator", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "submit", "type": "USER_TASK", "name": "提交", "assignee": {"type": "INITIATOR"}},
      {"id": "approve", "type": "USER_TASK", "name": "审批", "assignee": {"type": "ROLE", "value": "MANAGER"},
       "rejectPolicy": "INITIATOR"},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "submit"},
      {"id": "e2", "source": "submit", "target": "approve"},
      {"id": "e3", "source": "approve", "target": "end"}
    ]
  })");
  const OperationResult flow = initiatorFixture.engine->startProcess(
      initiatorFixture.request("reject_to_initiator", "BK-R2", "U10086", Value::object()));
  const OperationResult submission = initiatorFixture.approve(flow.createdTasks.front().id, "U10086");
  const OperationResult bounced = initiatorFixture.reject(submission.createdTasks.front().id, "U20001");
  CHECK(bounced.ok);
  CHECK(bounced.instance.status == ProcessStatus::Running);
  CHECK_EQ(bounced.instance.currentNodeId, std::string("submit"));
  CHECK_EQ(bounced.createdTasks.size(), std::size_t(1));
  CHECK_EQ(bounced.createdTasks.front().assignee, std::string("U10086"));
}

WF_TEST(engine_return_task_to_named_node) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "return_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "first", "type": "USER_TASK", "name": "一审", "assignee": {"type": "ROLE", "value": "MANAGER"}},
      {"id": "second", "type": "USER_TASK", "name": "二审", "assignee": {"type": "ROLE", "value": "DIRECTOR"}},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "first"},
      {"id": "e2", "source": "first", "target": "second"},
      {"id": "e3", "source": "second", "target": "end"}
    ]
  })");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("return_flow", "BK-RT", "U10086", Value::object()));
  const OperationResult first = fixture.approve(started.createdTasks.front().id, "U20001");
  const OperationResult returned = fixture.engine->returnTask(first.createdTasks.front().id, "U30001",
                                                             "first", "材料不全，退回一审");
  CHECK(returned.ok);
  CHECK_EQ(returned.createdTasks.size(), std::size_t(1));
  CHECK_EQ(returned.createdTasks.front().nodeId, std::string("first"));
  CHECK_EQ(returned.createdTasks.front().assignee, std::string("U20001"));
  CHECK(fixture.hasAction(started.instance.id, "RETURN"));
}

WF_TEST(engine_withdraw_by_initiator) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "withdraw_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "submit", "type": "USER_TASK", "name": "提交", "assignee": {"type": "INITIATOR"}},
      {"id": "approve", "type": "USER_TASK", "name": "审批", "assignee": {"type": "ROLE", "value": "MANAGER"}},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "submit"},
      {"id": "e2", "source": "submit", "target": "approve"},
      {"id": "e3", "source": "approve", "target": "end"}
    ]
  })");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("withdraw_flow", "BK-WD", "U10086", Value::object()));
  fixture.approve(started.createdTasks.front().id, "U10086");  // 提交后进入主管审批
  const OperationResult withdrawn = fixture.engine->withdrawInstance(started.instance.id, "U10086", "撤回");
  CHECK(withdrawn.ok);
  CHECK_EQ(withdrawn.instance.currentNodeId, std::string("submit"));
  CHECK_EQ(withdrawn.createdTasks.size(), std::size_t(1));
  CHECK_EQ(withdrawn.createdTasks.front().assignee, std::string("U10086"));
}

WF_TEST(engine_parallel_fork_and_join) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "parallel_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "fork", "type": "PARALLEL_GATEWAY", "name": "并行分支"},
      {"id": "finance", "type": "USER_TASK", "name": "财务审批", "assignee": {"type": "ROLE", "value": "FINANCE_MANAGER"}},
      {"id": "legal", "type": "USER_TASK", "name": "法务审批", "assignee": {"type": "ROLE", "value": "DIRECTOR"}},
      {"id": "join", "type": "PARALLEL_GATEWAY", "name": "汇聚"},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "fork"},
      {"id": "e2", "source": "fork", "target": "finance"},
      {"id": "e3", "source": "fork", "target": "legal"},
      {"id": "e4", "source": "finance", "target": "join"},
      {"id": "e5", "source": "legal", "target": "join"},
      {"id": "e6", "source": "join", "target": "end"}
    ]
  })");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("parallel_flow", "BK-PAR", "U10086", Value::object()));
  CHECK(started.ok);
  CHECK_EQ(started.createdTasks.size(), std::size_t(2));
  CHECK_EQ(fixture.engine->branches(started.instance.id).size(), std::size_t(2));

  const Task financeTask = fixture.taskFor("U40001");
  const Task legalTask = fixture.taskFor("U30001");
  CHECK(financeTask.id != 0);
  CHECK(legalTask.id != 0);

  const OperationResult finance = fixture.approve(financeTask.id, "U40001");
  CHECK(finance.instance.status == ProcessStatus::Running);  // 等待法务分支
  CHECK_EQ(finance.instance.currentNodeId, std::string("join"));
  CHECK_EQ(finance.instance.activeTokens, 1);  // 一个令牌挂在汇聚点

  const OperationResult legal = fixture.approve(legalTask.id, "U30001");
  CHECK(legal.instance.status == ProcessStatus::Completed);
  CHECK_EQ(legal.instance.activeTokens, 0);
}

WF_TEST(engine_subprocess_maps_variables_back) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "child_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "childApprove", "type": "USER_TASK", "name": "子流程审批",
       "assignee": {"type": "ROLE", "value": "MANAGER"}},
      {"id": "compute", "type": "SCRIPT_TASK", "name": "计算",
       "script": {"language": "wfscript", "content": "result = 'OK'; level = amount > 10000 ? 'HIGH' : 'LOW';"}},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "c1", "source": "start", "target": "childApprove"},
      {"id": "c2", "source": "childApprove", "target": "compute"},
      {"id": "c3", "source": "compute", "target": "end"}
    ]
  })");
  fixture.deploy(R"({
    "processCode": "parent_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "child", "type": "SUB_PROCESS", "name": "调用子流程", "processCode": "child_flow",
       "subProcessInput": {"amount": "${amount}"},
       "subProcessOutputs": ["result"]},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "p1", "source": "start", "target": "child"},
      {"id": "p2", "source": "child", "target": "end"}
    ]
  })");

  const OperationResult started = fixture.engine->startProcess(
      fixture.request("parent_flow", "BK-SUB", "U10086", Value::parse(R"({"amount": 20000})")));
  CHECK(started.ok);
  CHECK(started.instance.status == ProcessStatus::Running);
  CHECK_EQ(started.createdTasks.size(), std::size_t(1));
  CHECK_EQ(started.createdTasks.front().assignee, std::string("U20001"));

  const OperationResult childDone = fixture.approve(started.createdTasks.front().id, "U20001");
  CHECK(childDone.ok);
  // 子流程完成后父流程继续并回写输出变量
  CHECK(childDone.instance.status == ProcessStatus::Completed);
  CHECK_EQ(childDone.instance.variables.at("result").asString(), std::string("OK"));
  CHECK_EQ(fixture.engine->instances("default", "", 100).size(), std::size_t(2));
}

WF_TEST(engine_wait_node_resumes_on_event) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "wait_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "waitPay", "type": "WAIT_TASK", "name": "等待支付回调", "eventKey": "payment.paid"},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "waitPay"},
      {"id": "e2", "source": "waitPay", "target": "end"}
    ]
  })");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("wait_flow", "BK-WAIT", "U10086", Value::object()));
  CHECK(started.instance.status == ProcessStatus::Running);
  CHECK_EQ(started.instance.currentNodeId, std::string("waitPay"));

  const OperationResult signalled =
      fixture.engine->signalEvent("payment.paid", Value::parse(R"({"paid": true, "amount": 12000})"));
  CHECK(signalled.ok);
  CHECK(signalled.instance.status == ProcessStatus::Completed);
  CHECK(signalled.instance.variables.at("paid").asBool());
  CHECK_EQ(signalled.instance.variables.at("event").at("amount").asInt(), 12000);
}

WF_TEST(engine_timer_node_fires_on_tick) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "timer_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "delay", "type": "TIMER_TASK", "name": "等待1小时", "duration": "PT1H"},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "delay"},
      {"id": "e2", "source": "delay", "target": "end"}
    ]
  })");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("timer_flow", "BK-TIMER", "U10086", Value::object()));
  CHECK(started.instance.status == ProcessStatus::Running);
  CHECK_EQ(fixture.engine->tick(), 0);  // 还没到点

  fixture.clock.advance(3600 * 1000);
  CHECK_EQ(fixture.engine->tick(), 1);
  CHECK(fixture.engine->findInstance(started.instance.id)->status == ProcessStatus::Completed);
}

WF_TEST(engine_task_timeout_reminds_and_escalates) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "timeout_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "approve", "type": "USER_TASK", "name": "审批", "assignee": {"type": "INITIATOR"},
       "taskTimeout": {"duration": "PT24H",
                       "actions": [{"type": "REMIND", "channel": "EMAIL"},
                                   {"type": "ESCALATE", "target": "2"}]}},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "approve"},
      {"id": "e2", "source": "approve", "target": "end"}
    ]
  })");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("timeout_flow", "BK-TO", "U10086", Value::object()));
  CHECK_EQ(started.createdTasks.front().dueTime, fixture.clock.now() + 24LL * 3600 * 1000);

  fixture.clock.advance(24LL * 3600 * 1000);
  CHECK_EQ(fixture.engine->tick(), 1);
  CHECK_EQ(fixture.notifier.countOf("EMAIL"), 1);
  // 升级到二级主管 U30001
  const Task escalated = fixture.taskFor("U30001");
  CHECK(escalated.id != 0);
  CHECK_EQ(escalated.nodeId, std::string("approve"));
  CHECK(fixture.hasAction(started.instance.id, "TIMEOUT_ESCALATE"));

  const OperationResult done = fixture.approve(escalated.id, "U30001");
  CHECK(done.instance.status == ProcessStatus::Completed);
}

WF_TEST(engine_service_retry_and_failure_recovery) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "retry_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "call", "type": "SERVICE_TASK", "name": "调用风控",
       "service": {"type": "HTTP", "url": "http://risk-service/check", "method": "POST",
                   "body": {"userId": "${initiator}"},
                   "retry": {"maxAttempts": 3, "backoff": 100}}},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "call"},
      {"id": "e2", "source": "call", "target": "end"}
    ]
  })");
  fixture.invoker.failTimes("http://risk-service/check", 2, "connection refused");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("retry_flow", "BK-RETRY", "U10086", Value::object()));
  CHECK(started.ok);
  CHECK(started.instance.status == ProcessStatus::Completed);
  CHECK_EQ(fixture.invoker.callCount("http://risk-service/check"), 3);
  CHECK(fixture.hasAction(started.instance.id, "SERVICE_RETRIED"));

  // 服务一直失败 -> 实例进入 ERROR，管理员重试后恢复
  Fixture failing;
  failing.deploy(R"({
    "processCode": "fail_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "call", "type": "SERVICE_TASK", "name": "调用外部服务",
       "service": {"type": "HTTP", "url": "http://down-service/x", "retry": {"maxAttempts": 2}}},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "call"},
      {"id": "e2", "source": "call", "target": "end"}
    ]
  })");
  failing.invoker.setOffline(true);
  const OperationResult broken = failing.engine->startProcess(
      failing.request("fail_flow", "BK-FAIL", "U10086", Value::object()));
  CHECK(broken.ok);  // 启动调用本身成功，但流程进入异常
  CHECK(broken.instance.status == ProcessStatus::Error);
  CHECK(failing.hasAction(broken.instance.id, "NODE_FAILED"));

  failing.invoker.setOffline(false);
  const OperationResult retried = failing.engine->retryNode(broken.instance.id, "call", "U99999");
  CHECK(retried.ok);
  CHECK(retried.instance.status == ProcessStatus::Completed);
}

WF_TEST(engine_script_node_drives_gateway) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "script_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "risk", "type": "SCRIPT_TASK", "name": "风控打分",
       "script": {"language": "wfscript",
                  "content": "riskLevel = amount > 10000 ? 'HIGH' : 'LOW';\nscore = round(amount / 1000, 1);"}},
      {"id": "gate", "type": "EXCLUSIVE_GATEWAY"},
      {"id": "highRisk", "type": "USER_TASK", "name": "高风险复核", "assignee": {"type": "ROLE", "value": "DIRECTOR"}},
      {"id": "lowRisk", "type": "USER_TASK", "name": "普通审批", "assignee": {"type": "ROLE", "value": "MANAGER"}},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "risk"},
      {"id": "e2", "source": "risk", "target": "gate"},
      {"id": "e3", "source": "gate", "target": "highRisk", "condition": "${riskLevel == 'HIGH'}"},
      {"id": "e4", "source": "gate", "target": "lowRisk", "default": true},
      {"id": "e5", "source": "highRisk", "target": "end"},
      {"id": "e6", "source": "lowRisk", "target": "end"}
    ]
  })");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("script_flow", "BK-SCRIPT", "U10086", Value::parse(R"({"amount": 20000})")));
  CHECK(started.ok);
  CHECK_EQ(started.instance.variables.at("riskLevel").asString(), std::string("HIGH"));
  CHECK_EQ(started.instance.variables.at("score").asInt(), 20);
  CHECK_EQ(started.createdTasks.size(), std::size_t(1));
  CHECK_EQ(started.createdTasks.front().assignee, std::string("U30001"));
}

WF_TEST(engine_field_permissions_control_writes) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "form_flow", "version": 1,
    "variables": {"amount": 12000},
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "approve", "type": "USER_TASK", "name": "财务审批", "assignee": {"type": "INITIATOR"},
       "formPermissions": [
         {"field": "amount", "visible": true, "editable": false},
         {"field": "reason", "visible": true, "editable": true, "required": true}
       ]},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "approve"},
      {"id": "e2", "source": "approve", "target": "end"}
    ]
  })");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("form_flow", "BK-FORM", "U10086", Value::object()));
  CHECK_EQ(started.instance.variables.at("amount").asInt(), 12000);

  // 缺少必填字段 -> 拒绝
  const OperationResult missing = fixture.approve(started.createdTasks.front().id, "U10086", "",
                                                  Value::parse(R"({"amount": 1})"));
  CHECK(!missing.ok);
  CHECK(missing.error.find("必填") != std::string::npos);

  // 不可编辑字段被忽略，可编辑字段写入成功
  const OperationResult written = fixture.approve(started.createdTasks.front().id, "U10086", "",
                                                  Value::parse(R"({"amount": 1, "reason": "补充说明"})"));
  CHECK(written.ok);
  CHECK_EQ(written.instance.variables.at("amount").asInt(), 12000);
  CHECK_EQ(written.instance.variables.at("reason").asString(), std::string("补充说明"));
}

WF_TEST(engine_event_outbox_retries_with_backoff) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "event_flow", "version": 1,
    "config": {"events": [
      {"type": "PROCESS_COMPLETED", "listener": {"type": "HTTP", "url": "http://business/callback"}}
    ]},
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "approve", "type": "USER_TASK", "name": "审批", "assignee": {"type": "INITIATOR"}},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "approve"},
      {"id": "e2", "source": "approve", "target": "end"}
    ]
  })");
  fixture.publisher.failTimes(1, "endpoint unavailable");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("event_flow", "BK-EVT", "U10086", Value::object()));
  const OperationResult done = fixture.approve(started.createdTasks.front().id, "U10086");
  CHECK(done.instance.status == ProcessStatus::Completed);

  // 第一次投递失败：事件保留在 outbox 并安排重试（PRD §29.3）
  bool foundPending = false;
  for (const auto& event : fixture.engine->events(started.instance.id)) {
    if (event.eventType == EventType::ProcessCompleted && event.status == "PENDING") {
      foundPending = true;
      CHECK_EQ(event.retryCount, 1);
      CHECK_EQ(event.error, std::string("endpoint unavailable"));
      CHECK(event.nextRetryTime > fixture.clock.now());
    }
  }
  CHECK(foundPending);
  CHECK_EQ(fixture.publisher.deliveries().size(), std::size_t(0));

  fixture.clock.advance(10000);
  const int delivered = fixture.engine->dispatchEvents();
  CHECK(delivered >= 1);
  CHECK_EQ(fixture.publisher.deliveries().size(), std::size_t(1));
  CHECK_EQ(fixture.publisher.deliveries().front().destination, std::string("http://business/callback"));
  CHECK_EQ(fixture.publisher.deliveries().front().payload.at("businessKey").asString(),
           std::string("BK-EVT"));
}

WF_TEST(engine_suspend_blocks_progress_and_timers) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "suspend_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "approve", "type": "USER_TASK", "name": "审批", "assignee": {"type": "INITIATOR"}},
      {"id": "delay", "type": "TIMER_TASK", "duration": "PT1H"},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "approve"},
      {"id": "e2", "source": "approve", "target": "delay"},
      {"id": "e3", "source": "delay", "target": "end"}
    ]
  })");
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("suspend_flow", "BK-SUSP", "U10086", Value::object()));
  CHECK(fixture.engine->suspendInstance(started.instance.id, "U10086").ok);
  const OperationResult blocked = fixture.approve(started.createdTasks.front().id, "U10086");
  CHECK(!blocked.ok);
  CHECK(blocked.error.find("挂起") != std::string::npos);

  CHECK(fixture.engine->resumeInstance(started.instance.id, "U10086").ok);
  const OperationResult approved = fixture.approve(started.createdTasks.front().id, "U10086");
  CHECK(approved.ok);
  CHECK(approved.instance.status == ProcessStatus::Running);

  fixture.clock.advance(3600 * 1000);
  CHECK_EQ(fixture.engine->tick(), 1);
  CHECK(fixture.engine->findInstance(started.instance.id)->status == ProcessStatus::Completed);
}

WF_TEST(engine_task_optimistic_lock_and_idempotent_replay) {
  Fixture fixture;
  fixture.deploy(fixture.approvalDefinition("ANY", R"(["U20001"])"));
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("multi_approval", "BK-LOCK", "U10086", Value::object()));
  const Task task = started.createdTasks.front();

  TaskOperation stale;
  stale.action = TaskAction::Approve;
  stale.operatorUser = "U20001";
  stale.expectedVersion = task.version + 7;
  const OperationResult conflict = fixture.engine->completeTask(task.id, stale);
  CHECK(!conflict.ok);
  CHECK(conflict.error.find("版本") != std::string::npos);

  const OperationResult first = fixture.approve(task.id, "U20001");
  CHECK(first.ok);
  const OperationResult replay = fixture.approve(task.id, "U20001");
  CHECK(!replay.ok);
  CHECK(replay.error.find("已处理") != std::string::npos);
}

WF_TEST(engine_cancel_terminate_and_jump) {
  Fixture fixture;
  fixture.deploy(kExpenseDefinition);
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("expense_approval", "BK-CTL", "U10086", Value::parse(R"({"amount": 100})")));
  const OperationResult cancelled = fixture.engine->cancelInstance(started.instance.id, "U99999", "业务撤销");
  CHECK(cancelled.ok);
  CHECK(cancelled.instance.status == ProcessStatus::Cancelled);
  for (const auto& task : fixture.engine->tasksByInstance(started.instance.id)) {
    CHECK(task.status == TaskStatus::Cancelled);
  }

  const OperationResult started2 = fixture.engine->startProcess(
      fixture.request("expense_approval", "BK-CTL2", "U10086", Value::parse(R"({"amount": 100})")));
  const OperationResult jumped = fixture.engine->jumpToNode(started2.instance.id, "managerApproval", "U99999", "管理员干预");
  CHECK(jumped.ok);
  CHECK_EQ(jumped.createdTasks.size(), std::size_t(1));
  CHECK_EQ(jumped.createdTasks.front().assignee, std::string("U20001"));

  const OperationResult terminated = fixture.engine->terminateInstance(started2.instance.id, "U99999", "终止");
  CHECK(terminated.ok);
  CHECK(terminated.instance.status == ProcessStatus::Terminated);
}

WF_TEST(engine_assignee_rules_and_expression_context) {
  Fixture fixture;
  fixture.deploy(R"({
    "processCode": "assignee_flow", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "leader", "type": "USER_TASK", "name": "二级主管审批", "assignee": {"type": "LEADER", "level": 2}},
      {"id": "dynamic", "type": "USER_TASK", "name": "动态审批人",
       "assignee": {"type": "EXPRESSION", "value": "${deptManager}"}},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "leader"},
      {"id": "e2", "source": "leader", "target": "dynamic"},
      {"id": "e3", "source": "dynamic", "target": "end"}
    ]
  })");
  const OperationResult started = fixture.engine->startProcess(fixture.request(
      "assignee_flow", "BK-ASG", "U10086", Value::parse(R"({"deptManager": "U40001"})")));
  CHECK_EQ(started.createdTasks.front().assignee, std::string("U30001"));  // U10086 的二级主管

  const OperationResult next = fixture.approve(started.createdTasks.front().id, "U30001");
  CHECK_EQ(next.createdTasks.front().assignee, std::string("U40001"));  // 表达式从变量取

  // 组织函数可在表达式中使用
  const Value context = fixture.engine->expressionContext(*fixture.engine->findInstance(started.instance.id));
  CHECK(fixture.engine->functionsFor(*fixture.engine->findInstance(started.instance.id))
            .at("hasRole")({Value("U40001"), Value("FINANCE_MANAGER")})
            .asBool());
  CHECK_EQ(context.at("processInstance").at("processCode").asString(), std::string("assignee_flow"));
}

WF_TEST(engine_deploy_rejects_invalid_definition) {
  Fixture fixture;
  const ProcessDefinition broken = ProcessDefinition::fromJson(Value::parse(R"({
    "processCode": "broken", "version": 1,
    "nodes": [{"id": "a", "type": "USER_TASK"}],
    "edges": []
  })"));
  const ValidationResult validation = fixture.engine->validateDefinition(broken);
  CHECK(!validation.ok());
  CHECK_THROWS(fixture.deploy(broken.toJson().dump(), true));
}

WF_TEST(engine_queries_and_statistics) {
  Fixture fixture;
  fixture.deploy(kExpenseDefinition);
  const OperationResult started = fixture.engine->startProcess(
      fixture.request("expense_approval", "BK-Q", "U10086", Value::parse(R"({"amount": 500})")));
  const OperationResult submitted = fixture.approve(started.createdTasks.front().id, "U10086");
  CHECK_EQ(submitted.createdTasks.front().assignee, std::string("U20001"));

  TodoQuery todo;
  todo.assignee = "U20001";
  CHECK_EQ(fixture.engine->todoTasks(todo).size(), std::size_t(1));
  TodoQuery done;
  done.assignee = "U10086";
  CHECK_EQ(fixture.engine->doneTasks(done).size(), std::size_t(1));
  CHECK_EQ(fixture.engine->nodeInstances(started.instance.id).size(), std::size_t(4));
  CHECK_EQ(fixture.engine->fieldPermissions(started.instance.id, "submit").size(), std::size_t(0));

  const Value stats = fixture.engine->statistics("default");
  CHECK_EQ(stats.at("运行中").asInt(), 1);
  CHECK_EQ(stats.at("待办任务").asInt(), 1);

  const std::vector<HistoryRecord> history = fixture.engine->history(started.instance.id);
  CHECK(history.size() >= 5);
  CHECK(!history.front().traceId.empty());
  CHECK_EQ(history.front().operatorUser, std::string("U10086"));
}

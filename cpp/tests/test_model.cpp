#include "test.hpp"
#include "wf/model.hpp"

using namespace wf;
using wf::json::Value;

namespace {

const char* kPrdExpenseDefinition = R"({
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
     "service": {"type": "HTTP", "url": "http://message-service/send"}},
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

ProcessDefinition minimalDefinition(const std::string& extraNodes = "",
                                    const std::string& extraEdges = "") {
  (void)extraNodes;
  (void)extraEdges;
  const std::string text = std::string(R"({
    "processCode": "p1", "processName": "p1", "version": 1,
    "nodes": [
      {"id": "start", "type": "START", "name": "开始"},
      {"id": "approve", "type": "USER_TASK", "name": "审批",
       "assignee": {"type": "ROLE", "value": "M"}})") +
                           extraNodes + R"(,
      {"id": "end", "type": "END", "name": "结束"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "approve"},
      {"id": "e2", "source": "approve", "target": "end"})" +
                           extraEdges + R"(]
  })";
  return ProcessDefinition::fromJson(Value::parse(text));
}

}  // namespace

WF_TEST(definition_parses_prd_dsl) {
  const ProcessDefinition definition = ProcessDefinition::fromJson(Value::parse(kPrdExpenseDefinition));
  CHECK_EQ(definition.code, std::string("expense_approval"));
  CHECK_EQ(definition.name, std::string("报销审批流程"));
  CHECK_EQ(definition.version, 1);
  CHECK_EQ(definition.nodes.size(), std::size_t(7));
  CHECK_EQ(definition.edges.size(), std::size_t(7));
  CHECK(definition.startNode() != nullptr);
  CHECK_EQ(definition.startNode()->id, std::string("start"));
  CHECK(definition.findNode("notify")->type == NodeType::ServiceTask);
  CHECK_EQ(definition.findNode("notify")->service.url, std::string("http://message-service/send"));
  CHECK_EQ(definition.findNode("managerApproval")->assignee.type, std::string("role"));
  CHECK_EQ(definition.findNode("managerApproval")->assignee.value, std::string("MANAGER"));
  CHECK_EQ(definition.findNode("submit")->assignee.type, std::string("initiator"));
  CHECK_EQ(definition.fanOut("amountGateway"), 2);
  CHECK_EQ(definition.fanIn("notify"), 2);
  CHECK_EQ(definition.outEdges("amountGateway").front()->condition, std::string("${amount <= 10000}"));
  CHECK_EQ(definition.inEdges("notify").size(), std::size_t(2));

  const ValidationResult result = validateDefinition(definition);
  CHECK(result.ok());
  CHECK(result.hasCode("WF_CONDITION_INCOMPLETE"));  // 没有默认分支，仅告警
}

WF_TEST(definition_parses_nested_payload_shape) {
  // PRD §18.1: POST /api/process-definitions {processCode, processName, definition:{}}
  const Value payload = Value::parse(R"({
    "processCode": "leave_approval",
    "processName": "请假审批",
    "definition": {
      "nodes": [
        {"id": "start", "type": "START"},
        {"id": "approve", "type": "USER_TASK", "assignee": "ROLE:MANAGER"},
        {"id": "end", "type": "END"}
      ],
      "edges": [
        {"source": "start", "target": "approve"},
        {"source": "approve", "target": "end"}
      ]
    }
  })");
  const ProcessDefinition definition = ProcessDefinition::fromJson(payload);
  CHECK_EQ(definition.code, std::string("leave_approval"));
  CHECK_EQ(definition.nodes.size(), std::size_t(3));
  CHECK_EQ(definition.findNode("approve")->assignee.type, std::string("role"));
  CHECK_EQ(definition.findNode("approve")->assignee.value, std::string("MANAGER"));
  CHECK_EQ(definition.edges.front().id, std::string("start->approve"));
  CHECK(validateDefinition(definition).ok());
}

WF_TEST(definition_parses_assignee_rules) {
  const Value leader = Value::parse(R"({"type": "LEADER", "level": 2})");
  CHECK_EQ(ApprovalRule::fromJson(leader).level, 2);
  CHECK_EQ(ApprovalRule::fromJson(leader).type, std::string("leader"));

  const Value users = Value::parse(R"({"type": "USERS", "users": ["U1", "U2"]})");
  CHECK_EQ(ApprovalRule::fromJson(users).users.size(), std::size_t(2));

  const Value expression = Value::parse(R"({"type": "EXPRESSION", "value": "${deptManager}"})");
  CHECK_EQ(ApprovalRule::fromJson(expression).value, std::string("${deptManager}"));

  const Value shorthand = Value::parse(R"("ROLE:FINANCE_MANAGER")");
  CHECK_EQ(ApprovalRule::fromJson(shorthand).type, std::string("role"));
  CHECK_EQ(ApprovalRule::fromJson(shorthand).value, std::string("FINANCE_MANAGER"));

  CHECK_EQ(parseCompleteStrategy("or-sign"), CompleteStrategy::Any);
}

WF_TEST(definition_parses_timeout_and_listeners) {
  const Value node = Value::parse(R"({
    "id": "approve", "type": "USER_TASK", "name": "审批",
    "assignee": {"type": "INITIATOR"},
    "completeStrategy": "ALL",
    "timeout": {"duration": "PT24H", "actions": [{"type": "REMIND", "channel": "EMAIL"},
                                                 {"type": "ESCALATE", "target": "LEADER"}]},
    "formPermissions": [{"field": "amount", "visible": true, "editable": false, "required": false}],
    "listeners": [{"type": "TASK_CREATED", "listener": {"type": "HTTP", "url": "http://todo/hook"}}]
  })");
  const NodeDefinition parsed = NodeDefinition::fromJson(node);
  CHECK(parsed.strategy == CompleteStrategy::All);
  CHECK_EQ(parsed.nodeTimeout.duration, std::string("PT24H"));
  CHECK_EQ(parsed.nodeTimeout.steps.size(), std::size_t(2));
  CHECK(parsed.nodeTimeout.steps[1].action == TimeoutAction::Escalate);
  CHECK_EQ(parsed.nodeTimeout.steps[1].target, std::string("LEADER"));
  long long millis = 0;
  CHECK(parsed.nodeTimeout.durationMillis(&millis));
  CHECK_EQ(millis, 24LL * 3600 * 1000);
  CHECK_EQ(parsed.fieldPermissions.size(), std::size_t(1));
  CHECK_EQ(parsed.fieldPermissions.front().field, std::string("amount"));
  CHECK(!parsed.fieldPermissions.front().editable);
  CHECK_EQ(parsed.listeners.size(), std::size_t(1));
  CHECK(parsed.listeners.front().event == EventType::TaskCreated);
  CHECK_EQ(parsed.listeners.front().url, std::string("http://todo/hook"));
}

WF_TEST(definition_round_trips_through_json) {
  const ProcessDefinition definition = ProcessDefinition::fromJson(Value::parse(kPrdExpenseDefinition));
  const ProcessDefinition reparsed = ProcessDefinition::fromJson(definition.toJson());
  CHECK_EQ(reparsed.code, definition.code);
  CHECK_EQ(reparsed.nodes.size(), definition.nodes.size());
  CHECK_EQ(reparsed.edges.size(), definition.edges.size());
  CHECK_EQ(reparsed.findNode("managerApproval")->assignee.value, std::string("MANAGER"));
  CHECK_EQ(reparsed.findNode("amountGateway")->type, NodeType::ExclusiveGateway);
  CHECK_EQ(reparsed.toJson().dump(), definition.toJson().dump());
}

WF_TEST(validation_reports_structural_errors) {
  // 缺少开始节点 + 孤立节点 + 死端
  const Value broken = Value::parse(R"({
    "processCode": "bad", "version": 1,
    "nodes": [
      {"id": "a", "type": "USER_TASK", "name": "A", "assignee": {"type": "INITIATOR"}},
      {"id": "lone", "type": "SERVICE_TASK", "name": "孤立", "service": {"url": "http://x"}},
      {"id": "end", "type": "END"}
    ],
    "edges": [{"id": "e1", "source": "a", "target": "end"}]
  })");
  const ValidationResult result = validateDefinition(ProcessDefinition::fromJson(broken));
  CHECK(!result.ok());
  CHECK(result.hasCode("WF_NO_START"));
  CHECK(result.hasCode("WF_ORPHAN_NODE"));
}

WF_TEST(validation_reports_missing_assignee_and_service) {
  const Value broken = Value::parse(R"({
    "processCode": "bad2", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "a", "type": "USER_TASK", "name": "A"},
      {"id": "s", "type": "SERVICE_TASK", "name": "S"},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "a"},
      {"id": "e2", "source": "a", "target": "s"},
      {"id": "e3", "source": "s", "target": "end"}
    ]
  })");
  const ValidationResult result = validateDefinition(ProcessDefinition::fromJson(broken));
  CHECK(result.hasCode("WF_ASSIGNEE_MISSING"));
  CHECK(result.hasCode("WF_SERVICE_MISSING"));
}

WF_TEST(validation_detects_cycle_and_gateway_gaps) {
  const Value cyclic = Value::parse(R"({
    "processCode": "cycle", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "g", "type": "EXCLUSIVE_GATEWAY"},
      {"id": "a", "type": "USER_TASK", "assignee": {"type": "INITIATOR"}},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "g"},
      {"id": "e2", "source": "g", "target": "a"},
      {"id": "e3", "source": "a", "target": "g"},
      {"id": "e4", "source": "g", "target": "end", "condition": "${done}"}
    ]
  })");
  const ValidationResult result = validateDefinition(ProcessDefinition::fromJson(cyclic));
  CHECK(result.hasCode("WF_CYCLE"));
  CHECK(result.hasCode("WF_CONDITION_MISSING") == false);  // 有 e4 条件
}

WF_TEST(validation_checks_parallel_join_and_durations) {
  const Value parallelBroken = Value::parse(R"({
    "processCode": "p", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "fork", "type": "PARALLEL_GATEWAY"},
      {"id": "a", "type": "USER_TASK", "assignee": {"type": "INITIATOR"}},
      {"id": "b", "type": "USER_TASK", "assignee": {"type": "ROLE", "value": "M"}},
      {"id": "endA", "type": "END"},
      {"id": "endB", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "fork"},
      {"id": "e2", "source": "fork", "target": "a"},
      {"id": "e3", "source": "fork", "target": "b"},
      {"id": "e4", "source": "a", "target": "endA"},
      {"id": "e5", "source": "b", "target": "endB"}
    ]
  })");
  const ValidationResult result = validateDefinition(ProcessDefinition::fromJson(parallelBroken));
  CHECK(result.hasCode("WF_PARALLEL_NO_JOIN"));

  const Value timerBroken = Value::parse(R"({
    "processCode": "t", "version": 1,
    "nodes": [
      {"id": "start", "type": "START"},
      {"id": "t1", "type": "TIMER_TASK", "duration": "24x"},
      {"id": "end", "type": "END"}
    ],
    "edges": [
      {"id": "e1", "source": "start", "target": "t1"},
      {"id": "e2", "source": "t1", "target": "end"}
    ]
  })");
  const ValidationResult timerResult = validateDefinition(ProcessDefinition::fromJson(timerBroken));
  CHECK(timerResult.hasCode("WF_DURATION_INVALID"));
  CHECK(timerResult.hasCode("WF_NO_END") == false);
}

WF_TEST(definition_reachability_and_cycle_helpers) {
  const ProcessDefinition definition = ProcessDefinition::fromJson(Value::parse(kPrdExpenseDefinition));
  const std::vector<std::string> reachable = definition.reachableFrom("start");
  CHECK_EQ(reachable.size(), std::size_t(7));
  std::string cycle;
  CHECK(!definition.hasCycle(&cycle));

  const ProcessDefinition loop = ProcessDefinition::fromJson(Value::parse(R"({
    "processCode": "loop", "version": 1,
    "nodes": [
      {"id": "s", "type": "START"},
      {"id": "a", "type": "USER_TASK", "assignee": {"type": "INITIATOR"}},
      {"id": "b", "type": "USER_TASK", "assignee": {"type": "INITIATOR"}},
      {"id": "e", "type": "END"}
    ],
    "edges": [
      {"id": "1", "source": "s", "target": "a"},
      {"id": "2", "source": "a", "target": "b"},
      {"id": "3", "source": "b", "target": "a"},
      {"id": "4", "source": "b", "target": "e"}
    ]
  })"));
  CHECK(loop.hasCycle(&cycle));
  CHECK(cycle.find("a") != std::string::npos);
}

WF_TEST(validation_summary_is_human_readable) {
  const ValidationResult result = validateDefinition(ProcessDefinition::fromJson(Value::parse(R"({
    "processCode": "x", "version": 1,
    "nodes": [{"id": "a", "type": "USER_TASK"}],
    "edges": []
  })")));
  const std::string summary = result.summary();
  CHECK(summary.find("校验失败") != std::string::npos);
  CHECK(summary.find("WF_NO_START") != std::string::npos);
  CHECK(summary.find("WF_NO_END") != std::string::npos);
}

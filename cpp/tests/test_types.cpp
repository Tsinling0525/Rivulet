#include "test.hpp"
#include "wf/types.hpp"

using namespace wf;

WF_TEST(types_string_round_trip) {
  CHECK_EQ(std::string(toString(ProcessStatus::Running)), std::string("RUNNING"));
  CHECK(parseProcessStatus("completed") == ProcessStatus::Completed);
  CHECK(parseNodeInstanceStatus("WAITING") == NodeInstanceStatus::Waiting);
  CHECK(parseTaskStatus("DelegateD") == TaskStatus::Delegated);
  CHECK(parseNodeType("USER_TASK") == NodeType::UserTask);
  CHECK(parseNodeType("approval") == NodeType::UserTask);
  CHECK(parseNodeType("fork") == NodeType::ParallelGateway);
  CHECK(parseNodeType("timer") == NodeType::TimerTask);
  CHECK(parseNodeType("nope") == NodeType::Unknown);
  CHECK(parseCompleteStrategy("or-sign") == CompleteStrategy::Any);
  CHECK(parseCompleteStrategy("countersign") == CompleteStrategy::All);
  CHECK(parseCompleteStrategy("ratio") == CompleteStrategy::Ratio);
  CHECK(parseTaskAction("agree") == TaskAction::Approve);
  CHECK(parseTaskAction("退回") == TaskAction::Return);
  CHECK(parseTimeoutAction("escalate") == TimeoutAction::Escalate);
  CHECK_EQ(std::string(toString(EventType::TaskCreated)), std::string("TASK_CREATED"));
  CHECK(parseEventType("process_completed") == EventType::ProcessCompleted);
}

WF_TEST(types_process_state_machine) {
  CHECK(canTransit(ProcessStatus::Running, ProcessStatus::Suspended));
  CHECK(canTransit(ProcessStatus::Suspended, ProcessStatus::Running));
  CHECK(canTransit(ProcessStatus::Error, ProcessStatus::Running));
  CHECK(canTransit(ProcessStatus::Running, ProcessStatus::Completed));
  CHECK(!canTransit(ProcessStatus::Completed, ProcessStatus::Running));
  CHECK(!canTransit(ProcessStatus::Cancelled, ProcessStatus::Completed));
  CHECK(isTerminal(ProcessStatus::Terminated));
  CHECK(!isTerminal(ProcessStatus::Error));
}

WF_TEST(types_node_state_machine) {
  CHECK(canTransit(NodeInstanceStatus::Pending, NodeInstanceStatus::Running));
  CHECK(canTransit(NodeInstanceStatus::Running, NodeInstanceStatus::Waiting));
  CHECK(canTransit(NodeInstanceStatus::Waiting, NodeInstanceStatus::Completed));
  CHECK(!canTransit(NodeInstanceStatus::Completed, NodeInstanceStatus::Running));
  CHECK(!canTransit(NodeInstanceStatus::Pending, NodeInstanceStatus::Completed));
  CHECK(isOpen(NodeInstanceStatus::Waiting));
  CHECK(!isOpen(NodeInstanceStatus::Failed));
}

WF_TEST(types_task_state_machine) {
  CHECK(canTransit(TaskStatus::Pending, TaskStatus::Claimed));
  CHECK(canTransit(TaskStatus::Claimed, TaskStatus::Completed));
  CHECK(canTransit(TaskStatus::Pending, TaskStatus::Completed));
  CHECK(canTransit(TaskStatus::Pending, TaskStatus::Transferred));
  CHECK(canTransit(TaskStatus::Transferred, TaskStatus::Pending));
  CHECK(canTransit(TaskStatus::Delegated, TaskStatus::Pending));
  CHECK(!canTransit(TaskStatus::Completed, TaskStatus::Pending));
  CHECK(!canTransit(TaskStatus::Cancelled, TaskStatus::Claimed));
  CHECK(isOpen(TaskStatus::Delegated));
  CHECK(!isOpen(TaskStatus::Completed));
}

WF_TEST(types_duration_parsing) {
  long long millis = 0;
  CHECK(parseDuration("PT24H", &millis));
  CHECK_EQ(millis, 24LL * 3600 * 1000);
  CHECK(parseDuration("P1DT12H", &millis));
  CHECK_EQ(millis, 36LL * 3600 * 1000);
  CHECK(parseDuration("30m", &millis));
  CHECK_EQ(millis, 30LL * 60 * 1000);
  CHECK(parseDuration("45s", &millis));
  CHECK_EQ(millis, 45000);
  CHECK(parseDuration("2h", &millis));
  CHECK_EQ(millis, 2LL * 3600 * 1000);
  CHECK(parseDuration("500ms", &millis));
  CHECK_EQ(millis, 500);
  CHECK(parseDuration("1d", &millis));
  CHECK_EQ(millis, 86400000);
  CHECK(parseDuration("", &millis) == false);
  CHECK(parseDuration("PT", &millis) == false);
  CHECK(parseDuration("24x", &millis) == false);
  CHECK_EQ(formatDuration(90000), std::string("1m30s"));
  CHECK_EQ(formatDuration(3600000), std::string("1h"));
}

WF_TEST(types_time_helpers) {
  const long long stamp = parseTime("2026-09-17 10:00:00");
  CHECK(stamp > 0);
  CHECK_EQ(formatTime(stamp), std::string("2026-09-17 10:00:00"));
  CHECK_EQ(trimCopy("  a b  "), std::string("a b"));
  CHECK_EQ(toLower("AbC"), std::string("abc"));
  const std::vector<std::string> users = splitList("U1, U2 ,U3");
  CHECK_EQ(users.size(), std::size_t(3));
  CHECK_EQ(users[1], std::string("U2"));
}

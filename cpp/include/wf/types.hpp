// Shared enumerations, their string forms, legal state transitions (PRD §6,
// §20) and small time helpers used across the engine.
#pragma once

#include <cstdint>
#include <ostream>
#include <string>
#include <type_traits>
#include <vector>

namespace wf {

// Printable enums: diagnostics only (test framework messages, logs).
template <typename Enum>
typename std::enable_if<std::is_enum<Enum>::value, std::ostream&>::type operator<<(std::ostream& os,
                                                                                   Enum value) {
  return os << static_cast<long long>(value);
}

// PRD §6.1
enum class ProcessStatus { Running, Suspended, Completed, Terminated, Cancelled, Error };
// PRD §6.2
enum class NodeInstanceStatus { Pending, Running, Completed, Skipped, Failed, Cancelled, Waiting };
// PRD §6.3
enum class TaskStatus { Pending, Claimed, Completed, Transferred, Delegated, Cancelled };
// Definition lifecycle (PRD §2.2: 草稿 / 发布 / 停用)
enum class DefinitionStatus { Draft, Published, Disabled };
// PRD §4.3 node types
enum class NodeType {
  Start,
  End,
  UserTask,
  ServiceTask,
  ScriptTask,
  ExclusiveGateway,
  ParallelGateway,
  SubProcess,
  WaitTask,
  TimerTask,
  MessageTask,
  Unknown
};
// PRD §11.4 countersign strategies
enum class CompleteStrategy { All, Any, Ratio, Sequence };
// PRD §10 approval / task operations
enum class TaskAction {
  Claim,
  Approve,
  Reject,
  Return,
  Withdraw,
  Transfer,
  Delegate,
  AddSign,
  Terminate
};
// PRD §12.1 timeout handling
enum class TimeoutAction { Remind, Escalate, AutoApprove, AutoReject, Transfer, Terminate };
// PRD §15.1 event types
enum class EventType {
  ProcessStarted,
  ProcessCompleted,
  ProcessCancelled,
  ProcessSuspended,
  ProcessResumed,
  ProcessTerminated,
  NodeStarted,
  NodeCompleted,
  NodeFailed,
  NodeWaiting,
  TaskCreated,
  TaskAssigned,
  TaskCompleted,
  TaskTransferred,
  TaskDelegated,
  TaskTimeout,
  ServiceRetried,
  ListenerFailed
};

const char* toString(ProcessStatus status);
const char* toString(NodeInstanceStatus status);
const char* toString(TaskStatus status);
const char* toString(DefinitionStatus status);
const char* toString(NodeType type);
const char* toString(CompleteStrategy strategy);
const char* toString(TaskAction action);
const char* toString(TimeoutAction action);
const char* toString(EventType type);

ProcessStatus parseProcessStatus(const std::string& text);
NodeInstanceStatus parseNodeInstanceStatus(const std::string& text);
TaskStatus parseTaskStatus(const std::string& text);
DefinitionStatus parseDefinitionStatus(const std::string& text);
NodeType parseNodeType(const std::string& text);
CompleteStrategy parseCompleteStrategy(const std::string& text);
TaskAction parseTaskAction(const std::string& text);
TimeoutAction parseTimeoutAction(const std::string& text);
EventType parseEventType(const std::string& text);

// State machine guards (PRD §20). Illegal transitions are rejected instead of
// silently applied, which keeps "节点已完成但任务未关闭" style drift out.
bool canTransit(ProcessStatus from, ProcessStatus to);
bool canTransit(NodeInstanceStatus from, NodeInstanceStatus to);
bool canTransit(TaskStatus from, TaskStatus to);

bool isTerminal(ProcessStatus status);
bool isOpen(TaskStatus status);
bool isOpen(NodeInstanceStatus status);

// Duration helpers: ISO-8601 ("PT24H", "P1DT12H") and short forms ("30m",
// "2h", "45s", "1d", "500ms") as used by PRD §12.3 timeout config.
bool parseDuration(const std::string& text, long long* outMillis);
std::string formatDuration(long long millis);

// Epoch milliseconds <-> "2026-09-17 10:00:00"
std::string formatTime(long long millis);
long long parseTime(const std::string& text);

std::string toLower(const std::string& text);
std::string trimCopy(const std::string& text);
// Splits "U1,U2" or "U1, U2" into users.
std::vector<std::string> splitList(const std::string& text);

}  // namespace wf

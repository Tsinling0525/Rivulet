#include "wf/types.hpp"

#include <algorithm>
#include <cctype>
#include <cstdio>
#include <cstdlib>
#include <ctime>
#include <sstream>

namespace wf {
namespace {

template <typename Enum>
struct EnumName {
  Enum value;
  const char* name;
};

const EnumName<ProcessStatus> kProcessStatuses[] = {
    {ProcessStatus::Running, "RUNNING"},   {ProcessStatus::Suspended, "SUSPENDED"},
    {ProcessStatus::Completed, "COMPLETED"}, {ProcessStatus::Terminated, "TERMINATED"},
    {ProcessStatus::Cancelled, "CANCELLED"}, {ProcessStatus::Error, "ERROR"},
};

const EnumName<NodeInstanceStatus> kNodeStatuses[] = {
    {NodeInstanceStatus::Pending, "PENDING"},     {NodeInstanceStatus::Running, "RUNNING"},
    {NodeInstanceStatus::Completed, "COMPLETED"}, {NodeInstanceStatus::Skipped, "SKIPPED"},
    {NodeInstanceStatus::Failed, "FAILED"},       {NodeInstanceStatus::Cancelled, "CANCELLED"},
    {NodeInstanceStatus::Waiting, "WAITING"},
};

const EnumName<TaskStatus> kTaskStatuses[] = {
    {TaskStatus::Pending, "PENDING"},         {TaskStatus::Claimed, "CLAIMED"},
    {TaskStatus::Completed, "COMPLETED"},     {TaskStatus::Transferred, "TRANSFERRED"},
    {TaskStatus::Delegated, "DELEGATED"},     {TaskStatus::Cancelled, "CANCELLED"},
};

const EnumName<DefinitionStatus> kDefinitionStatuses[] = {
    {DefinitionStatus::Draft, "DRAFT"},
    {DefinitionStatus::Published, "PUBLISHED"},
    {DefinitionStatus::Disabled, "DISABLED"},
};

const EnumName<NodeType> kNodeTypes[] = {
    {NodeType::Start, "START"},
    {NodeType::End, "END"},
    {NodeType::UserTask, "USER_TASK"},
    {NodeType::ServiceTask, "SERVICE_TASK"},
    {NodeType::ScriptTask, "SCRIPT_TASK"},
    {NodeType::ExclusiveGateway, "EXCLUSIVE_GATEWAY"},
    {NodeType::ParallelGateway, "PARALLEL_GATEWAY"},
    {NodeType::SubProcess, "SUB_PROCESS"},
    {NodeType::WaitTask, "WAIT_TASK"},
    {NodeType::TimerTask, "TIMER_TASK"},
    {NodeType::MessageTask, "MESSAGE_TASK"},
};

const EnumName<CompleteStrategy> kStrategies[] = {
    {CompleteStrategy::All, "ALL"},
    {CompleteStrategy::Any, "ANY"},
    {CompleteStrategy::Ratio, "RATIO"},
    {CompleteStrategy::Sequence, "SEQUENCE"},
};

const EnumName<TaskAction> kTaskActions[] = {
    {TaskAction::Claim, "CLAIM"},       {TaskAction::Approve, "APPROVE"},
    {TaskAction::Reject, "REJECT"},     {TaskAction::Return, "RETURN"},
    {TaskAction::Withdraw, "WITHDRAW"}, {TaskAction::Transfer, "TRANSFER"},
    {TaskAction::Delegate, "DELEGATE"}, {TaskAction::AddSign, "ADD_SIGN"},
    {TaskAction::Terminate, "TERMINATE"},
};

const EnumName<TimeoutAction> kTimeoutActions[] = {
    {TimeoutAction::Remind, "REMIND"},           {TimeoutAction::Escalate, "ESCALATE"},
    {TimeoutAction::AutoApprove, "AUTO_APPROVE"}, {TimeoutAction::AutoReject, "AUTO_REJECT"},
    {TimeoutAction::Transfer, "TRANSFER"},       {TimeoutAction::Terminate, "TERMINATE"},
};

const EnumName<EventType> kEventTypes[] = {
    {EventType::ProcessStarted, "PROCESS_STARTED"},
    {EventType::ProcessCompleted, "PROCESS_COMPLETED"},
    {EventType::ProcessCancelled, "PROCESS_CANCELLED"},
    {EventType::ProcessSuspended, "PROCESS_SUSPENDED"},
    {EventType::ProcessResumed, "PROCESS_RESUMED"},
    {EventType::ProcessTerminated, "PROCESS_TERMINATED"},
    {EventType::NodeStarted, "NODE_STARTED"},
    {EventType::NodeCompleted, "NODE_COMPLETED"},
    {EventType::NodeFailed, "NODE_FAILED"},
    {EventType::NodeWaiting, "NODE_WAITING"},
    {EventType::TaskCreated, "TASK_CREATED"},
    {EventType::TaskAssigned, "TASK_ASSIGNED"},
    {EventType::TaskCompleted, "TASK_COMPLETED"},
    {EventType::TaskTransferred, "TASK_TRANSFERRED"},
    {EventType::TaskDelegated, "TASK_DELEGATED"},
    {EventType::TaskTimeout, "TASK_TIMEOUT"},
    {EventType::ServiceRetried, "SERVICE_RETRIED"},
    {EventType::ListenerFailed, "LISTENER_FAILED"},
};

template <typename Enum, std::size_t N>
const char* nameOf(const EnumName<Enum> (&table)[N], Enum value) {
  for (const auto& entry : table) {
    if (entry.value == value) return entry.name;
  }
  return "UNKNOWN";
}

template <typename Enum, std::size_t N>
bool parseFrom(const EnumName<Enum> (&table)[N], const std::string& text, Enum* out) {
  const std::string normalized = toLower(trimCopy(text));
  if (normalized.empty()) return false;
  for (const auto& entry : table) {
    if (toLower(entry.name) == normalized) {
      if (out != nullptr) *out = entry.value;
      return true;
    }
  }
  return false;
}

std::string upperCopy(const std::string& text) {
  std::string out = text;
  std::transform(out.begin(), out.end(), out.begin(),
                 [](unsigned char c) { return static_cast<char>(std::toupper(c)); });
  return out;
}

}  // namespace

const char* toString(ProcessStatus status) { return nameOf(kProcessStatuses, status); }
const char* toString(NodeInstanceStatus status) { return nameOf(kNodeStatuses, status); }
const char* toString(TaskStatus status) { return nameOf(kTaskStatuses, status); }
const char* toString(DefinitionStatus status) { return nameOf(kDefinitionStatuses, status); }
const char* toString(NodeType type) { return nameOf(kNodeTypes, type); }
const char* toString(CompleteStrategy strategy) { return nameOf(kStrategies, strategy); }
const char* toString(TaskAction action) { return nameOf(kTaskActions, action); }
const char* toString(TimeoutAction action) { return nameOf(kTimeoutActions, action); }
const char* toString(EventType type) { return nameOf(kEventTypes, type); }

ProcessStatus parseProcessStatus(const std::string& text) {
  ProcessStatus value = ProcessStatus::Running;
  parseFrom(kProcessStatuses, text, &value);
  return value;
}

NodeInstanceStatus parseNodeInstanceStatus(const std::string& text) {
  NodeInstanceStatus value = NodeInstanceStatus::Pending;
  parseFrom(kNodeStatuses, text, &value);
  return value;
}

TaskStatus parseTaskStatus(const std::string& text) {
  TaskStatus value = TaskStatus::Pending;
  parseFrom(kTaskStatuses, text, &value);
  return value;
}

DefinitionStatus parseDefinitionStatus(const std::string& text) {
  DefinitionStatus value = DefinitionStatus::Draft;
  parseFrom(kDefinitionStatuses, text, &value);
  return value;
}

NodeType parseNodeType(const std::string& text) {
  const std::string normalized = upperCopy(trimCopy(text));
  NodeType parsed = NodeType::Unknown;
  if (parseFrom(kNodeTypes, normalized, &parsed)) {
    return parsed;
  }
  // Friendly aliases so hand written DSL stays readable.
  if (normalized == "TASK" || normalized == "APPROVAL" || normalized == "APPROVE" ||
      normalized == "USER") {
    return NodeType::UserTask;
  }
  if (normalized == "HTTP" || normalized == "SERVICE" || normalized == "AUTO" ||
      normalized == "AUTO_TASK") {
    return NodeType::ServiceTask;
  }
  if (normalized == "GATEWAY" || normalized == "CONDITION" || normalized == "XOR") {
    return NodeType::ExclusiveGateway;
  }
  if (normalized == "AND" || normalized == "FORK" || normalized == "JOIN" ||
      normalized == "PARALLEL") {
    return NodeType::ParallelGateway;
  }
  if (normalized == "SUBPROCESS" || normalized == "CALL_ACTIVITY") {
    return NodeType::SubProcess;
  }
  if (normalized == "WAIT" || normalized == "EVENT_WAIT" || normalized == "RECEIVE_TASK") {
    return NodeType::WaitTask;
  }
  if (normalized == "TIMER" || normalized == "DELAY" || normalized == "TIMER_CATCH") {
    return NodeType::TimerTask;
  }
  if (normalized == "MESSAGE" || normalized == "NOTIFY" || normalized == "NOTIFICATION") {
    return NodeType::MessageTask;
  }
  return NodeType::Unknown;
}

CompleteStrategy parseCompleteStrategy(const std::string& text) {
  const std::string normalized = upperCopy(trimCopy(text));
  if (normalized == "OR" || normalized == "OR_SIGN" || normalized == "OR-SIGN" || normalized == "第一人") {
    return CompleteStrategy::Any;
  }
  if (normalized == "COUNTERSIGN" || normalized == "AND" || normalized == "ALL_SIGN" ||
      normalized == "会签") {
    return CompleteStrategy::All;
  }
  if (normalized == "RATIO" || normalized == "PERCENT" || normalized == "比例") {
    return CompleteStrategy::Ratio;
  }
  if (normalized == "SEQUENCE" || normalized == "SEQ" || normalized == "依次") {
    return CompleteStrategy::Sequence;
  }
  CompleteStrategy value = CompleteStrategy::All;
  if (!parseFrom(kStrategies, normalized, &value)) {
    return CompleteStrategy::All;
  }
  return value;
}

TaskAction parseTaskAction(const std::string& text) {
  const std::string normalized = upperCopy(trimCopy(text));
  TaskAction value = TaskAction::Approve;
  if (parseFrom(kTaskActions, normalized, &value)) return value;
  if (normalized == "AGREE" || normalized == "PASS" || normalized == "同意") {
    return TaskAction::Approve;
  }
  if (normalized == "REFUSE" || normalized == "DENY" || normalized == "拒绝") {
    return TaskAction::Reject;
  }
  if (normalized == "BACK" || normalized == "SEND_BACK" || normalized == "退回") {
    return TaskAction::Return;
  }
  if (normalized == "RECALL" || normalized == "撤回") {
    return TaskAction::Withdraw;
  }
  if (normalized == "转办") return TaskAction::Transfer;
  if (normalized == "委派") return TaskAction::Delegate;
  if (normalized == "加签") return TaskAction::AddSign;
  return value;
}

TimeoutAction parseTimeoutAction(const std::string& text) {
  const std::string normalized = upperCopy(trimCopy(text));
  if (normalized == "NOTIFY" || normalized == "URGE" || normalized == "催办") {
    return TimeoutAction::Remind;
  }
  if (normalized == "UPGRADE" || normalized == "升级") return TimeoutAction::Escalate;
  if (normalized == "PASS" || normalized == "AUTO_PASS" || normalized == "自动通过") {
    return TimeoutAction::AutoApprove;
  }
  if (normalized == "REJECT_AUTO" || normalized == "自动拒绝") return TimeoutAction::AutoReject;
  if (normalized == "转交") return TimeoutAction::Transfer;
  if (normalized == "终止") return TimeoutAction::Terminate;
  TimeoutAction value = TimeoutAction::Remind;
  parseFrom(kTimeoutActions, normalized, &value);
  return value;
}

EventType parseEventType(const std::string& text) {
  EventType value = EventType::NodeStarted;
  parseFrom(kEventTypes, text, &value);
  return value;
}

bool canTransit(ProcessStatus from, ProcessStatus to) {
  if (from == to) return true;
  switch (from) {
    case ProcessStatus::Running:
      return to == ProcessStatus::Completed || to == ProcessStatus::Cancelled ||
             to == ProcessStatus::Terminated || to == ProcessStatus::Suspended ||
             to == ProcessStatus::Error;
    case ProcessStatus::Suspended:
      return to == ProcessStatus::Running || to == ProcessStatus::Cancelled ||
             to == ProcessStatus::Terminated;
    case ProcessStatus::Error:
      return to == ProcessStatus::Running || to == ProcessStatus::Terminated ||
             to == ProcessStatus::Cancelled;
    case ProcessStatus::Completed:
    case ProcessStatus::Terminated:
    case ProcessStatus::Cancelled:
      return false;
  }
  return false;
}

bool canTransit(NodeInstanceStatus from, NodeInstanceStatus to) {
  if (from == to) return true;
  switch (from) {
    case NodeInstanceStatus::Pending:
      return to == NodeInstanceStatus::Running || to == NodeInstanceStatus::Skipped ||
             to == NodeInstanceStatus::Cancelled;
    case NodeInstanceStatus::Running:
      return to == NodeInstanceStatus::Completed || to == NodeInstanceStatus::Failed ||
             to == NodeInstanceStatus::Waiting || to == NodeInstanceStatus::Cancelled;
    case NodeInstanceStatus::Waiting:
      return to == NodeInstanceStatus::Running || to == NodeInstanceStatus::Completed ||
             to == NodeInstanceStatus::Cancelled || to == NodeInstanceStatus::Failed;
    case NodeInstanceStatus::Completed:
    case NodeInstanceStatus::Skipped:
    case NodeInstanceStatus::Failed:
    case NodeInstanceStatus::Cancelled:
      return false;
  }
  return false;
}

bool canTransit(TaskStatus from, TaskStatus to) {
  if (from == to) return true;
  switch (from) {
    case TaskStatus::Pending:
      return to == TaskStatus::Claimed || to == TaskStatus::Completed ||
             to == TaskStatus::Transferred || to == TaskStatus::Delegated ||
             to == TaskStatus::Cancelled;
    case TaskStatus::Claimed:
      return to == TaskStatus::Completed || to == TaskStatus::Transferred ||
             to == TaskStatus::Delegated || to == TaskStatus::Cancelled ||
             to == TaskStatus::Pending;
    case TaskStatus::Transferred:
      return to == TaskStatus::Pending || to == TaskStatus::Cancelled;
    case TaskStatus::Delegated:
      return to == TaskStatus::Pending || to == TaskStatus::Completed ||
             to == TaskStatus::Cancelled;
    case TaskStatus::Completed:
    case TaskStatus::Cancelled:
      return false;
  }
  return false;
}

bool isTerminal(ProcessStatus status) {
  return status == ProcessStatus::Completed || status == ProcessStatus::Cancelled ||
         status == ProcessStatus::Terminated;
}

bool isOpen(TaskStatus status) {
  return status == TaskStatus::Pending || status == TaskStatus::Claimed ||
         status == TaskStatus::Transferred || status == TaskStatus::Delegated;
}

bool isOpen(NodeInstanceStatus status) {
  return status == NodeInstanceStatus::Pending || status == NodeInstanceStatus::Running ||
         status == NodeInstanceStatus::Waiting;
}

bool parseDuration(const std::string& text, long long* outMillis) {
  const std::string source = trimCopy(text);
  if (source.empty()) return false;

  auto readNumber = [&](std::size_t& index) -> double {
    const std::size_t start = index;
    while (index < source.size() &&
           (std::isdigit(static_cast<unsigned char>(source[index])) != 0 || source[index] == '.')) {
      ++index;
    }
    if (start == index) return -1;
    try {
      return std::stod(source.substr(start, index - start));
    } catch (...) {
      return -1;
    }
  };

  if (source[0] == 'P' || source[0] == 'p') {
    std::size_t index = 1;
    bool inTime = false;
    bool matched = false;
    double total = 0;
    while (index < source.size()) {
      if (source[index] == 'T' || source[index] == 't') {
        inTime = true;
        ++index;
        continue;
      }
      const double amount = readNumber(index);
      if (amount < 0 || index >= source.size()) return false;
      const char unit = static_cast<char>(std::toupper(static_cast<unsigned char>(source[index++])));
      double millis = 0;
      switch (unit) {
        case 'Y': millis = amount * 365.0 * 86400000.0; break;
        case 'W': millis = amount * 7.0 * 86400000.0; break;
        case 'D': millis = amount * 86400000.0; break;
        case 'H': millis = amount * 3600000.0; break;
        case 'M':
          millis = inTime ? amount * 60000.0 : amount * 30.0 * 86400000.0;
          break;
        case 'S': millis = amount * 1000.0; break;
        default: return false;
      }
      total += millis;
      matched = true;
    }
    if (!matched) return false;
    if (outMillis != nullptr) *outMillis = static_cast<long long>(total);
    return true;
  }

  std::size_t index = 0;
  const double amount = readNumber(index);
  if (amount < 0) return false;
  std::string unit = toLower(trimCopy(source.substr(index)));
  double millis = 0;
  if (unit.empty() || unit == "ms" || unit == "millis") {
    millis = amount;
  } else if (unit == "s" || unit == "sec" || unit == "second" || unit == "seconds") {
    millis = amount * 1000.0;
  } else if (unit == "m" || unit == "min" || unit == "minute" || unit == "minutes") {
    millis = amount * 60000.0;
  } else if (unit == "h" || unit == "hour" || unit == "hours") {
    millis = amount * 3600000.0;
  } else if (unit == "d" || unit == "day" || unit == "days") {
    millis = amount * 86400000.0;
  } else if (unit == "w" || unit == "week" || unit == "weeks") {
    millis = amount * 7.0 * 86400000.0;
  } else {
    return false;
  }
  if (outMillis != nullptr) *outMillis = static_cast<long long>(millis);
  return true;
}

std::string formatDuration(long long millis) {
  if (millis < 0) millis = 0;
  const long long days = millis / 86400000;
  const long long hours = (millis % 86400000) / 3600000;
  const long long minutes = (millis % 3600000) / 60000;
  const long long seconds = (millis % 60000) / 1000;
  std::ostringstream out;
  if (days > 0) out << days << "d";
  if (hours > 0) out << hours << "h";
  if (minutes > 0) out << minutes << "m";
  if (seconds > 0 || (days == 0 && hours == 0 && minutes == 0)) out << seconds << "s";
  return out.str();
}

std::string formatTime(long long millis) {
  const std::time_t seconds = static_cast<std::time_t>(millis / 1000);
  std::tm tm{};
#if defined(_WIN32)
  localtime_s(&tm, &seconds);
#else
  localtime_r(&seconds, &tm);
#endif
  char buffer[32];
  std::strftime(buffer, sizeof(buffer), "%Y-%m-%d %H:%M:%S", &tm);
  return std::string(buffer);
}

long long parseTime(const std::string& text) {
  std::tm tm{};
  if (std::sscanf(text.c_str(), "%d-%d-%d %d:%d:%d", &tm.tm_year, &tm.tm_mon, &tm.tm_mday,
                  &tm.tm_hour, &tm.tm_min, &tm.tm_sec) != 6) {
    return 0;
  }
  tm.tm_year -= 1900;
  tm.tm_mon -= 1;
  tm.tm_isdst = -1;
  return static_cast<long long>(std::mktime(&tm)) * 1000;
}

std::string toLower(const std::string& text) {
  std::string out = text;
  std::transform(out.begin(), out.end(), out.begin(),
                 [](unsigned char c) { return static_cast<char>(std::tolower(c)); });
  return out;
}

std::string trimCopy(const std::string& text) {
  std::size_t begin = 0;
  std::size_t end = text.size();
  while (begin < end && std::isspace(static_cast<unsigned char>(text[begin])) != 0) ++begin;
  while (end > begin && std::isspace(static_cast<unsigned char>(text[end - 1])) != 0) --end;
  return text.substr(begin, end - begin);
}

std::vector<std::string> splitList(const std::string& text) {
  std::vector<std::string> out;
  std::string current;
  for (const char c : text) {
    if (c == ',' || c == ';' || c == '|') {
      const std::string item = trimCopy(current);
      if (!item.empty()) out.push_back(item);
      current.clear();
      continue;
    }
    current.push_back(c);
  }
  const std::string item = trimCopy(current);
  if (!item.empty()) out.push_back(item);
  return out;
}

}  // namespace wf

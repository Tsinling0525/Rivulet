// Pluggable support services: clock, organization directory, service invoker,
// notifier and event publisher (PRD §5.1 支撑服务 / §32 集成). The engine
// depends only on these interfaces, so production deployments can plug in
// HTTP/RPC/MQ/directory clients without touching engine code.
#pragma once

#include <map>
#include <string>
#include <vector>

#include "wf/json.hpp"
#include "wf/model.hpp"

namespace wf {

// ---------------------------------------------------------------------------
// Clock: makes timers/timeouts deterministic in tests.
// ---------------------------------------------------------------------------
class Clock {
 public:
  virtual ~Clock() = default;
  virtual long long now() const = 0;
};

class SystemClock : public Clock {
 public:
  long long now() const override;
};

class ManualClock : public Clock {
 public:
  explicit ManualClock(long long startMillis = 0);
  long long now() const override;
  void advance(long long millis);
  void set(long long millis);

 private:
  long long now_;
};

// ---------------------------------------------------------------------------
// Organization directory (PRD §9 审批人规则 needs roles/leaders/departments).
// ---------------------------------------------------------------------------
class OrganizationService {
 public:
  virtual ~OrganizationService() = default;

  // level 1 = 直接主管, 2 = 二级主管 ...
  virtual std::string leaderOf(const std::string& userId, int level) = 0;
  virtual std::vector<std::string> usersByRole(const std::string& role) = 0;
  virtual std::vector<std::string> usersByPosition(const std::string& position) = 0;
  virtual bool hasRole(const std::string& userId, const std::string& role) = 0;
  virtual std::string departmentOf(const std::string& userId) = 0;
  virtual std::vector<std::string> rolesOf(const std::string& userId) = 0;
  virtual bool exists(const std::string& userId) = 0;
};

class EmptyOrganization : public OrganizationService {
 public:
  std::string leaderOf(const std::string&, int) override { return {}; }
  std::vector<std::string> usersByRole(const std::string&) override { return {}; }
  std::vector<std::string> usersByPosition(const std::string&) override { return {}; }
  bool hasRole(const std::string&, const std::string&) override { return false; }
  std::string departmentOf(const std::string&) override { return {}; }
  std::vector<std::string> rolesOf(const std::string&) override { return {}; }
  bool exists(const std::string&) override { return false; }
};

class InMemoryOrganization : public OrganizationService {
 public:
  struct User {
    std::string id;
    std::string name;
    std::string department;
    std::string leader;  // 直接主管
    std::vector<std::string> roles;
    std::vector<std::string> positions;
  };

  void addUser(const std::string& id, const std::string& name = "", const std::string& department = "",
               const std::string& leader = "");
  void grantRole(const std::string& userId, const std::string& role);
  void assignPosition(const std::string& userId, const std::string& position);
  const User* user(const std::string& id) const;

  std::string leaderOf(const std::string& userId, int level) override;
  std::vector<std::string> usersByRole(const std::string& role) override;
  std::vector<std::string> usersByPosition(const std::string& position) override;
  bool hasRole(const std::string& userId, const std::string& role) override;
  std::string departmentOf(const std::string& userId) override;
  std::vector<std::string> rolesOf(const std::string& userId) override;
  bool exists(const std::string& userId) override;

 private:
  std::map<std::string, User> users_;
};

// ---------------------------------------------------------------------------
// Service invoker: SERVICE_TASK execution (PRD §16.1 HTTP / §16.2 RPC).
// ---------------------------------------------------------------------------
struct ServiceResponse {
  bool ok = true;
  int status = 200;
  json::Value body;
  std::string error;
};

class ServiceInvoker {
 public:
  virtual ~ServiceInvoker() = default;
  virtual ServiceResponse invoke(const ServiceSpec& spec, const json::Value& context) = 0;
};

// Deterministic stand-in for a real HTTP/RPC client: records calls, returns
// canned responses and can simulate failures so retry logic is testable.
class MockServiceInvoker : public ServiceInvoker {
 public:
  struct Call {
    std::string url;
    std::string method;
    json::Value headers;
    json::Value body;
    int attempt = 1;
  };

  void setResponse(const std::string& url, const ServiceResponse& response);
  void setDefaultResponse(const ServiceResponse& response);
  void failTimes(const std::string& url, int times, const std::string& error = "connection refused");
  void setOffline(bool offline) { offline_ = offline; }
  ServiceResponse invoke(const ServiceSpec& spec, const json::Value& context) override;

  const std::vector<Call>& calls() const { return calls_; }
  int callCount(const std::string& url) const;
  void clear();

 private:
  std::map<std::string, ServiceResponse> responses_;
  std::map<std::string, std::pair<int, std::string>> failures_;
  std::vector<Call> calls_;
  ServiceResponse defaultResponse_;
  bool offline_ = false;
};

// ---------------------------------------------------------------------------
// Notifier: 消息节点 / 超时催办 (PRD §4.3.10, §12)
// ---------------------------------------------------------------------------
class Notifier {
 public:
  virtual ~Notifier() = default;
  virtual void send(const std::string& channel, const std::string& target,
                    const std::string& templateName, const json::Value& payload) = 0;
};

class RecordingNotifier : public Notifier {
 public:
  struct Message {
    std::string channel;
    std::string target;
    std::string templateName;
    json::Value payload;
  };

  void send(const std::string& channel, const std::string& target, const std::string& templateName,
            const json::Value& payload) override;
  const std::vector<Message>& messages() const { return messages_; }
  int countOf(const std::string& channel) const;
  void clear() { messages_.clear(); }

 private:
  std::vector<Message> messages_;
};

// ---------------------------------------------------------------------------
// Event publisher: consumes the event outbox (PRD §15 listeners, §21.2 异步化)
// ---------------------------------------------------------------------------
class EventPublisher {
 public:
  virtual ~EventPublisher() = default;
  // destination: HTTP url / MQ topic depending on listenerType.
  virtual bool publish(EventType type, const std::string& listenerType,
                       const std::string& destination, const json::Value& payload,
                       std::string* error) = 0;
};

class RecordingEventPublisher : public EventPublisher {
 public:
  struct Delivery {
    EventType type = EventType::NodeStarted;
    std::string listenerType;
    std::string destination;
    json::Value payload;
  };

  bool publish(EventType type, const std::string& listenerType, const std::string& destination,
               const json::Value& payload, std::string* error) override;
  void failTimes(int times, const std::string& error = "endpoint unavailable");
  const std::vector<Delivery>& deliveries() const { return deliveries_; }
  void clear() {
    deliveries_.clear();
    remainingFailures_ = 0;
  }

 private:
  std::vector<Delivery> deliveries_;
  int remainingFailures_ = 0;
  std::string failureMessage_;
};

// Always-succeeding publisher that prints to stdout (CLI default).
class LogEventPublisher : public EventPublisher {
 public:
  bool publish(EventType type, const std::string& listenerType, const std::string& destination,
               const json::Value& payload, std::string* error) override;
};

// Prints notifications to stdout (CLI default for 消息节点 / 超时催办).
class LogNotifier : public Notifier {
 public:
  void send(const std::string& channel, const std::string& target, const std::string& templateName,
            const json::Value& payload) override;
};

}  // namespace wf

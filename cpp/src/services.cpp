#include "wf/services.hpp"

#include <chrono>
#include <iostream>

#include "wf/expression.hpp"

namespace wf {

long long SystemClock::now() const {
  return std::chrono::duration_cast<std::chrono::milliseconds>(
             std::chrono::system_clock::now().time_since_epoch())
      .count();
}

ManualClock::ManualClock(long long startMillis) : now_(startMillis) {}

long long ManualClock::now() const { return now_; }

void ManualClock::advance(long long millis) { now_ += millis; }

void ManualClock::set(long long millis) { now_ = millis; }

void InMemoryOrganization::addUser(const std::string& id, const std::string& name,
                                   const std::string& department, const std::string& leader) {
  User& user = users_[id];
  user.id = id;
  user.name = name.empty() ? id : name;
  user.department = department;
  user.leader = leader;
}

void InMemoryOrganization::grantRole(const std::string& userId, const std::string& role) {
  User& user = users_[userId];
  user.id = userId;
  for (const auto& existing : user.roles) {
    if (existing == role) return;
  }
  user.roles.push_back(role);
}

void InMemoryOrganization::assignPosition(const std::string& userId, const std::string& position) {
  User& user = users_[userId];
  user.id = userId;
  for (const auto& existing : user.positions) {
    if (existing == position) return;
  }
  user.positions.push_back(position);
}

const InMemoryOrganization::User* InMemoryOrganization::user(const std::string& id) const {
  const auto it = users_.find(id);
  return it == users_.end() ? nullptr : &it->second;
}

std::string InMemoryOrganization::leaderOf(const std::string& userId, int level) {
  std::string current = userId;
  for (int i = 0; i < level; ++i) {
    const auto it = users_.find(current);
    if (it == users_.end() || it->second.leader.empty()) return {};
    current = it->second.leader;
  }
  return current;
}

std::vector<std::string> InMemoryOrganization::usersByRole(const std::string& role) {
  std::vector<std::string> out;
  for (const auto& entry : users_) {
    for (const auto& userRole : entry.second.roles) {
      if (userRole == role) {
        out.push_back(entry.first);
        break;
      }
    }
  }
  return out;
}

std::vector<std::string> InMemoryOrganization::usersByPosition(const std::string& position) {
  std::vector<std::string> out;
  for (const auto& entry : users_) {
    for (const auto& userPosition : entry.second.positions) {
      if (userPosition == position) {
        out.push_back(entry.first);
        break;
      }
    }
  }
  return out;
}

bool InMemoryOrganization::hasRole(const std::string& userId, const std::string& role) {
  const User* found = user(userId);
  if (found == nullptr) return false;
  for (const auto& userRole : found->roles) {
    if (userRole == role) return true;
  }
  return false;
}

std::string InMemoryOrganization::departmentOf(const std::string& userId) {
  const User* found = user(userId);
  return found == nullptr ? std::string() : found->department;
}

std::vector<std::string> InMemoryOrganization::rolesOf(const std::string& userId) {
  const User* found = user(userId);
  return found == nullptr ? std::vector<std::string>() : found->roles;
}

bool InMemoryOrganization::exists(const std::string& userId) { return users_.count(userId) > 0; }

void MockServiceInvoker::setResponse(const std::string& url, const ServiceResponse& response) {
  responses_[url] = response;
}

void MockServiceInvoker::setDefaultResponse(const ServiceResponse& response) {
  defaultResponse_ = response;
}

void MockServiceInvoker::failTimes(const std::string& url, int times, const std::string& error) {
  failures_[url] = std::make_pair(times, error);
}

ServiceResponse MockServiceInvoker::invoke(const ServiceSpec& spec, const json::Value& context) {
  Call call;
  call.url = spec.url.empty() ? spec.serviceName : spec.url;
  call.method = spec.method;
  call.headers = expr::renderValue(spec.headers, context);
  call.body = expr::renderValue(spec.body, context);
  call.attempt = static_cast<int>(calls_.size()) + 1;
  calls_.push_back(call);

  if (offline_) {
    ServiceResponse response;
    response.ok = false;
    response.status = 0;
    response.error = "service offline";
    return response;
  }
  const auto failure = failures_.find(call.url);
  if (failure != failures_.end() && failure->second.first > 0) {
    failure->second.first -= 1;
    ServiceResponse failed;
    failed.ok = false;
    failed.status = 503;
    failed.error = failure->second.second;
    return failed;
  }
  const auto it = responses_.find(call.url);
  if (it != responses_.end()) return it->second;
  if (!defaultResponse_.body.isNull() || !defaultResponse_.ok) return defaultResponse_;
  ServiceResponse ok;
  ok.ok = true;
  ok.status = 200;
  json::Value body = json::Value::object();
  body.set("status", json::Value("OK"));
  ok.body = body;
  return ok;
}

int MockServiceInvoker::callCount(const std::string& url) const {
  int count = 0;
  for (const auto& call : calls_) {
    if (url.empty() || call.url == url) ++count;
  }
  return count;
}

void MockServiceInvoker::clear() {
  calls_.clear();
  failures_.clear();
}

void RecordingNotifier::send(const std::string& channel, const std::string& target,
                             const std::string& templateName, const json::Value& payload) {
  Message message;
  message.channel = channel;
  message.target = target;
  message.templateName = templateName;
  message.payload = payload;
  messages_.push_back(message);
}

int RecordingNotifier::countOf(const std::string& channel) const {
  int count = 0;
  for (const auto& message : messages_) {
    if (message.channel == channel) ++count;
  }
  return count;
}

bool RecordingEventPublisher::publish(EventType type, const std::string& listenerType,
                                      const std::string& destination, const json::Value& payload,
                                      std::string* error) {
  if (remainingFailures_ > 0) {
    --remainingFailures_;
    if (error != nullptr) *error = failureMessage_;
    return false;
  }
  Delivery delivery;
  delivery.type = type;
  delivery.listenerType = listenerType;
  delivery.destination = destination;
  delivery.payload = payload;
  deliveries_.push_back(delivery);
  return true;
}

void RecordingEventPublisher::failTimes(int times, const std::string& error) {
  remainingFailures_ = times;
  failureMessage_ = error;
}

bool LogEventPublisher::publish(EventType type, const std::string& listenerType,
                                const std::string& destination, const json::Value& payload,
                                std::string*) {
  // 写 stderr，保持 stdout 只输出 API 响应 JSON
  std::cerr << "[event] " << toString(type) << " -> " << listenerType << ":" << destination << " "
            << payload.dump() << "\n";
  return true;
}

void LogNotifier::send(const std::string& channel, const std::string& target,
                       const std::string& templateName, const json::Value& payload) {
  std::cerr << "[message] channel=" << channel << " target=" << target
            << " template=" << templateName << " payload=" << payload.dump() << "\n";
}

}  // namespace wf

// API facade: mirrors the REST surface of PRD §18 as plain C++ calls and
// returns the same JSON envelopes (code / message / data) that the HTTP layer
// would, so a thin HTTP adapter can be put in front later without touching the
// engine. Services are grouped by PRD §5.2 (定义服务 / 实例服务 / 任务服务 /
// 查询服务).
#pragma once

#include <string>
#include <vector>

#include "wf/engine.hpp"
#include "wf/json.hpp"
#include "wf/repository.hpp"

namespace wf {

// 统一响应体：{"code":0,"message":"ok","data":{...}}
json::Value okResponse(json::Value data);
json::Value errorResponse(const std::string& message);

// 流程定义服务（PRD §5.2.2、§18.1）
class ProcessDefinitionService {
 public:
  ProcessDefinitionService(ProcessEngine& engine, Repository& repository);

  json::Value validate(const json::Value& body);
  json::Value create(const json::Value& body);   // 草稿
  json::Value publish(const json::Value& body);  // 直接发布（校验失败返回错误）
  json::Value publishByCode(const std::string& tenantId, const std::string& code, int version);
  json::Value disable(const std::string& tenantId, const std::string& code, int version);
  json::Value list(const std::string& tenantId, const std::string& code) const;
  const ProcessDefinition* active(const std::string& tenantId, const std::string& code) const;

 private:
  ProcessEngine& engine_;
  Repository& repository_;
};

// 流程实例服务（PRD §5.2.3、§18.2）
class ProcessInstanceService {
 public:
  ProcessInstanceService(ProcessEngine& engine, Repository& repository);

  json::Value start(const json::Value& body);
  json::Value get(long long instanceId);
  json::Value list(const std::string& tenantId, const std::string& status, std::size_t limit);
  json::Value cancel(long long instanceId, const std::string& user, const std::string& comment);
  json::Value suspend(long long instanceId, const std::string& user);
  json::Value resume(long long instanceId, const std::string& user);
  json::Value terminate(long long instanceId, const std::string& user, const std::string& comment);
  json::Value withdraw(long long instanceId, const std::string& user, const std::string& comment);
  json::Value retryNode(long long instanceId, const std::string& nodeId, const std::string& user);
  json::Value jumpToNode(long long instanceId, const std::string& nodeId, const std::string& user);
  json::Value signal(const std::string& eventKey, const json::Value& payload, long long instanceId);
  json::Value histories(long long instanceId);      // §18.4 流程轨迹
  json::Value nodeInstances(long long instanceId);  // 节点执行详情
  json::Value events(long long instanceId);         // 事件 outbox
  json::Value statistics(const std::string& tenantId);

 private:
  ProcessEngine& engine_;
  Repository& repository_;
};

// 任务服务（PRD §5.2.4、§18.3）
class TaskService {
 public:
  TaskService(ProcessEngine& engine, Repository& repository);

  json::Value todo(const std::string& tenantId, const std::string& assignee, std::size_t page,
                   std::size_t size);
  json::Value done(const std::string& tenantId, const std::string& assignee, std::size_t page,
                   std::size_t size);
  json::Value candidates(const std::string& tenantId, const std::string& user);
  json::Value get(long long taskId);
  json::Value claim(long long taskId, const std::string& user);
  json::Value complete(long long taskId, const json::Value& body);
  json::Value transfer(long long taskId, const json::Value& body);
  json::Value delegate(long long taskId, const json::Value& body);
  json::Value addSign(long long taskId, const json::Value& body);
  json::Value returnBack(long long taskId, const json::Value& body);

 private:
  ProcessEngine& engine_;
  Repository& repository_;
};

}  // namespace wf

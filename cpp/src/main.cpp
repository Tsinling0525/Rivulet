// wf-cli: command line front door for the workflow engine.
//
// Every subcommand maps to one PRD §18 API operation and prints the same JSON
// envelope ({"code","message","data"}) that the HTTP layer would return, so the
// CLI doubles as an executable API reference:
//
//   wf-cli validate examples/expense_approval.json
//   wf-cli publish  examples/expense_approval.json --repo /tmp/wf
//   wf-cli start --code expense_approval --key EXP-1 --initiator U10086 --var amount=12000
//   wf-cli todo --user U10086
//   wf-cli approve --task 1001 --user U10086 --comment 同意
//   wf-cli history 1
//   wf-cli tick --advance 24h
//   wf-cli demo
#include <algorithm>
#include <fstream>
#include <iostream>
#include <memory>
#include <sstream>
#include <string>
#include <vector>

#include "wf/api.hpp"

namespace {

struct Options {
  std::string command;
  std::vector<std::string> positional;
  std::vector<std::pair<std::string, std::string>> flags;
  std::vector<std::pair<std::string, std::string>> variables;
  bool help = false;
  bool demoOrg = false;
  std::string repoPath;
  std::string orgPath;
  std::string tenant = "default";

  std::string flag(const std::string& name, const std::string& fallback = "") const {
    for (const auto& item : flags) {
      if (item.first == name) return item.second;
    }
    return fallback;
  }
  bool hasFlag(const std::string& name) const { return !flag(name).empty(); }
  long long number(const std::string& name, long long fallback) const {
    const std::string value = flag(name);
    if (value.empty()) return fallback;
    try {
      return std::stoll(value);
    } catch (...) {
      return fallback;
    }
  }
};

const char* kUsage = R"(wf-cli - 轻量工作流流程引擎（PRD 实现）

用法: wf-cli <command> [options]

流程定义
  validate <file>                        校验流程定义（PRD §34.2 发布前校验）
  publish  <file>                        校验并发布流程定义
  definitions [--code CODE]              列出流程定义及其版本
  disable  --code CODE --version N       停用某个版本
  publish-version --code CODE [--version N]  发布指定版本

流程实例
  start --code CODE [--key KEY] [--initiator USER] [--var k=v]...
  instances [--status RUNNING] [--limit N]
  instance <id>                          查看实例 + 节点实例 + 任务
  history  <id>                          流程轨迹（PRD §18.4）
  events   <id>                          事件 outbox 记录
  cancel|terminate --instance ID --user U [--comment C]
  suspend|resume   --instance ID --user U
  withdraw         --instance ID --user U [--comment C]
  retry-node       --instance ID --node NODE --user U
  jump             --instance ID --node NODE --user U
  signal --event KEY [--data JSON] [--instance ID]

任务
  todo --user U [--page N --size N]      我的待办（PRD §18.3）
  done --user U                          我的已办
  candidates --user U                    候选人任务（角色/候选池）
  claim    --task ID --user U            认领
  approve  --task ID --user U [--comment C] [--var k=v]...
  reject   --task ID --user U [--comment C]
  transfer --task ID --user U --to U2 [--comment C]
  delegate --task ID --user U --to U2 [--comment C]
  add-sign --task ID --user U --to U2,U3 [--type BEFORE|AFTER] [--mode ALL|ANY]
  return-back --task ID --user U [--node NODE] [--comment C]

调度与运维
  tick [--advance 1h] [--max N]          触发定时节点/超时策略/事件重投
  dispatch-events [--max N]              仅投递事件 outbox
  stats                                  运行指标（PRD §33.5）
  demo                                   跑通 PRD 报销审批示例流程

全局
  --repo PATH        使用 JSON 文件仓储（默认纯内存）
  --org PATH         从 JSON 文件加载组织（users/roles/positions/leader）
  --demo-org         载入演示组织（U10086/U20001/U30001/U40001）
  --tenant NAME      租户，默认 default
  --help             显示帮助
)";

Options parseArgs(int argc, char** argv) {
  Options options;
  std::vector<std::string> args(argv + 1, argv + argc);
  for (std::size_t i = 0; i < args.size(); ++i) {
    const std::string& arg = args[i];
    if (arg == "--help" || arg == "-h") {
      options.help = true;
      continue;
    }
    if (arg == "--demo-org") {
      options.demoOrg = true;
      continue;
    }
    if (arg.rfind("--", 0) == 0) {
      const std::string key = arg.substr(2);
      std::string value;
      if (i + 1 < args.size() && args[i + 1].rfind("--", 0) != 0) {
        value = args[++i];
      }
      if (key == "var") {
        const std::size_t equals = value.find('=');
        if (equals != std::string::npos) {
          options.variables.emplace_back(value.substr(0, equals), value.substr(equals + 1));
        }
      } else if (key == "repo") {
        options.repoPath = value;
      } else if (key == "org") {
        options.orgPath = value;
      } else if (key == "tenant") {
        options.tenant = value;
      } else {
        options.flags.emplace_back(key, value);
      }
      continue;
    }
    if (options.command.empty()) {
      options.command = arg;
    } else {
      options.positional.push_back(arg);
    }
  }
  return options;
}

std::string readFile(const std::string& path, bool* ok) {
  std::ifstream in(path, std::ios::binary);
  if (!in) {
    *ok = false;
    return {};
  }
  std::ostringstream buffer;
  buffer << in.rdbuf();
  *ok = true;
  return buffer.str();
}

void printJson(const wf::json::Value& value) { std::cout << value.dump(2) << "\n"; }

bool printResult(const wf::json::Value& envelope) {
  printJson(envelope);
  return envelope.intOr("code", 1) == 0;
}

wf::json::Value buildVariables(const Options& options) {
  wf::json::Value variables = wf::json::Value::object();
  for (const auto& item : options.variables) {
    variables.set(item.first, wf::json::Value::fromLiteral(item.second));
  }
  return variables;
}

void seedDemoOrganization(wf::InMemoryOrganization& org) {
  org.addUser("U10086", "张三", "D1", "U20001");
  org.addUser("U20001", "李四", "D1", "U30001");
  org.addUser("U30001", "王五", "D0", "");
  org.addUser("U40001", "赵六", "D2", "U30001");
  org.grantRole("U10086", "STAFF");
  org.grantRole("U20001", "MANAGER");
  org.grantRole("U30001", "DIRECTOR");
  org.grantRole("U40001", "FINANCE_MANAGER");
  org.assignPosition("U20001", "FINANCE_REVIEWER");
}

bool loadOrganization(wf::InMemoryOrganization& org, const std::string& path) {
  bool ok = false;
  const std::string text = readFile(path, &ok);
  if (!ok) {
    std::cerr << "无法读取组织文件：" << path << "\n";
    return false;
  }
  const wf::json::Value data = wf::json::Value::parse(text);
  if (data.contains("users")) {
    for (const auto& item : data.at("users").items()) {
      org.addUser(item.stringOr("id", ""), item.stringOr("name", ""), item.stringOr("department", ""),
                  item.stringOr("leader", ""));
      if (item.contains("roles")) {
        for (const auto& role : item.at("roles").items()) org.grantRole(item.stringOr("id", ""), role.toString());
      }
      if (item.contains("positions")) {
        for (const auto& position : item.at("positions").items()) {
          org.assignPosition(item.stringOr("id", ""), position.toString());
        }
      }
    }
  }
  return true;
}

// 演示：PRD §2.3 混合流程（人工 + 自动 + 外部系统）
int runDemo(wf::ProcessDefinitionService& definitions, wf::TaskService& tasks,
            wf::ProcessInstanceService& instances, const std::string& definitionPath) {
  bool ok = false;
  const std::string text = readFile(definitionPath, &ok);
  if (!ok) {
    std::cerr << "无法读取流程定义：" << definitionPath << "\n";
    return 1;
  }
  const wf::json::Value validation = definitions.validate(wf::json::Value::parse(text));
  std::cout << "① 校验流程定义\n" << validation.at("data").at("summary").asString() << "\n\n";
  const wf::json::Value published = definitions.publish(wf::json::Value::parse(text));
  if (published.intOr("code", 1) != 0) {
    std::cerr << "发布失败：" << published.stringOr("message", "") << "\n";
    return 1;
  }
  std::cout << "② 发布成功：expense_approval v"
            << published.at("data").intOr("version", 0) << "\n\n";

  wf::json::Value startBody = wf::json::Value::object();
  startBody.set("processCode", wf::json::Value("expense_approval"));
  startBody.set("businessKey", wf::json::Value("EXPENSE-20260918-0001"));
  startBody.set("initiator", wf::json::Value("U10086"));
  startBody.set("variables", wf::json::Value::parse(R"({"amount": 12000, "type": "travel", "days": 4})"));
  const wf::json::Value started = instances.start(startBody);
  if (started.intOr("code", 1) != 0) {
    std::cerr << "发起失败：" << started.stringOr("message", "") << "\n";
    return 1;
  }
  const long long instanceId = started.at("data").at("instance").intOr("id", 0);
  std::cout << "③ 发起流程：实例 " << instanceId << "，金额 12000 元\n";
  std::cout << "   待办：" << started.at("data").at("tasks").at(0).stringOr("assignee", "")
            << "（" << started.at("data").at("tasks").at(0).stringOr("taskName", "") << "）\n\n";

  const long long submitTask = started.at("data").at("tasks").at(0).intOr("id", 0);
  wf::json::Value approveBody = wf::json::Value::object();
  approveBody.set("action", wf::json::Value("APPROVE"));
  approveBody.set("operator", wf::json::Value("U10086"));
  approveBody.set("comment", wf::json::Value("提交报销申请"));
  const wf::json::Value submitted = tasks.complete(submitTask, approveBody);
  const long long directorTask = submitted.at("data").at("tasks").at(0).intOr("id", 0);
  std::cout << "④ 提交申请 → 金额判断走总监审批，待办给 "
            << submitted.at("data").at("tasks").at(0).stringOr("assignee", "") << "\n\n";

  approveBody.set("operator", wf::json::Value("U30001"));
  approveBody.set("comment", wf::json::Value("同意报销"));
  const wf::json::Value approved = tasks.complete(directorTask, approveBody);
  std::cout << "⑤ 总监同意 → 自动节点调用通知服务 → 流程结束\n";
  std::cout << "   实例状态：" << approved.at("data").at("instance").stringOr("status", "") << "\n\n";

  std::cout << "⑥ 流程轨迹\n";
  const wf::json::Value history = instances.histories(instanceId);
  for (const auto& record : history.at("data").items()) {
    std::cout << "   " << wf::formatTime(record.intOr("createdAt", 0)) << "  "
              << record.stringOr("nodeName", "") << "  " << record.stringOr("action", "") << "  "
              << record.stringOr("operator", "") << "  " << record.stringOr("comment", "") << "\n";
  }
  std::cout << "\n" << instances.statistics("default").at("data").dump(2) << "\n";
  return 0;
}

}  // namespace

int main(int argc, char** argv) {
  const Options options = parseArgs(argc, argv);
  if (options.help || options.command.empty()) {
    std::cout << kUsage;
    return options.command.empty() && !options.help ? 1 : 0;
  }

  wf::InMemoryRepository repository;
  wf::ManualClock clock(wf::SystemClock().now());
  wf::InMemoryOrganization organization;
  wf::MockServiceInvoker invoker;
  wf::LogNotifier notifier;
  wf::LogEventPublisher publisher;

  wf::ProcessEngine engine(repository, clock);
  engine.setOrganization(&organization);
  engine.setServiceInvoker(&invoker);
  engine.setNotifier(&notifier);
  engine.setEventPublisher(&publisher);

  if (options.demoOrg) seedDemoOrganization(organization);
  if (!options.orgPath.empty() && !loadOrganization(organization, options.orgPath)) return 1;

  std::string repoFile;
  if (!options.repoPath.empty()) {
    repoFile = options.repoPath;
    if (repoFile.size() > 5 && repoFile.compare(repoFile.size() - 5, 5, ".json") != 0) {
      if (repoFile.back() != '/') repoFile.push_back('/');
      repoFile += "wfengine-repository.json";
    }
    std::ifstream probe(repoFile);
    if (probe.good()) {
      std::string error;
      if (!wf::loadRepositoryFromFile(repository, repoFile, &error)) {
        std::cerr << error << "\n";
        return 1;
      }
    }
  }

  wf::ProcessDefinitionService definitions(engine, repository);
  wf::ProcessInstanceService instances(engine, repository);
  wf::TaskService tasks(engine, repository);

  const std::string& command = options.command;
  int status = 0;
  bool okResult = true;

  auto definitionFile = [&](bool* ok) { return readFile(options.positional.empty() ? "" : options.positional[0], ok); };

  if (command == "validate") {
    bool ok = false;
    const std::string text = definitionFile(&ok);
    if (!ok) {
      std::cerr << "用法: wf-cli validate <定义文件>\n";
      return 1;
    }
    okResult = printResult(definitions.validate(wf::json::Value::parse(text)));
  } else if (command == "publish") {
    bool ok = false;
    const std::string text = definitionFile(&ok);
    if (!ok) {
      std::cerr << "用法: wf-cli publish <定义文件>\n";
      return 1;
    }
    okResult = printResult(definitions.publish(wf::json::Value::parse(text)));
  } else if (command == "publish-version") {
    okResult = printResult(definitions.publishByCode(options.tenant, options.flag("code"),
                                                     static_cast<int>(options.number("version", 0))));
  } else if (command == "disable") {
    okResult = printResult(definitions.disable(options.tenant, options.flag("code"),
                                               static_cast<int>(options.number("version", 0))));
  } else if (command == "definitions") {
    okResult = printResult(definitions.list(options.tenant, options.flag("code")));
  } else if (command == "start") {
    wf::json::Value body = wf::json::Value::object();
    body.set("processCode", wf::json::Value(options.flag("code")));
    body.set("businessKey", wf::json::Value(options.flag("key")));
    body.set("initiator", wf::json::Value(options.flag("initiator")));
    body.set("tenantId", wf::json::Value(options.tenant));
    body.set("variables", buildVariables(options));
    okResult = printResult(instances.start(body));
  } else if (command == "instances") {
    okResult = printResult(instances.list(options.tenant, options.flag("status"),
                                          static_cast<std::size_t>(options.number("limit", 50))));
  } else if (command == "instance") {
    okResult = printResult(instances.get(options.number("instance", options.positional.empty() ? 0
                                                                                             : std::stoll(options.positional[0]))));
  } else if (command == "history") {
    okResult = printResult(instances.histories(options.positional.empty() ? 0 : std::stoll(options.positional[0])));
  } else if (command == "events") {
    okResult = printResult(instances.events(options.positional.empty() ? 0 : std::stoll(options.positional[0])));
  } else if (command == "tasks") {
    okResult = printResult(instances.get(options.positional.empty() ? 0 : std::stoll(options.positional[0])));
  } else if (command == "todo" || command == "done") {
    const std::string user = options.flag("user");
    const std::size_t page = static_cast<std::size_t>(options.number("page", 1));
    const std::size_t size = static_cast<std::size_t>(options.number("size", 20));
    okResult = printResult(command == "todo" ? tasks.todo(options.tenant, user, page, size)
                                            : tasks.done(options.tenant, user, page, size));
  } else if (command == "candidates") {
    okResult = printResult(tasks.candidates(options.tenant, options.flag("user")));
  } else if (command == "claim") {
    okResult = printResult(tasks.claim(options.number("task", 0), options.flag("user")));
  } else if (command == "approve" || command == "reject") {
    wf::json::Value body = wf::json::Value::object();
    body.set("action", wf::json::Value(command == "approve" ? "APPROVE" : "REJECT"));
    body.set("operator", wf::json::Value(options.flag("user")));
    body.set("comment", wf::json::Value(options.flag("comment")));
    body.set("variables", buildVariables(options));
    if (options.hasFlag("version")) body.set("version", wf::json::Value(options.number("version", -1)));
    okResult = printResult(tasks.complete(options.number("task", 0), body));
  } else if (command == "transfer" || command == "delegate") {
    wf::json::Value body = wf::json::Value::object();
    body.set("operator", wf::json::Value(options.flag("user")));
    body.set("targetUser", wf::json::Value(options.flag("to")));
    body.set("comment", wf::json::Value(options.flag("comment")));
    okResult = printResult(command == "transfer" ? tasks.transfer(options.number("task", 0), body)
                                                : tasks.delegate(options.number("task", 0), body));
  } else if (command == "add-sign") {
    wf::json::Value body = wf::json::Value::object();
    body.set("operator", wf::json::Value(options.flag("user")));
    body.set("type", wf::json::Value(options.flag("type", "AFTER")));
    body.set("mode", wf::json::Value(options.flag("mode", "ALL")));
    body.set("comment", wf::json::Value(options.flag("comment")));
    wf::json::Value users = wf::json::Value::array();
    for (const auto& user : wf::splitList(options.flag("to"))) users.push_back(wf::json::Value(user));
    body.set("users", users);
    okResult = printResult(tasks.addSign(options.number("task", 0), body));
  } else if (command == "return-back") {
    wf::json::Value body = wf::json::Value::object();
    body.set("operator", wf::json::Value(options.flag("user")));
    body.set("targetNodeId", wf::json::Value(options.flag("node")));
    body.set("comment", wf::json::Value(options.flag("comment")));
    okResult = printResult(tasks.returnBack(options.number("task", 0), body));
  } else if (command == "cancel" || command == "terminate") {
    const long long id = options.number("instance", 0);
    okResult = printResult(command == "cancel"
                               ? instances.cancel(id, options.flag("user"), options.flag("comment"))
                               : instances.terminate(id, options.flag("user"), options.flag("comment")));
  } else if (command == "suspend") {
    okResult = printResult(instances.suspend(options.number("instance", 0), options.flag("user")));
  } else if (command == "resume") {
    okResult = printResult(instances.resume(options.number("instance", 0), options.flag("user")));
  } else if (command == "withdraw") {
    okResult = printResult(instances.withdraw(options.number("instance", 0), options.flag("user"),
                                              options.flag("comment")));
  } else if (command == "retry-node") {
    okResult = printResult(instances.retryNode(options.number("instance", 0), options.flag("node"),
                                               options.flag("user")));
  } else if (command == "jump") {
    okResult = printResult(instances.jumpToNode(options.number("instance", 0), options.flag("node"),
                                                options.flag("user")));
  } else if (command == "signal") {
    wf::json::Value payload = options.hasFlag("data") ? wf::json::Value::parse(options.flag("data"))
                                                      : wf::json::Value::object();
    okResult = printResult(instances.signal(options.flag("event"), payload,
                                            options.number("instance", 0)));
  } else if (command == "tick") {
    const std::string advance = options.flag("advance");
    if (!advance.empty()) {
      long long millis = 0;
      if (!wf::parseDuration(advance, &millis)) {
        std::cerr << "无法解析时间：" << advance << "（示例：30m / PT24H）\n";
        return 1;
      }
      clock.advance(millis);
    }
    const int jobs = engine.tick(static_cast<int>(options.number("max", 100)));
    wf::json::Value data = wf::json::Value::object();
    data.set("processedJobs", wf::json::Value(jobs));
    data.set("now", wf::json::Value(wf::formatTime(clock.now())));
    okResult = printResult(wf::okResponse(data));
  } else if (command == "dispatch-events") {
    const int delivered = engine.dispatchEvents(static_cast<int>(options.number("max", 100)));
    wf::json::Value data = wf::json::Value::object();
    data.set("handled", wf::json::Value(delivered));
    okResult = printResult(wf::okResponse(data));
  } else if (command == "stats") {
    okResult = printResult(instances.statistics(options.tenant));
  } else if (command == "demo") {
    const std::string path = options.positional.empty() ? "examples/expense_approval.json"
                                                        : options.positional[0];
    status = runDemo(definitions, tasks, instances, path);
  } else {
    std::cerr << "未知命令：" << command << "\n\n" << kUsage;
    status = 1;
  }

  if (!repoFile.empty()) {
    std::string error;
    if (!wf::saveRepositoryToFile(repository, repoFile, &error)) {
      std::cerr << error << "\n";
      status = 1;
    }
  }
  if (status == 0 && !okResult) status = 1;
  return status;
}

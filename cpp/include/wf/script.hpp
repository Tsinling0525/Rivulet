// Minimal sandboxed script language for SCRIPT_TASK (PRD §16.3).
//
// Supported statements:
//   amount = amount * 1.1;          // 写流程变量
//   variables.level = 'HIGH';       // 兼容 variables. 前缀
//   set flag = true;                // set 关键字可选
//   if (amount > 10000) { level = 'HIGH'; } else { level = 'LOW'; }
//   return level;                   // 作为节点输出
//   // 注释 或 # 注释
//
// There is deliberately no I/O, no host access and no loops, which keeps the
// sandbox guarantee of PRD §30.3 without pulling in a Groovy/JS runtime.
#pragma once

#include <string>

#include "wf/expression.hpp"
#include "wf/json.hpp"

namespace wf {
namespace script {

struct ScriptOutcome {
  bool ok = true;
  std::string error;
  bool hasReturn = false;
  json::Value returnValue;
  int statements = 0;
};

ScriptOutcome run(const std::string& source, json::Value& variables,
                  const expr::Functions& functions = expr::Functions());

}  // namespace script
}  // namespace wf

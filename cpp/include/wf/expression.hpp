// Expression engine (PRD §13): `${amount > 10000}`, `${form.type == 'travel' && form.days > 3}`.
//
// Self-contained recursive-descent parser + evaluator over json::Value, so no
// Aviator/SpEL/Groovy runtime is needed. Evaluation is sandboxed by design:
// there is no I/O, no reflection and no host access — only literals, paths,
// operators and a whitelist of injected functions (PRD §30.2).
#pragma once

#include <functional>
#include <map>
#include <memory>
#include <stdexcept>
#include <string>
#include <vector>

#include "wf/json.hpp"

namespace wf {
namespace expr {

class ExprError : public std::runtime_error {
 public:
  explicit ExprError(const std::string& message) : std::runtime_error("expr: " + message) {}
};

// Injected callables, e.g. `hasRole(initiator, 'FINANCE_MANAGER')`.
using Function = std::function<json::Value(const std::vector<json::Value>&)>;
using Functions = std::map<std::string, Function>;

struct EvalResult {
  json::Value value;
  bool ok = true;
  std::string error;
};

class Expression {
 public:
  Expression() = default;
  explicit Expression(std::string source) : source_(std::move(source)) {}

  static Expression parse(const std::string& source);

  // Throwing form: raises ExprError on parse/eval failure.
  json::Value eval(const json::Value& context, const Functions& functions = Functions()) const;

  // Non-throwing form used by the engine so one bad rule cannot break a run.
  EvalResult evaluate(const json::Value& context, const Functions& functions = Functions()) const;
  bool test(const json::Value& context, const Functions& functions = Functions()) const;

  const std::string& source() const { return source_; }
  bool empty() const { return source_.empty(); }

  // AST node. Public only so the parser/evaluator translation units can share
  // it; nothing outside this library should build trees by hand.
  struct Node;

 private:
  std::string source_;
  std::shared_ptr<Node> root_;
};

// True when the raw condition looks like `${...}` (or a bare expression).
bool isCondition(const std::string& text);
// Strips the surrounding `${ ... }` when present.
std::string unwrap(const std::string& text);
std::string trim(const std::string& text);

// Evaluates a condition, returning `fallback` when the expression is empty or
// fails to evaluate.
bool evaluateCondition(const std::string& source,
                       const json::Value& context,
                       const Functions& functions = Functions(),
                       bool fallback = false,
                       std::string* error = nullptr);

// Evaluates any expression source (${...} or plain) and returns its value.
json::Value evaluateValue(const std::string& source,
                          const json::Value& context,
                          const Functions& functions = Functions());

// Template rendering for HTTP bodies / URLs / messages:
//   "${amount}"        -> typed value of the variable
//   "order-${id}"      -> string interpolation
json::Value interpolate(const std::string& text, const json::Value& context,
                        const Functions& functions = Functions());

// Recursively renders every string inside a JSON structure.
json::Value renderValue(const json::Value& value, const json::Value& context,
                        const Functions& functions = Functions());

// Built-ins available to every expression (len/upper/int/now/...).
Functions builtinFunctions();

// Convenience wrapper adding callables to a table.
void registerFunction(Functions& functions, const std::string& name, Function fn);

}  // namespace expr
}  // namespace wf

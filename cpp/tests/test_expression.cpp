#include "test.hpp"
#include "wf/expression.hpp"
#include "wf/json.hpp"

using wf::expr::Expression;
using wf::json::Value;

namespace {

Value context() {
  return Value::parse(R"({
    "amount": 12000,
    "type": "travel",
    "days": 4,
    "initiator": "U10086",
    "manager": "U20001",
    "form": {"type": "travel", "days": 4, "items": [10, 20, 30]},
    "flags": {"urgent": true},
    "user": {"departmentId": "FINANCE", "roles": ["STAFF", "FINANCE_MANAGER"]}
  })");
}

}  // namespace

WF_TEST(expr_prd_condition_examples) {
  const Value ctx = context();
  CHECK(wf::expr::evaluateCondition("${amount > 10000}", ctx, {}, false));
  CHECK(wf::expr::evaluateCondition("${amount <= 10000}", ctx, {}, true) == false);
  CHECK(wf::expr::evaluateCondition("${form.type == 'travel' && form.days > 3}", ctx, {}, false));
  CHECK(wf::expr::evaluateCondition("${user.departmentId == 'FINANCE'}", ctx, {}, false));
  CHECK(wf::expr::evaluateCondition("${amount > 10000 || days > 10}", ctx, {}, false));
  CHECK(wf::expr::evaluateCondition("${!(amount > 10000)}", ctx, {}, true) == false);
  CHECK(wf::expr::evaluateCondition("${flags.urgent}", ctx, {}, false));
}

WF_TEST(expr_arithmetic_and_precedence) {
  const Value ctx = context();
  CHECK_EQ(wf::expr::evaluateValue("${10 + 2 * 3}", ctx).asInt(), 16);
  CHECK_EQ(wf::expr::evaluateValue("${(10 + 2) * 3}", ctx).asInt(), 36);
  CHECK_EQ(wf::expr::evaluateValue("${amount / 1000}", ctx).asInt(), 12);
  CHECK_EQ(wf::expr::evaluateValue("${amount % 7}", ctx).asInt(), 12000 % 7);
  CHECK_EQ(wf::expr::evaluateValue("${-days + 10}", ctx).asInt(), 6);
  CHECK_EQ(wf::expr::evaluateValue("${'a' + 'b'}", ctx).asString(), std::string("ab"));
}

WF_TEST(expr_path_index_and_missing_values) {
  const Value ctx = context();
  CHECK_EQ(wf::expr::evaluateValue("${form.items[1]}", ctx).asInt(), 20);
  CHECK_EQ(wf::expr::evaluateValue("${user.roles[1]}", ctx).asString(), std::string("FINANCE_MANAGER"));
  CHECK(wf::expr::evaluateValue("${missing.deep.path}", ctx).isNull());
  CHECK(!wf::expr::evaluateCondition("${missing.deep.path}", ctx, {}, false));
}

WF_TEST(expr_functions) {
  const Value ctx = context();
  CHECK_EQ(wf::expr::evaluateValue("${len(form.items)}", ctx).asInt(), 3);
  CHECK_EQ(wf::expr::evaluateValue("${upper(type)}", ctx).asString(), std::string("TRAVEL"));
  CHECK(wf::expr::evaluateCondition("${contains(user.roles, 'STAFF')}", ctx, {}, false));
  CHECK(wf::expr::evaluateCondition("${endsWith(initiator, '086')}", ctx, {}, false));
  CHECK(wf::expr::evaluateCondition("${max(1, days, 3) == 4}", ctx, {}, false));
}

WF_TEST(expr_custom_functions_are_injected) {
  wf::expr::Functions functions = wf::expr::builtinFunctions();
  wf::expr::registerFunction(functions, "hasRole", [](const std::vector<Value>& args) {
    const std::string role = args.size() > 1 ? args[1].toString() : "";
    return Value(role == "FINANCE_MANAGER");
  });
  CHECK(wf::expr::evaluateCondition("${hasRole(initiator, 'FINANCE_MANAGER')}", context(), functions, false));
  CHECK(!wf::expr::evaluateCondition("${hasRole(initiator, 'OTHER')}", context(), functions, false));
}

WF_TEST(expr_interpolation) {
  const Value ctx = context();
  CHECK_EQ(wf::expr::interpolate("${amount}", ctx).asInt(), 12000);
  CHECK_EQ(wf::expr::interpolate("EXPENSE-${initiator}-${amount}", ctx).asString(),
           std::string("EXPENSE-U10086-12000"));
  CHECK_EQ(wf::expr::interpolate("no-placeholder", ctx).asString(), std::string("no-placeholder"));
}

WF_TEST(expr_render_value_keeps_numbers_typed) {
  const Value ctx = context();
  const Value templateValue = Value::parse(R"({"userId": "${initiator}", "amount": "${amount}", "note": "金额=${amount}"})");
  const Value rendered = wf::expr::renderValue(templateValue, ctx);
  CHECK_EQ(rendered["userId"].asString(), std::string("U10086"));
  CHECK(rendered["amount"].isNumber());
  CHECK_EQ(rendered["amount"].asInt(), 12000);
  CHECK_EQ(rendered["note"].asString(), std::string("金额=12000"));
}

WF_TEST(expr_reports_errors_without_throwing) {
  const wf::expr::EvalResult result = Expression::parse("bogusFunc(1)").evaluate(context());
  CHECK(!result.ok);
  CHECK(!result.error.empty());

  std::string error;
  const bool value = wf::expr::evaluateCondition("${bogusFunc(1)}", context(), {}, true, &error);
  CHECK(value);
  CHECK(!error.empty());
}

WF_TEST(expr_helpers) {
  CHECK(wf::expr::isCondition("  ${amount > 1}  "));
  CHECK(!wf::expr::isCondition("amount > 1"));
  CHECK_EQ(wf::expr::unwrap("${amount > 1}"), std::string("amount > 1"));
  CHECK_EQ(wf::expr::unwrap("amount > 1"), std::string("amount > 1"));
  CHECK_THROWS(Expression::parse("(1 + 2"));
  CHECK_THROWS(Expression::parse(""));
}

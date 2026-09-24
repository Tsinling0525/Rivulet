#include "test.hpp"
#include "wf/script.hpp"

using wf::json::Value;

WF_TEST(script_assigns_variables) {
  Value variables = Value::parse(R"({"amount": 12000})");
  const wf::script::ScriptOutcome outcome =
      wf::script::run("if (amount > 10000) { level = 'HIGH'; } else { level = 'LOW'; }\n"
                      "score = amount / 1000;",
                      variables);
  CHECK(outcome.ok);
  CHECK_EQ(variables.at("level").asString(), std::string("HIGH"));
  CHECK_EQ(variables.at("score").asInt(), 12);
}

WF_TEST(script_supports_set_prefix_and_variables_path) {
  Value variables = Value::parse(R"({"amount": 100})");
  const wf::script::ScriptOutcome outcome =
      wf::script::run("set doubled = amount * 2;\nvariables.label = 'ok';", variables);
  CHECK(outcome.ok);
  CHECK_EQ(variables.at("doubled").asInt(), 200);
  CHECK_EQ(variables.at("label").asString(), std::string("ok"));
}

WF_TEST(script_returns_value) {
  Value variables = Value::parse(R"({"a": 3, "b": 4})");
  const wf::script::ScriptOutcome outcome = wf::script::run("total = a + b;\nreturn total;", variables);
  CHECK(outcome.ok);
  CHECK(outcome.hasReturn);
  CHECK_EQ(outcome.returnValue.asInt(), 7);
}

WF_TEST(script_reports_errors) {
  Value variables = Value::parse(R"({})");
  wf::script::ScriptOutcome outcome = wf::script::run("a = ;", variables);
  CHECK(!outcome.ok);
  CHECK(!outcome.error.empty());

  outcome = wf::script::run("2bad = 1;", variables);
  CHECK(!outcome.ok);

  outcome = wf::script::run("if (a > 1) { b = 1;", variables);
  CHECK(!outcome.ok);
}

WF_TEST(script_nested_conditions_and_comments) {
  Value variables = Value::parse(R"({"amount": 5000, "vip": true})");
  const wf::script::ScriptOutcome outcome = wf::script::run(
      "// 审批级别判断\n"
      "if (amount > 10000) {\n"
      "  level = 'DIRECTOR';\n"
      "} else {\n"
      "  if (vip) { level = 'MANAGER_FAST'; } else { level = 'MANAGER'; }\n"
      "}\n"
      "notified = true; # 记录已通知\n",
      variables);
  CHECK(outcome.ok);
  CHECK_EQ(variables.at("level").asString(), std::string("MANAGER_FAST"));
  CHECK(variables.at("notified").asBool());
}

WF_TEST(script_custom_functions_are_visible) {
  wf::expr::Functions functions = wf::expr::builtinFunctions();
  wf::expr::registerFunction(functions, "risk", [](const std::vector<Value>& args) {
    return Value(args.at(0).asNumber() > 5000 ? "HIGH" : "LOW");
  });
  Value variables = Value::parse(R"({"amount": 9000})");
  const wf::script::ScriptOutcome outcome =
      wf::script::run("riskLevel = risk(amount);", variables, functions);
  CHECK(outcome.ok);
  CHECK_EQ(variables.at("riskLevel").asString(), std::string("HIGH"));
}

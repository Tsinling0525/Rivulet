#include "test.hpp"
#include "wf/json.hpp"

using wf::json::Value;

WF_TEST(json_parses_prd_expense_definition) {
  const char* text = R"({
    "processCode": "expense_approval",
    "processName": "报销审批流程",
    "version": 1,
    "nodes": [
      {"id": "start", "type": "START", "name": "开始"},
      {"id": "amountGateway", "type": "EXCLUSIVE_GATEWAY", "name": "金额判断"}
    ],
    "flags": [true, false, null],
    "amount": 12000,
    "ratio": 0.5
  })";
  const Value root = Value::parse(text);
  CHECK(root.isObject());
  CHECK_EQ(root["processCode"].asString(), std::string("expense_approval"));
  CHECK_EQ(root["processName"].asString(), std::string("报销审批流程"));
  CHECK_EQ(root["version"].asInt(), 1);
  CHECK_EQ(root["nodes"].size(), std::size_t(2));
  CHECK_EQ(root["nodes"].at(1).at("id").asString(), std::string("amountGateway"));
  CHECK_EQ(root["flags"].at(0).asBool(), true);
  CHECK(root["flags"].at(2).isNull());
  CHECK_EQ(root["amount"].asInt(), 12000);
  CHECK_EQ(root["ratio"].asNumber(), 0.5);
  CHECK_EQ(root["missing"].stringOr("k", "dflt"), std::string("dflt"));
}

WF_TEST(json_round_trips_dumps) {
  Value object = Value::object();
  object.set("code", Value("flow_a"));
  object.set("version", Value(3));
  object.set("tags", Value::array({Value("a"), Value("b")}));
  object["nested"]["flag"] = Value(true);
  const std::string dumped = object.dump();
  const Value reparsed = Value::parse(dumped);
  CHECK_EQ(reparsed.dump(), dumped);
  CHECK_EQ(reparsed["nested"]["flag"].asBool(), true);
}

WF_TEST(json_escapes_and_unicode) {
  const Value parsed = Value::parse(R"({"text": "line\nquote\"tab\t\u4e2d\u6587"})");
  CHECK_EQ(parsed["text"].asString(), std::string("line\nquote\"tab\t中文"));
  const std::string dumped = parsed.dump();
  CHECK_EQ(Value::parse(dumped)["text"].asString(), parsed["text"].asString());
}

WF_TEST(json_pretty_print_is_stable) {
  const Value parsed = Value::parse(R"({"a":{"b":[1,2]}})");
  const std::string pretty = parsed.dump(2);
  CHECK_EQ(pretty, std::string("{\n  \"a\": {\n    \"b\": [\n      1,\n      2\n    ]\n  }\n}"));
}

WF_TEST(json_rejects_malformed_input) {
  CHECK_THROWS(Value::parse("{"));
  CHECK_THROWS(Value::parse("[1,]"));
  CHECK_THROWS(Value::parse("{\"a\":1} trailing"));
  CHECK_THROWS(Value::parse("\"unterminated"));
}

WF_TEST(json_path_lookup_and_set) {
  Value root = Value::parse(R"({"form": {"amount": 500}, "list": [{"id": "n1"}]})");
  CHECK(root.find("form.amount") != nullptr);
  CHECK_EQ(root.find("form.amount")->asInt(), 500);
  CHECK(root.find("list.0.id") != nullptr);
  CHECK_EQ(root.find("list.0.id")->asString(), std::string("n1"));
  CHECK(root.find("list.7.id") == nullptr);
  root.setPath("form.audit.owner", Value("U1"));
  CHECK_EQ(root.find("form.audit.owner")->asString(), std::string("U1"));
}

WF_TEST(json_merge_is_deep_for_objects) {
  Value target = Value::parse(R"({"amount": 100, "form": {"a": 1, "b": 2}})");
  const Value patch = Value::parse(R"({"amount": 200, "form": {"b": 20, "c": 30}})");
  Value::merge(target, patch);
  CHECK_EQ(target["amount"].asInt(), 200);
  CHECK_EQ(target["form"]["a"].asInt(), 1);
  CHECK_EQ(target["form"]["b"].asInt(), 20);
  CHECK_EQ(target["form"]["c"].asInt(), 30);
}

WF_TEST(json_literal_fallback_for_cli_vars) {
  CHECK_EQ(Value::fromLiteral("12000").asInt(), 12000);
  CHECK(Value::fromLiteral("true").asBool());
  CHECK_EQ(Value::fromLiteral("travel").asString(), std::string("travel"));
}

WF_TEST(json_equality_and_ordering) {
  CHECK(wf::json::equals(Value(1), Value(1.0)));
  CHECK(wf::json::equals(Value("a"), Value("a")));
  CHECK(!wf::json::equals(Value("1"), Value(2)));
  CHECK(wf::json::equals(Value("3"), Value(3)));
  CHECK(wf::json::compare(Value(1), Value(2)) < 0);
  CHECK(wf::json::compare(Value("b"), Value("a")) > 0);
}

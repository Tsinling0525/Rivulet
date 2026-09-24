// Minimal dependency-free JSON value / parser / serializer.
//
// The workflow engine stores definitions, instance variables, form data and
// audit payloads as JSON, so a self-contained implementation keeps the whole
// project buildable with nothing but a C++17 compiler (see PRD §8 DSL, §17
// data model).
#pragma once

#include <cstddef>
#include <cstdint>
#include <initializer_list>
#include <ostream>
#include <stdexcept>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

namespace wf {
namespace json {

class Value;

using Array = std::vector<Value>;
using Member = std::pair<std::string, Value>;
using Object = std::vector<Member>;

class ParseError : public std::runtime_error {
 public:
  explicit ParseError(const std::string& message) : std::runtime_error("json: " + message) {}
};

// Dynamic JSON value. Objects keep insertion order so dumps are stable.
class Value {
 public:
  enum class Type { Null, Bool, Number, String, Array, Object };

  Value() = default;
  Value(std::nullptr_t) {}  // NOLINT
  Value(bool v) : type_(Type::Bool), bool_(v) {}
  Value(int v) : type_(Type::Number), num_(static_cast<double>(v)) {}
  Value(unsigned int v) : type_(Type::Number), num_(static_cast<double>(v)) {}
  Value(long v) : type_(Type::Number), num_(static_cast<double>(v)) {}
  Value(unsigned long v) : type_(Type::Number), num_(static_cast<double>(v)) {}
  Value(long long v) : type_(Type::Number), num_(static_cast<double>(v)) {}
  Value(unsigned long long v) : type_(Type::Number), num_(static_cast<double>(v)) {}
  Value(double v) : type_(Type::Number), num_(v) {}
  Value(const char* v) : type_(Type::String), str_(v == nullptr ? "" : v) {}
  Value(std::string v) : type_(Type::String), str_(std::move(v)) {}

  static Value array();
  static Value array(std::initializer_list<Value> items);
  static Value object();
  static Value parse(std::string_view text);
  // Parses `text` as a JSON literal; falls back to a plain string when it is
  // not valid JSON (used by the CLI for `--var k=v` arguments).
  static Value fromLiteral(const std::string& text);

  Type type() const { return type_; }
  bool isNull() const { return type_ == Type::Null; }
  bool isBool() const { return type_ == Type::Bool; }
  bool isNumber() const { return type_ == Type::Number; }
  bool isString() const { return type_ == Type::String; }
  bool isArray() const { return type_ == Type::Array; }
  bool isObject() const { return type_ == Type::Object; }
  bool isScalar() const { return type_ != Type::Array && type_ != Type::Object; }

  bool asBool() const;
  double asNumber() const;
  long long asInt() const;
  const std::string& asString() const;

  const Array& items() const;
  Array& items();
  const Object& members() const;
  Object& members();

  std::size_t size() const;
  bool empty() const;

  // --- object access -------------------------------------------------------
  bool contains(const std::string& key) const;
  const Value& operator[](const std::string& key) const;
  Value& operator[](const std::string& key);
  const Value& at(const std::string& key) const;  // null value when missing
  void set(const std::string& key, Value value);
  bool erase(const std::string& key);
  std::vector<std::string> keys() const;

  bool boolOr(const std::string& key, bool fallback) const;
  long long intOr(const std::string& key, long long fallback) const;
  double numberOr(const std::string& key, double fallback) const;
  std::string stringOr(const std::string& key, const std::string& fallback) const;
  Array arrayOr(const std::string& key) const;

  // --- array access --------------------------------------------------------
  const Value& at(std::size_t index) const;
  Value& at(std::size_t index);
  void push_back(Value value);

  // --- path helpers ("form.amount", "list.0.id") ---------------------------
  const Value* find(const std::string& path) const;
  Value* find(const std::string& path);
  void setPath(const std::string& path, Value value);
  bool isTruthy() const;

  std::string dump(int indent = -1) const;
  // Human friendly rendering: strings are unquoted, everything else dumped.
  std::string toString() const;
  static std::string typeName(Type type);
  std::string typeName() const { return typeName(type_); }

  // Deep merge of `patch` into `target` (objects merge key-wise, everything
  // else replaces). Used to apply form/variable patches on task completion.
  static void merge(Value& target, const Value& patch);

 private:
  void dumpTo(std::string& out, int indent, int depth) const;

  Type type_ = Type::Null;
  bool bool_ = false;
  double num_ = 0.0;
  std::string str_;
  Array arr_;
  Object obj_;
};

std::ostream& operator<<(std::ostream& os, const Value& value);

// Loose equality / ordering used by the expression engine.
bool equals(const Value& a, const Value& b);
int compare(const Value& a, const Value& b);  // <0, 0, >0 (orderable types)

}  // namespace json
}  // namespace wf

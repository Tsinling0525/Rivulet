#include "wf/expression.hpp"

#include <algorithm>
#include <cctype>
#include <cmath>
#include <cstdlib>
#include <sstream>

namespace wf {
namespace expr {
namespace {

using json::Value;

std::string lowerCopy(const std::string& text) {
  std::string out = text;
  std::transform(out.begin(), out.end(), out.begin(),
                 [](unsigned char c) { return static_cast<char>(std::tolower(c)); });
  return out;
}

bool isIdentifierStart(char c) {
  return std::isalpha(static_cast<unsigned char>(c)) != 0 || c == '_' || c == '$';
}

bool isIdentifierChar(char c) {
  return std::isalnum(static_cast<unsigned char>(c)) != 0 || c == '_' || c == '$';
}

}  // namespace

struct Expression::Node {
  enum class Kind { Literal, Path, Unary, Binary, Call, List, Ternary };

  Kind kind = Kind::Literal;
  Value literal;
  std::vector<std::string> path;
  std::string op;
  std::vector<std::shared_ptr<Node>> args;
};

namespace {

class Parser {
 public:
  explicit Parser(const std::string& source) : source_(source) {}

  std::shared_ptr<Expression::Node> parse() {
    skip();
    if (atEnd()) {
      throw ExprError("empty expression");
    }
    auto node = parseConditional();
    skip();
    if (!atEnd()) {
      throw ExprError("unexpected trailing input near '" + source_.substr(pos_) + "'");
    }
    return node;
  }

 private:
  using Node = Expression::Node;
  using NodePtr = std::shared_ptr<Node>;

  // 条件表达式：cond ? a : b（右结合），与 SpEL/Aviator 用法一致
  NodePtr parseConditional() {
    NodePtr condition = parseOr();
    if (!match("?")) return condition;
    NodePtr thenBranch = parseConditional();
    expect(":");
    NodePtr elseBranch = parseConditional();
    NodePtr node = make(Node::Kind::Ternary);
    node->op = "?:";
    node->args = {condition, thenBranch, elseBranch};
    return node;
  }

  bool atEnd() const { return pos_ >= source_.size(); }
  char peek() const { return atEnd() ? '\0' : source_[pos_]; }
  char peek(std::size_t ahead) const {
    return pos_ + ahead >= source_.size() ? '\0' : source_[pos_ + ahead];
  }

  void skip() {
    while (!atEnd() && std::isspace(static_cast<unsigned char>(source_[pos_])) != 0) ++pos_;
  }

  bool match(const char* token) {
    skip();
    const std::size_t len = std::strlen(token);
    if (source_.compare(pos_, len, token) == 0) {
      pos_ += len;
      return true;
    }
    return false;
  }

  bool matchWord(const char* word) {
    skip();
    const std::size_t len = std::strlen(word);
    if (source_.compare(pos_, len, word) != 0) return false;
    const char after = pos_ + len < source_.size() ? source_[pos_ + len] : '\0';
    if (isIdentifierChar(after)) return false;
    pos_ += len;
    return true;
  }

  void expect(const char* token) {
    if (!match(token)) {
      throw ExprError(std::string("expected '") + token + "' near '" + source_.substr(pos_) + "'");
    }
  }

  NodePtr make(Node::Kind kind) {
    auto node = std::make_shared<Node>();
    node->kind = kind;
    return node;
  }

  static NodePtr literal(Value value) {
    auto node = std::make_shared<Node>();
    node->kind = Node::Kind::Literal;
    node->literal = std::move(value);
    return node;
  }

  NodePtr parseOr() {
    NodePtr left = parseAnd();
    while (match("||")) {
      NodePtr node = make(Node::Kind::Binary);
      node->op = "||";
      node->args = {left, parseAnd()};
      left = node;
    }
    return left;
  }

  NodePtr parseAnd() {
    NodePtr left = parseEquality();
    while (match("&&")) {
      NodePtr node = make(Node::Kind::Binary);
      node->op = "&&";
      node->args = {left, parseEquality()};
      left = node;
    }
    return left;
  }

  NodePtr parseEquality() {
    NodePtr left = parseComparison();
    while (true) {
      if (match("==")) {
        left = binary("==", left, parseComparison());
      } else if (match("!=")) {
        left = binary("!=", left, parseComparison());
      } else {
        return left;
      }
    }
  }

  NodePtr parseComparison() {
    NodePtr left = parseTerm();
    while (true) {
      if (match(">=")) {
        left = binary(">=", left, parseTerm());
      } else if (match("<=")) {
        left = binary("<=", left, parseTerm());
      } else if (match(">")) {
        left = binary(">", left, parseTerm());
      } else if (match("<")) {
        left = binary("<", left, parseTerm());
      } else {
        return left;
      }
    }
  }

  NodePtr parseTerm() {
    NodePtr left = parseFactor();
    while (true) {
      if (match("+")) {
        left = binary("+", left, parseFactor());
      } else if (match("-")) {
        left = binary("-", left, parseFactor());
      } else {
        return left;
      }
    }
  }

  NodePtr parseFactor() {
    NodePtr left = parseUnary();
    while (true) {
      if (match("*")) {
        left = binary("*", left, parseUnary());
      } else if (match("/")) {
        left = binary("/", left, parseUnary());
      } else if (match("%")) {
        left = binary("%", left, parseUnary());
      } else {
        return left;
      }
    }
  }

  NodePtr binary(const std::string& op, NodePtr left, NodePtr right) {
    NodePtr node = make(Node::Kind::Binary);
    node->op = op;
    node->args = {left, right};
    return node;
  }

  NodePtr parseUnary() {
    if (match("!")) {
      NodePtr node = make(Node::Kind::Unary);
      node->op = "!";
      node->args = {parseUnary()};
      return node;
    }
    if (match("-")) {
      NodePtr node = make(Node::Kind::Unary);
      node->op = "neg";
      node->args = {parseUnary()};
      return node;
    }
    return parsePostfix();
  }

  NodePtr parsePostfix() {
    NodePtr node = parsePrimary();
    while (true) {
      if (match(".")) {
        skip();
        if (atEnd() || !isIdentifierStart(peek())) {
          throw ExprError("expected property name in path");
        }
        const std::string property = parseIdentifier();
        if (node->kind != Node::Kind::Path) {
          throw ExprError("property access is only supported on variables");
        }
        node->path.push_back(property);
        continue;
      }
      if (match("[")) {
        skip();
        // Only literal indexes keep path semantics; other indexes degrade to
        // an explicit lookup call so the tree stays simple.
        if (peek() == '\'' || peek() == '"') {
          const std::string key = parseQuoted();
          expect("]");
          if (node->kind != Node::Kind::Path) {
            throw ExprError("index access is only supported on variables");
          }
          node->path.push_back(key);
          continue;
        }
        NodePtr index = parseOr();
        expect("]");
        NodePtr call = make(Node::Kind::Call);
        call->op = "__index";
        call->args = {node, index};
        node = call;
        continue;
      }
      return node;
    }
  }

  std::string parseIdentifier() {
    const std::size_t start = pos_;
    if (!isIdentifierStart(peek())) {
      throw ExprError("expected identifier");
    }
    while (!atEnd() && isIdentifierChar(peek())) ++pos_;
    return source_.substr(start, pos_ - start);
  }

  std::string parseQuoted() {
    const char quote = peek();
    ++pos_;
    std::string out;
    while (!atEnd()) {
      const char c = source_[pos_++];
      if (c == quote) return out;
      if (c == '\\' && !atEnd()) {
        const char escaped = source_[pos_++];
        switch (escaped) {
          case 'n': out.push_back('\n'); break;
          case 't': out.push_back('\t'); break;
          case 'r': out.push_back('\r'); break;
          case '\\': out.push_back('\\'); break;
          case '\'': out.push_back('\''); break;
          case '"': out.push_back('"'); break;
          default: out.push_back(escaped); break;
        }
        continue;
      }
      out.push_back(c);
    }
    throw ExprError("unterminated string literal");
  }

  NodePtr parsePrimary() {
    skip();
    if (atEnd()) {
      throw ExprError("unexpected end of expression");
    }
    const char c = peek();
    if (c == '(') {
      ++pos_;
      NodePtr node = parseOr();
      expect(")");
      return node;
    }
    if (c == '\'' || c == '"') {
      return literal(Value(parseQuoted()));
    }
    if (c == '[') {
      ++pos_;
      NodePtr node = make(Node::Kind::List);
      skip();
      if (peek() == ']') {
        ++pos_;
        return node;
      }
      while (true) {
        node->args.push_back(parseOr());
        skip();
        if (peek() == ',') {
          ++pos_;
          continue;
        }
        break;
      }
      expect("]");
      return node;
    }
    if (std::isdigit(static_cast<unsigned char>(c)) != 0 || c == '.') {
      return literal(Value(parseNumber()));
    }
    if (isIdentifierStart(c)) {
      const std::string name = parseIdentifier();
      skip();
      if (peek() == '(') {
        ++pos_;
        NodePtr node = make(Node::Kind::Call);
        node->op = name;
        skip();
        if (peek() == ')') {
          ++pos_;
          return node;
        }
        while (true) {
          node->args.push_back(parseOr());
          skip();
          if (peek() == ',') {
            ++pos_;
            continue;
          }
          break;
        }
        expect(")");
        return node;
      }
      const std::string lowered = lowerCopy(name);
      if (lowered == "true") return literal(Value(true));
      if (lowered == "false") return literal(Value(false));
      if (lowered == "null" || lowered == "nil") return literal(Value());
      NodePtr node = make(Node::Kind::Path);
      node->path.push_back(name);
      return node;
    }
    throw ExprError(std::string("unexpected character '") + c + "'");
  }

  double parseNumber() {
    const std::size_t start = pos_;
    while (!atEnd()) {
      const char c = peek();
      if (std::isdigit(static_cast<unsigned char>(c)) != 0 || c == '.' || c == 'e' || c == 'E' ||
          c == '+' || c == '-') {
        ++pos_;
      } else {
        break;
      }
    }
    const std::string token = source_.substr(start, pos_ - start);
    try {
      return std::stod(token);
    } catch (...) {
      throw ExprError("invalid number '" + token + "'");
    }
  }

  const std::string& source_;
  std::size_t pos_ = 0;
};

Value resolvePath(const std::vector<std::string>& path, const Value& context) {
  const Value* current = &context;
  for (const std::string& segment : path) {
    if (current->isObject()) {
      const Value* next = nullptr;
      for (const auto& member : current->members()) {
        if (member.first == segment) {
          next = &member.second;
          break;
        }
      }
      if (next == nullptr) return Value();
      current = next;
    } else if (current->isArray()) {
      char* end = nullptr;
      const long index = std::strtol(segment.c_str(), &end, 10);
      if (end == nullptr || *end != '\0' || index < 0 ||
          static_cast<std::size_t>(index) >= current->items().size()) {
        return Value();
      }
      current = &current->items()[static_cast<std::size_t>(index)];
    } else {
      return Value();
    }
  }
  return *current;
}

Value evalNode(const Expression::Node& node, const Value& context, const Functions& functions);

Value evalBinary(const std::string& op, const Expression::Node& node, const Value& context,
                 const Functions& functions) {
  if (op == "&&") {
    if (!evalNode(*node.args[0], context, functions).isTruthy()) return Value(false);
    return Value(evalNode(*node.args[1], context, functions).isTruthy());
  }
  if (op == "||") {
    if (evalNode(*node.args[0], context, functions).isTruthy()) return Value(true);
    return Value(evalNode(*node.args[1], context, functions).isTruthy());
  }
  const Value left = evalNode(*node.args[0], context, functions);
  const Value right = evalNode(*node.args[1], context, functions);
  if (op == "==") return Value(json::equals(left, right));
  if (op == "!=") return Value(!json::equals(left, right));
  if (op == ">") return Value(json::compare(left, right) > 0);
  if (op == ">=") return Value(json::compare(left, right) >= 0);
  if (op == "<") return Value(json::compare(left, right) < 0);
  if (op == "<=") return Value(json::compare(left, right) <= 0);
  if (op == "+") {
    if (left.isString() || right.isString()) {
      return Value(left.toString() + right.toString());
    }
    return Value(left.asNumber() + right.asNumber());
  }
  if (op == "-") return Value(left.asNumber() - right.asNumber());
  if (op == "*") return Value(left.asNumber() * right.asNumber());
  if (op == "/") {
    const double divisor = right.asNumber();
    if (divisor == 0) throw ExprError("division by zero");
    return Value(left.asNumber() / divisor);
  }
  if (op == "%") {
    const long long divisor = right.asInt();
    if (divisor == 0) throw ExprError("modulo by zero");
    return Value(static_cast<long long>(left.asInt() % divisor));
  }
  throw ExprError("unsupported operator '" + op + "'");
}

Value evalNode(const Expression::Node& node, const Value& context, const Functions& functions) {
  switch (node.kind) {
    case Expression::Node::Kind::Literal:
      return node.literal;
    case Expression::Node::Kind::Path:
      return resolvePath(node.path, context);
    case Expression::Node::Kind::List: {
      Value list = Value::array();
      for (const auto& arg : node.args) {
        list.push_back(evalNode(*arg, context, functions));
      }
      return list;
    }
    case Expression::Node::Kind::Unary: {
      const Value operand = evalNode(*node.args[0], context, functions);
      if (node.op == "!") return Value(!operand.isTruthy());
      return Value(-operand.asNumber());
    }
    case Expression::Node::Kind::Binary:
      return evalBinary(node.op, node, context, functions);
    case Expression::Node::Kind::Ternary: {
      const bool condition = evalNode(*node.args[0], context, functions).isTruthy();
      return evalNode(*node.args[condition ? 1 : 2], context, functions);
    }
    case Expression::Node::Kind::Call: {
      std::vector<Value> args;
      args.reserve(node.args.size());
      for (const auto& arg : node.args) {
        args.push_back(evalNode(*arg, context, functions));
      }
      if (node.op == "__index") {
        const Value& target = args[0];
        if (target.isArray()) {
          const long long index = args[1].asInt();
          if (index < 0 || static_cast<std::size_t>(index) >= target.items().size()) return Value();
          return target.items()[static_cast<std::size_t>(index)];
        }
        if (target.isObject()) return target.at(args[1].toString());
        return Value();
      }
      const auto it = functions.find(node.op);
      if (it == functions.end()) {
        throw ExprError("unknown function '" + node.op + "'");
      }
      return it->second(args);
    }
  }
  return Value();
}

}  // namespace

Expression Expression::parse(const std::string& source) {
  Expression expression(unwrap(source));
  Parser parser(expression.source_);
  expression.root_ = parser.parse();
  return expression;
}

json::Value Expression::eval(const json::Value& context, const Functions& functions) const {
  if (root_ == nullptr) {
    return json::Value();
  }
  // An empty table means "just the built-ins", so callers that do not care
  // about injection (interpolation, tests) still get len/upper/now/...
  return evalNode(*root_, context, functions.empty() ? builtinFunctions() : functions);
}

EvalResult Expression::evaluate(const json::Value& context, const Functions& functions) const {
  EvalResult result;
  try {
    result.value = eval(context, functions);
  } catch (const std::exception& error) {
    result.ok = false;
    result.error = error.what();
    result.value = json::Value();
  }
  return result;
}

bool Expression::test(const json::Value& context, const Functions& functions) const {
  return evaluate(context, functions).value.isTruthy();
}

bool isCondition(const std::string& text) {
  const std::string trimmed = trim(text);
  return trimmed.size() >= 3 && trimmed.front() == '$' && trimmed[1] == '{' &&
         trimmed.back() == '}';
}

std::string trim(const std::string& text) {
  std::size_t begin = 0;
  std::size_t end = text.size();
  while (begin < end && std::isspace(static_cast<unsigned char>(text[begin])) != 0) ++begin;
  while (end > begin && std::isspace(static_cast<unsigned char>(text[end - 1])) != 0) --end;
  return text.substr(begin, end - begin);
}

std::string unwrap(const std::string& text) {
  const std::string trimmed = trim(text);
  if (isCondition(trimmed)) {
    return trim(trimmed.substr(2, trimmed.size() - 3));
  }
  return trimmed;
}

bool evaluateCondition(const std::string& source, const json::Value& context,
                       const Functions& functions, bool fallback, std::string* error) {
  const std::string body = unwrap(source);
  if (body.empty()) {
    if (error != nullptr) *error = "empty condition";
    return fallback;
  }
  const EvalResult result = Expression::parse(body).evaluate(context, functions);
  if (!result.ok) {
    if (error != nullptr) *error = result.error;
    return fallback;
  }
  return result.value.isTruthy();
}

json::Value evaluateValue(const std::string& source, const json::Value& context,
                          const Functions& functions) {
  const std::string body = unwrap(source);
  if (body.empty()) return json::Value();
  return Expression::parse(body).evaluate(context, functions).value;
}

json::Value interpolate(const std::string& text, const json::Value& context,
                        const Functions& functions) {
  const std::string trimmed = trim(text);
  if (isCondition(trimmed)) {
    // Whole-string placeholder keeps the original type (number stays number).
    return evaluateValue(trimmed, context, functions);
  }
  if (text.find("${") == std::string::npos) {
    return json::Value(text);
  }
  std::string out;
  std::size_t pos = 0;
  while (pos < text.size()) {
    const std::size_t start = text.find("${", pos);
    if (start == std::string::npos) {
      out += text.substr(pos);
      break;
    }
    out += text.substr(pos, start - pos);
    const std::size_t end = text.find('}', start);
    if (end == std::string::npos) {
      out += text.substr(start);
      break;
    }
    const std::string inner = text.substr(start + 2, end - start - 2);
    const Value value = evaluateValue(inner, context, functions);
    out += value.toString();
    pos = end + 1;
  }
  return json::Value(out);
}

json::Value renderValue(const json::Value& value, const json::Value& context,
                        const Functions& functions) {
  if (value.isString()) {
    return interpolate(value.asString(), context, functions);
  }
  if (value.isArray()) {
    Value out = Value::array();
    for (const auto& item : value.items()) {
      out.push_back(renderValue(item, context, functions));
    }
    return out;
  }
  if (value.isObject()) {
    Value out = Value::object();
    for (const auto& member : value.members()) {
      out.set(member.first, renderValue(member.second, context, functions));
    }
    return out;
  }
  return value;
}

void registerFunction(Functions& functions, const std::string& name, Function fn) {
  functions[name] = std::move(fn);
}

Functions builtinFunctions() {
  Functions functions;

  registerFunction(functions, "len", [](const std::vector<Value>& args) {
    if (args.empty()) return Value(0);
    if (args[0].isArray() || args[0].isObject()) return Value(static_cast<long long>(args[0].size()));
    return Value(static_cast<long long>(args[0].toString().size()));
  });
  registerFunction(functions, "upper", [](const std::vector<Value>& args) {
    if (args.empty()) return Value(std::string());
    std::string text = args[0].toString();
    std::transform(text.begin(), text.end(), text.begin(),
                   [](unsigned char c) { return static_cast<char>(std::toupper(c)); });
    return Value(text);
  });
  registerFunction(functions, "lower", [](const std::vector<Value>& args) {
    if (args.empty()) return Value(std::string());
    return Value(lowerCopy(args[0].toString()));
  });
  registerFunction(functions, "trim", [](const std::vector<Value>& args) {
    if (args.empty()) return Value(std::string());
    return Value(trim(args[0].toString()));
  });
  registerFunction(functions, "contains", [](const std::vector<Value>& args) {
    if (args.size() < 2) return Value(false);
    if (args[0].isArray()) {
      for (const auto& item : args[0].items()) {
        if (json::equals(item, args[1])) return Value(true);
      }
      return Value(false);
    }
    return Value(args[0].toString().find(args[1].toString()) != std::string::npos);
  });
  registerFunction(functions, "startsWith", [](const std::vector<Value>& args) {
    if (args.size() < 2) return Value(false);
    const std::string haystack = args[0].toString();
    const std::string needle = args[1].toString();
    return Value(haystack.rfind(needle, 0) == 0);
  });
  registerFunction(functions, "endsWith", [](const std::vector<Value>& args) {
    if (args.size() < 2) return Value(false);
    const std::string haystack = args[0].toString();
    const std::string needle = args[1].toString();
    return Value(haystack.size() >= needle.size() &&
                 haystack.compare(haystack.size() - needle.size(), needle.size(), needle) == 0);
  });
  registerFunction(functions, "number", [](const std::vector<Value>& args) {
    return Value(args.empty() ? 0.0 : args[0].asNumber());
  });
  registerFunction(functions, "string", [](const std::vector<Value>& args) {
    return Value(args.empty() ? std::string() : args[0].toString());
  });
  registerFunction(functions, "boolean", [](const std::vector<Value>& args) {
    return Value(!args.empty() && args[0].isTruthy());
  });
  registerFunction(functions, "abs", [](const std::vector<Value>& args) {
    return Value(args.empty() ? 0.0 : std::fabs(args[0].asNumber()));
  });
  registerFunction(functions, "min", [](const std::vector<Value>& args) {
    if (args.empty()) return Value(0.0);
    double best = args[0].asNumber();
    for (std::size_t i = 1; i < args.size(); ++i) best = std::min(best, args[i].asNumber());
    return Value(best);
  });
  registerFunction(functions, "max", [](const std::vector<Value>& args) {
    if (args.empty()) return Value(0.0);
    double best = args[0].asNumber();
    for (std::size_t i = 1; i < args.size(); ++i) best = std::max(best, args[i].asNumber());
    return Value(best);
  });
  registerFunction(functions, "round", [](const std::vector<Value>& args) {
    if (args.empty()) return Value(0.0);
    const double digits = args.size() > 1 ? args[1].asNumber() : 0;
    const double factor = std::pow(10.0, digits);
    return Value(std::round(args[0].asNumber() * factor) / factor);
  });
  registerFunction(functions, "now", [](const std::vector<Value>&) {
    return Value(static_cast<long long>(std::chrono::duration_cast<std::chrono::milliseconds>(
                                            std::chrono::system_clock::now().time_since_epoch())
                                            .count()));
  });
  return functions;
}

}  // namespace expr
}  // namespace wf

#include "wf/json.hpp"

#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <limits>

namespace wf {
namespace json {
namespace {

const Value& nullValue() {
  static const Value kNull;
  return kNull;
}

void encodeUtf8(unsigned int codePoint, std::string& out) {
  if (codePoint <= 0x7F) {
    out.push_back(static_cast<char>(codePoint));
  } else if (codePoint <= 0x7FF) {
    out.push_back(static_cast<char>(0xC0 | (codePoint >> 6)));
    out.push_back(static_cast<char>(0x80 | (codePoint & 0x3F)));
  } else if (codePoint <= 0xFFFF) {
    out.push_back(static_cast<char>(0xE0 | (codePoint >> 12)));
    out.push_back(static_cast<char>(0x80 | ((codePoint >> 6) & 0x3F)));
    out.push_back(static_cast<char>(0x80 | (codePoint & 0x3F)));
  } else {
    out.push_back(static_cast<char>(0xF0 | (codePoint >> 18)));
    out.push_back(static_cast<char>(0x80 | ((codePoint >> 12) & 0x3F)));
    out.push_back(static_cast<char>(0x80 | ((codePoint >> 6) & 0x3F)));
    out.push_back(static_cast<char>(0x80 | (codePoint & 0x3F)));
  }
}

void escapeTo(const std::string& in, std::string& out) {
  out.push_back('"');
  for (unsigned char c : in) {
    switch (c) {
      case '"': out += "\\\""; break;
      case '\\': out += "\\\\"; break;
      case '\n': out += "\\n"; break;
      case '\r': out += "\\r"; break;
      case '\t': out += "\\t"; break;
      case '\b': out += "\\b"; break;
      case '\f': out += "\\f"; break;
      default:
        if (c < 0x20) {
          char buf[8];
          std::snprintf(buf, sizeof(buf), "\\u%04x", c);
          out += buf;
        } else {
          out.push_back(static_cast<char>(c));
        }
    }
  }
  out.push_back('"');
}

void numberTo(const double value, std::string& out) {
  if (std::isnan(value) || std::isinf(value)) {
    out += "null";
    return;
  }
  char buf[40];
  if (value == std::floor(value) && std::fabs(value) < 1e15) {
    std::snprintf(buf, sizeof(buf), "%lld", static_cast<long long>(value));
  } else {
    std::snprintf(buf, sizeof(buf), "%.15g", value);
  }
  out += buf;
}

class Parser {
 public:
  explicit Parser(std::string_view text) : text_(text) {}

  Value parseDocument() {
    skipWhitespace();
    Value value = parseValue(0);
    skipWhitespace();
    if (pos_ != text_.size()) {
      fail("trailing characters");
    }
    return value;
  }

 private:
  [[noreturn]] void fail(const std::string& message) const {
    throw ParseError(message + " at offset " + std::to_string(pos_));
  }

  void skipWhitespace() {
    while (pos_ < text_.size()) {
      const char c = text_[pos_];
      if (c == ' ' || c == '\t' || c == '\n' || c == '\r') {
        ++pos_;
      } else {
        break;
      }
    }
  }

  char peek() const {
    if (pos_ >= text_.size()) {
      fail("unexpected end of input");
    }
    return text_[pos_];
  }

  void expect(char c) {
    if (peek() != c) {
      fail(std::string("expected '") + c + "'");
    }
    ++pos_;
  }

  bool consumeLiteral(const char* literal) {
    const std::size_t len = std::strlen(literal);
    if (text_.size() - pos_ >= len && text_.compare(pos_, len, literal) == 0) {
      pos_ += len;
      return true;
    }
    return false;
  }

  Value parseValue(int depth) {
    if (depth > 64) {
      fail("nesting too deep");
    }
    const char c = peek();
    switch (c) {
      case '{': return parseObject(depth);
      case '[': return parseArray(depth);
      case '"': return Value(parseString());
      case 't':
        if (consumeLiteral("true")) return Value(true);
        fail("invalid literal");
      case 'f':
        if (consumeLiteral("false")) return Value(false);
        fail("invalid literal");
      case 'n':
        if (consumeLiteral("null")) return Value();
        fail("invalid literal");
      default:
        if (c == '-' || (c >= '0' && c <= '9')) {
          return Value(parseNumber());
        }
        fail("unexpected character");
    }
  }

  double parseNumber() {
    const std::size_t start = pos_;
    if (pos_ < text_.size() && text_[pos_] == '-') ++pos_;
    while (pos_ < text_.size()) {
      const char c = text_[pos_];
      if ((c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-') {
        ++pos_;
      } else {
        break;
      }
    }
    if (pos_ == start) {
      fail("invalid number");
    }
    const std::string token(text_.substr(start, pos_ - start));
    char* end = nullptr;
    const double value = std::strtod(token.c_str(), &end);
    if (end == nullptr || *end != '\0') {
      fail("invalid number '" + token + "'");
    }
    return value;
  }

  unsigned int parseHex4() {
    if (pos_ + 4 > text_.size()) {
      fail("truncated \\u escape");
    }
    unsigned int code = 0;
    for (int i = 0; i < 4; ++i) {
      const char c = text_[pos_++];
      code <<= 4;
      if (c >= '0' && c <= '9') {
        code |= static_cast<unsigned int>(c - '0');
      } else if (c >= 'a' && c <= 'f') {
        code |= static_cast<unsigned int>(c - 'a' + 10);
      } else if (c >= 'A' && c <= 'F') {
        code |= static_cast<unsigned int>(c - 'A' + 10);
      } else {
        fail("bad hex digit in \\u escape");
      }
    }
    return code;
  }

  std::string parseString() {
    expect('"');
    std::string out;
    while (true) {
      if (pos_ >= text_.size()) {
        fail("unterminated string");
      }
      const char c = text_[pos_++];
      if (c == '"') {
        break;
      }
      if (c != '\\') {
        out.push_back(c);
        continue;
      }
      if (pos_ >= text_.size()) {
        fail("unterminated escape");
      }
      const char escape = text_[pos_++];
      switch (escape) {
        case '"': out.push_back('"'); break;
        case '\\': out.push_back('\\'); break;
        case '/': out.push_back('/'); break;
        case 'n': out.push_back('\n'); break;
        case 't': out.push_back('\t'); break;
        case 'r': out.push_back('\r'); break;
        case 'b': out.push_back('\b'); break;
        case 'f': out.push_back('\f'); break;
        case 'u': {
          unsigned int code = parseHex4();
          if (code >= 0xD800 && code <= 0xDBFF && pos_ + 1 < text_.size() && text_[pos_] == '\\' &&
              text_[pos_ + 1] == 'u') {
            pos_ += 2;
            const unsigned int low = parseHex4();
            if (low >= 0xDC00 && low <= 0xDFFF) {
              code = 0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00);
            }
          }
          encodeUtf8(code, out);
          break;
        }
        default: fail("invalid escape");
      }
    }
    return out;
  }

  Value parseArray(int depth) {
    expect('[');
    Value array = Value::array();
    skipWhitespace();
    if (peek() == ']') {
      ++pos_;
      return array;
    }
    while (true) {
      skipWhitespace();
      array.push_back(parseValue(depth + 1));
      skipWhitespace();
      const char c = peek();
      if (c == ',') {
        ++pos_;
        continue;
      }
      if (c == ']') {
        ++pos_;
        break;
      }
      fail("expected ',' or ']'");
    }
    return array;
  }

  Value parseObject(int depth) {
    expect('{');
    Value object = Value::object();
    skipWhitespace();
    if (peek() == '}') {
      ++pos_;
      return object;
    }
    while (true) {
      skipWhitespace();
      if (peek() != '"') {
        fail("expected object key");
      }
      const std::string key = parseString();
      skipWhitespace();
      expect(':');
      skipWhitespace();
      object.set(key, parseValue(depth + 1));
      skipWhitespace();
      const char c = peek();
      if (c == ',') {
        ++pos_;
        continue;
      }
      if (c == '}') {
        ++pos_;
        break;
      }
      fail("expected ',' or '}'");
    }
    return object;
  }

  std::string_view text_;
  std::size_t pos_ = 0;
};

std::vector<std::string> splitPath(const std::string& path) {
  std::vector<std::string> parts;
  std::string current;
  for (const char c : path) {
    if (c == '.') {
      if (!current.empty()) {
        parts.push_back(current);
        current.clear();
      }
    } else {
      current.push_back(c);
    }
  }
  if (!current.empty()) {
    parts.push_back(current);
  }
  return parts;
}

}  // namespace

Value Value::array() {
  Value value;
  value.type_ = Type::Array;
  return value;
}

Value Value::array(std::initializer_list<Value> items) {
  Value value = Value::array();
  value.arr_.assign(items.begin(), items.end());
  return value;
}

Value Value::object() {
  Value value;
  value.type_ = Type::Object;
  return value;
}

Value Value::parse(std::string_view text) { return Parser(text).parseDocument(); }

Value Value::fromLiteral(const std::string& text) {
  try {
    return parse(text);
  } catch (const ParseError&) {
    return Value(text);
  }
}

bool Value::asBool() const {
  if (type_ == Type::Bool) return bool_;
  if (type_ == Type::Number) return num_ != 0;
  if (type_ == Type::String) return !str_.empty() && str_ != "false";
  return false;
}

double Value::asNumber() const {
  if (type_ == Type::Number) return num_;
  if (type_ == Type::Bool) return bool_ ? 1 : 0;
  if (type_ == Type::String) {
    try {
      return std::stod(str_);
    } catch (...) {
      return 0;
    }
  }
  return 0;
}

long long Value::asInt() const { return static_cast<long long>(asNumber()); }

const std::string& Value::asString() const {
  static const std::string kEmpty;
  if (type_ != Type::String) return kEmpty;
  return str_;
}

const Array& Value::items() const {
  static const Array kEmpty;
  return type_ == Type::Array ? arr_ : kEmpty;
}

Array& Value::items() {
  if (type_ != Type::Array) {
    type_ = Type::Array;
    arr_.clear();
    obj_.clear();
    str_.clear();
    bool_ = false;
    num_ = 0;
  }
  return arr_;
}

const Object& Value::members() const {
  static const Object kEmpty;
  return type_ == Type::Object ? obj_ : kEmpty;
}

Object& Value::members() {
  if (type_ != Type::Object) {
    type_ = Type::Object;
    obj_.clear();
    arr_.clear();
    str_.clear();
    bool_ = false;
    num_ = 0;
  }
  return obj_;
}

std::size_t Value::size() const {
  if (type_ == Type::Array) return arr_.size();
  if (type_ == Type::Object) return obj_.size();
  if (type_ == Type::String) return str_.size();
  return 0;
}

bool Value::empty() const { return size() == 0; }

bool Value::contains(const std::string& key) const {
  if (type_ != Type::Object) return false;
  for (const auto& member : obj_) {
    if (member.first == key) return true;
  }
  return false;
}

const Value& Value::operator[](const std::string& key) const { return at(key); }

Value& Value::operator[](const std::string& key) {
  members();
  for (auto& member : obj_) {
    if (member.first == key) return member.second;
  }
  obj_.emplace_back(key, Value());
  return obj_.back().second;
}

const Value& Value::at(const std::string& key) const {
  if (type_ == Type::Object) {
    for (const auto& member : obj_) {
      if (member.first == key) return member.second;
    }
  }
  return nullValue();
}

void Value::set(const std::string& key, Value value) { (*this)[key] = std::move(value); }

bool Value::erase(const std::string& key) {
  if (type_ != Type::Object) return false;
  for (auto it = obj_.begin(); it != obj_.end(); ++it) {
    if (it->first == key) {
      obj_.erase(it);
      return true;
    }
  }
  return false;
}

std::vector<std::string> Value::keys() const {
  std::vector<std::string> out;
  if (type_ == Type::Object) {
    out.reserve(obj_.size());
    for (const auto& member : obj_) out.push_back(member.first);
  }
  return out;
}

bool Value::boolOr(const std::string& key, bool fallback) const {
  const Value& v = at(key);
  return v.isNull() ? fallback : v.asBool();
}

long long Value::intOr(const std::string& key, long long fallback) const {
  const Value& v = at(key);
  return v.isNull() ? fallback : v.asInt();
}

double Value::numberOr(const std::string& key, double fallback) const {
  const Value& v = at(key);
  return v.isNull() ? fallback : v.asNumber();
}

std::string Value::stringOr(const std::string& key, const std::string& fallback) const {
  const Value& v = at(key);
  return v.isNull() ? fallback : v.toString();
}

Array Value::arrayOr(const std::string& key) const {
  const Value& v = at(key);
  return v.isArray() ? v.items() : Array();
}

const Value& Value::at(std::size_t index) const {
  if (type_ != Type::Array || index >= arr_.size()) return nullValue();
  return arr_[index];
}

Value& Value::at(std::size_t index) {
  items();
  if (index >= arr_.size()) arr_.resize(index + 1);
  return arr_[index];
}

void Value::push_back(Value value) { items().push_back(std::move(value)); }

const Value* Value::find(const std::string& path) const {
  const Value* current = this;
  for (const auto& part : splitPath(path)) {
    if (current->isObject()) {
      const Value* next = nullptr;
      for (const auto& member : current->obj_) {
        if (member.first == part) {
          next = &member.second;
          break;
        }
      }
      if (next == nullptr) return nullptr;
      current = next;
    } else if (current->isArray()) {
      char* end = nullptr;
      const long index = std::strtol(part.c_str(), &end, 10);
      if (end == nullptr || *end != '\0' || index < 0 ||
          static_cast<std::size_t>(index) >= current->arr_.size()) {
        return nullptr;
      }
      current = &current->arr_[static_cast<std::size_t>(index)];
    } else {
      return nullptr;
    }
  }
  return current;
}

Value* Value::find(const std::string& path) {
  return const_cast<Value*>(static_cast<const Value*>(this)->find(path));
}

void Value::setPath(const std::string& path, Value value) {
  const std::vector<std::string> parts = splitPath(path);
  if (parts.empty()) {
    *this = std::move(value);
    return;
  }
  Value* current = this;
  for (std::size_t i = 0; i + 1 < parts.size(); ++i) {
    Value& next = (*current)[parts[i]];
    if (!next.isObject() && !next.isArray()) {
      next = Value::object();
    }
    current = &next;
  }
  (*current)[parts.back()] = std::move(value);
}

bool Value::isTruthy() const {
  switch (type_) {
    case Type::Null: return false;
    case Type::Bool: return bool_;
    case Type::Number: return num_ != 0;
    case Type::String: return !str_.empty();
    case Type::Array: return !arr_.empty();
    case Type::Object: return !obj_.empty();
  }
  return false;
}

void Value::dumpTo(std::string& out, int indent, int depth) const {
  switch (type_) {
    case Type::Null: out += "null"; return;
    case Type::Bool: out += bool_ ? "true" : "false"; return;
    case Type::Number: numberTo(num_, out); return;
    case Type::String: escapeTo(str_, out); return;
    case Type::Array: {
      if (arr_.empty()) {
        out += "[]";
        return;
      }
      out.push_back('[');
      for (std::size_t i = 0; i < arr_.size(); ++i) {
        if (i > 0) out.push_back(',');
        if (indent >= 0) {
          out.push_back('\n');
          out.append(static_cast<std::size_t>((depth + 1) * indent), ' ');
        }
        arr_[i].dumpTo(out, indent, depth + 1);
      }
      if (indent >= 0) {
        out.push_back('\n');
        out.append(static_cast<std::size_t>(depth * indent), ' ');
      }
      out.push_back(']');
      return;
    }
    case Type::Object: {
      if (obj_.empty()) {
        out += "{}";
        return;
      }
      out.push_back('{');
      for (std::size_t i = 0; i < obj_.size(); ++i) {
        if (i > 0) out.push_back(',');
        if (indent >= 0) {
          out.push_back('\n');
          out.append(static_cast<std::size_t>((depth + 1) * indent), ' ');
        }
        escapeTo(obj_[i].first, out);
        out.push_back(':');
        if (indent >= 0) out.push_back(' ');
        obj_[i].second.dumpTo(out, indent, depth + 1);
      }
      if (indent >= 0) {
        out.push_back('\n');
        out.append(static_cast<std::size_t>(depth * indent), ' ');
      }
      out.push_back('}');
      return;
    }
  }
}

std::string Value::dump(int indent) const {
  std::string out;
  dumpTo(out, indent, 0);
  return out;
}

std::string Value::toString() const {
  if (type_ == Type::String) return str_;
  if (type_ == Type::Null) return "";
  return dump();
}

std::string Value::typeName(Type type) {
  switch (type) {
    case Type::Null: return "null";
    case Type::Bool: return "bool";
    case Type::Number: return "number";
    case Type::String: return "string";
    case Type::Array: return "array";
    case Type::Object: return "object";
  }
  return "unknown";
}

void Value::merge(Value& target, const Value& patch) {
  if (target.isObject() && patch.isObject()) {
    for (const auto& member : patch.members()) {
      Value& slot = target[member.first];
      if (slot.isObject() && member.second.isObject()) {
        merge(slot, member.second);
      } else {
        slot = member.second;
      }
    }
    return;
  }
  target = patch;
}

std::ostream& operator<<(std::ostream& os, const Value& value) { return os << value.dump(); }

bool equals(const Value& a, const Value& b) {
  if (a.isNull() && b.isNull()) return true;
  if (a.isNull() || b.isNull()) return false;
  if (a.isNumber() && b.isNumber()) return a.asNumber() == b.asNumber();
  if (a.isString() && b.isString()) return a.asString() == b.asString();
  if (a.type() != b.type()) {
    // number/string comparison fallback so `${days == '3'}` behaves sanely.
    if ((a.isNumber() && b.isString()) || (a.isString() && b.isNumber())) {
      return a.asNumber() == b.asNumber();
    }
    return false;
  }
  return a.dump() == b.dump();
}

int compare(const Value& a, const Value& b) {
  if (a.isNumber() && b.isNumber()) {
    const double x = a.asNumber();
    const double y = b.asNumber();
    return x < y ? -1 : (x > y ? 1 : 0);
  }
  const std::string x = a.toString();
  const std::string y = b.toString();
  return x < y ? -1 : (x > y ? 1 : 0);
}

}  // namespace json
}  // namespace wf

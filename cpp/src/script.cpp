#include "wf/script.hpp"

#include <cctype>

#include "wf/types.hpp"

namespace wf {
namespace script {
namespace {

using json::Value;

// Trims a leading `variables.` / `form.` style alias so scripts can use both
// `amount` and `variables.amount`.
std::string normalizeTarget(const std::string& target) {
  std::string path = trimCopy(target);
  if (path.rfind("variables.", 0) == 0) return path.substr(10);
  if (path.rfind("vars.", 0) == 0) return path.substr(5);
  if (path == "variables" || path == "vars") return std::string();
  return path;
}

class Runner {
 public:
  Runner(const std::string& source, Value& variables, const expr::Functions& functions)
      : source_(source), variables_(variables), functions_(functions) {}

  ScriptOutcome run() {
    ScriptOutcome outcome;
    while (true) {
      skipSpaceAndComments();
      if (atEnd()) break;
      if (!runStatement(&outcome)) {
        outcome.ok = false;
        return outcome;
      }
      if (!outcome.ok) return outcome;
    }
    return outcome;
  }

 private:
  bool atEnd() const { return pos_ >= source_.size(); }
  char peek() const { return atEnd() ? '\0' : source_[pos_]; }

  void skipSpaceAndComments() {
    while (!atEnd()) {
      const char c = source_[pos_];
      if (std::isspace(static_cast<unsigned char>(c)) != 0) {
        ++pos_;
        continue;
      }
      if (c == '/' && pos_ + 1 < source_.size() && source_[pos_ + 1] == '/') {
        while (!atEnd() && source_[pos_] != '\n') ++pos_;
        continue;
      }
      if (c == '#') {
        while (!atEnd() && source_[pos_] != '\n') ++pos_;
        continue;
      }
      break;
    }
  }

  std::string readUntilDelimiter() {
    const std::size_t start = pos_;
    int depth = 0;
    char quote = '\0';
    while (!atEnd()) {
      const char c = source_[pos_];
      if (quote != '\0') {
        if (c == '\\') {
          pos_ += 2;
          continue;
        }
        if (c == quote) quote = '\0';
        ++pos_;
        continue;
      }
      if (c == '\'' || c == '"') {
        quote = c;
        ++pos_;
        continue;
      }
      if (c == '(' || c == '[') ++depth;
      if (c == ')' || c == ']') --depth;
      if (depth <= 0 && (c == ';' || c == '\n' || c == '}')) break;
      ++pos_;
    }
    return source_.substr(start, pos_ - start);
  }

  void consumeStatementEnd() {
    skipSpaceAndComments();
    if (!atEnd() && source_[pos_] == ';') ++pos_;
  }

  bool expect(char expected, ScriptOutcome* outcome) {
    skipSpaceAndComments();
    if (atEnd() || source_[pos_] != expected) {
      outcome->ok = false;
      outcome->error = std::string("expected '") + expected + "' at offset " + std::to_string(pos_);
      return false;
    }
    ++pos_;
    return true;
  }

  bool runBlock(ScriptOutcome* outcome) {
    if (!expect('{', outcome)) return false;
    while (true) {
      skipSpaceAndComments();
      if (atEnd()) {
        outcome->ok = false;
        outcome->error = "unterminated block";
        return false;
      }
      if (peek() == '}') {
        ++pos_;
        return true;
      }
      if (!runStatement(outcome)) return false;
      if (!outcome->ok) return false;
      if (outcome->hasReturn) return true;  // propagate return out of the block
    }
  }

  bool runStatement(ScriptOutcome* outcome) {
    skipSpaceAndComments();
    outcome->statements += 1;

    if (matchWord("if")) {
      const std::size_t conditionStart = pos_;
      skipSpaceAndComments();
      if (!expect('(', outcome)) return false;
      const std::size_t innerStart = pos_;
      int depth = 1;
      char quote = '\0';
      while (!atEnd() && depth > 0) {
        const char c = source_[pos_];
        if (quote != '\0') {
          if (c == '\\') {
            pos_ += 2;
            continue;
          }
          if (c == quote) quote = '\0';
          ++pos_;
          continue;
        }
        if (c == '\'' || c == '"') {
          quote = c;
        } else if (c == '(') {
          ++depth;
        } else if (c == ')') {
          --depth;
        }
        if (depth == 0) break;
        ++pos_;
      }
      const std::string condition = source_.substr(innerStart, pos_ - innerStart);
      if (!expect(')', outcome)) return false;
      const bool taken = expr::evaluateCondition(condition, root(), functions_, false);
      if (taken) {
        if (!runBlock(outcome)) return false;
      } else {
        // skip the then-branch
        skipSpaceAndComments();
        if (!skipBlock(outcome)) return false;
      }
      skipSpaceAndComments();
      if (matchWord("else")) {
        if (taken) {
          skipSpaceAndComments();
          if (!skipBlock(outcome)) return false;
        } else if (!runBlock(outcome)) {
          return false;
        }
      }
      (void)conditionStart;
      consumeStatementEnd();
      return true;
    }

    if (matchWord("return")) {
      const std::string expression = readUntilDelimiter();
      outcome->returnValue = expr::evaluateValue(expression, root(), functions_);
      outcome->hasReturn = true;
      consumeStatementEnd();
      return true;
    }

    if (matchWord("set")) {
      skipSpaceAndComments();
    }

    const std::size_t start = pos_;
    std::string statement = readUntilDelimiter();
    const std::string trimmed = trimCopy(statement);
    if (trimmed.empty()) {
      consumeStatementEnd();
      return true;
    }

    // assignment?  name = expr
    std::size_t equals = std::string::npos;
    int depth = 0;
    char quote = '\0';
    for (std::size_t i = 0; i < statement.size(); ++i) {
      const char c = statement[i];
      if (quote != '\0') {
        if (c == '\\') {
          ++i;
          continue;
        }
        if (c == quote) quote = '\0';
        continue;
      }
      if (c == '\'' || c == '"') {
        quote = c;
        continue;
      }
      if (c == '(' || c == '[' || c == '{') ++depth;
      if (c == ')' || c == ']' || c == '}') --depth;
      if (depth == 0 && c == '=') {
        const bool comparison = (i + 1 < statement.size() && statement[i + 1] == '=') ||
                                (i > 0 && (statement[i - 1] == '!' || statement[i - 1] == '<' ||
                                           statement[i - 1] == '>'));
        if (!comparison) {
          equals = i;
          break;
        }
      }
    }

    if (equals == std::string::npos) {
      // A bare expression is evaluated for side-effect free checks.
      const expr::EvalResult result =
          expr::Expression::parse(expr::unwrap(trimmed)).evaluate(root(), functions_);
      if (!result.ok) {
        outcome->ok = false;
        outcome->error = result.error + " (语句: " + trimmed + ")";
        return false;
      }
      consumeStatementEnd();
      return true;
    }

    const std::string target = normalizeTarget(statement.substr(0, equals));
    const std::string expression = trimCopy(statement.substr(equals + 1));
    if (target.empty()) {
      outcome->ok = false;
      outcome->error = "赋值语句缺少变量名";
      return false;
    }
    if (!isValidPath(target)) {
      outcome->ok = false;
      outcome->error = "非法的变量名：" + target;
      return false;
    }
    if (expression.empty()) {
      outcome->ok = false;
      outcome->error = "赋值语句缺少表达式：" + target;
      return false;
    }
    Value value = expr::evaluateValue(expression, root(), functions_);
    variables_.setPath(target, value);
    consumeStatementEnd();
    (void)start;
    return true;
  }

  static bool isValidPath(const std::string& path) {
    if (path.empty()) return false;
    for (const char c : path) {
      if (std::isalnum(static_cast<unsigned char>(c)) == 0 && c != '_' && c != '.' && c != '$') {
        return false;
      }
    }
    return std::isdigit(static_cast<unsigned char>(path.front())) == 0;
  }

  bool skipBlock(ScriptOutcome* outcome) {
    skipSpaceAndComments();
    if (atEnd() || peek() != '{') {
      outcome->ok = false;
      outcome->error = "expected block '{'";
      return false;
    }
    int depth = 0;
    char quote = '\0';
    while (!atEnd()) {
      const char c = source_[pos_];
      if (quote != '\0') {
        if (c == '\\') {
          pos_ += 2;
          continue;
        }
        if (c == quote) quote = '\0';
        ++pos_;
        continue;
      }
      if (c == '\'' || c == '"') {
        quote = c;
        ++pos_;
        continue;
      }
      if (c == '{') ++depth;
      if (c == '}') {
        --depth;
        ++pos_;
        if (depth == 0) return true;
        continue;
      }
      ++pos_;
    }
    outcome->ok = false;
    outcome->error = "unterminated block";
    return false;
  }

  bool matchWord(const char* word) {
    skipSpaceAndComments();
    const std::size_t length = std::strlen(word);
    if (source_.compare(pos_, length, word) != 0) return false;
    const char after = pos_ + length < source_.size() ? source_[pos_ + length] : '\0';
    if (std::isalnum(static_cast<unsigned char>(after)) != 0 || after == '_') return false;
    pos_ += length;
    return true;
  }

  Value root() const {
    Value root = variables_;
    if (!root.isObject()) root = Value::object();
    root.set("variables", variables_);
    root.set("form", variables_.contains("form") ? variables_.at("form") : variables_);
    return root;
  }

  const std::string& source_;
  Value& variables_;
  const expr::Functions& functions_;
  std::size_t pos_ = 0;
};

}  // namespace

ScriptOutcome run(const std::string& source, json::Value& variables, const expr::Functions& functions) {
  if (!variables.isObject()) variables = json::Value::object();
  Runner runner(source, variables, functions);
  return runner.run();
}

}  // namespace script
}  // namespace wf

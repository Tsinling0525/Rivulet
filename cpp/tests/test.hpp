// Micro test framework: no external dependency, mirrors the table-driven
// style used elsewhere in the repository.
#pragma once

#include <functional>
#include <iostream>
#include <sstream>
#include <stdexcept>
#include <string>
#include <vector>

namespace wftest {

struct Case {
  std::string name;
  std::function<void()> fn;
};

inline std::vector<Case>& cases() {
  static std::vector<Case> registry;
  return registry;
}

struct Registrar {
  Registrar(const std::string& name, std::function<void()> fn) {
    cases().push_back({name, std::move(fn)});
  }
};

struct AssertionFailure : std::runtime_error {
  explicit AssertionFailure(const std::string& message) : std::runtime_error(message) {}
};

inline int runAll() {
  int failedCases = 0;
  std::size_t total = 0;
  for (const Case& test : cases()) {
    ++total;
    try {
      test.fn();
      std::cout << "  ok   " << test.name << "\n";
    } catch (const std::exception& error) {
      ++failedCases;
      std::cout << "  FAIL " << test.name << ": " << error.what() << "\n";
    }
  }
  std::cout << (failedCases == 0 ? "PASS" : "FAIL") << " " << (total - static_cast<std::size_t>(failedCases))
            << "/" << total << " cases\n";
  return failedCases == 0 ? 0 : 1;
}

inline std::string describe(const std::string& text) { return text; }

}  // namespace wftest

#define WF_TEST(name)                                                  \
  static void name();                                                  \
  static ::wftest::Registrar wftest_registrar_##name(#name, name);      \
  static void name()

#define WF_FAIL(message)                                                          \
  do {                                                                            \
    std::ostringstream wftest_ss;                                                  \
    wftest_ss << __FILE__ << ":" << __LINE__ << " " << (message);                  \
    throw ::wftest::AssertionFailure(wftest_ss.str());                             \
  } while (false)

#define CHECK(condition)                                                          \
  do {                                                                            \
    if (!(condition)) {                                                           \
      std::ostringstream wftest_ss;                                                \
      wftest_ss << __FILE__ << ":" << __LINE__ << " CHECK failed: " #condition;    \
      throw ::wftest::AssertionFailure(wftest_ss.str());                          \
    }                                                                             \
  } while (false)

// Copies (not references) so `CHECK_EQ(make().field(), x)` cannot dangle.
#define CHECK_EQ(actual, expected)                                                \
  do {                                                                            \
    const auto wftest_actual = (actual);                                          \
    const auto wftest_expected = (expected);                                      \
    if (!(wftest_actual == wftest_expected)) {                                    \
      std::ostringstream wftest_ss;                                                \
      wftest_ss << __FILE__ << ":" << __LINE__ << " CHECK_EQ failed: " #actual     \
                << " == " #expected << " (actual=" << wftest_actual               \
                << ", expected=" << wftest_expected << ")";                       \
      throw ::wftest::AssertionFailure(wftest_ss.str());                          \
    }                                                                             \
  } while (false)

#define CHECK_THROWS(expression)                                                  \
  do {                                                                            \
    bool wftest_thrown = false;                                                   \
    try {                                                                         \
      (void)(expression);                                                         \
    } catch (...) {                                                               \
      wftest_thrown = true;                                                       \
    }                                                                             \
    if (!wftest_thrown) {                                                         \
      std::ostringstream wftest_ss;                                                \
      wftest_ss << __FILE__ << ":" << __LINE__ << " expected exception from "       \
                << #expression;                                                   \
      throw ::wftest::AssertionFailure(wftest_ss.str());                          \
    }                                                                             \
  } while (false)

cmake_minimum_required(VERSION 3.16)
project(wfengine LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE RelWithDebInfo)
endif()

set(WF_CORE_SOURCES
  src/json.cpp
  src/expression.cpp
  src/types.cpp
  src/model.cpp
  src/entity.cpp
  src/repository.cpp
  src/services.cpp
  src/script.cpp
  src/engine.cpp
  src/engine_tasks.cpp
  src/engine_scheduler.cpp
  src/api.cpp
)

add_library(wfengine STATIC ${WF_CORE_SOURCES})
target_include_directories(wfengine PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/include)
target_compile_options(wfengine PRIVATE -Wall -Wextra -Wpedantic)

add_executable(wf-cli src/main.cpp)
target_link_libraries(wf-cli PRIVATE wfengine)
target_compile_options(wf-cli PRIVATE -Wall -Wextra -Wpedantic)

enable_testing()
file(GLOB WF_TEST_SOURCES ${CMAKE_CURRENT_SOURCE_DIR}/tests/test_*.cpp)
add_executable(wf-tests tests/main.cpp ${WF_TEST_SOURCES})
target_link_libraries(wf-tests PRIVATE wfengine)
target_compile_options(wf-tests PRIVATE -Wall -Wextra -Wpedantic)
add_test(NAME wf-tests COMMAND wf-tests)

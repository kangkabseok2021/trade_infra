cmake_minimum_required(VERSION 3.20)
project(risk_svc CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

include(FetchContent)
FetchContent_Declare(googletest
    GIT_REPOSITORY https://github.com/google/googletest.git
    GIT_TAG        v1.14.0)
set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(googletest)

add_library(riskcalc SHARED src/risk_calc.cpp)
target_include_directories(riskcalc PUBLIC include PRIVATE src)
if(APPLE)
    target_link_options(riskcalc PRIVATE -undefined dynamic_lookup)
endif()

enable_testing()
add_executable(test_riskcalc tests/test_risk_calc.cpp)
target_link_libraries(test_riskcalc riskcalc GTest::gtest_main)
include(GoogleTest)
gtest_discover_tests(test_riskcalc)

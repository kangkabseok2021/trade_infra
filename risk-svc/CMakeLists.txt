cmake_minimum_required(VERSION 3.20)
project(risk_svc CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

include(FetchContent)
FetchContent_Declare(corrosion
    GIT_REPOSITORY https://github.com/corrosion-rs/corrosion.git
    GIT_TAG        v0.5.0)
FetchContent_MakeAvailable(corrosion)
corrosion_import_crate(MANIFEST_PATH risk-calc-rs/Cargo.toml)

option(BUILD_TESTS "Build test suite" ON)
if(BUILD_TESTS)
    FetchContent_Declare(googletest
        GIT_REPOSITORY https://github.com/google/googletest.git
        GIT_TAG        v1.14.0)
    set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
    FetchContent_MakeAvailable(googletest)

    enable_testing()
    add_executable(test_riskcalc tests/test_risk_calc.cpp)
    target_include_directories(test_riskcalc PRIVATE ${CMAKE_SOURCE_DIR}/include)
    target_link_libraries(test_riskcalc riskcalc GTest::gtest_main)
    include(GoogleTest)
    gtest_discover_tests(test_riskcalc)
endif()

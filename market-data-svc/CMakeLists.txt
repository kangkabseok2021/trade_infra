cmake_minimum_required(VERSION 3.20)
project(market_data_svc CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

include(FetchContent)

# Corrosion: build Rust tick-engine cdylib as CMake target "marketdata"
FetchContent_Declare(corrosion
    GIT_REPOSITORY https://github.com/corrosion-rs/corrosion.git
    GIT_TAG        v0.5.0)
FetchContent_MakeAvailable(corrosion)
corrosion_import_crate(MANIFEST_PATH tick-engine/Cargo.toml)

FetchContent_Declare(httplib
    GIT_REPOSITORY https://github.com/yhirose/cpp-httplib.git
    GIT_TAG        v0.15.3)
FetchContent_MakeAvailable(httplib)

find_package(PostgreSQL REQUIRED)

# Main service binary — unchanged
add_executable(market_data_svc
    src/main.cpp src/db_writer.cpp src/metrics_server.cpp)
target_include_directories(market_data_svc PRIVATE include src)
target_link_libraries(market_data_svc marketdata httplib::httplib PostgreSQL::PostgreSQL)

option(BUILD_TESTS "Build test suite" ON)
if(BUILD_TESTS)
    FetchContent_Declare(googletest
        GIT_REPOSITORY https://github.com/google/googletest.git
        GIT_TAG        v1.14.0)
    set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
    FetchContent_MakeAvailable(googletest)

    enable_testing()
    add_executable(test_marketdata tests/test_tick_generator.cpp)
    target_include_directories(test_marketdata PRIVATE include)
    target_link_libraries(test_marketdata marketdata GTest::gtest_main)
    include(GoogleTest)
    gtest_discover_tests(test_marketdata)
endif()

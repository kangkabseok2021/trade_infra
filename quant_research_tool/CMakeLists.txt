cmake_minimum_required(VERSION 3.15)
project(quant_research_tool)

include(FetchContent)
FetchContent_Declare(
  corrosion
  GIT_REPOSITORY https://github.com/corrosion-rs/corrosion.git
  GIT_TAG v0.5
)
FetchContent_MakeAvailable(corrosion)

find_package(Python COMPONENTS Interpreter Development.Module REQUIRED)
corrosion_import_crate(MANIFEST_PATH Cargo.toml)
corrosion_set_env_vars(quant_engine
  "PYO3_PYTHON=${Python_EXECUTABLE}"
  "PYTHON_SYS_EXECUTABLE=${Python_EXECUTABLE}"
)

# Install the Rust-produced dynamic library into the backtester/ Python package
# with a name Python's import machinery accepts (no "lib" prefix, .so suffix).
install(
  FILES "$<TARGET_FILE:quant_engine-shared>"
  DESTINATION backtester
  RENAME "quant_engine.${Python_SOABI}.so"
)

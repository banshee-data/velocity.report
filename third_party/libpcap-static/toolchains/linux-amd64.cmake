set(CMAKE_SYSTEM_NAME Linux)
set(CMAKE_SYSTEM_PROCESSOR x86_64)

set(_TC_DIR "${CMAKE_CURRENT_LIST_DIR}/wrappers/amd64")
set(CMAKE_C_COMPILER   "${_TC_DIR}/zig-cc")
set(CMAKE_AR           "${_TC_DIR}/zig-ar"     CACHE FILEPATH "" FORCE)
set(CMAKE_RANLIB       "${_TC_DIR}/zig-ranlib" CACHE FILEPATH "" FORCE)

# We're building a static archive — skip CMake's compiler-link smoke test
# (it tries to produce an executable, which pulls in libc bits we don't need).
set(CMAKE_C_COMPILER_WORKS 1)

# Don't search the host filesystem for libraries / headers when cross-compiling.
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_PACKAGE ONLY)

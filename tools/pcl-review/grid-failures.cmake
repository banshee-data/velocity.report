# Hand-authored failure fixtures; generated files stay in the external build.
set(dir "${CMAKE_CURRENT_BINARY_DIR}/grid-failure-fixtures")
file(MAKE_DIRECTORY "${dir}")
set(header "WIDTH 8\nHEIGHT 40\nDATA ascii\n")
string(REPEAT "1 0 0 1\n" 320 points)
file(WRITE "${dir}/finite.pcd" "${header}${points}")
string(REPEAT "nan nan nan nan\n" 320 missing)
file(WRITE "${dir}/missing.pcd" "${header}${missing}")
file(WRITE "${dir}/malformed.pcd" "${header}1 0\n")
file(WRITE "${dir}/binary.pcd" "WIDTH 8\nHEIGHT 40\nDATA binary\n")

function(expect_failure input prefix diagnostic)
  execute_process(COMMAND "${CHECKER}" "${dir}/${input}.pcd" "${prefix}"
    RESULT_VARIABLE status OUTPUT_VARIABLE summary ERROR_VARIABLE error)
  if(NOT "${status}" STREQUAL "1" OR NOT error MATCHES "${diagnostic}")
    message(FATAL_ERROR "${input}: status=${status}; stdout=${summary}; stderr=${error}")
  endif()
endfunction()

# A directory cannot be opened as a CSV file, regardless of user permissions.
file(MAKE_DIRECTORY "${dir}/blocked-normals.csv")
expect_failure(finite "${dir}/blocked" "cannot open CSV")
expect_failure(missing "${dir}/empty" "no finite retained points")
expect_failure(malformed "${dir}/bad" "malformed PCD data row")
expect_failure(binary "${dir}/binary" "expected DATA ascii")

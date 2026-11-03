cmake_minimum_required(VERSION 3.16)
project(pcl_ground_oracle LANGUAGES C CXX)
set(CMAKE_CXX_STANDARD 17)
if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()
# ground0006's oracle links an installed PCL (find_package), unlike the review harnesses beside
# it, which build against the reviewed 1.15.1 source tree. The PCL version is printed in each
# run's summary.txt; record it in the report.
# Debian's PCL config loads VTK, whose VTK::mpi target needs MPI::MPI_C to exist first.
find_package(MPI QUIET COMPONENTS C)
find_package(PCL 1.13 REQUIRED COMPONENTS common io features search kdtree sample_consensus segmentation)
find_package(OpenMP)
add_executable(pcl-ground-oracle ground-oracle.cpp)
target_include_directories(pcl-ground-oracle PRIVATE ${PCL_INCLUDE_DIRS})
target_compile_definitions(pcl-ground-oracle PRIVATE ${PCL_DEFINITIONS})
target_link_libraries(pcl-ground-oracle PRIVATE ${PCL_LIBRARIES})
if(OpenMP_CXX_FOUND)
  target_link_libraries(pcl-ground-oracle PRIVATE OpenMP::OpenMP_CXX)
endif()

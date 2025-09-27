# Compilation Steps for LidarView

This document outlines the steps required to compile LidarView based on the shell command history.

## Prerequisites

Ensure the following dependencies are installed on your system:

- [Homebrew](https://brew.sh/) (for macOS)
- CMake
- Ninja
- Qt5
- VTK
- ParaView
- libpcap
- yaml-cpp
- libpng
- libffi
- libtins

You can install these dependencies using Homebrew:

```bash
brew install cmake qt@5 vtk paraview libpcap yaml-cpp libpng libffi libtins
```

## Steps to Compile LidarView

1. Clone the LidarView Superbuild repository:

   ```bash
   git clone --recursive https://gitlab.kitware.com/LidarView/lidarview-superbuild.git
   ```

2. Navigate to the cloned repository:

   ```bash
   cd lidarview-superbuild
   ```

3. Create a build directory and navigate into it:

   ```bash
   mkdir build && cd build
   ```

4. Configure the build using CMake:

   ```bash
   cmake .. -GNinja -DCMAKE_BUILD_TYPE=Release \
     -DQt5_DIR=$(brew --prefix qt@5)/lib/cmake/Qt5 \
     -DUSE_SYSTEM_pcap=ON \
     -DUSE_SYSTEM_yaml=OFF \
     -DUSE_SYSTEM_png=ON \
     -DUSE_SYSTEM_ffi=ON \
     -DUSE_SYSTEM_tins=ON \
     -DUSE_SYSTEM_python3=ON \
    -DCMAKE_POLICY_VERSION_MINIMUM=3.5
   ```

5. Build the project using Ninja:

   ```bash
   ninja -j$(sysctl -n hw.ncpu)
   ```

6. If you encounter issues with specific dependencies, you may need to disable or adjust them. For example:

   ```bash
   cmake .. -GNinja -DCMAKE_BUILD_TYPE=Release \
     -DQt5_DIR=$(brew --prefix qt@5)/lib/cmake/Qt5 \
     -DENABLE_yaml=OFF \
     -DENABLE_tins=OFF
   ```

7. Rebuild the project:

   ```bash
   ninja
   ```

8. If you encounter errors related to `cmake_minimum_required` versions, you may need to patch the `CMakeLists.txt` files. For example:

   ```bash
   sed -i.bak 's/cmake_minimum_required(VERSION 2.8.3)/cmake_minimum_required(VERSION 3.5)/' path/to/CMakeLists.txt
   ```

9. Once the build completes successfully, the compiled binaries will be available in the `build` directory.

## Additional Notes

- Use `brew --prefix <package>` to locate the installation paths of dependencies.
- If you encounter issues, refer to the `README.md` or other documentation in the repository for troubleshooting tips.
- For verbose build output, use:

  ```bash
  ninja -j$(sysctl -n hw.ncpu) --verbose
  ```

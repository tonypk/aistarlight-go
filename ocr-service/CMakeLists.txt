cmake_minimum_required(VERSION 3.14)
project(ocr-service VERSION 2.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# ---------- Dependencies ----------

# OpenCV (system)
find_package(OpenCV REQUIRED COMPONENTS core imgproc imgcodecs)

# ONNX Runtime (installed to /usr/local or ONNXRUNTIME_DIR)
if(DEFINED ENV{ONNXRUNTIME_DIR})
    set(ONNXRUNTIME_DIR $ENV{ONNXRUNTIME_DIR})
else()
    set(ONNXRUNTIME_DIR "/usr/local")
endif()
find_library(ONNXRUNTIME_LIB onnxruntime HINTS ${ONNXRUNTIME_DIR}/lib)
if(NOT ONNXRUNTIME_LIB)
    message(FATAL_ERROR "ONNX Runtime library not found in ${ONNXRUNTIME_DIR}/lib")
endif()

# Clipper (polyclipping, system)
find_library(CLIPPER_LIB polyclipping HINTS /usr/lib/x86_64-linux-gnu /usr/lib)
if(NOT CLIPPER_LIB)
    message(FATAL_ERROR "libpolyclipping not found. Install: apt-get install libpolyclipping-dev")
endif()

# cpp-httplib (FetchContent)
include(FetchContent)
FetchContent_Declare(
    httplib
    GIT_REPOSITORY https://github.com/yhirose/cpp-httplib.git
    GIT_TAG v0.18.3
)
FetchContent_MakeAvailable(httplib)

# nlohmann/json (FetchContent)
FetchContent_Declare(
    json
    GIT_REPOSITORY https://github.com/nlohmann/json.git
    GIT_TAG v3.11.3
)
FetchContent_MakeAvailable(json)

# ---------- Build ----------

add_executable(ocr-service
    src/main.cpp
    src/OcrUtils.cpp
    src/DbNet.cpp
    src/AngleNet.cpp
    src/CrnnNet.cpp
)

target_include_directories(ocr-service PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/include
    ${ONNXRUNTIME_DIR}/include
    /usr/include/polyclipping
)

target_link_libraries(ocr-service PRIVATE
    ${OpenCV_LIBS}
    ${ONNXRUNTIME_LIB}
    ${CLIPPER_LIB}
    httplib::httplib
    nlohmann_json::nlohmann_json
    pthread
)

install(TARGETS ocr-service DESTINATION bin)

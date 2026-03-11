# Multi-stage build for C++ OCR service
# Stage 1: Build
FROM ubuntu:22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive
ENV ONNXRUNTIME_VERSION=1.17.0

# Install build tools and dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    cmake g++ make git wget ca-certificates \
    libopencv-dev \
    libpolyclipping-dev \
    python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Download and install ONNX Runtime
RUN wget -q "https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}.tgz" \
    -O /tmp/ort.tgz && \
    tar xzf /tmp/ort.tgz -C /tmp && \
    cp -r /tmp/onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}/lib/* /usr/local/lib/ && \
    cp -r /tmp/onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}/include/* /usr/local/include/ && \
    ldconfig && \
    rm -rf /tmp/ort.tgz /tmp/onnxruntime-*

# Download ONNX models from rapidocr-onnxruntime Python package
RUN pip3 install --no-cache-dir rapidocr_onnxruntime==1.4.4 && \
    MODELS_DIR=$(python3 -c "import rapidocr_onnxruntime as r; import os; print(os.path.dirname(r.__file__))") && \
    mkdir -p /models && \
    cp ${MODELS_DIR}/models/ch_PP-OCRv4_det_infer.onnx /models/det.onnx && \
    cp ${MODELS_DIR}/models/ch_ppocr_mobile_v2.0_cls_infer.onnx /models/cls.onnx && \
    cp ${MODELS_DIR}/models/ch_PP-OCRv4_rec_infer.onnx /models/rec.onnx && \
    cp ${MODELS_DIR}/models/ppocr_keys_v1.txt /models/keys.txt && \
    pip3 uninstall -y rapidocr_onnxruntime && \
    ls -la /models/

# Build OCR service
WORKDIR /build
COPY CMakeLists.txt .
COPY include/ include/
COPY src/ src/
RUN cmake -B build -DCMAKE_BUILD_TYPE=Release \
    -DONNXRUNTIME_DIR=/usr/local && \
    cmake --build build --parallel $(nproc)

# Stage 2: Runtime
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    libopencv-core4.5d \
    libopencv-imgproc4.5d \
    libopencv-imgcodecs4.5d \
    libpolyclipping22 \
    libgomp1 \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Copy ONNX Runtime shared libraries
COPY --from=builder /usr/local/lib/libonnxruntime* /usr/local/lib/
RUN ldconfig

# Copy binary and models
COPY --from=builder /build/build/ocr-service /usr/local/bin/ocr-service
COPY --from=builder /models /models

ENV OCR_MODEL_DIR=/models
ENV OCR_PORT=8001
EXPOSE 8001

HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=10s \
    CMD wget -q -O /dev/null http://localhost:8001/health || exit 1

CMD ["ocr-service"]

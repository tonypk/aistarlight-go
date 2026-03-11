/**
 * OCR Microservice — C++ HTTP server wrapping RapidOCR (ONNX Runtime).
 * Provides /health and /ocr endpoints compatible with the AIStarlight Go backend.
 */

#include <cstdio>
#include <cstdlib>
#include <string>
#include <vector>
#include <chrono>
#include <fstream>
#include <sstream>
#include <mutex>

#include <opencv2/opencv.hpp>
#include "httplib.h"
#include "nlohmann/json.hpp"

#include "OcrStruct.h"
#include "OcrUtils.h"
#include "DbNet.h"
#include "AngleNet.h"
#include "CrnnNet.h"

using json = nlohmann::json;

// ---------- Configuration ----------

static const int MAX_SIDE_LEN   = 1600;
static const int PADDING        = 0;
static const float BOX_SCORE_TH = 0.5f;
static const float BOX_THRESH   = 0.3f;
static const float UNCLIP_RATIO = 1.6f;
static const bool  DO_ANGLE     = true;
static const bool  MOST_ANGLE   = true;
static const int   NUM_THREADS  = 2;
static const size_t MAX_FILE_SIZE = 20 * 1024 * 1024; // 20 MB
static const int   MAX_IMAGE_DIM = 1600;

// ---------- Global OCR Engine ----------

static DbNet      g_dbNet;
static AngleNet   g_angleNet;
static CrnnNet    g_crnnNet;
static std::mutex g_ocrMutex; // Serialize OCR calls (models not thread-safe)

static bool initOcrEngine(const std::string &modelDir) {
    std::string detPath = modelDir + "/det.onnx";
    std::string clsPath = modelDir + "/cls.onnx";
    std::string recPath = modelDir + "/rec.onnx";
    std::string keysPath = modelDir + "/keys.txt";

    printf("Initializing OCR engine (ONNX Runtime, C++)...\n");
    printf("  det: %s\n", detPath.c_str());
    printf("  cls: %s\n", clsPath.c_str());
    printf("  rec: %s\n", recPath.c_str());
    printf("  keys: %s\n", keysPath.c_str());

    g_dbNet.setNumThread(NUM_THREADS);
    g_angleNet.setNumThread(NUM_THREADS);
    g_crnnNet.setNumThread(NUM_THREADS);

    try {
        g_dbNet.initModel(detPath);
        g_angleNet.initModel(clsPath);
        g_crnnNet.initModel(recPath, keysPath);
    } catch (const Ort::Exception &e) {
        fprintf(stderr, "ONNX Runtime error: %s\n", e.what());
        return false;
    } catch (const std::exception &e) {
        fprintf(stderr, "Init error: %s\n", e.what());
        return false;
    }

    printf("OCR engine ready.\n");
    return true;
}

// ---------- OCR Pipeline ----------

static cv::Mat resizeIfNeeded(const cv::Mat &src) {
    int maxDim = std::max(src.cols, src.rows);
    if (maxDim <= MAX_IMAGE_DIM) return src;

    float ratio = (float)MAX_IMAGE_DIM / (float)maxDim;
    int newW = int(src.cols * ratio);
    int newH = int(src.rows * ratio);
    printf("Resizing image from %dx%d to %dx%d\n", src.cols, src.rows, newW, newH);

    cv::Mat resized;
    cv::resize(src, resized, cv::Size(newW, newH), 0, 0, cv::INTER_AREA);
    return resized;
}

static OcrResult runOcr(const cv::Mat &originSrc) {
    cv::Mat src = resizeIfNeeded(originSrc);

    int originMaxSide = std::max(src.cols, src.rows);
    int resize = (MAX_SIDE_LEN <= 0 || MAX_SIDE_LEN > originMaxSide) ? originMaxSide : MAX_SIDE_LEN;
    resize += 2 * PADDING;

    cv::Mat paddingSrc = src.clone();
    if (PADDING > 0) {
        cv::copyMakeBorder(src, paddingSrc, PADDING, PADDING, PADDING, PADDING,
                           cv::BORDER_ISOLATED, cv::Scalar(255, 255, 255));
    }

    ScaleParam scale = getScaleParam(paddingSrc, resize);

    double startTime = getCurrentTime();

    // Step 1: Text detection
    std::vector<TextBox> textBoxes = g_dbNet.getTextBoxes(paddingSrc, scale,
                                                           BOX_SCORE_TH, BOX_THRESH, UNCLIP_RATIO);
    double dbNetTime = getCurrentTime() - startTime;

    // Step 2: Crop text regions
    std::vector<cv::Mat> partImages;
    for (auto &textBox : textBoxes) {
        cv::Mat partImg = getRotateCropImage(paddingSrc, textBox.boxPoint);
        partImages.emplace_back(partImg);
    }

    // Step 3: Angle classification
    std::vector<Angle> angles = g_angleNet.getAngles(partImages, DO_ANGLE, MOST_ANGLE);

    // Step 4: Rotate 180° images
    for (size_t i = 0; i < partImages.size(); ++i) {
        if (angles[i].index == 1) {
            partImages[i] = matRotateClockWise180(partImages[i]);
        }
    }

    // Step 5: Text recognition
    std::vector<TextLine> textLines = g_crnnNet.getTextLines(partImages);

    // Build result
    std::vector<TextBlock> textBlocks;
    for (size_t i = 0; i < textLines.size(); ++i) {
        std::vector<cv::Point> boxPoint(4);
        for (int j = 0; j < 4; j++) {
            boxPoint[j] = cv::Point(
                textBoxes[i].boxPoint[j].x - PADDING,
                textBoxes[i].boxPoint[j].y - PADDING
            );
        }
        TextBlock tb{boxPoint, textBoxes[i].score,
                     angles[i].index, angles[i].score, angles[i].time,
                     textLines[i].text, textLines[i].charScores, textLines[i].time,
                     angles[i].time + textLines[i].time};
        textBlocks.emplace_back(tb);
    }

    double fullTime = getCurrentTime() - startTime;

    std::string strRes;
    for (auto &block : textBlocks) {
        strRes += block.text + "\n";
    }

    return OcrResult{dbNetTime, textBlocks, fullTime, strRes};
}

// ---------- HTTP Handlers ----------

static void handleHealth(const httplib::Request &, httplib::Response &res) {
    json j;
    j["status"] = "ok";
    j["engine"] = "RapidOCR-ONNX-CPP";
    res.set_content(j.dump(), "application/json");
}

static void handleOcr(const httplib::Request &req, httplib::Response &res) {
    if (!req.has_file("file")) {
        json err;
        err["detail"] = "No file uploaded";
        res.status = 400;
        res.set_content(err.dump(), "application/json");
        return;
    }

    auto file = req.get_file_value("file");
    if (file.content.size() > MAX_FILE_SIZE) {
        json err;
        err["detail"] = "File too large (max 20MB)";
        res.status = 400;
        res.set_content(err.dump(), "application/json");
        return;
    }

    // Decode image from memory
    std::vector<uchar> buf(file.content.begin(), file.content.end());
    cv::Mat img = cv::imdecode(buf, cv::IMREAD_COLOR);
    if (img.empty()) {
        json err;
        err["detail"] = "Could not decode image";
        res.status = 400;
        res.set_content(err.dump(), "application/json");
        return;
    }

    OcrResult result;
    try {
        std::lock_guard<std::mutex> lock(g_ocrMutex);
        result = runOcr(img);
    } catch (const std::exception &e) {
        fprintf(stderr, "OCR failed: %s\n", e.what());
        json err;
        err["detail"] = std::string("OCR processing failed: ") + e.what();
        res.status = 500;
        res.set_content(err.dump(), "application/json");
        return;
    }

    // Build JSON response (compatible with Python OCR service format)
    json lines = json::array();
    std::vector<std::string> textParts;

    for (auto &block : result.textBlocks) {
        // Compute average character score as confidence
        float confidence = 0.0f;
        if (!block.charScores.empty()) {
            float sum = 0.0f;
            for (float s : block.charScores) sum += s;
            confidence = sum / block.charScores.size();
        }

        // Convert box points to nested array [[x1,y1],[x2,y2],[x3,y3],[x4,y4]]
        json bbox = json::array();
        for (auto &pt : block.boxPoint) {
            bbox.push_back({pt.x, pt.y});
        }

        json lineObj;
        lineObj["text"] = block.text;
        lineObj["confidence"] = std::round(confidence * 10000.0f) / 10000.0f;
        lineObj["bbox"] = bbox;
        lines.push_back(lineObj);

        textParts.push_back(block.text);
    }

    std::string fullText;
    for (size_t i = 0; i < textParts.size(); i++) {
        if (i > 0) fullText += "\n";
        fullText += textParts[i];
    }

    json j;
    j["text"] = fullText;
    j["lines"] = lines;
    j["line_count"] = (int)lines.size();

    printf("OCR processed: %d lines in %.1fms\n", (int)lines.size(), result.detectTime);
    res.set_content(j.dump(), "application/json");
}

// ---------- Main ----------

int main(int argc, char *argv[]) {
    const char *modelDir = getenv("OCR_MODEL_DIR");
    if (!modelDir) modelDir = "/models";

    const char *portStr = getenv("OCR_PORT");
    int port = portStr ? atoi(portStr) : 8001;

    if (!initOcrEngine(modelDir)) {
        fprintf(stderr, "Failed to initialize OCR engine\n");
        return 1;
    }

    httplib::Server svr;
    svr.Get("/health", handleHealth);
    svr.Post("/ocr", handleOcr);

    // Set payload max (25MB to allow overhead)
    svr.set_payload_max_length(25 * 1024 * 1024);

    printf("OCR service listening on port %d\n", port);
    if (!svr.listen("0.0.0.0", port)) {
        fprintf(stderr, "Failed to start server on port %d\n", port);
        return 1;
    }

    return 0;
}

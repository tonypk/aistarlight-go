#include "OcrUtils.h"
#include <opencv2/imgproc.hpp>
#include <opencv2/imgcodecs.hpp>
#include <numeric>
#include <polyclipping/clipper.hpp>

double getCurrentTime() {
    return (static_cast<double>(cv::getTickCount())) / cv::getTickFrequency() * 1000;
}

ScaleParam getScaleParam(cv::Mat &src, const float scale) {
    int srcWidth = src.cols;
    int srcHeight = src.rows;
    int dstWidth = int((float)srcWidth * scale);
    int dstHeight = int((float)srcHeight * scale);
    if (dstWidth % 32 != 0) {
        dstWidth = (dstWidth / 32 - 1) * 32;
        dstWidth = (std::max)(dstWidth, 32);
    }
    if (dstHeight % 32 != 0) {
        dstHeight = (dstHeight / 32 - 1) * 32;
        dstHeight = (std::max)(dstHeight, 32);
    }
    float scaleWidth = (float)dstWidth / (float)srcWidth;
    float scaleHeight = (float)dstHeight / (float)srcHeight;
    return {srcWidth, srcHeight, dstWidth, dstHeight, scaleWidth, scaleHeight};
}

ScaleParam getScaleParam(cv::Mat &src, const int targetSize) {
    int srcWidth = src.cols;
    int srcHeight = src.rows;
    int dstWidth = srcWidth;
    int dstHeight = srcHeight;

    float ratio = 1.f;
    if (srcWidth > srcHeight) {
        ratio = float(targetSize) / float(srcWidth);
    } else {
        ratio = float(targetSize) / float(srcHeight);
    }
    dstWidth = int(float(srcWidth) * ratio);
    dstHeight = int(float(srcHeight) * ratio);
    if (dstWidth % 32 != 0) {
        dstWidth = (dstWidth / 32) * 32;
        dstWidth = (std::max)(dstWidth, 32);
    }
    if (dstHeight % 32 != 0) {
        dstHeight = (dstHeight / 32) * 32;
        dstHeight = (std::max)(dstHeight, 32);
    }
    float ratioWidth = (float)dstWidth / (float)srcWidth;
    float ratioHeight = (float)dstHeight / (float)srcHeight;
    return {srcWidth, srcHeight, dstWidth, dstHeight, ratioWidth, ratioHeight};
}

std::vector<cv::Point2f> getBox(const cv::RotatedRect &rect) {
    cv::Point2f vertices[4];
    rect.points(vertices);
    return std::vector<cv::Point2f>(vertices, vertices + 4);
}

std::vector<cv::Point2f> getMinBoxes(const cv::RotatedRect &boxRect, float &maxSideLen) {
    maxSideLen = std::max(boxRect.size.width, boxRect.size.height);
    std::vector<cv::Point2f> boxPoint = getBox(boxRect);
    std::sort(boxPoint.begin(), boxPoint.end(),
              [](const cv::Point2f &a, const cv::Point2f &b) { return a.x < b.x; });

    int index1, index2, index3, index4;
    if (boxPoint[1].y > boxPoint[0].y) { index1 = 0; index4 = 1; }
    else { index1 = 1; index4 = 0; }
    if (boxPoint[3].y > boxPoint[2].y) { index2 = 2; index3 = 3; }
    else { index2 = 3; index3 = 2; }

    std::vector<cv::Point2f> minBox(4);
    minBox[0] = boxPoint[index1];
    minBox[1] = boxPoint[index2];
    minBox[2] = boxPoint[index3];
    minBox[3] = boxPoint[index4];
    return minBox;
}

float boxScoreFast(const std::vector<cv::Point2f> &boxes, const cv::Mat &pred) {
    int width = pred.cols;
    int height = pred.rows;

    float arrayX[4] = {boxes[0].x, boxes[1].x, boxes[2].x, boxes[3].x};
    float arrayY[4] = {boxes[0].y, boxes[1].y, boxes[2].y, boxes[3].y};

    int minX = clamp(int(std::floor(*std::min_element(arrayX, arrayX + 4))), 0, width - 1);
    int maxX = clamp(int(std::ceil(*std::max_element(arrayX, arrayX + 4))), 0, width - 1);
    int minY = clamp(int(std::floor(*std::min_element(arrayY, arrayY + 4))), 0, height - 1);
    int maxY = clamp(int(std::ceil(*std::max_element(arrayY, arrayY + 4))), 0, height - 1);

    cv::Mat mask = cv::Mat::zeros(maxY - minY + 1, maxX - minX + 1, CV_8UC1);
    cv::Point box[4];
    box[0] = cv::Point(int(boxes[0].x) - minX, int(boxes[0].y) - minY);
    box[1] = cv::Point(int(boxes[1].x) - minX, int(boxes[1].y) - minY);
    box[2] = cv::Point(int(boxes[2].x) - minX, int(boxes[2].y) - minY);
    box[3] = cv::Point(int(boxes[3].x) - minX, int(boxes[3].y) - minY);
    const cv::Point *pts[1] = {box};
    int npts[] = {4};
    cv::fillPoly(mask, pts, npts, 1, cv::Scalar(1));

    cv::Mat croppedImg;
    pred(cv::Rect(minX, minY, maxX - minX + 1, maxY - minY + 1)).copyTo(croppedImg);
    return (float)cv::mean(croppedImg, mask)[0];
}

static float getContourArea(const std::vector<cv::Point2f> &box, float unClipRatio) {
    size_t size = box.size();
    float area = 0.0f;
    float dist = 0.0f;
    for (size_t i = 0; i < size; i++) {
        area += box[i].x * box[(i + 1) % size].y - box[i].y * box[(i + 1) % size].x;
        dist += sqrtf((box[i].x - box[(i + 1) % size].x) * (box[i].x - box[(i + 1) % size].x) +
                       (box[i].y - box[(i + 1) % size].y) * (box[i].y - box[(i + 1) % size].y));
    }
    area = fabs(area / 2.0f);
    return area * unClipRatio / dist;
}

cv::RotatedRect unClip(std::vector<cv::Point2f> box, float unClipRatio) {
    float distance = getContourArea(box, unClipRatio);

    ClipperLib::ClipperOffset offset;
    ClipperLib::Path p;
    p << ClipperLib::IntPoint(int(box[0].x), int(box[0].y))
      << ClipperLib::IntPoint(int(box[1].x), int(box[1].y))
      << ClipperLib::IntPoint(int(box[2].x), int(box[2].y))
      << ClipperLib::IntPoint(int(box[3].x), int(box[3].y));
    offset.AddPath(p, ClipperLib::jtRound, ClipperLib::etClosedPolygon);

    ClipperLib::Paths soln;
    offset.Execute(soln, distance);
    std::vector<cv::Point2f> points;
    for (size_t j = 0; j < soln.size(); j++) {
        for (size_t i = 0; i < soln[j].size(); i++) {
            points.emplace_back(soln[j][i].X, soln[j][i].Y);
        }
    }

    if (points.empty()) {
        return cv::RotatedRect(cv::Point2f(0, 0), cv::Size2f(1, 1), 0);
    }
    return cv::minAreaRect(points);
}

cv::Mat matRotateClockWise180(cv::Mat src) {
    flip(src, src, 0);
    flip(src, src, 1);
    return src;
}

cv::Mat getRotateCropImage(const cv::Mat &src, std::vector<cv::Point> box) {
    cv::Mat image;
    src.copyTo(image);
    std::vector<cv::Point> points = box;

    int collectX[4] = {box[0].x, box[1].x, box[2].x, box[3].x};
    int collectY[4] = {box[0].y, box[1].y, box[2].y, box[3].y};
    int left = *std::min_element(collectX, collectX + 4);
    int right = *std::max_element(collectX, collectX + 4);
    int top = *std::min_element(collectY, collectY + 4);
    int bottom = *std::max_element(collectY, collectY + 4);

    cv::Mat imgCrop;
    image(cv::Rect(left, top, right - left, bottom - top)).copyTo(imgCrop);

    for (auto &point : points) {
        point.x -= left;
        point.y -= top;
    }

    int imgCropWidth = int(sqrt(pow(points[0].x - points[1].x, 2) + pow(points[0].y - points[1].y, 2)));
    int imgCropHeight = int(sqrt(pow(points[0].x - points[3].x, 2) + pow(points[0].y - points[3].y, 2)));

    cv::Point2f ptsDst[4] = {
        cv::Point2f(0.f, 0.f),
        cv::Point2f((float)imgCropWidth, 0.f),
        cv::Point2f((float)imgCropWidth, (float)imgCropHeight),
        cv::Point2f(0.f, (float)imgCropHeight)
    };
    cv::Point2f ptsSrc[4] = {
        cv::Point2f((float)points[0].x, (float)points[0].y),
        cv::Point2f((float)points[1].x, (float)points[1].y),
        cv::Point2f((float)points[2].x, (float)points[2].y),
        cv::Point2f((float)points[3].x, (float)points[3].y)
    };

    cv::Mat M = cv::getPerspectiveTransform(ptsSrc, ptsDst);
    cv::Mat partImg;
    cv::warpPerspective(imgCrop, partImg, M, cv::Size(imgCropWidth, imgCropHeight), cv::BORDER_REPLICATE);

    if (float(partImg.rows) >= float(partImg.cols) * 1.5f) {
        cv::Mat srcCopy;
        cv::transpose(partImg, srcCopy);
        cv::flip(srcCopy, srcCopy, 0);
        return srcCopy;
    }
    return partImg;
}

std::vector<float> substractMeanNormalize(cv::Mat &src, const float *meanVals, const float *normVals) {
    auto inputTensorSize = src.cols * src.rows * src.channels();
    std::vector<float> inputTensorValues(inputTensorSize);
    size_t numChannels = src.channels();
    size_t imageSize = src.cols * src.rows;

    for (size_t pid = 0; pid < imageSize; pid++) {
        for (size_t ch = 0; ch < numChannels; ++ch) {
            float data = (float)(src.data[pid * numChannels + ch] * normVals[ch] - meanVals[ch] * normVals[ch]);
            inputTensorValues[ch * imageSize + pid] = data;
        }
    }
    return inputTensorValues;
}

std::vector<int> getAngleIndexes(std::vector<Angle> &angles) {
    std::vector<int> angleIndexes;
    angleIndexes.reserve(angles.size());
    for (auto &angle : angles) {
        angleIndexes.push_back(angle.index);
    }
    return angleIndexes;
}

std::vector<Ort::AllocatedStringPtr> getInputNames(Ort::Session *session) {
    Ort::AllocatorWithDefaultOptions allocator;
    const size_t numInputNodes = session->GetInputCount();
    std::vector<Ort::AllocatedStringPtr> inputNamesPtr;
    inputNamesPtr.reserve(numInputNodes);
    for (size_t i = 0; i < numInputNodes; i++) {
        inputNamesPtr.push_back(session->GetInputNameAllocated(i, allocator));
    }
    return inputNamesPtr;
}

std::vector<Ort::AllocatedStringPtr> getOutputNames(Ort::Session *session) {
    Ort::AllocatorWithDefaultOptions allocator;
    const size_t numOutputNodes = session->GetOutputCount();
    std::vector<Ort::AllocatedStringPtr> outputNamesPtr;
    outputNamesPtr.reserve(numOutputNodes);
    for (size_t i = 0; i < numOutputNodes; i++) {
        outputNamesPtr.push_back(session->GetOutputNameAllocated(i, allocator));
    }
    return outputNamesPtr;
}

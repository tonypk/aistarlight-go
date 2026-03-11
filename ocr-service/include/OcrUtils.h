#ifndef __OCR_UTILS_H__
#define __OCR_UTILS_H__

#include <opencv2/core.hpp>
#include "OcrStruct.h"
#include "onnxruntime_cxx_api.h"
#include <numeric>

template<typename T>
static double getMean(std::vector<T> &input) {
    auto sum = accumulate(input.begin(), input.end(), 0.0);
    return sum / input.size();
}

template<class T>
inline T clamp(T x, T min, T max) {
    if (x > max) return max;
    if (x < min) return min;
    return x;
}

double getCurrentTime();

ScaleParam getScaleParam(cv::Mat &src, const float scale);
ScaleParam getScaleParam(cv::Mat &src, const int targetSize);

std::vector<cv::Point2f> getBox(const cv::RotatedRect &rect);
std::vector<cv::Point2f> getMinBoxes(const cv::RotatedRect &boxRect, float &maxSideLen);

float boxScoreFast(const std::vector<cv::Point2f> &boxes, const cv::Mat &pred);
cv::RotatedRect unClip(std::vector<cv::Point2f> box, float unClipRatio);

cv::Mat matRotateClockWise180(cv::Mat src);
cv::Mat getRotateCropImage(const cv::Mat &src, std::vector<cv::Point> box);

std::vector<float> substractMeanNormalize(cv::Mat &src, const float *meanVals, const float *normVals);
std::vector<int> getAngleIndexes(std::vector<Angle> &angles);

std::vector<Ort::AllocatedStringPtr> getInputNames(Ort::Session *session);
std::vector<Ort::AllocatedStringPtr> getOutputNames(Ort::Session *session);

#endif //__OCR_UTILS_H__

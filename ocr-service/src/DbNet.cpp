#include "DbNet.h"
#include "OcrUtils.h"
#include <numeric>

DbNet::~DbNet() {
    delete session;
    inputNamesPtr.clear();
    outputNamesPtr.clear();
}

void DbNet::setNumThread(int numOfThread) {
    numThread = numOfThread;
    sessionOptions.SetInterOpNumThreads(numThread);
    sessionOptions.SetIntraOpNumThreads(numThread);
    sessionOptions.SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_ENABLE_ALL);
}

void DbNet::initModel(const std::string &pathStr) {
    session = new Ort::Session(env, pathStr.c_str(), sessionOptions);
    inputNamesPtr = getInputNames(session);
    outputNamesPtr = getOutputNames(session);
}

static std::vector<TextBox> findRsBoxes(const cv::Mat &predMat, const cv::Mat &dilateMat,
                                         ScaleParam &s, float boxScoreThresh, float unClipRatio) {
    const int longSideThresh = 3;
    const int maxCandidates = 1000;

    std::vector<std::vector<cv::Point>> contours;
    std::vector<cv::Vec4i> hierarchy;
    cv::findContours(dilateMat, contours, hierarchy, cv::RETR_LIST, cv::CHAIN_APPROX_SIMPLE);

    size_t numContours = std::min(contours.size(), (size_t)maxCandidates);
    std::vector<TextBox> rsBoxes;

    for (size_t i = 0; i < numContours; i++) {
        if (contours[i].size() <= 2) continue;

        cv::RotatedRect minAreaRect = cv::minAreaRect(contours[i]);
        float longSide;
        std::vector<cv::Point2f> minBoxes = getMinBoxes(minAreaRect, longSide);
        if (longSide < longSideThresh) continue;

        float boxScore = boxScoreFast(minBoxes, predMat);
        if (boxScore < boxScoreThresh) continue;

        cv::RotatedRect clipRect = unClip(minBoxes, unClipRatio);
        if (clipRect.size.height < 1.001 && clipRect.size.width < 1.001) continue;

        std::vector<cv::Point2f> clipMinBoxes = getMinBoxes(clipRect, longSide);
        if (longSide < longSideThresh + 2) continue;

        std::vector<cv::Point> intClipMinBoxes;
        for (auto &clipMinBox : clipMinBoxes) {
            int ptX = clamp(int(clipMinBox.x / s.ratioWidth), 0, s.srcWidth - 1);
            int ptY = clamp(int(clipMinBox.y / s.ratioHeight), 0, s.srcHeight - 1);
            intClipMinBoxes.push_back(cv::Point{ptX, ptY});
        }
        rsBoxes.push_back(TextBox{intClipMinBoxes, boxScore});
    }
    std::reverse(rsBoxes.begin(), rsBoxes.end());
    return rsBoxes;
}

std::vector<TextBox> DbNet::getTextBoxes(cv::Mat &src, ScaleParam &s,
                                          float boxScoreThresh, float boxThresh, float unClipRatio) {
    cv::Mat srcResize;
    cv::resize(src, srcResize, cv::Size(s.dstWidth, s.dstHeight));

    std::vector<float> inputTensorValues = substractMeanNormalize(srcResize, meanValues, normValues);
    std::array<int64_t, 4> inputShape{1, srcResize.channels(), srcResize.rows, srcResize.cols};

    auto memoryInfo = Ort::MemoryInfo::CreateCpu(OrtDeviceAllocator, OrtMemTypeCPU);
    Ort::Value inputTensor = Ort::Value::CreateTensor<float>(
        memoryInfo, inputTensorValues.data(), inputTensorValues.size(),
        inputShape.data(), inputShape.size());

    std::vector<const char *> inputNames = {inputNamesPtr.data()->get()};
    std::vector<const char *> outputNames = {outputNamesPtr.data()->get()};

    auto outputTensor = session->Run(Ort::RunOptions{nullptr},
                                     inputNames.data(), &inputTensor, inputNames.size(),
                                     outputNames.data(), outputNames.size());

    std::vector<int64_t> outputShape = outputTensor[0].GetTensorTypeAndShapeInfo().GetShape();
    int64_t outputCount = std::accumulate(outputShape.begin(), outputShape.end(), 1,
                                          std::multiplies<int64_t>());
    float *floatArray = outputTensor.front().GetTensorMutableData<float>();

    int outHeight = (int)outputShape[2];
    int outWidth = (int)outputShape[3];
    size_t area = outHeight * outWidth;

    std::vector<float> predData(area);
    std::vector<unsigned char> cbufData(area);
    for (size_t i = 0; i < area; i++) {
        predData[i] = floatArray[i];
        cbufData[i] = (unsigned char)(floatArray[i] * 255);
    }

    cv::Mat predMat(outHeight, outWidth, CV_32F, predData.data());
    cv::Mat cBufMat(outHeight, outWidth, CV_8UC1, cbufData.data());

    cv::Mat thresholdMat;
    cv::threshold(cBufMat, thresholdMat, boxThresh * 255, 255, cv::THRESH_BINARY);

    cv::Mat dilateMat;
    cv::Mat dilateElement = cv::getStructuringElement(cv::MORPH_RECT, cv::Size(2, 2));
    cv::dilate(thresholdMat, dilateMat, dilateElement);

    return findRsBoxes(predMat, dilateMat, s, boxScoreThresh, unClipRatio);
}
